package bootstrap

import (
	"bufio"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"go-llm-demo/configs"
	"go-llm-demo/internal/tui/services"
)

type errReader struct{}

func (errReader) Read([]byte) (int, error) {
	return 0, io.ErrUnexpectedEOF
}

func restoreSetupGlobals(t *testing.T) {
	t.Helper()

	origResolveWorkspaceRoot := resolveWorkspaceRoot
	origSetWorkspaceRoot := setWorkspaceRoot
	origEnsureConfigFile := ensureConfigFile
	origValidateChatAPIKey := validateChatAPIKey
	origWriteAppConfig := writeAppConfig
	origGlobalConfig := configs.GlobalAppConfig

	t.Cleanup(func() {
		resolveWorkspaceRoot = origResolveWorkspaceRoot
		setWorkspaceRoot = origSetWorkspaceRoot
		ensureConfigFile = origEnsureConfigFile
		validateChatAPIKey = origValidateChatAPIKey
		writeAppConfig = origWriteAppConfig
		configs.GlobalAppConfig = origGlobalConfig
	})
}

func TestApplyAPIKeyEnvNameUpdatesConfig(t *testing.T) {
	cfg := configs.DefaultAppConfig()
	applyAPIKeyEnvName(cfg, "  TEST_KEY_ENV  ")

	if got := cfg.AI.APIKey; got != "TEST_KEY_ENV" {
		t.Fatalf("expected API key env name to be trimmed, got %q", got)
	}
}

func TestReadInteractiveLineRejectsEmptyInputThenReadsValue(t *testing.T) {
	scanner := bufio.NewScanner(strings.NewReader("\n  /retry  \n"))

	got, ok, err := readInteractiveLine(scanner, "> ")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !ok {
		t.Fatal("expected ok=true")
	}
	if got != "/retry" {
		t.Fatalf("expected trimmed input, got %q", got)
	}
}

func TestReadInteractiveLineTreatsExitAsStop(t *testing.T) {
	scanner := bufio.NewScanner(strings.NewReader("/exit\n"))

	got, ok, err := readInteractiveLine(scanner, "> ")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if ok {
		t.Fatal("expected ok=false for /exit")
	}
	if got != "" {
		t.Fatalf("expected empty value, got %q", got)
	}
}

func TestHandleSetupDecisionHandlesProviderSwitch(t *testing.T) {
	cfg := configs.DefaultAppConfig()
	scanner := bufio.NewScanner(strings.NewReader("/provider openai\n"))

	decision, err := handleSetupDecision(scanner, cfg, false, "config.yaml")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if decision != setupRetry {
		t.Fatalf("expected setupRetry, got %v", decision)
	}
	if cfg.AI.Provider != "openai" {
		t.Fatalf("expected provider to switch, got %q", cfg.AI.Provider)
	}
	if cfg.AI.Model == "" {
		t.Fatal("expected provider switch to set a default model")
	}
}

func TestHandleSetupDecisionRejectsContinueWhenNotAllowed(t *testing.T) {
	cfg := configs.DefaultAppConfig()
	scanner := bufio.NewScanner(strings.NewReader("/continue\n/retry\n"))

	decision, err := handleSetupDecision(scanner, cfg, false, "config.yaml")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if decision != setupRetry {
		t.Fatalf("expected setupRetry after rejecting continue, got %v", decision)
	}
}

func TestPrepareWorkspaceResolvesAndSetsWorkspaceRoot(t *testing.T) {
	restoreSetupGlobals(t)

	var setRoot string
	resolveWorkspaceRoot = func(workspaceFlag string) (string, error) {
		if workspaceFlag != "./workspace" {
			t.Fatalf("expected workspace flag to flow through, got %q", workspaceFlag)
		}
		return "D:/neo-code/workspace", nil
	}
	setWorkspaceRoot = func(root string) error {
		setRoot = root
		return nil
	}

	root, err := PrepareWorkspace("./workspace")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if root != "D:/neo-code/workspace" {
		t.Fatalf("unexpected workspace root %q", root)
	}
	if setRoot != root {
		t.Fatalf("expected SetWorkspaceRoot to receive %q, got %q", root, setRoot)
	}
}

