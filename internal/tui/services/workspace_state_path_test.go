package services

import "testing"

func TestBuildWorkspaceStatePathUsesStableHash(t *testing.T) {
	baseDir := "./data/workspaces"
	root := "D:/neo-code"

	first := BuildWorkspaceStatePath(baseDir, root)
	second := BuildWorkspaceStatePath(baseDir, root)
	if first == "" || first != second {
		t.Fatalf("expected stable workspace state path, got %q and %q", first, second)
	}
}
