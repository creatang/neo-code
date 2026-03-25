package repository

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"go-llm-demo/internal/server/domain"
)

func TestWorkingMemoryStorePersistsStateToDisk(t *testing.T) {
	path := filepath.Join(t.TempDir(), "workspace", "session_state.json")
	store := NewWorkingMemoryStore(path)

	state := &domain.WorkingMemoryState{
		CurrentTask:         "修复记忆模块",
		LastCompletedAction: "已完成规则修复",
		NextStep:            "补测试",
		RecentFiles:         []string{"internal/server/service/memory_service.go"},
		UpdatedAt:           time.Now().UTC(),
	}
	if err := store.Save(context.Background(), state); err != nil {
		t.Fatalf("save state: %v", err)
	}

	reloaded := NewWorkingMemoryStore(path)
	got, err := reloaded.Get(context.Background())
	if err != nil {
		t.Fatalf("get state: %v", err)
	}
	if got.CurrentTask != state.CurrentTask || got.NextStep != state.NextStep {
		t.Fatalf("expected persisted state, got %+v", got)
	}
}

func TestWorkingMemoryStoreClearRemovesPersistedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "workspace", "session_state.json")
	store := NewWorkingMemoryStore(path)
	if err := store.Save(context.Background(), &domain.WorkingMemoryState{CurrentTask: "task"}); err != nil {
		t.Fatalf("save state: %v", err)
	}
	if err := store.Clear(context.Background()); err != nil {
		t.Fatalf("clear state: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected persisted file to be removed, got %v", err)
	}
}