func TestPrepareWorkspaceReturnsSetWorkspaceRootError(t *testing.T) {
	restoreSetupGlobals(t)

	resolveWorkspaceRoot = func(string) (string, error) { return "D:/neo-code/workspace", nil }
	setWorkspaceRoot = func(string) error { return errors.New("set failed") }

	_, err := PrepareWorkspace("./workspace")
	if err == nil || !strings.Contains(err.Error(), "set failed") {
		t.Fatalf("expected SetWorkspaceRoot error, got %v", err)
	}
}

func TestPrepareWorkspaceReturnsResolveError(t *testing.T) {
	restoreSetupGlobals(t)

	resolveWorkspaceRoot = func(string) (string, error) { return "", errors.New("resolve failed") }

	_, err := PrepareWorkspace("./workspace")
	if err == nil || !strings.Contains(err.Error(), "resolve failed") {
		t.Fatalf("expected resolve error, got %v", err)
	}
}

func TestReadInteractiveLineReturnsEOF(t *testing.T) {
	scanner := bufio.NewScanner(strings.NewReader(""))

	got, ok, err := readInteractiveLine(scanner, "> ")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if ok || got != "" {
		t.Fatalf("expected EOF stop, got value=%q ok=%v", got, ok)
	}
}

func TestReadInteractiveLineReturnsScannerError(t *testing.T) {
	scanner := bufio.NewScanner(errReader{})

	_, _, err := readInteractiveLine(scanner, "> ")
	if err == nil {
		t.Fatal("expected scanner error")
	}
}

func TestHandleSetupDecisionAPIKeyRequiresArgument(t *testing.T) {
	cfg := configs.DefaultAppConfig()
	scanner := bufio.NewScanner(strings.NewReader("/apikey\n/apikey TEST_ENV\n"))

	decision, err := handleSetupDecision(scanner, cfg, false, "config.yaml")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if decision != setupRetry {
		t.Fatalf("expected setupRetry, got %v", decision)
	}
	if cfg.AI.APIKey != "TEST_ENV" {
		t.Fatalf("expected API key env to switch, got %q", cfg.AI.APIKey)
	}
}

func TestHandleSetupDecisionAllowsContinue(t *testing.T) {
	restoreSetupGlobals(t)

	cfg := configs.DefaultAppConfig()
	writeCalled := false
	writeAppConfig = func(string, *configs.AppConfiguration) error {
		writeCalled = true
		return nil
	}

	decision, err := handleSetupDecision(bufio.NewScanner(strings.NewReader("/continue\n")), cfg, true, "config.yaml")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if decision != setupContinue {
		t.Fatalf("expected setupContinue, got %v", decision)
	}
	if !writeCalled {
		t.Fatal("expected config write on continue")
	}
}

func TestHandleSetupDecisionContinueWriteFailure(t *testing.T) {
	restoreSetupGlobals(t)

	cfg := configs.DefaultAppConfig()
	writeAppConfig = func(string, *configs.AppConfiguration) error { return errors.New("write failed") }

	decision, err := handleSetupDecision(bufio.NewScanner(strings.NewReader("/continue\n")), cfg, true, "config.yaml")
	if err == nil || !strings.Contains(err.Error(), "write failed") {
		t.Fatalf("expected write failure, got decision=%v err=%v", decision, err)
	}
}

func TestHandleSetupDecisionProviderRequiresArgumentAndRejectsUnknownProvider(t *testing.T) {
	cfg := configs.DefaultAppConfig()
	scanner := bufio.NewScanner(strings.NewReader("/provider\n/provider invalid\n/provider openai\n"))

	decision, err := handleSetupDecision(scanner, cfg, false, "config.yaml")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if decision != setupRetry {
		t.Fatalf("expected setupRetry, got %v", decision)
	}
	if cfg.AI.Provider != "openai" {
		t.Fatalf("expected provider to switch after retries, got %q", cfg.AI.Provider)
	}
}

func TestHandleSetupDecisionSwitchRequiresArgumentThenSucceeds(t *testing.T) {
	cfg := configs.DefaultAppConfig()
	scanner := bufio.NewScanner(strings.NewReader("/switch\n/switch gpt-5.4-mini\n"))

	decision, err := handleSetupDecision(scanner, cfg, false, "config.yaml")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if decision != setupRetry {
		t.Fatalf("expected setupRetry, got %v", decision)
	}
	if cfg.AI.Model != "gpt-5.4-mini" {
		t.Fatalf("expected model switch, got %q", cfg.AI.Model)
	}
}

