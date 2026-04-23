package feishu

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"neo-code/internal/gateway"
	"neo-code/internal/gateway/protocol"
)

const (
	defaultHTTPReadLimit = 2 << 20
	maxGatewayAttempts   = 3
)

type runState struct {
	record        RunRecord
	route         TenantRoute
	aggregator    *ChunkAggregator
	permissionIDs map[string]struct{}
}

// ServiceOptions defines adapter service dependencies.
type ServiceOptions struct {
	Config    Config
	Gateway   *GatewayWSClient
	Messenger Messenger
	Logger    *log.Logger
	Metrics   *Metrics
	Alerts    *AlertManager
	Now       func() time.Time
}

// Service orchestrates Feishu webhook ingress and Gateway bridge lifecycle.
type Service struct {
	cfg                 Config
	gateway             *GatewayWSClient
	messenger           Messenger
	logger              *log.Logger
	metrics             *Metrics
	now                 func() time.Time
	signer              SignatureVerifier
	deduper             *DedupeStore
	alerts              *AlertManager
	gatewayCall         func(ctx context.Context, method string, params any, result any) error
	gatewayAuthenticate func(ctx context.Context) error
	gatewayBindStream   func(ctx context.Context, sessionID, runID string) error
	gatewayPing         func(ctx context.Context) error
	gatewayClose        func() error

	mu              sync.Mutex
	runs            map[string]*runState
	permissionIndex map[string]string

	httpServer *http.Server
	closeOnce  sync.Once
	done       chan struct{}

	alertWindowMu     sync.Mutex
	reconnectEvents   []time.Time
	authFailureEvents []time.Time
}

// NewService creates a Feishu adapter service instance.
func NewService(options ServiceOptions) (*Service, error) {
	cfg := options.Config
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	logger := options.Logger
	if logger == nil {
		logger = log.New(os.Stderr, "neocode-feishu-adapter: ", log.LstdFlags)
	}
	metrics := options.Metrics
	if metrics == nil {
		metrics = &Metrics{}
	}
	now := options.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}

	gatewayClient := options.Gateway
	if gatewayClient == nil {
		var err error
		gatewayClient, err = NewGatewayWSClient(GatewayWSClientOptions{
			URL:       cfg.GatewayWSURL,
			Origin:    cfg.GatewayWSOrigin,
			TokenFile: cfg.GatewayTokenFile,
			Logger:    logger,
		})
		if err != nil {
			return nil, err
		}
	}
	messenger := options.Messenger
	if messenger == nil {
		messenger = NewHTTPMessenger(HTTPMessengerOptions{
			BaseURL: cfg.FeishuBaseURL,
			Logger:  logger,
		})
	}
	deduper := NewDedupeStore(cfg.EventDedupeTTL)
	if strings.TrimSpace(cfg.DedupeStoreFile) != "" {
		deduper = NewPersistentDedupeStore(cfg.EventDedupeTTL, cfg.DedupeStoreFile, logger)
	}
	alerts := options.Alerts
	if alerts == nil {
		var sink AlertSink
		if strings.TrimSpace(cfg.AlertWebhookURL) != "" {
			sink = WebhookAlertSink{URL: cfg.AlertWebhookURL}
		}
		alerts = NewAlertManager(sink, cfg.AlertCooldown, logger)
	}

	service := &Service{
		cfg:       cfg,
		gateway:   gatewayClient,
		messenger: messenger,
		logger:    logger,
		metrics:   metrics,
		alerts:    alerts,
		now:       now,
		signer: SignatureVerifier{
			Secret:  cfg.SigningSecret,
			MaxSkew: cfg.SignatureMaxSkew,
			Now:     now,
		},
		deduper:             deduper,
		gatewayCall:         gatewayClient.Call,
		gatewayAuthenticate: gatewayClient.Authenticate,
		gatewayBindStream:   gatewayClient.BindStream,
		gatewayPing:         gatewayClient.Ping,
		gatewayClose:        gatewayClient.Close,
		runs:                map[string]*runState{},
		permissionIndex:     map[string]string{},
		done:                make(chan struct{}),
	}
	return service, nil
}

