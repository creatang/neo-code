package service

import (
	"context"
	"strings"
	"testing"

	"go-llm-demo/internal/server/domain"
	"go-llm-demo/internal/server/infra/repository"
)

func TestWorkingMemoryServiceBuildsCheckpointFields(t *testing.T) {
	svc := NewWorkingMemoryService(repository.NewWorkingMemoryStore(), 6, "D:/neo-code")
	messages := []domain.Message{
		{Role: "user", Content: "请修复 internal/server/service/memory_service.go 的记忆问题"},
		{Role: "assistant", Content: "已完成工具 JSON 过滤，接下来补 working memory 测试。"},
		{Role: "user", Content: "下一步应该先验证哪些场景？"},
	}

	if err := svc.Refresh(context.Background(), messages); err != nil {
		t.Fatalf("refresh state: %v", err)
	}
	state, err := svc.Get(context.Background())
	if err != nil {
		t.Fatalf("get state: %v", err)
	}
	if state.CurrentTask == "" || state.NextStep == "" || state.LastCompletedAction == "" {
		t.Fatalf("expected checkpoint fields to be populated, got %+v", state)
	}
	if len(state.RecentFiles) == 0 || state.RecentFiles[0] != "internal/server/service/memory_service.go" {
		t.Fatalf("expected recent files to be collected, got %+v", state.RecentFiles)
	}
}

func TestWorkingMemoryServiceFormatsExtendedContext(t *testing.T) {
	state := &domain.WorkingMemoryState{
		CurrentTask:         "修复记忆模块",
		LastCompletedAction: "已修复持久化问题",
		CurrentInProgress:   "正在补恢复测试",
		NextStep:            "继续验证跨 workspace 隔离",
	}

	got := formatWorkingMemoryContext(state, "D:/neo-code")
	for _, want := range []string{"Current task:", "Last completed action:", "Current in progress:", "Next step:"} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected %q in formatted context, got %q", want, got)
		}
	}
}
