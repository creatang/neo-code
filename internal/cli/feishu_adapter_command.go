package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"neo-code/internal/gateway"
	"neo-code/internal/gateway/adapters/feishu"
	"neo-code/internal/gateway/protocol"
)

type feishuAdapterCommandOptions struct {
	ListenAddress        string
	GatewayWSURL         string
	GatewayWSOrigin      string
	GatewayTokenFile     string
	FeishuBaseURL        string
	SigningSecret        string
	RouteFile            string
	StreamingEnabled     bool
	RequestTimeoutSec    int
	PingIntervalSec      int
	WatchdogTimeoutSec   int
	EventDedupeTTLMin    int
	ChunkFlushIntervalMs int
	ChunkFlushMaxChars   int

	DedupeStoreFile      string
	AlertWebhookURL      string
	AlertCooldownSec     int
	ReconnectAlertThresh int
	AuthFailureThresh    int
	BacklogAlertThresh   int

	CompatCheck   bool
	CompatTargets string
}

var runFeishuAdapterCommand = defaultFeishuAdapterCommandRunner
var runFeishuAdapterCompatibilityCheck = defaultFeishuAdapterCompatibilityCheck

func newFeishuAdapterCommand() *cobra.Command {
	defaultConfig := feishu.DefaultConfig()
	options := &feishuAdapterCommandOptions{}

	cmd := &cobra.Command{
		Use:   "feishu-adapter",
		Short: "Run Feishu gateway adapter",
		Args:  cobra.NoArgs,
		Annotations: map[string]string{
			commandAnnotationSkipSilentUpdateCheck: "true",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := defaultConfig
			cfg.ListenAddress = strings.TrimSpace(options.ListenAddress)
			cfg.GatewayWSURL = strings.TrimSpace(options.GatewayWSURL)
			cfg.GatewayWSOrigin = strings.TrimSpace(options.GatewayWSOrigin)
			cfg.GatewayTokenFile = strings.TrimSpace(options.GatewayTokenFile)
			cfg.FeishuBaseURL = strings.TrimSpace(options.FeishuBaseURL)
			cfg.SigningSecret = strings.TrimSpace(options.SigningSecret)
			cfg.StreamingEnabled = options.StreamingEnabled
			cfg.Routes.Default.Streaming = options.StreamingEnabled
			cfg.DedupeStoreFile = strings.TrimSpace(options.DedupeStoreFile)
			cfg.AlertWebhookURL = strings.TrimSpace(options.AlertWebhookURL)

			if options.RequestTimeoutSec > 0 {
				cfg.RequestTimeout = time.Duration(options.RequestTimeoutSec) * time.Second
			}
			if options.PingIntervalSec > 0 {
				cfg.PingInterval = time.Duration(options.PingIntervalSec) * time.Second
			}
			if options.WatchdogTimeoutSec > 0 {
				cfg.WatchdogTimeout = time.Duration(options.WatchdogTimeoutSec) * time.Second
			}
			if options.EventDedupeTTLMin > 0 {
				cfg.EventDedupeTTL = time.Duration(options.EventDedupeTTLMin) * time.Minute
			}
			if options.ChunkFlushIntervalMs > 0 {
				cfg.ChunkFlushInterval = time.Duration(options.ChunkFlushIntervalMs) * time.Millisecond
			}
			if options.ChunkFlushMaxChars > 0 {
				cfg.ChunkFlushMaxChars = options.ChunkFlushMaxChars
			}
			if options.AlertCooldownSec > 0 {
				cfg.AlertCooldown = time.Duration(options.AlertCooldownSec) * time.Second
			}
			if options.ReconnectAlertThresh > 0 {
				cfg.ReconnectAlertThresh = options.ReconnectAlertThresh
			}
			if options.AuthFailureThresh > 0 {
				cfg.AuthFailureThresh = options.AuthFailureThresh
			}
			if options.BacklogAlertThresh > 0 {
				cfg.BacklogAlertThresh = options.BacklogAlertThresh
			}

			routeFile := strings.TrimSpace(options.RouteFile)
			if routeFile != "" {
				routes, err := feishu.LoadRoutesFromFile(routeFile)
				if err != nil {
					return err
				}
				cfg.Routes = routes
				if !cfg.Routes.Default.Enabled {
					cfg.Routes.Default.Enabled = true
				}
			}

			if options.CompatCheck {
				targets := parseCompatTargets(options.CompatTargets, cfg.GatewayWSURL)
				return runFeishuAdapterCompatibilityCheck(cmd.Context(), cfg, targets)
			}
			return runFeishuAdapterCommand(cmd.Context(), cfg)
		},
	}

	cmd.Flags().StringVar(&options.ListenAddress, "listen", defaultConfig.ListenAddress, "feishu adapter listen address")
	cmd.Flags().StringVar(&options.GatewayWSURL, "gateway-ws", defaultConfig.GatewayWSURL, "gateway websocket endpoint")
	cmd.Flags().StringVar(&options.GatewayWSOrigin, "gateway-origin", defaultConfig.GatewayWSOrigin, "gateway websocket origin")
	cmd.Flags().StringVar(&options.GatewayTokenFile, "gateway-token-file", "", "gateway auth token file path")
	cmd.Flags().StringVar(&options.FeishuBaseURL, "feishu-base-url", defaultConfig.FeishuBaseURL, "feishu open api base url")
	cmd.Flags().StringVar(&options.SigningSecret, "signing-secret", "", "feishu signing secret")
	cmd.Flags().StringVar(&options.RouteFile, "route-file", "", "tenant/chat route json file")
	cmd.Flags().BoolVar(&options.StreamingEnabled, "streaming-enabled", defaultConfig.StreamingEnabled, "enable chunk streaming replies")
	cmd.Flags().IntVar(&options.RequestTimeoutSec, "request-timeout-sec", int(defaultConfig.RequestTimeout/time.Second), "gateway rpc timeout seconds")
	cmd.Flags().IntVar(&options.PingIntervalSec, "ping-interval-sec", int(defaultConfig.PingInterval/time.Second), "gateway ping interval seconds")
	cmd.Flags().IntVar(&options.WatchdogTimeoutSec, "watchdog-timeout-sec", int(defaultConfig.WatchdogTimeout/time.Second), "run watchdog timeout seconds")
	cmd.Flags().IntVar(&options.EventDedupeTTLMin, "event-dedupe-ttl-min", int(defaultConfig.EventDedupeTTL/time.Minute), "event dedupe ttl minutes")
	cmd.Flags().IntVar(
		&options.ChunkFlushIntervalMs,
		"chunk-flush-interval-ms",
		int(defaultConfig.ChunkFlushInterval/time.Millisecond),
		"chunk flush interval milliseconds",
	)
	cmd.Flags().IntVar(&options.ChunkFlushMaxChars, "chunk-flush-max-chars", defaultConfig.ChunkFlushMaxChars, "chunk flush max chars")

	cmd.Flags().StringVar(&options.DedupeStoreFile, "dedupe-store-file", "", "event dedupe store file path")
	cmd.Flags().StringVar(&options.AlertWebhookURL, "alert-webhook-url", "", "feishu webhook for adapter alerts")
	cmd.Flags().IntVar(&options.AlertCooldownSec, "alert-cooldown-sec", int(defaultConfig.AlertCooldown/time.Second), "alert cooldown seconds")
	cmd.Flags().IntVar(&options.ReconnectAlertThresh, "reconnect-alert-thresh", defaultConfig.ReconnectAlertThresh, "reconnect threshold for alerting")
	cmd.Flags().IntVar(&options.AuthFailureThresh, "auth-failure-thresh", defaultConfig.AuthFailureThresh, "auth failure threshold for alerting")
	cmd.Flags().IntVar(&options.BacklogAlertThresh, "backlog-alert-thresh", defaultConfig.BacklogAlertThresh, "inflight backlog threshold for alerting")

	cmd.Flags().BoolVar(&options.CompatCheck, "compat-check", false, "run gateway compatibility checks and exit")
	cmd.Flags().StringVar(&options.CompatTargets, "compat-targets", "", "comma-separated gateway ws endpoints for compatibility check")

	return cmd
}