// Serve starts HTTP handlers and background loops.
func (s *Service) Serve(ctx context.Context) error {
	if s == nil {
		return errors.New("feishu adapter service is nil")
	}
	if err := s.gatewayAuthenticate(ctx); err != nil {
		return fmt.Errorf("authenticate gateway: %w", err)
	}
	if err := s.runCompatibilityProbe(ctx); err != nil {
		return fmt.Errorf("gateway compatibility probe failed: %w", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.handleHealthz)
	mux.HandleFunc("/metrics", s.handleMetrics)
	mux.HandleFunc("/feishu/events", s.handleFeishuEvents)
	mux.HandleFunc("/feishu/actions/cancel", s.handleCancelAction)
	mux.HandleFunc("/feishu/actions/permission", s.handlePermissionAction)

	server := &http.Server{
		Addr:    s.cfg.ListenAddress,
		Handler: mux,
	}
	s.httpServer = server

	go s.runGatewayEventLoop(ctx)
	go s.runPingLoop(ctx)
	go s.runWatchdogLoop(ctx)
	go s.runChunkFlushLoop(ctx)

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	err := server.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) || ctx.Err() != nil {
		return nil
	}
	return err
}

// Close gracefully stops the service.
func (s *Service) Close() error {
	if s == nil {
		return nil
	}
	var closeErr error
	s.closeOnce.Do(func() {
		if s.httpServer != nil {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			closeErr = errors.Join(closeErr, s.httpServer.Shutdown(shutdownCtx))
		}
		if s.gatewayClose != nil {
			closeErr = errors.Join(closeErr, s.gatewayClose())
		}
		close(s.done)
	})
	return closeErr
}

func (s *Service) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	snapshot := s.metrics.Snapshot()
	status := "ok"
	if !snapshot.LastGatewayPingAt.IsZero() && s.now().Sub(snapshot.LastGatewayPingAt) > s.cfg.WatchdogTimeout {
		status = "degraded"
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":  status,
		"metrics": snapshot,
	})
}

func (s *Service) handleMetrics(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.metrics.Snapshot())
}

func (s *Service) handleFeishuEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"code": 405, "msg": "method not allowed"})
		return
	}
	body, ok := s.readSignedBody(w, r, defaultHTTPReadLimit)
	if !ok {
		return
	}

	var envelope IncomingEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		s.metrics.IncRequestsRejected()
		writeJSON(w, http.StatusBadRequest, map[string]any{"code": 400, "msg": "invalid json payload"})
		return
	}

	if strings.EqualFold(strings.TrimSpace(envelope.Type), "url_verification") {
		writeJSON(w, http.StatusOK, map[string]any{
			"challenge": strings.TrimSpace(envelope.Challenge),
		})
		return
	}

	s.metrics.IncRequestsTotal()
	message, err := normalizeIncomingMessage(envelope, s.now())
	if err != nil {
		s.metrics.IncRequestsRejected()
		writeJSON(w, http.StatusBadRequest, map[string]any{"code": 400, "msg": err.Error()})
		return
	}

	if s.deduper.SeenOrAdd(message.EventID, s.now()) {
		s.metrics.IncEventDuplicates()
		writeJSON(w, http.StatusOK, map[string]any{"code": 0, "msg": "duplicate event ignored"})
		return
	}

	route := s.resolveRoute(message.TenantKey, message.ChatID)
	if !route.Enabled {
		writeJSON(w, http.StatusOK, map[string]any{"code": 0, "msg": "tenant route disabled"})
		return
	}

	runID := buildRunID(message.EventID)
	sessionID := BuildSessionID(message.AppID, message.ChatID, message.ThreadID)
	requestID := buildRequestID(message.EventID)
	target := MessageTarget{
		TenantKey: message.TenantKey,
		ChatID:    message.ChatID,
		ThreadID:  message.ThreadID,
		MessageID: message.MessageID,
		UserID:    message.SenderUserID,
	}

	if err := s.startGatewayRun(r.Context(), route, message.Text, sessionID, runID, requestID, message.EventID, target); err != nil {
		s.metrics.IncRunsFailed()
		_ = s.messenger.SendText(r.Context(), route, target, fmt.Sprintf("request execution failed: %s", err.Error()))
		writeJSON(w, http.StatusOK, map[string]any{"code": 0, "msg": "accepted with error"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"code": 0, "msg": "success"})
}

func (s *Service) handleCancelAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"code": 405, "msg": "method not allowed"})
		return
	}
	body, ok := s.readSignedBody(w, r, 1<<20)
	if !ok {
		return
	}
	var callback CancelCallback
	if err := json.Unmarshal(body, &callback); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"code": 400, "msg": "invalid cancel payload"})
		return
	}
	runID := strings.TrimSpace(callback.RunID)
	if runID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"code": 400, "msg": "missing run_id"})
		return
	}

	run, exists := s.getRun(runID)
	if !exists {
		writeJSON(w, http.StatusNotFound, map[string]any{"code": 404, "msg": "run not found"})
		return
	}
	cancelCtx, cancel := context.WithTimeout(r.Context(), s.cfg.RequestTimeout)
	defer cancel()
	_, err := s.callGatewayFrameWithRetry(cancelCtx, protocol.MethodGatewayCancel, protocol.CancelParams{
		SessionID: run.record.SessionID,
		RunID:     run.record.RunID,
	})
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"code": 0, "msg": err.Error()})
		return
	}
	s.metrics.IncRunsCanceled()
	_ = s.messenger.SendText(r.Context(), run.route, run.record.Target, "Cancel received, task is stopping.")
	writeJSON(w, http.StatusOK, map[string]any{"code": 0, "msg": "cancel accepted"})
}