func TestHandleSetupDecisionUnknownCommandThenExit(t *testing.T) {
	cfg := configs.DefaultAppConfig()
	scanner := bufio.NewScanner(strings.NewReader("/unknown\n/exit\n"))

	decision, err := handleSetupDecision(scanner, cfg, false, "config.yaml")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if decision != setupExit {
		t.Fatalf("expected setupExit, got %v", decision)
	}
}

func TestEnsureAPIKeyInteractiveReturnsConfigError(t *testing.T) {
	restoreSetupGlobals(t)

	ensureConfigFile = func(string) (*configs.AppConfiguration, bool, error) {
		return nil, false, errors.New("config failed")
	}

	ready, err := EnsureAPIKeyInteractive(context.Background(), bufio.NewScanner(strings.NewReader("")), "config.yaml")
	if err == nil || !strings.Contains(err.Error(), "config failed") {
		t.Fatalf("expected config error, got ready=%v err=%v", ready, err)
	}
}

func TestEnsureAPIKeyInteractiveExitsWhenAPIKeyMissing(t *testing.T) {
	restoreSetupGlobals(t)

	cfg := configs.DefaultAppConfig()
	cfg.AI.APIKey = "MISSING_ENV"
	ensureConfigFile = func(string) (*configs.AppConfiguration, bool, error) {
		return cfg, false, nil
	}
	validateCalled := false
	validateChatAPIKey = func(context.Context, *configs.AppConfiguration) error {
		validateCalled = true
		return nil
	}

	ready, err := EnsureAPIKeyInteractive(context.Background(), bufio.NewScanner(strings.NewReader("/exit\n")), "config.yaml")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if ready {
		t.Fatal("expected setup to stop without becoming ready")
	}
	if validateCalled {
		t.Fatal("validation should not run when runtime API key is missing")
	}
}

func TestEnsureAPIKeyInteractiveWritesConfigAfterSuccessfulValidation(t *testing.T) {
	restoreSetupGlobals(t)

	cfg := configs.DefaultAppConfig()
	cfg.AI.APIKey = "READY_ENV"
	t.Setenv("READY_ENV", "secret")

	ensureConfigFile = func(string) (*configs.AppConfiguration, bool, error) {
		return cfg, false, nil
	}
	validateChatAPIKey = func(context.Context, *configs.AppConfiguration) error { return nil }
	var writePath string
	writeAppConfig = func(path string, gotCfg *configs.AppConfiguration) error {
		writePath = path
		if gotCfg != cfg {
			t.Fatal("expected the same config instance to be written")
		}
		return nil
	}

	ready, err := EnsureAPIKeyInteractive(context.Background(), bufio.NewScanner(strings.NewReader("")), "config.yaml")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !ready {
		t.Fatal("expected setup to become ready")
	}
	if writePath != "config.yaml" {
		t.Fatalf("expected config write path config.yaml, got %q", writePath)
	}
	if configs.GlobalAppConfig != cfg {
		t.Fatal("expected global config to be updated after successful validation")
	}
}

func TestEnsureAPIKeyInteractiveAllowsContinueOnSoftValidationError(t *testing.T) {
	restoreSetupGlobals(t)

	cfg := configs.DefaultAppConfig()
	cfg.AI.APIKey = "READY_ENV"
	t.Setenv("READY_ENV", "secret")

	ensureConfigFile = func(string) (*configs.AppConfiguration, bool, error) {
		return cfg, false, nil
	}
	validateChatAPIKey = func(context.Context, *configs.AppConfiguration) error {
		return services.ErrAPIKeyValidationSoft
	}
	writeCount := 0
	writeAppConfig = func(string, *configs.AppConfiguration) error {
		writeCount++
		return nil
	}

	ready, err := EnsureAPIKeyInteractive(context.Background(), bufio.NewScanner(strings.NewReader("/continue\n")), "config.yaml")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !ready {
		t.Fatal("expected continue to allow startup")
	}
	if writeCount != 1 {
		t.Fatalf("expected config to be written once, got %d", writeCount)
	}
	if configs.GlobalAppConfig != cfg {
		t.Fatal("expected global config to be updated on continue")
	}
}