func defaultFeishuAdapterCommandRunner(ctx context.Context, cfg feishu.Config) error {
	logger := log.New(os.Stderr, "neocode-feishu-adapter: ", log.LstdFlags)
	service, err := feishu.NewService(feishu.ServiceOptions{
		Config: cfg,
		Logger: logger,
	})
	if err != nil {
		return fmt.Errorf("init feishu adapter service: %w", err)
	}
	defer func() {
		_ = service.Close()
	}()

	logger.Printf("starting feishu adapter on %s", cfg.ListenAddress)
	return service.Serve(ctx)
}

func defaultFeishuAdapterCompatibilityCheck(ctx context.Context, cfg feishu.Config, targets []string) error {
	logger := log.New(os.Stderr, "neocode-feishu-compat: ", log.LstdFlags)
	if len(targets) == 0 {
		return fmt.Errorf("compatibility check requires at least one target")
	}

	var failures []string
	for _, target := range targets {
		target = strings.TrimSpace(target)
		if target == "" {
			continue
		}
		if err := probeGatewayCompatibility(ctx, cfg, target, logger); err != nil {
			failures = append(failures, fmt.Sprintf("%s => %v", target, err))
			continue
		}
		logger.Printf("compatibility check passed: %s", target)
	}
	if len(failures) > 0 {
		return fmt.Errorf("compatibility check failed: %s", strings.Join(failures, "; "))
	}
	return nil
}