func (s *Service) handlePermissionAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"code": 405, "msg": "method not allowed"})
		return
	}
	body, ok := s.readSignedBody(w, r, 1<<20)
	if !ok {
		return
	}
	var err error
	callback, err := decodePermissionCallback(body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"code": 400, "msg": "invalid permission payload"})
		return
	}
	requestID := strings.TrimSpace(callback.RequestID)
	decision := strings.ToLower(strings.TrimSpace(callback.Decision))
	if requestID == "" || decision == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"code": 400, "msg": "missing request_id or decision"})
		return
	}
	if decision != string(gateway.PermissionResolutionAllowOnce) &&
		decision != string(gateway.PermissionResolutionAllowSession) &&
		decision != string(gateway.PermissionResolutionReject) {
		writeJSON(w, http.StatusBadRequest, map[string]any{"code": 400, "msg": "invalid decision"})
		return
	}

	runID, found := s.getRunIDByPermission(requestID)
	if !found {
		writeJSON(w, http.StatusNotFound, map[string]any{"code": 404, "msg": "permission request not found"})
		return
	}
	run, exists := s.getRun(runID)
	if !exists {
		writeJSON(w, http.StatusNotFound, map[string]any{"code": 404, "msg": "run not found"})
		return
	}

	resolveCtx, cancel := context.WithTimeout(r.Context(), s.cfg.RequestTimeout)
	defer cancel()
	_, err = s.callGatewayFrameWithRetry(resolveCtx, protocol.MethodGatewayResolvePermission, protocol.ResolvePermissionParams{
		RequestID: requestID,
		Decision:  decision,
	})
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"code": 0, "msg": err.Error()})
		return
	}
	s.unbindPermission(requestID)
	_ = s.messenger.SendText(r.Context(), run.route, run.record.Target, "Permission decision submitted: "+decision)
	writeJSON(w, http.StatusOK, map[string]any{"code": 0, "msg": "permission resolved"})
}

func (s *Service) startGatewayRun(
	ctx context.Context,
	route TenantRoute,
	text string,
	sessionID string,
	runID string,
	requestID string,
	eventID string,
	target MessageTarget,
) error {
	if s.deduper.Seen("run:"+runID, s.now()) {
		s.metrics.IncEventDuplicates()
		return nil
	}
	startCtx, cancel := context.WithTimeout(ctx, s.cfg.RequestTimeout)
	defer cancel()

	if err := s.gatewayBindStream(startCtx, sessionID, runID); err != nil {
		return err
	}
	params := protocol.RunParams{
		SessionID: sessionID,
		RunID:     runID,
		InputText: text,
		Workdir:   strings.TrimSpace(route.Workdir),
	}
	frame, err := s.callGatewayFrameWithRetry(startCtx, protocol.MethodGatewayRun, params)
	if err != nil {
		return err
	}
	acceptedRunID := strings.TrimSpace(frame.RunID)
	if acceptedRunID == "" {
		acceptedRunID = runID
	}
	s.deduper.Add("run:"+acceptedRunID, s.now())
	s.setRun(&runState{
		record: RunRecord{
			RunID:       acceptedRunID,
			SessionID:   sessionID,
			RequestID:   requestID,
			EventID:     eventID,
			TenantKey:   target.TenantKey,
			Target:      target,
			AcceptedAt:  s.now(),
			LastEventAt: s.now(),
			Streaming:   route.Streaming,
		},
		route:         route,
		aggregator:    NewChunkAggregator(s.cfg.ChunkFlushMaxChars, s.cfg.ChunkFlushInterval),
		permissionIDs: map[string]struct{}{},
	})
	s.metrics.IncRunsAccepted()
	return nil
}

