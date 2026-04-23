package feishu

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"neo-code/internal/gateway"
	"neo-code/internal/gateway/protocol"
)

type fakeMessenger struct {
	messages []string
	cards    []PermissionCardData
}

type fakeAlertSink struct {
	alerts []Alert
}

func (f *fakeAlertSink) Send(_ context.Context, alert Alert) error {
	f.alerts = append(f.alerts, alert)
	return nil
}

func (f *fakeMessenger) SendText(_ context.Context, _ TenantRoute, _ MessageTarget, text string) error {
	f.messages = append(f.messages, strings.TrimSpace(text))
	return nil
}

func (f *fakeMessenger) SendPermissionCard(
	_ context.Context,
	_ TenantRoute,
	_ MessageTarget,
	card PermissionCardData,
) error {
	f.cards = append(f.cards, card)
	return nil
}

func TestBuildSessionIDStable(t *testing.T) {
	first := BuildSessionID("app_1", "chat_1", "thread_1")
	second := BuildSessionID("app_1", "chat_1", "thread_1")
	if first != second {
		t.Fatalf("session hash should be stable: %q != %q", first, second)
	}

	withThread := BuildSessionID("app_1", "chat_1", "")
	withChatAsThread := BuildSessionID("app_1", "chat_1", "chat_1")
	if withThread != withChatAsThread {
		t.Fatalf("empty thread should fallback to chat id")
	}
}

func TestDedupeStoreSeenOrAdd(t *testing.T) {
	now := time.Unix(1700000000, 0).UTC()
	store := NewDedupeStore(time.Minute)
	if seen := store.SeenOrAdd("event-1", now); seen {
		t.Fatal("first event should not be seen")
	}
	if seen := store.SeenOrAdd("event-1", now.Add(30*time.Second)); !seen {
		t.Fatal("second event should be deduplicated")
	}
	if seen := store.SeenOrAdd("event-1", now.Add(2*time.Minute)); seen {
		t.Fatal("event should expire after ttl")
	}
}

func TestSignatureVerifier(t *testing.T) {
	secret := "test-secret"
	body := []byte(`{"event":"ok"}`)
	timestamp := "1700000000"
	nonce := "nonce-1"

	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(timestamp + nonce + string(body)))
	signature := hex.EncodeToString(mac.Sum(nil))

	verifier := SignatureVerifier{
		Secret:  secret,
		MaxSkew: 10 * time.Minute,
		Now: func() time.Time {
			return time.Unix(1700000000, 0).UTC()
		},
	}
	if err := verifier.Verify(timestamp, nonce, signature, body); err != nil {
		t.Fatalf("verify signature failed: %v", err)
	}
	if err := verifier.Verify(timestamp, nonce, "bad-signature", body); err == nil {
		t.Fatal("invalid signature should fail")
	}
}

func TestChunkAggregatorFlush(t *testing.T) {
	agg := NewChunkAggregator(10, time.Minute)
	now := time.Unix(1700000000, 0).UTC()
	if flushed, ok := agg.Add("hello", now); ok || flushed != "" {
		t.Fatalf("unexpected flush: %q", flushed)
	}
	flushed, ok := agg.Add("world", now.Add(time.Second))
	if !ok {
		t.Fatal("expected flush by max chars")
	}
	if !strings.Contains(flushed, "hello") || !strings.Contains(flushed, "world") {
		t.Fatalf("unexpected flushed payload: %q", flushed)
	}
}

func TestResolveRouteOverlay(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Routes.Default = TenantRoute{Enabled: true, Streaming: true, Workdir: "/default"}
	cfg.Routes.Tenants["tenant-a"] = TenantRoute{Enabled: true, Streaming: false, Workdir: "/tenant"}
	cfg.Routes.Chats["tenant-a:chat-1"] = TenantRoute{Enabled: true, Streaming: true}

	service := &Service{cfg: cfg}
	route := service.resolveRoute("tenant-a", "chat-1")
	if !route.Enabled {
		t.Fatal("route should be enabled")
	}
	if !route.Streaming {
		t.Fatal("chat override should enable streaming")
	}
	if route.Workdir != "/tenant" {
		t.Fatalf("workdir = %q, want /tenant", route.Workdir)
	}
}

