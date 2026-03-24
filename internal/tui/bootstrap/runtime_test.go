package bootstrap

import (
	"path/filepath"
	"testing"

	"go-llm-demo/configs"
)

func TestNewProgramReturnsErrorWhenGlobalConfigMissing(t *testing.T) {
	origGlobalConfig := configs.GlobalAppConfig
	t.Cleanup(func() { configs.GlobalAppConfig = origGlobalConfig })

	configs.GlobalAppConfig = nil

	p, err := NewProgram("persona", 4, "config.yaml", "D:/neo-code")
	if err == nil {
		t.Fatalf("expected error, got program %+v", p)
	}
}

func TestNewProgramBuildsBubbleTeaProgram(t *testing.T) {
	origGlobalConfig := configs.GlobalAppConfig
	t.Cleanup(func() { configs.GlobalAppConfig = origGlobalConfig })

	cfg := configs.DefaultAppConfig()
	cfg.Memory.StoragePath = filepath.Join(t.TempDir(), "memory.json")
	configs.GlobalAppConfig = cfg
	t.Setenv(cfg.APIKeyEnvVarName(), "secret")

	p, err := NewProgram("persona", 4, "config.yaml", "D:/neo-code")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if p == nil {
		t.Fatal("expected non-nil program")
	}
}