func (s *Service) callGatewayFrameWithRetry(ctx context.Context, method string, params any) (gateway.MessageFrame, error) {
	var lastErr error
	for attempt := 1; attempt <= maxGatewayAttempts; attempt++ {
		var frame gateway.MessageFrame
		err := s.gatewayCall(ctx, method, params, &frame)
		if err == nil {
			return frame, nil
		}
		lastErr = err

		rpcErr, ok := err.(*GatewayRPCError)
		if !ok {
			if attempt == maxGatewayAttempts {
				break
			}
			s.metrics.IncConnectionReconnects()
			s.emitReconnectAlert(ctx, method, err)
			time.Sleep(backoffForAttempt(s.cfg.ReconnectBaseBackoff, s.cfg.ReconnectMaxBackoff, attempt))
			continue
		}

		gatewayCode := strings.TrimSpace(strings.ToLower(rpcErr.GatewayCode))
		if gatewayCode == protocol.GatewayCodeUnauthorized {
			s.metrics.IncAuthFailures()
			s.emitAuthFailureAlert(ctx, method, err)
		}
		if !shouldRetryGatewayCode(gatewayCode, attempt, maxGatewayAttempts) {
			break
		}
		switch gatewayRetryClass(gatewayCode) {
		case retryClassReauth:
			_ = s.gatewayAuthenticate(ctx)
		case retryClassBackoff:
			s.metrics.IncConnectionReconnects()
			s.emitReconnectAlert(ctx, method, err)
			time.Sleep(backoffForAttempt(s.cfg.ReconnectBaseBackoff, s.cfg.ReconnectMaxBackoff, attempt))
		}
	}
	return gateway.MessageFrame{}, lastErr
}

func (s *Service) runCompatibilityProbe(ctx context.Context) error {
	probeCtx, cancel := context.WithTimeout(ctx, s.cfg.RequestTimeout)
	defer cancel()

	type probe struct {
		method string
		params any
	}
	probes := []probe{
		{method: protocol.MethodGatewayPing, params: struct{}{}},
		{method: protocol.MethodGatewayListSessions, params: nil},
	}
	for _, item := range probes {
		var frame gateway.MessageFrame
		err := s.gatewayCall(probeCtx, item.method, item.params, &frame)
		if err == nil {
			continue
		}
		rpcErr, ok := err.(*GatewayRPCError)
		if !ok {
			return err
		}
		if strings.EqualFold(strings.TrimSpace(rpcErr.GatewayCode), protocol.GatewayCodeUnsupportedAction) {
			return fmt.Errorf("unsupported stable method %s", item.method)
		}
		return err
	}
	return nil
}

func (s *Service) runGatewayEventLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.done:
			return
		case notification, ok := <-s.gateway.Notifications():
			if !ok {
				return
			}
			if strings.TrimSpace(notification.Method) != protocol.MethodGatewayEvent {
				continue
			}
			if err := s.handleGatewayNotification(ctx, notification); err != nil {
				s.logger.Printf("handle gateway notification failed: %v", err)
			}
		}
	}
}