func TestEnsureAPIKeyInteractiveReportsCreatedConfigThenSucceeds(t *testing.T) {
	restoreSetupGlobals(t)

	cfg := configs.DefaultAppConfig()
	cfg.AI.APIKey = "READY_ENV"
	t.Setenv("READY_ENV", "secret")

	ensureConfigFile = func(string) (*configs.AppConfiguration, bool, error) {
		return cfg, true, nil
	}
	validateChatAPIKey = func(context.Context, *configs.AppConfiguration) error { return nil }
	writeAppConfig = func(string, *configs.AppConfiguration) error { return nil }

	ready, err := EnsureAPIKeyInteractive(context.Background(), bufio.NewScanner(strings.NewReader("")), "config.yaml")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !ready {
		t.Fatal("expected setup to become ready")
	}
}

func TestEnsureAPIKeyInteractiveRetriesAfterChangingAPIKeyEnv(t *testing.T) {
	restoreSetupGlobals(t)

	cfg := configs.DefaultAppConfig()
	cfg.AI.APIKey = "MISSING_ENV"
	t.Setenv("RECOVERED_ENV", "secret")

	ensureConfigFile = func(string) (*configs.AppConfiguration, bool, error) {
		return cfg, false, nil
	}
	validateCount := 0
	validateChatAPIKey = func(context.Context, *configs.AppConfiguration) error {
		validateCount++
		return nil
	}
	writeAppConfig = func(string, *configs.AppConfiguration) error { return nil }

	ready, err := EnsureAPIKeyInteractive(context.Background(), bufio.NewScanner(strings.NewReader("/apikey RECOVERED_ENV\n/retry\n")), "config.yaml")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !ready {
		t.Fatal("expected setup to become ready after retry")
	}
	if cfg.AI.APIKey != "RECOVERED_ENV" {
		t.Fatalf("expected env name update, got %q", cfg.AI.APIKey)
	}
	if validateCount != 1 {
		t.Fatalf("expected one validation call after retry, got %d", validateCount)
	}
}

func TestEnsureAPIKeyInteractiveHandlesInvalidAPIKeyAndExit(t *testing.T) {
	restoreSetupGlobals(t)

	cfg := configs.DefaultAppConfig()
	cfg.AI.APIKey = "READY_ENV"
	t.Setenv("READY_ENV", "secret")

	ensureConfigFile = func(string) (*configs.AppConfiguration, bool, error) {
		return cfg, false, nil
	}
	validateChatAPIKey = func(context.Context, *configs.AppConfiguration) error {
		return services.ErrInvalidAPIKey
	}

	ready, err := EnsureAPIKeyInteractive(context.Background(), bufio.NewScanner(strings.NewReader("/exit\n")), "config.yaml")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if ready {
		t.Fatal("expected setup to stop on invalid key + exit")
	}
}

func TestEnsureAPIKeyInteractiveHandlesGenericValidationErrorAndExit(t *testing.T) {
	restoreSetupGlobals(t)

	cfg := configs.DefaultAppConfig()
	cfg.AI.APIKey = "READY_ENV"
	t.Setenv("READY_ENV", "secret")

	ensureConfigFile = func(string) (*configs.AppConfiguration, bool, error) {
		return cfg, false, nil
	}
	validateChatAPIKey = func(context.Context, *configs.AppConfiguration) error {
		return errors.New("validation failed")
	}

	ready, err := EnsureAPIKeyInteractive(context.Background(), bufio.NewScanner(strings.NewReader("/exit\n")), "config.yaml")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if ready {
		t.Fatal("expected setup to stop on generic validation failure")
	}
}

func TestEnsureAPIKeyInteractiveReturnsWriteErrorAfterValidationSuccess(t *testing.T) {
	restoreSetupGlobals(t)

	cfg := configs.DefaultAppConfig()
	cfg.AI.APIKey = "READY_ENV"
	t.Setenv("READY_ENV", "secret")

	ensureConfigFile = func(string) (*configs.AppConfiguration, bool, error) {
		return cfg, false, nil
	}
	validateChatAPIKey = func(context.Context, *configs.AppConfiguration) error { return nil }
	writeAppConfig = func(string, *configs.AppConfiguration) error { return errors.New("write failed") }

	ready, err := EnsureAPIKeyInteractive(context.Background(), bufio.NewScanner(strings.NewReader("")), "config.yaml")
	if err == nil || !strings.Contains(err.Error(), "write failed") {
		t.Fatalf("expected write error, got ready=%v err=%v", ready, err)
	}
}
