package services

import (
	"context"
	"strings"
	"testing"

	servertools "go-llm-demo/internal/server/infra/tools"
)

func TestResolveWorkspaceRootUsesEnvOverride(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(servertools.WorkspaceEnvVar, dir)

	got, err := ResolveWorkspaceRoot("")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if got != dir {
		t.Fatalf("expected env workspace root %q, got %q", dir, got)
	}
}

func TestSetAndGetWorkspaceRoot(t *testing.T) {
	origRoot := GetWorkspaceRoot()
	t.Cleanup(func() {
		_ = SetWorkspaceRoot(origRoot)
	})

	dir := t.TempDir()
	if err := SetWorkspaceRoot(dir); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if got := GetWorkspaceRoot(); got != dir {
		t.Fatalf("expected workspace root %q, got %q", dir, got)
	}
}

func TestNormalizeToolParamsRecursivelyConvertsSnakeCase(t *testing.T) {
	got := NormalizeToolParams(map[string]interface{}{
		"file_path": "README.md",
		"nested_map": map[string]interface{}{
			"line_number": 12,
		},
	})

	if got["filePath"] != "README.md" {
		t.Fatalf("expected filePath key, got %+v", got)
	}
	nested, ok := got["nestedMap"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected nestedMap, got %+v", got["nestedMap"])
	}
	if nested["lineNumber"] != 12 {
		t.Fatalf("expected nested camelCase key, got %+v", nested)
	}
}

func TestExecuteToolCallReturnsUnknownToolError(t *testing.T) {
	result := ExecuteToolCall(ToolCall{Tool: "unknown-tool", Params: map[string]interface{}{}})
	if result == nil {
		t.Fatal("expected tool result")
	}
	if result.Success {
		t.Fatalf("expected failure for unknown tool, got %+v", result)
	}
	if result.ToolName != "unknown-tool" {
		t.Fatalf("expected tool name to round-trip, got %q", result.ToolName)
	}
}

func TestValidateChatAPIKeyRejectsNilConfig(t *testing.T) {
	err := ValidateChatAPIKey(context.Background(), nil)
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "config") {
		t.Fatalf("expected nil-config error, got %v", err)
	}
}

func TestNormalizeProviderNameSupportedProvidersAndDefaultModel(t *testing.T) {
	name, ok := NormalizeProviderName("openai")
	if !ok || name != "openai" {
		t.Fatalf("expected normalized openai provider, got %q ok=%v", name, ok)
	}
	if _, ok := NormalizeProviderName("unknown-provider"); ok {
		t.Fatal("expected unknown provider to be rejected")
	}

	providers := SupportedProviders()
	if len(providers) == 0 {
		t.Fatal("expected supported providers")
	}
	foundOpenAI := false
	for _, provider := range providers {
		if provider == "openai" {
			foundOpenAI = true
			break
		}
	}
	if !foundOpenAI {
		t.Fatalf("expected openai in supported providers, got %+v", providers)
	}

	if model := DefaultModelForProvider("openai"); strings.TrimSpace(model) == "" {
		t.Fatal("expected default model for openai")
	}
}