func (s *Service) handleGatewayNotification(ctx context.Context, notification protocol.JSONRPCNotification) error {
	var (
		paramsRaw []byte
		err       error
	)
	switch typed := notification.Params.(type) {
	case json.RawMessage:
		paramsRaw = append(paramsRaw, typed...)
	case []byte:
		paramsRaw = append(paramsRaw, typed...)
	default:
		paramsRaw, err = json.Marshal(notification.Params)
		if err != nil {
			return err
		}
	}
	var frame gateway.MessageFrame
	if err := json.Unmarshal(paramsRaw, &frame); err != nil {
		return err
	}

	runID := strings.TrimSpace(frame.RunID)
	run, ok := s.getRun(runID)
	if !ok {
		return nil
	}
	s.touchRun(run.record.RunID)

	eventPayload := asMap(frame.Payload)
	eventType := strings.TrimSpace(readMapString(eventPayload, "event_type"))
	runtimeEnvelope := asMap(readMapValue(eventPayload, "payload"))
	runtimeType := strings.TrimSpace(readMapString(runtimeEnvelope, "runtime_event_type"))
	runtimePayload := readMapValue(runtimeEnvelope, "payload")

	switch runtimeType {
	case "agent_chunk", "tool_chunk":
		chunk := renderRuntimeText(runtimePayload)
		if run.record.Streaming {
			if flushed, ok := run.aggregator.Add(chunk, s.now()); ok && strings.TrimSpace(flushed) != "" {
				_ = s.messenger.SendText(ctx, run.route, run.record.Target, flushed)
			}
		}
	case "permission_requested":
		runtimeMap := asMap(runtimePayload)
		requestID := strings.TrimSpace(readMapString(runtimeMap, "request_id"))
		if requestID != "" {
			s.bindPermission(requestID, run.record.RunID)
			card := buildPermissionCardData(runtimeMap)
			card.RequestID = requestID
			if err := s.messenger.SendPermissionCard(ctx, run.route, run.record.Target, card); err != nil {
				s.logger.Printf("send permission card failed: %v", err)
				_ = s.messenger.SendText(
					ctx,
					run.route,
					run.record.Target,
					fmt.Sprintf("检测到权限请求，请调用 /feishu/actions/permission 提交决策。request_id=%s", requestID),
				)
			}
		}
	}

	switch eventType {
	case string(gateway.RuntimeEventTypeRunDone):
		final := strings.TrimSpace(run.aggregator.Flush(s.now()))
		doneText := strings.TrimSpace(renderRuntimeText(runtimePayload))
		if final == "" {
			final = doneText
		} else if doneText != "" && !strings.Contains(final, doneText) {
			final = final + "\n" + doneText
		}
		if final == "" {
			final = "Task completed."
		}
		_ = s.messenger.SendText(ctx, run.route, run.record.Target, final)
		s.metrics.IncRunsCompleted()
		s.removeRun(run.record.RunID)
	case string(gateway.RuntimeEventTypeRunError):
		flushed := strings.TrimSpace(run.aggregator.Flush(s.now()))
		errText := strings.TrimSpace(renderRuntimeText(runtimePayload))
		message := "Task failed, please retry later."
		if runtimeType == "run_canceled" {
			message = "Task canceled."
			s.metrics.IncRunsCanceled()
		} else {
			s.metrics.IncRunsFailed()
		}
		if flushed != "" {
			message = flushed + "\n" + message
		}
		if errText != "" {
			message = message + "\nReason: " + errText
		}
		_ = s.messenger.SendText(ctx, run.route, run.record.Target, message)
		s.removeRun(run.record.RunID)
	}
	return nil
}

func (s *Service) runPingLoop(ctx context.Context) {
	ticker := time.NewTicker(s.cfg.PingInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.done:
			return
		case <-ticker.C:
			pingCtx, cancel := context.WithTimeout(ctx, s.cfg.RequestTimeout)
			err := s.gatewayPing(pingCtx)
			cancel()
			if err != nil {
				s.metrics.IncConnectionReconnects()
				s.emitReconnectAlert(ctx, protocol.MethodGatewayPing, err)
				s.logger.Printf("gateway ping failed: %v", err)
				continue
			}
			s.metrics.MarkGatewayPing(s.now())
		}
	}
}

func (s *Service) runWatchdogLoop(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.done:
			return
		case <-ticker.C:
			s.expireTimedOutRuns(ctx)
		}
	}
}

func (s *Service) runChunkFlushLoop(ctx context.Context) {
	ticker := time.NewTicker(s.cfg.ChunkFlushInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.done:
			return
		case <-ticker.C:
			s.flushAllChunks(ctx)
		}
	}
}

func (s *Service) flushAllChunks(ctx context.Context) {
	for _, run := range s.snapshotRuns() {
		if run == nil || !run.record.Streaming {
			continue
		}
		flushed := strings.TrimSpace(run.aggregator.Flush(s.now()))
		if flushed == "" {
			continue
		}
		_ = s.messenger.SendText(ctx, run.route, run.record.Target, flushed)
	}
}

func (s *Service) expireTimedOutRuns(ctx context.Context) {
	now := s.now()
	for _, run := range s.snapshotRuns() {
		if run == nil {
			continue
		}
		if now.Sub(run.record.LastEventAt) < s.cfg.WatchdogTimeout {
			continue
		}
		s.metrics.IncWatchdogTimeouts()
		s.logger.Printf("run watchdog timeout: run_id=%s session_id=%s", run.record.RunID, run.record.SessionID)
		s.emitWatchdogAlert(ctx, run)

		cancelCtx, cancel := context.WithTimeout(ctx, s.cfg.RequestTimeout)
		_, _ = s.callGatewayFrameWithRetry(cancelCtx, protocol.MethodGatewayCancel, protocol.CancelParams{
			SessionID: run.record.SessionID,
			RunID:     run.record.RunID,
		})
		cancel()
		_ = s.messenger.SendText(ctx, run.route, run.record.Target, "Task timed out and was canceled automatically.")
		s.removeRun(run.record.RunID)
	}
}

func (s *Service) setRun(run *runState) {
	if run == nil {
		return
	}
	var inflight int
	s.mu.Lock()
	s.runs[run.record.RunID] = run
	inflight = len(s.runs)
	s.metrics.SetInflightRuns(inflight)
	s.mu.Unlock()
	s.emitBacklogAlert(context.Background(), inflight)
}

