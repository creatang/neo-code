package cli

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"neo-code/internal/gateway/adapters/feishu"
)

func TestFeishuAdapterCommandPassesFlags(t *testing.T) {
	originalRunner := runFeishuAdapterCommand
	originalCompatRunner := runFeishuAdapterCompatibilityCheck
	originalPreload := runGlobalPreload
	t.Cleanup(func() {
		runFeishuAdapterCommand = originalRunner
		runFeishuAdapterCompatibilityCheck = originalCompatRunner
		runGlobalPreload = originalPreload
	})
	runGlobalPreload = func(context.Context) error { return nil }
	runFeishuAdapterCompatibilityCheck = func(context.Context, feishu.Config, []string) error { return nil }

	var captured feishu.Config
	runFeishuAdapterCommand = func(_ context.Context, cfg feishu.Config) error {
		captured = cfg
		return nil
	}

	command := NewRootCommand()
	command.SetArgs([]string{
		"feishu-adapter",
		"--listen", "127.0.0.1:29100",
		"--gateway-ws", "ws://127.0.0.1:8088/ws",
		"--gateway-origin", "http://localhost:8088",
		"--gateway-token-file", "./testdata/auth.json",
		"--signing-secret", "sec-test",
		"--streaming-enabled=false",
		"--request-timeout-sec", "5",
		"--watchdog-timeout-sec", "60",
		"--dedupe-store-file", "./tmp/dedupe.json",
		"--alert-webhook-url", "https://example.com/alert",
		"--reconnect-alert-thresh", "9",
		"--auth-failure-thresh", "4",
		"--backlog-alert-thresh", "12",
	})
	if err := command.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("ExecuteContext() error = %v", err)
	}
	if captured.ListenAddress != "127.0.0.1:29100" {
		t.Fatalf("listen = %q", captured.ListenAddress)
	}
	if captured.GatewayWSURL != "ws://127.0.0.1:8088/ws" {
		t.Fatalf("gateway ws = %q", captured.GatewayWSURL)
	}
	if captured.SigningSecret != "sec-test" {
		t.Fatalf("signing secret = %q", captured.SigningSecret)
	}
	if captured.StreamingEnabled {
		t.Fatal("streaming should be disabled by flag")
	}
	if captured.DedupeStoreFile != "./tmp/dedupe.json" {
		t.Fatalf("dedupe store file = %q", captured.DedupeStoreFile)
	}
	if captured.AlertWebhookURL != "https://example.com/alert" {
		t.Fatalf("alert webhook url = %q", captured.AlertWebhookURL)
	}
	if captured.ReconnectAlertThresh != 9 {
		t.Fatalf("reconnect alert threshold = %d", captured.ReconnectAlertThresh)
	}
	if captured.AuthFailureThresh != 4 {
		t.Fatalf("auth failure threshold = %d", captured.AuthFailureThresh)
	}
	if captured.BacklogAlertThresh != 12 {
		t.Fatalf("backlog alert threshold = %d", captured.BacklogAlertThresh)
	}
}

func TestFeishuAdapterCommandLoadsRouteFile(t *testing.T) {
	originalRunner := runFeishuAdapterCommand
	originalCompatRunner := runFeishuAdapterCompatibilityCheck
	originalPreload := runGlobalPreload
	t.Cleanup(func() {
		runFeishuAdapterCommand = originalRunner
		runFeishuAdapterCompatibilityCheck = originalCompatRunner
		runGlobalPreload = originalPreload
	})
	runGlobalPreload = func(context.Context) error { return nil }
	runFeishuAdapterCompatibilityCheck = func(context.Context, feishu.Config, []string) error { return nil }

	routeFile := filepath.Join(t.TempDir(), "routes.json")
	payload := `{
  "default": {"enabled": true, "streaming": true, "workdir": "D:/default"},
  "tenants": {"tenant-a": {"enabled": true, "streaming": false, "workdir": "D:/tenant-a"}},
  "chats": {"tenant-a:chat-1": {"enabled": true, "streaming": true}}
}`
	if err := os.WriteFile(routeFile, []byte(payload), 0o600); err != nil {
		t.Fatalf("write route file: %v", err)
	}

	var captured feishu.Config
	runFeishuAdapterCommand = func(_ context.Context, cfg feishu.Config) error {
		captured = cfg
		return nil
	}

	command := NewRootCommand()
	command.SetArgs([]string{"feishu-adapter", "--route-file", routeFile})
	if err := command.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("ExecuteContext() error = %v", err)
	}

	if captured.Routes.Default.Workdir != "D:/default" {
		t.Fatalf("default workdir = %q", captured.Routes.Default.Workdir)
	}
	if route, ok := captured.Routes.Tenants["tenant-a"]; !ok || route.Workdir != "D:/tenant-a" {
		t.Fatalf("tenant route missing or mismatch: %#v", route)
	}
}

func TestFeishuAdapterCompatCheckMode(t *testing.T) {
	originalRunner := runFeishuAdapterCommand
	originalCompatRunner := runFeishuAdapterCompatibilityCheck
	originalPreload := runGlobalPreload
	t.Cleanup(func() {
		runFeishuAdapterCommand = originalRunner
		runFeishuAdapterCompatibilityCheck = originalCompatRunner
		runGlobalPreload = originalPreload
	})
	runGlobalPreload = func(context.Context) error { return nil }

	calledRun := false
	runFeishuAdapterCommand = func(context.Context, feishu.Config) error {
		calledRun = true
		return nil
	}

	var capturedCfg feishu.Config
	var capturedTargets []string
	runFeishuAdapterCompatibilityCheck = func(_ context.Context, cfg feishu.Config, targets []string) error {
		capturedCfg = cfg
		capturedTargets = append([]string(nil), targets...)
		return nil
	}

	command := NewRootCommand()
	command.SetArgs([]string{
		"feishu-adapter",
		"--compat-check",
		"--gateway-ws", "ws://127.0.0.1:8080/ws",
		"--compat-targets", "ws://10.0.0.1:8080/ws, ws://10.0.0.2:8080/ws",
	})
	if err := command.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("ExecuteContext() error = %v", err)
	}

	if calledRun {
		t.Fatal("service runner should not be called in compat-check mode")
	}
	wantTargets := []string{"ws://10.0.0.1:8080/ws", "ws://10.0.0.2:8080/ws"}
	if !reflect.DeepEqual(capturedTargets, wantTargets) {
		t.Fatalf("compat targets = %#v, want %#v", capturedTargets, wantTargets)
	}
	if capturedCfg.GatewayWSURL != "ws://127.0.0.1:8080/ws" {
		t.Fatalf("captured gateway ws = %q", capturedCfg.GatewayWSURL)
	}
}

func TestParseCompatTargets(t *testing.T) {
	cases := []struct {
		name     string
		raw      string
		fallback string
		want     []string
	}{
		{
			name:     "raw targets",
			raw:      "ws://a,ws://b",
			fallback: "ws://fallback",
			want:     []string{"ws://a", "ws://b"},
		},
		{
			name:     "fallback used",
			raw:      "",
			fallback: "ws://fallback",
			want:     []string{"ws://fallback"},
		},
		{
			name:     "empty all",
			raw:      "",
			fallback: "",
			want:     nil,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseCompatTargets(tc.raw, tc.fallback)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("parseCompatTargets() = %#v, want %#v", got, tc.want)
			}
		})
	}
}