func TestHandleGatewayNotificationRunDone(t *testing.T) {
	messenger := &fakeMessenger{}
	cfg := DefaultConfig()
	service := &Service{
		cfg:       cfg,
		messenger: messenger,
		metrics:   &Metrics{},
		now:       func() time.Time { return time.Unix(1700000000, 0).UTC() },
		runs: map[string]*runState{
			"run-1": {
				record: RunRecord{
					RunID:      "run-1",
					SessionID:  "session-1",
					Target:     MessageTarget{ChatID: "chat-1"},
					Streaming:  true,
					AcceptedAt: time.Unix(1700000000, 0).UTC(),
				},
				route:         TenantRoute{Enabled: true, Streaming: true},
				aggregator:    NewChunkAggregator(100, time.Minute),
				permissionIDs: map[string]struct{}{},
			},
		},
		permissionIndex: map[string]string{},
	}
	_, _ = service.runs["run-1"].aggregator.Add("chunk text", time.Unix(1700000000, 0).UTC())

	frame := gateway.MessageFrame{
		RunID:     "run-1",
		SessionID: "session-1",
		Payload: map[string]any{
			"event_type": string(gateway.RuntimeEventTypeRunDone),
			"payload": map[string]any{
				"runtime_event_type": "agent_done",
				"payload": map[string]any{
					"text": "final text",
				},
			},
		},
	}
	err := service.handleGatewayNotification(context.Background(), protocol.JSONRPCNotification{
		Method: protocol.MethodGatewayEvent,
		Params: frame,
	})
	if err != nil {
		t.Fatalf("handleGatewayNotification() error = %v", err)
	}
	if len(messenger.messages) == 0 {
		t.Fatal("expected final message to be sent")
	}
	if _, exists := service.runs["run-1"]; exists {
		t.Fatal("run should be removed after run_done")
	}
	if service.metrics.Snapshot().RunsCompleted != 1 {
		t.Fatal("runs_completed should be increased")
	}
}

func TestHandleGatewayNotificationPermissionRequested(t *testing.T) {
	messenger := &fakeMessenger{}
	service := &Service{
		cfg:       DefaultConfig(),
		messenger: messenger,
		metrics:   &Metrics{},
		now:       func() time.Time { return time.Unix(1700000000, 0).UTC() },
		runs: map[string]*runState{
			"run-1": {
				record: RunRecord{
					RunID:      "run-1",
					SessionID:  "session-1",
					Target:     MessageTarget{ChatID: "chat-1"},
					Streaming:  true,
					AcceptedAt: time.Unix(1700000000, 0).UTC(),
				},
				route:         TenantRoute{Enabled: true, Streaming: true},
				aggregator:    NewChunkAggregator(100, time.Minute),
				permissionIDs: map[string]struct{}{},
			},
		},
		permissionIndex: map[string]string{},
	}
	frame := gateway.MessageFrame{
		RunID: "run-1",
		Payload: map[string]any{
			"event_type": string(gateway.RuntimeEventTypeRunProgress),
			"payload": map[string]any{
				"runtime_event_type": "permission_requested",
				"payload": map[string]any{
					"request_id": "perm-1",
				},
			},
		},
	}
	if err := service.handleGatewayNotification(context.Background(), protocol.JSONRPCNotification{
		Method: protocol.MethodGatewayEvent,
		Params: frame,
	}); err != nil {
		t.Fatalf("handleGatewayNotification() error = %v", err)
	}
	if runID, ok := service.permissionIndex["perm-1"]; !ok || runID != "run-1" {
		t.Fatalf("permission index mismatch: run_id=%q ok=%v", runID, ok)
	}
	if service.metrics.Snapshot().PermissionsPending != 1 {
		t.Fatal("permissions_pending should be updated")
	}
	if len(messenger.cards) != 1 {
		t.Fatalf("expected permission card message, got %d", len(messenger.cards))
	}
	if messenger.cards[0].RequestID != "perm-1" {
		t.Fatalf("card request id = %q, want perm-1", messenger.cards[0].RequestID)
	}
}

func TestCallGatewayFrameWithRetryUnauthorizedThenSuccess(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ReconnectBaseBackoff = 1 * time.Millisecond
	cfg.ReconnectMaxBackoff = 2 * time.Millisecond

	service := &Service{
		cfg:     cfg,
		metrics: &Metrics{},
		now:     func() time.Time { return time.Now().UTC() },
	}

	callCount := 0
	authCount := 0
	service.gatewayAuthenticate = func(context.Context) error {
		authCount++
		return nil
	}
	service.gatewayCall = func(_ context.Context, _ string, _ any, result any) error {
		callCount++
		if callCount == 1 {
			return &GatewayRPCError{
				Method:      protocol.MethodGatewayRun,
				GatewayCode: protocol.GatewayCodeUnauthorized,
				Message:     "unauthorized",
			}
		}
		frame, ok := result.(*gateway.MessageFrame)
		if !ok {
			t.Fatalf("result type = %T", result)
		}
		*frame = gateway.MessageFrame{RunID: "run-1"}
		return nil
	}

	frame, err := service.callGatewayFrameWithRetry(context.Background(), protocol.MethodGatewayRun, nil)
	if err != nil {
		t.Fatalf("callGatewayFrameWithRetry() error = %v", err)
	}
	if frame.RunID != "run-1" {
		t.Fatalf("frame.run_id = %q", frame.RunID)
	}
	if authCount == 0 {
		t.Fatal("expected re-auth when unauthorized")
	}
	if callCount != 2 {
		t.Fatalf("call count = %d, want 2", callCount)
	}
}