func (s *Service) touchRun(runID string) {
	s.mu.Lock()
	if run, exists := s.runs[runID]; exists && run != nil {
		run.record.LastEventAt = s.now()
	}
	s.mu.Unlock()
}

func (s *Service) removeRun(runID string) {
	s.mu.Lock()
	run, exists := s.runs[runID]
	if exists && run != nil {
		for permissionID := range run.permissionIDs {
			delete(s.permissionIndex, permissionID)
		}
	}
	delete(s.runs, runID)
	s.metrics.SetInflightRuns(len(s.runs))
	s.metrics.SetPermissionsPending(len(s.permissionIndex))
	s.mu.Unlock()
}

func (s *Service) getRun(runID string) (*runState, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	run, exists := s.runs[runID]
	return run, exists
}

func (s *Service) snapshotRuns() []*runState {
	s.mu.Lock()
	defer s.mu.Unlock()
	runs := make([]*runState, 0, len(s.runs))
	for _, run := range s.runs {
		runs = append(runs, run)
	}
	return runs
}

func (s *Service) bindPermission(requestID, runID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if run, exists := s.runs[runID]; exists && run != nil {
		run.permissionIDs[requestID] = struct{}{}
	}
	s.permissionIndex[requestID] = runID
	s.metrics.SetPermissionsPending(len(s.permissionIndex))
}

func (s *Service) getRunIDByPermission(requestID string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	runID, exists := s.permissionIndex[requestID]
	return runID, exists
}

func (s *Service) unbindPermission(requestID string) {
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		return
	}
	s.mu.Lock()
	runID, exists := s.permissionIndex[requestID]
	if exists {
		delete(s.permissionIndex, requestID)
		if run, runExists := s.runs[runID]; runExists && run != nil {
			delete(run.permissionIDs, requestID)
		}
	}
	s.metrics.SetPermissionsPending(len(s.permissionIndex))
	s.mu.Unlock()
}

func (s *Service) resolveRoute(tenantKey, chatID string) TenantRoute {
	route := s.cfg.Routes.Default
	tenantKey = strings.TrimSpace(tenantKey)
	chatID = strings.TrimSpace(chatID)
	if tenantKey != "" {
		if tenantRoute, exists := s.cfg.Routes.Tenants[tenantKey]; exists {
			route = overlayRoute(route, tenantRoute)
		}
	}
	if tenantKey != "" && chatID != "" {
		if chatRoute, exists := s.cfg.Routes.Chats[tenantKey+":"+chatID]; exists {
			route = overlayRoute(route, chatRoute)
		}
	}
	if strings.TrimSpace(route.AppID) == "" {
		route.AppID = strings.TrimSpace(s.cfg.Routes.Default.AppID)
	}
	if strings.TrimSpace(route.AppSecret) == "" {
		route.AppSecret = strings.TrimSpace(s.cfg.Routes.Default.AppSecret)
	}
	return route
}

func overlayRoute(base TenantRoute, override TenantRoute) TenantRoute {
	base.Enabled = override.Enabled
	base.Streaming = override.Streaming
	if strings.TrimSpace(override.Workdir) != "" {
		base.Workdir = strings.TrimSpace(override.Workdir)
	}
	if strings.TrimSpace(override.AppID) != "" {
		base.AppID = strings.TrimSpace(override.AppID)
	}
	if strings.TrimSpace(override.AppSecret) != "" {
		base.AppSecret = strings.TrimSpace(override.AppSecret)
	}
	if strings.TrimSpace(override.ReplyReceiveBy) != "" {
		base.ReplyReceiveBy = strings.TrimSpace(override.ReplyReceiveBy)
	}
	return base
}

func buildRunID(eventID string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(eventID)))
	return "run_fs_" + hex.EncodeToString(sum[:10])
}

func buildRequestID(eventID string) string {
	sum := sha256.Sum256([]byte("req:" + strings.TrimSpace(eventID)))
	return "req_fs_" + hex.EncodeToString(sum[:8])
}

func backoffForAttempt(base, max time.Duration, attempt int) time.Duration {
	if base <= 0 {
		base = defaultReconnectBaseBackoff
	}
	if max <= 0 {
		max = defaultReconnectMaxBackoff
	}
	delay := base
	for i := 1; i < attempt; i++ {
		delay *= 2
		if delay >= max {
			return max
		}
	}
	if delay > max {
		return max
	}
	return delay
}