func probeGatewayCompatibility(ctx context.Context, cfg feishu.Config, target string, logger *log.Logger) error {
	client, err := feishu.NewGatewayWSClient(feishu.GatewayWSClientOptions{
		URL:       strings.TrimSpace(target),
		Origin:    cfg.GatewayWSOrigin,
		TokenFile: cfg.GatewayTokenFile,
		Logger:    logger,
	})
	if err != nil {
		return err
	}
	defer func() {
		_ = client.Close()
	}()

	authCtx, cancel := context.WithTimeout(ctx, cfg.RequestTimeout)
	err = client.Authenticate(authCtx)
	cancel()
	if err != nil {
		return fmt.Errorf("gateway.authenticate failed: %w", err)
	}

	checks := []struct {
		method string
		params any
	}{
		{method: protocol.MethodGatewayPing, params: struct{}{}},
		{method: protocol.MethodGatewayListSessions, params: nil},
	}
	for _, check := range checks {
		probeCtx, probeCancel := context.WithTimeout(ctx, cfg.RequestTimeout)
		var result map[string]any
		err := client.Call(probeCtx, check.method, check.params, &result)
		probeCancel()
		if err == nil {
			continue
		}
		rpcErr, ok := err.(*feishu.GatewayRPCError)
		if ok && strings.EqualFold(strings.TrimSpace(rpcErr.GatewayCode), protocol.GatewayCodeUnsupportedAction) {
			return fmt.Errorf("unsupported stable method %s", check.method)
		}
		return fmt.Errorf("%s failed: %w", check.method, err)
	}
	if err := probeGatewayRunLifecycle(ctx, cfg, client); err != nil {
		return err
	}
	return nil
}

func probeGatewayRunLifecycle(ctx context.Context, cfg feishu.Config, client *feishu.GatewayWSClient) error {
	now := time.Now().UTC().UnixNano()
	sessionID := fmt.Sprintf("compat-session-%d", now)
	runID := fmt.Sprintf("compat-run-%d", now)

	bindCtx, bindCancel := context.WithTimeout(ctx, cfg.RequestTimeout)
	err := client.BindStream(bindCtx, sessionID, runID)
	bindCancel()
	if err != nil {
		return fmt.Errorf("gateway.bindStream failed: %w", err)
	}

	runCtx, runCancel := context.WithTimeout(ctx, cfg.RequestTimeout)
	var runFrame map[string]any
	err = client.Call(runCtx, protocol.MethodGatewayRun, protocol.RunParams{
		SessionID: sessionID,
		RunID:     runID,
		InputText: "compatibility probe",
	}, &runFrame)
	runCancel()
	if err != nil {
		return fmt.Errorf("gateway.run failed: %w", err)
	}

	cancelCtx, cancelCancel := context.WithTimeout(ctx, cfg.RequestTimeout)
	var cancelFrame map[string]any
	err = client.Call(cancelCtx, protocol.MethodGatewayCancel, protocol.CancelParams{
		SessionID: sessionID,
		RunID:     runID,
	}, &cancelFrame)
	cancelCancel()
	if err != nil {
		return fmt.Errorf("gateway.cancel failed: %w", err)
	}

	waitTimeout := cfg.RequestTimeout * 3
	if waitTimeout < 10*time.Second {
		waitTimeout = 10 * time.Second
	}
	waitCtx, waitCancel := context.WithTimeout(ctx, waitTimeout)
	defer waitCancel()

	for {
		select {
		case <-waitCtx.Done():
			return fmt.Errorf("terminal event timeout for run_id=%s", runID)
		case notification, ok := <-client.Notifications():
			if !ok {
				return fmt.Errorf("notification channel closed before terminal event")
			}
			if strings.TrimSpace(notification.Method) != protocol.MethodGatewayEvent {
				continue
			}
			frame, frameErr := decodeGatewayEventFrame(notification.Params)
			if frameErr != nil {
				continue
			}
			if strings.TrimSpace(frame.RunID) != runID {
				continue
			}
			eventType := strings.TrimSpace(readEventType(frame.Payload))
			if eventType == string(gateway.RuntimeEventTypeRunDone) || eventType == string(gateway.RuntimeEventTypeRunError) {
				return nil
			}
		}
	}
}

func decodeGatewayEventFrame(params any) (gateway.MessageFrame, error) {
	var raw []byte
	switch typed := params.(type) {
	case json.RawMessage:
		raw = append(raw, typed...)
	case []byte:
		raw = append(raw, typed...)
	default:
		marshaled, err := json.Marshal(typed)
		if err != nil {
			return gateway.MessageFrame{}, err
		}
		raw = marshaled
	}

	var frame gateway.MessageFrame
	if err := json.Unmarshal(raw, &frame); err != nil {
		return gateway.MessageFrame{}, err
	}
	return frame, nil
}

func readEventType(payload any) string {
	payloadMap, ok := payload.(map[string]any)
	if !ok {
		return ""
	}
	value, exists := payloadMap["event_type"]
	if !exists || value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprintf("%v", value))
}

func parseCompatTargets(raw string, fallback string) []string {
	joined := strings.TrimSpace(raw)
	if joined == "" {
		joined = strings.TrimSpace(fallback)
	}
	if joined == "" {
		return nil
	}

	normalized := strings.NewReplacer(";", ",", "\n", ",", "\t", ",", " ", ",").Replace(joined)
	parts := strings.Split(normalized, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		candidate := strings.TrimSpace(part)
		if candidate == "" {
			continue
		}
		out = append(out, candidate)
	}
	return out
}
