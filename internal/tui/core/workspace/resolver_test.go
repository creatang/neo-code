package workspace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveWorkspacePath(t *testing.T) {
	base := t.TempDir()
	childDir := filepath.Join(base, "project")
	if err := os.MkdirAll(childDir, 0o755); err != nil {
		t.Fatalf("mkdir child dir: %v", err)
	}

	resolved, err := ResolveWorkspacePath(base, "project")
	if err != nil {
		t.Fatalf("ResolveWorkspacePath(relative) error = %v", err)
	}
	if resolved != filepath.Clean(childDir) {
		t.Fatalf("unexpected resolved path: %q", resolved)
	}

	resolved, err = ResolveWorkspacePath(base, "")
	if err != nil {
		t.Fatalf("ResolveWorkspacePath(default current) error = %v", err)
	}
	if resolved != filepath.Clean(base) {
		t.Fatalf("expected base directory for empty requested path, got %q", resolved)
	}

	// Empty base falls back to os.Getwd().
	resolved, err = ResolveWorkspacePath("", ".")
	if err != nil {
		t.Fatalf("ResolveWorkspacePath(empty base) error = %v", err)
	}
	cwd, _ := os.Getwd()
	if resolved != filepath.Clean(cwd) {
		t.Fatalf("expected current directory for empty base, got %q", resolved)
	}
}

func TestResolveWorkspacePathErrors(t *testing.T) {
	base := t.TempDir()
	filePath := filepath.Join(base, "not-dir.txt")
	if err := os.WriteFile(filePath, []byte("x"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	if _, err := ResolveWorkspacePath(base, "missing-dir"); err == nil {
		t.Fatalf("expected missing path to return error")
	}

	if _, err := ResolveWorkspacePath(base, "not-dir.txt"); err == nil || !strings.Contains(err.Error(), "not a directory") {
		t.Fatalf("expected non-directory path error, got %v", err)
	}
}

func TestSelectSessionWorkdir(t *testing.T) {
	if got := SelectSessionWorkdir(" /session ", "/default"); got != "/session" {
		t.Fatalf("expected session workdir priority, got %q", got)
	}
	if got := SelectSessionWorkdir(" ", " /default "); got != "/default" {
		t.Fatalf("expected default workdir fallback, got %q", got)
	}
}