func (s *Service) readSignedBody(w http.ResponseWriter, r *http.Request, limit int64) ([]byte, bool) {
	if s == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"code": 500, "msg": "service unavailable"})
		return nil, false
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, limit))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"code": 400, "msg": "invalid request body"})
		return nil, false
	}
	if err := s.signer.Verify(
		r.Header.Get("X-Lark-Request-Timestamp"),
		r.Header.Get("X-Lark-Request-Nonce"),
		r.Header.Get("X-Lark-Signature"),
		body,
	); err != nil {
		s.metrics.IncRequestsRejected()
		writeJSON(w, http.StatusUnauthorized, map[string]any{"code": 401, "msg": err.Error()})
		return nil, false
	}
	return body, true
}

func decodePermissionCallback(raw []byte) (PermissionCallback, error) {
	var callback PermissionCallback
	if err := json.Unmarshal(raw, &callback); err == nil {
		callback.RequestID = strings.TrimSpace(callback.RequestID)
		callback.Decision = strings.ToLower(strings.TrimSpace(callback.Decision))
		if callback.RequestID != "" && callback.Decision != "" {
			return callback, nil
		}
	}

	var payload ActionCallbackPayload
	if err := json.Unmarshal(raw, &payload); err == nil {
		requestID, decision := parseActionDecision(payload)
		if requestID != "" && decision != "" {
			return PermissionCallback{RequestID: requestID, Decision: decision}, nil
		}
	}

	var fallback map[string]any
	if err := json.Unmarshal(raw, &fallback); err != nil {
		return PermissionCallback{}, err
	}
	requestID, decision := parseActionDecisionFromMap(fallback)
	if requestID == "" || decision == "" {
		return PermissionCallback{}, errors.New("missing request_id or decision")
	}
	return PermissionCallback{RequestID: requestID, Decision: decision}, nil
}

func parseActionDecision(payload ActionCallbackPayload) (string, string) {
	requestID := mapLookupString(payload.Action.Value, "request_id")
	decision := strings.ToLower(mapLookupString(payload.Action.Value, "decision"))
	if requestID != "" && decision != "" {
		return requestID, decision
	}
	requestID = mapLookupString(payload.Event.Action.Value, "request_id")
	decision = strings.ToLower(mapLookupString(payload.Event.Action.Value, "decision"))
	return requestID, decision
}

func parseActionDecisionFromMap(payload map[string]any) (string, string) {
	candidates := []map[string]any{
		payload,
		asMap(readMapValue(payload, "value")),
		asMap(readMapValue(asMap(readMapValue(payload, "action")), "value")),
		asMap(readMapValue(asMap(readMapValue(payload, "event")), "value")),
		asMap(readMapValue(asMap(readMapValue(asMap(readMapValue(payload, "event")), "action")), "value")),
	}
	for _, candidate := range candidates {
		if len(candidate) == 0 {
			continue
		}
		requestID := readMapString(candidate, "request_id")
		decision := strings.ToLower(readMapString(candidate, "decision"))
		if requestID != "" && decision != "" {
			return requestID, decision
		}
	}
	return "", ""
}

func mapLookupString(m map[string]any, key string) string {
	if len(m) == 0 {
		return ""
	}
	value, exists := m[key]
	if !exists || value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	default:
		return strings.TrimSpace(fmt.Sprintf("%v", typed))
	}
}

func buildPermissionCardData(payload map[string]any) PermissionCardData {
	return PermissionCardData{
		ToolName:     readMapString(payload, "tool_name"),
		ActionType:   readMapString(payload, "action_type"),
		Operation:    readMapString(payload, "operation"),
		TargetType:   readMapString(payload, "target_type"),
		Target:       readMapString(payload, "target"),
		Reason:       readMapString(payload, "reason"),
		DecisionHint: readMapString(payload, "decision"),
	}
}

func (s *Service) emitReconnectAlert(ctx context.Context, method string, sourceErr error) {
	if s == nil || s.alerts == nil {
		return
	}
	now := s.now()
	if !s.recordEventWithinWindow(&s.reconnectEvents, now, s.cfg.AlertCooldown, s.cfg.ReconnectAlertThresh) {
		return
	}
	s.alerts.Emit(ctx, Alert{
		Name:      "gateway_connection_jitter",
		Severity:  "warning",
		Message:   fmt.Sprintf("gateway reconnects exceeded %d within %s, method=%s error=%v", s.cfg.ReconnectAlertThresh, s.cfg.AlertCooldown, method, sourceErr),
		Timestamp: now,
		Labels: map[string]string{
			"method": strings.TrimSpace(method),
		},
	})
}

