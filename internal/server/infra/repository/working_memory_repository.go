package repository

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"go-llm-demo/internal/server/domain"
)

// WorkingMemoryStore 在当前进程内保存会话级工作记忆。
// 第一阶段先使用内存实现，后续如需跨进程恢复再替换为持久化版本。
type WorkingMemoryStore struct {
	mu    sync.RWMutex
	state *domain.WorkingMemoryState
	path  string
}

// NewWorkingMemoryStore 创建一个进程内工作记忆存储。
func NewWorkingMemoryStore(path ...string) *WorkingMemoryStore {
	storePath := ""
	if len(path) > 0 {
		storePath = strings.TrimSpace(path[0])
	}
	return &WorkingMemoryStore{path: storePath}
}

// Get 返回当前工作记忆快照的拷贝。
func (s *WorkingMemoryStore) Get(ctx context.Context) (*domain.WorkingMemoryState, error) {
	_ = ctx
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state == nil && strings.TrimSpace(s.path) != "" {
		state, err := s.readLocked()
		if err != nil {
			return nil, err
		}
		s.state = state
	}
	if s.state == nil {
		return &domain.WorkingMemoryState{}, nil
	}
	return cloneWorkingMemoryState(s.state), nil
}

// Save 替换当前保存的工作记忆快照。
func (s *WorkingMemoryStore) Save(ctx context.Context, state *domain.WorkingMemoryState) error {
	_ = ctx
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state = cloneWorkingMemoryState(state)
	if strings.TrimSpace(s.path) == "" {
		return nil
	}
	return s.writeLocked(s.state)
}

// Clear 清空已保存的工作记忆快照。
func (s *WorkingMemoryStore) Clear(ctx context.Context) error {
	_ = ctx
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state = nil
	if strings.TrimSpace(s.path) == "" {
		return nil
	}
	if err := os.Remove(s.path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func (s *WorkingMemoryStore) readLocked() (*domain.WorkingMemoryState, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &domain.WorkingMemoryState{}, nil
		}
		return nil, err
	}
	if len(data) == 0 {
		return &domain.WorkingMemoryState{}, nil
	}

	var state domain.WorkingMemoryState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	return cloneWorkingMemoryState(&state), nil
}

func (s *WorkingMemoryStore) writeLocked(state *domain.WorkingMemoryState) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.path), "working-memory-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, s.path); err == nil {
		return nil
	}
	if err := os.Remove(s.path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return os.Rename(tmpPath, s.path)
}

func cloneWorkingMemoryState(state *domain.WorkingMemoryState) *domain.WorkingMemoryState {
	if state == nil {
		return &domain.WorkingMemoryState{}
	}
	cloned := &domain.WorkingMemoryState{
		CurrentTask:         state.CurrentTask,
		TaskSummary:         state.TaskSummary,
		LastCompletedAction: state.LastCompletedAction,
		CurrentInProgress:   state.CurrentInProgress,
		NextStep:            state.NextStep,
		UpdatedAt:           state.UpdatedAt,
	}
	if len(state.RecentTurns) > 0 {
		cloned.RecentTurns = make([]domain.WorkingMemoryTurn, len(state.RecentTurns))
		copy(cloned.RecentTurns, state.RecentTurns)
	}
	if len(state.OpenQuestions) > 0 {
		cloned.OpenQuestions = make([]string, len(state.OpenQuestions))
		copy(cloned.OpenQuestions, state.OpenQuestions)
	}
	if len(state.RecentFiles) > 0 {
		cloned.RecentFiles = make([]string, len(state.RecentFiles))
		copy(cloned.RecentFiles, state.RecentFiles)
	}
	return cloned
}