func TestCallGatewayFrameWithRetryNonRetryable(t *testing.T) {
	service := &Service{
		cfg:     DefaultConfig(),
		metrics: &Metrics{},
		now:     func() time.Time { return time.Now().UTC() },
	}
	callCount := 0
	service.gatewayAuthenticate = func(context.Context) error { return nil }
	service.gatewayCall = func(_ context.Context, _ string, _ any, _ any) error {
		callCount++
		return &GatewayRPCError{
			Method:      protocol.MethodGatewayRun,
			GatewayCode: protocol.GatewayCodeInvalidAction,
			Message:     "invalid",
		}
	}
	_, err := service.callGatewayFrameWithRetry(context.Background(), protocol.MethodGatewayRun, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if callCount != 1 {
		t.Fatalf("non-retryable code should not retry, got %d attempts", callCount)
	}
}

func TestDedupeStorePersistent(t *testing.T) {
	now := time.Now().UTC().Add(-5 * time.Second)
	path := filepath.Join(t.TempDir(), "dedupe.json")
	store := NewPersistentDedupeStore(time.Minute, path, nil)
	if seen := store.SeenOrAdd("event-1", now); seen {
		t.Fatal("first event should not be seen")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("dedupe store should be persisted: %v", err)
	}

	storeReloaded := NewPersistentDedupeStore(time.Minute, path, nil)
	if !storeReloaded.Seen("event-1", now.Add(10*time.Second)) {
		t.Fatal("reloaded dedupe store should keep valid entry")
	}
}

func TestDecodePermissionCallbackFromCardPayload(t *testing.T) {
	raw := []byte(`{"event":{"action":{"value":{"request_id":"perm-1","decision":"allow_session"}}}}`)
	callback, err := decodePermissionCallback(raw)
	if err != nil {
		t.Fatalf("decodePermissionCallback() error = %v", err)
	}
	if callback.RequestID != "perm-1" {
		t.Fatalf("request_id = %q, want perm-1", callback.RequestID)
	}
	if callback.Decision != "allow_session" {
		t.Fatalf("decision = %q, want allow_session", callback.Decision)
	}
}

func TestSetRunEmitsBacklogAlert(t *testing.T) {
	sink := &fakeAlertSink{}
	cfg := DefaultConfig()
	cfg.BacklogAlertThresh = 1

	service := &Service{
		cfg:             cfg,
		metrics:         &Metrics{},
		now:             func() time.Time { return time.Unix(1700000000, 0).UTC() },
		alerts:          NewAlertManager(sink, time.Millisecond, nil),
		runs:            map[string]*runState{},
		permissionIndex: map[string]string{},
	}
	service.setRun(&runState{
		record: RunRecord{
			RunID:      "run-1",
			SessionID:  "session-1",
			Target:     MessageTarget{ChatID: "chat-1"},
			Streaming:  true,
			AcceptedAt: time.Unix(1700000000, 0).UTC(),
		},
		route:         TenantRoute{Enabled: true, Streaming: true},
		aggregator:    NewChunkAggregator(100, time.Minute),
		permissionIDs: map[string]struct{}{},
	})
	if len(sink.alerts) == 0 {
		t.Fatal("expected backlog alert")
	}
	if sink.alerts[0].Name != "adapter_event_backlog" {
		t.Fatalf("alert name = %q, want adapter_event_backlog", sink.alerts[0].Name)
	}
}

func TestCallGatewayFrameWithRetryEmitsAuthAlert(t *testing.T) {
	sink := &fakeAlertSink{}
	cfg := DefaultConfig()
	cfg.AuthFailureThresh = 1
	cfg.ReconnectBaseBackoff = 1 * time.Millisecond
	cfg.ReconnectMaxBackoff = 1 * time.Millisecond

	service := &Service{
		cfg:     cfg,
		metrics: &Metrics{},
		now:     func() time.Time { return time.Unix(1700000000, 0).UTC() },
		alerts:  NewAlertManager(sink, time.Millisecond, nil),
	}
	service.gatewayAuthenticate = func(context.Context) error { return nil }
	service.gatewayCall = func(_ context.Context, _ string, _ any, _ any) error {
		return &GatewayRPCError{
			Method:      protocol.MethodGatewayRun,
			GatewayCode: protocol.GatewayCodeUnauthorized,
			Message:     "unauthorized",
		}
	}
	_, _ = service.callGatewayFrameWithRetry(context.Background(), protocol.MethodGatewayRun, nil)
	if len(sink.alerts) == 0 {
		t.Fatal("expected auth failure alert")
	}
	if sink.alerts[0].Name != "gateway_auth_failure_spike" {
		t.Fatalf("alert name = %q, want gateway_auth_failure_spike", sink.alerts[0].Name)
	}
}

func TestActionCallbacksRequireSignature(t *testing.T) {
	service := &Service{
		signer: SignatureVerifier{
			Secret:  "secret-1",
			MaxSkew: 5 * time.Minute,
			Now:     func() time.Time { return time.Unix(1700000000, 0).UTC() },
		},
		metrics: &Metrics{},
	}

	cancelReq := httptest.NewRequest(http.MethodPost, "/feishu/actions/cancel", bytes.NewBufferString(`{"run_id":"run-1"}`))
	cancelRec := httptest.NewRecorder()
	service.handleCancelAction(cancelRec, cancelReq)
	if cancelRec.Code != http.StatusUnauthorized {
		t.Fatalf("cancel status = %d, want 401", cancelRec.Code)
	}

	permReq := httptest.NewRequest(http.MethodPost, "/feishu/actions/permission", bytes.NewBufferString(`{"request_id":"perm-1","decision":"reject"}`))
	permRec := httptest.NewRecorder()
	service.handlePermissionAction(permRec, permReq)
	if permRec.Code != http.StatusUnauthorized {
		t.Fatalf("permission status = %d, want 401", permRec.Code)
	}
}

func TestActionCallbacksAcceptValidSignature(t *testing.T) {
	now := time.Unix(1700000000, 0).UTC()
	service := &Service{
		cfg: DefaultConfig(),
		signer: SignatureVerifier{
			Secret:  "secret-1",
			MaxSkew: 5 * time.Minute,
			Now:     func() time.Time { return now },
		},
		metrics: &Metrics{},
		runs:    map[string]*runState{},
	}

	cancelBody := []byte(`{"run_id":"run-1"}`)
	cancelReq := httptest.NewRequest(http.MethodPost, "/feishu/actions/cancel", bytes.NewReader(cancelBody))
	applySignedHeaders(cancelReq, "secret-1", now, "nonce-1", cancelBody)
	cancelRec := httptest.NewRecorder()
	service.handleCancelAction(cancelRec, cancelReq)
	if cancelRec.Code != http.StatusNotFound {
		t.Fatalf("cancel status = %d, want 404 after signature passes", cancelRec.Code)
	}

	permBody := []byte(`{"request_id":"perm-1","decision":"reject"}`)
	permReq := httptest.NewRequest(http.MethodPost, "/feishu/actions/permission", bytes.NewReader(permBody))
	applySignedHeaders(permReq, "secret-1", now, "nonce-2", permBody)
	permRec := httptest.NewRecorder()
	service.handlePermissionAction(permRec, permReq)
	if permRec.Code != http.StatusNotFound {
		t.Fatalf("permission status = %d, want 404 after signature passes", permRec.Code)
	}
}

func TestAuthFailureAlertUsesTimeWindow(t *testing.T) {
	sink := &fakeAlertSink{}
	cfg := DefaultConfig()
	cfg.AlertCooldown = 1 * time.Second
	cfg.AuthFailureThresh = 2

	current := time.Unix(1700000000, 0).UTC()
	service := &Service{
		cfg:     cfg,
		metrics: &Metrics{},
		now:     func() time.Time { return current },
		alerts:  NewAlertManager(sink, time.Millisecond, nil),
	}

	service.emitAuthFailureAlert(context.Background(), protocol.MethodGatewayRun, errors.New("auth"))
	if len(sink.alerts) != 0 {
		t.Fatalf("unexpected alert count: %d", len(sink.alerts))
	}

	current = current.Add(2 * time.Second)
	service.emitAuthFailureAlert(context.Background(), protocol.MethodGatewayRun, errors.New("auth"))
	if len(sink.alerts) != 0 {
		t.Fatalf("unexpected alert count after window shift: %d", len(sink.alerts))
	}

	current = current.Add(100 * time.Millisecond)
	service.emitAuthFailureAlert(context.Background(), protocol.MethodGatewayRun, errors.New("auth"))
	if len(sink.alerts) != 1 {
		t.Fatalf("alert count = %d, want 1", len(sink.alerts))
	}
}

func applySignedHeaders(r *http.Request, secret string, now time.Time, nonce string, body []byte) {
	timestamp := fmt.Sprintf("%d", now.Unix())
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(timestamp + nonce + string(body)))
	signature := hex.EncodeToString(mac.Sum(nil))
	r.Header.Set("X-Lark-Request-Timestamp", timestamp)
	r.Header.Set("X-Lark-Request-Nonce", nonce)
	r.Header.Set("X-Lark-Signature", signature)
}