func (s *Service) emitAuthFailureAlert(ctx context.Context, method string, sourceErr error) {
	if s == nil || s.alerts == nil {
		return
	}
	now := s.now()
	if !s.recordEventWithinWindow(&s.authFailureEvents, now, s.cfg.AlertCooldown, s.cfg.AuthFailureThresh) {
		return
	}
	s.alerts.Emit(ctx, Alert{
		Name:      "gateway_auth_failure_spike",
		Severity:  "critical",
		Message:   fmt.Sprintf("gateway auth failures exceeded %d within %s, method=%s error=%v", s.cfg.AuthFailureThresh, s.cfg.AlertCooldown, method, sourceErr),
		Timestamp: now,
		Labels: map[string]string{
			"method": strings.TrimSpace(method),
		},
	})
}

func (s *Service) recordEventWithinWindow(events *[]time.Time, now time.Time, window time.Duration, threshold int) bool {
	if s == nil || events == nil || threshold <= 0 {
		return false
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if window <= 0 {
		window = defaultAlertCooldown
	}
	cutoff := now.Add(-window)

	s.alertWindowMu.Lock()
	defer s.alertWindowMu.Unlock()

	list := *events
	kept := make([]time.Time, 0, len(list)+1)
	for _, item := range list {
		if item.After(cutoff) {
			kept = append(kept, item)
		}
	}
	kept = append(kept, now)
	*events = kept
	return len(kept) >= threshold
}

func (s *Service) emitBacklogAlert(ctx context.Context, inflight int) {
	if s == nil || s.alerts == nil {
		return
	}
	if inflight < s.cfg.BacklogAlertThresh {
		return
	}
	s.alerts.Emit(ctx, Alert{
		Name:      "adapter_event_backlog",
		Severity:  "warning",
		Message:   fmt.Sprintf("inflight runs backlog reached %d", inflight),
		Timestamp: s.now(),
		Labels: map[string]string{
			"inflight": fmt.Sprintf("%d", inflight),
		},
	})
}

func (s *Service) emitWatchdogAlert(ctx context.Context, run *runState) {
	if s == nil || s.alerts == nil || run == nil {
		return
	}
	s.alerts.Emit(ctx, Alert{
		Name:      "run_terminal_timeout",
		Severity:  "critical",
		Message:   fmt.Sprintf("run timed out before terminal event, run_id=%s session_id=%s", run.record.RunID, run.record.SessionID),
		Timestamp: s.now(),
		Labels: map[string]string{
			"run_id":     run.record.RunID,
			"session_id": run.record.SessionID,
			"tenant":     run.record.TenantKey,
		},
	})
}

func writeJSON(w http.ResponseWriter, statusCode int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(payload)
}

func asMap(value any) map[string]any {
	switch typed := value.(type) {
	case map[string]any:
		return typed
	case nil:
		return map[string]any{}
	default:
		raw, err := json.Marshal(typed)
		if err != nil {
			return map[string]any{}
		}
		var out map[string]any
		if err := json.Unmarshal(raw, &out); err != nil {
			return map[string]any{}
		}
		return out
	}
}

func readMapValue(m map[string]any, key string) any {
	if len(m) == 0 {
		return nil
	}
	if value, exists := m[key]; exists {
		return value
	}
	for existingKey, value := range m {
		if strings.EqualFold(strings.TrimSpace(existingKey), strings.TrimSpace(key)) {
			return value
		}
	}
	return nil
}

func readMapString(m map[string]any, key string) string {
	value := readMapValue(m, key)
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case nil:
		return ""
	default:
		return strings.TrimSpace(fmt.Sprintf("%v", typed))
	}
}

func renderRuntimeText(payload any) string {
	switch typed := payload.(type) {
	case string:
		return strings.TrimSpace(typed)
	case nil:
		return ""
	}

	asMapPayload := asMap(payload)
	if text := readMapString(asMapPayload, "text"); text != "" {
		return text
	}
	if content := readMapValue(asMapPayload, "content"); content != nil {
		if text := renderRuntimeText(content); text != "" {
			return text
		}
	}
	if parts := readMapValue(asMapPayload, "parts"); parts != nil {
		if array, ok := parts.([]any); ok {
			var segments []string
			for _, item := range array {
				itemMap := asMap(item)
				if strings.EqualFold(readMapString(itemMap, "kind"), "text") || readMapString(itemMap, "kind") == "" {
					text := readMapString(itemMap, "text")
					if text != "" {
						segments = append(segments, text)
					}
				}
			}
			if len(segments) > 0 {
				return strings.Join(segments, "\n")
			}
		}
	}
	raw, _ := json.Marshal(payload)
	return strings.TrimSpace(string(raw))
}
