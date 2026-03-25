package service

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"go-llm-demo/internal/server/domain"
)

// fileRefPattern 只做轻量文件线索提取：允许相对路径、绝对路径前缀和常见文件名。
// 这里宁可多抓一些候选，也不在工作记忆阶段做过重的路径校验。
var fileRefPattern = regexp.MustCompile(`(?i)(?:[a-z]:\\|\./|\.\\|/)?[a-z0-9_./\\-]+\.[a-z0-9]+`)

type workingMemoryServiceImpl struct {
	repo             domain.WorkingMemoryRepository
	maxRecentTurns   int
	maxOpenQuestions int
	maxRecentFiles   int
	workspaceRoot    string
}

// NewWorkingMemoryService 创建第一阶段的工作记忆服务。
// 目标是把“最近几轮 + 当前任务 + 文件线索”整理成稳定的短期上下文。
func NewWorkingMemoryService(repo domain.WorkingMemoryRepository, maxRecentTurns int, workspaceRoot string) domain.WorkingMemoryService {
	if maxRecentTurns <= 0 {
		maxRecentTurns = 6
	}
	return &workingMemoryServiceImpl{
		repo:             repo,
		maxRecentTurns:   maxRecentTurns,
		maxOpenQuestions: 3,
		maxRecentFiles:   6,
		workspaceRoot:    strings.TrimSpace(workspaceRoot),
	}
}

// BuildContext 刷新工作记忆并格式化为提示上下文。
func (s *workingMemoryServiceImpl) BuildContext(ctx context.Context, messages []domain.Message) (string, error) {
	if err := s.Refresh(ctx, messages); err != nil {
		return "", err
	}
	state, err := s.Get(ctx)
	if err != nil {
		return "", err
	}
	return formatWorkingMemoryContext(state, s.workspaceRoot), nil
}

// Refresh 根据当前消息重建工作记忆快照。
func (s *workingMemoryServiceImpl) Refresh(ctx context.Context, messages []domain.Message) error {
	state := s.buildState(messages)
	return s.repo.Save(ctx, state)
}

// Clear 清空当前工作记忆快照。
func (s *workingMemoryServiceImpl) Clear(ctx context.Context) error {
	return s.repo.Clear(ctx)
}

// Get 返回当前工作记忆快照。
func (s *workingMemoryServiceImpl) Get(ctx context.Context) (*domain.WorkingMemoryState, error) {
	return s.repo.Get(ctx)
}

func (s *workingMemoryServiceImpl) buildState(messages []domain.Message) *domain.WorkingMemoryState {
	turns := collectRecentTurns(messages)
	if len(turns) > s.maxRecentTurns {
		turns = turns[len(turns)-s.maxRecentTurns:]
	}

	currentTask := latestUserMessage(messages)
	openQuestions := collectOpenQuestions(messages, s.maxOpenQuestions)
	state := &domain.WorkingMemoryState{
		CurrentTask:         currentTask,
		TaskSummary:         buildTaskSummary(turns, currentTask),
		LastCompletedAction: inferLastCompletedAction(messages),
		CurrentInProgress:   inferCurrentInProgress(messages, currentTask),
		NextStep:            inferNextStep(messages, openQuestions, currentTask),
		RecentTurns:         turns,
		OpenQuestions:       openQuestions,
		RecentFiles:         collectRecentFiles(messages, s.maxRecentFiles),
		UpdatedAt:           time.Now().UTC(),
	}
	return state
}

func collectRecentTurns(messages []domain.Message) []domain.WorkingMemoryTurn {
	turns := make([]domain.WorkingMemoryTurn, 0)
	var pendingUser string
	for _, msg := range messages {
		switch msg.Role {
		case "user":
			pendingUser = strings.TrimSpace(msg.Content)
		case "assistant":
			// 工作记忆按“一条 user + 下一条 assistant”配对，
			// 这样即使中间混有 system/tool 消息，也不会污染最近轮次摘要。
			assistant := strings.TrimSpace(msg.Content)
			if pendingUser == "" && assistant == "" {
				continue
			}
			turns = append(turns, domain.WorkingMemoryTurn{
				User:      pendingUser,
				Assistant: assistant,
			})
			pendingUser = ""
		}
	}
	if pendingUser != "" {
		turns = append(turns, domain.WorkingMemoryTurn{User: pendingUser})
	}
	return turns
}

func latestUserMessage(messages []domain.Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" {
			return strings.TrimSpace(messages[i].Content)
		}
	}
	return ""
}

func buildTaskSummary(turns []domain.WorkingMemoryTurn, currentTask string) string {
	if strings.TrimSpace(currentTask) != "" {
		return domain.SummarizeText(currentTask, 160)
	}
	if len(turns) == 0 {
		return ""
	}
	latest := turns[len(turns)-1]
	if latest.User != "" {
		return domain.SummarizeText(latest.User, 160)
	}
	if latest.Assistant != "" {
		return domain.SummarizeText(latest.Assistant, 160)
	}
	return ""
}

func inferLastCompletedAction(messages []domain.Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		msg := messages[i]
		if msg.Role != "assistant" {
			continue
		}
		for _, line := range strings.Split(msg.Content, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			if containsAnyFold(line, "已完成", "已修复", "已经", "完成了", "已处理", "修复了", "implemented", "fixed", "updated", "added", "created") {
				return domain.SummarizeText(line, 140)
			}
		}
	}
	return ""
}

func inferCurrentInProgress(messages []domain.Message, currentTask string) string {
	trimmedTask := strings.TrimSpace(currentTask)
	if trimmedTask == "" {
		return ""
	}
	for i := len(messages) - 1; i >= 0; i-- {
		msg := messages[i]
		if msg.Role != "assistant" {
			continue
		}
		content := strings.TrimSpace(msg.Content)
		if content == "" {
			continue
		}
		if containsAnyFold(content, "正在", "继续", "接下来", "当前", "处理中", "working on", "next", "continue") {
			return domain.SummarizeText(content, 140)
		}
		break
	}
	return domain.SummarizeText(trimmedTask, 140)
}

func inferNextStep(messages []domain.Message, openQuestions []string, currentTask string) string {
	for i := len(messages) - 1; i >= 0; i-- {
		msg := messages[i]
		if msg.Role != "assistant" {
			continue
		}
		for _, line := range strings.Split(msg.Content, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			if containsAnyFold(line, "下一步", "接下来", "建议", "可以继续", "后续", "next step", "next", "follow-up") {
				return domain.SummarizeText(line, 140)
			}
		}
	}
	if len(openQuestions) > 0 {
		return "先解决: " + domain.SummarizeText(openQuestions[0], 110)
	}
	if strings.TrimSpace(currentTask) != "" {
		return "继续推进: " + domain.SummarizeText(currentTask, 110)
	}
	return ""
}

func collectOpenQuestions(messages []domain.Message, limit int) []string {
	questions := make([]string, 0, limit)
	seen := map[string]struct{}{}
	for i := len(messages) - 1; i >= 0 && len(questions) < limit; i-- {
		msg := messages[i]
		if msg.Role != "user" {
			continue
		}
		content := strings.TrimSpace(msg.Content)
		if content == "" || !looksLikeOpenQuestion(content) {
			continue
		}
		key := strings.ToLower(content)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		questions = append(questions, domain.SummarizeText(content, 120))
	}
	return reverseStrings(questions)
}

func collectRecentFiles(messages []domain.Message, limit int) []string {
	files := make([]string, 0, limit)
	seen := map[string]struct{}{}
	for i := len(messages) - 1; i >= 0 && len(files) < limit; i-- {
		matches := fileRefPattern.FindAllString(messages[i].Content, -1)
		for _, match := range matches {
			// 统一成斜杠路径，便于后续去重和直接注入 prompt。
			normalized := strings.TrimSpace(strings.ReplaceAll(match, "\\", "/"))
			if normalized == "" {
				continue
			}
			lowered := strings.ToLower(normalized)
			if _, ok := seen[lowered]; ok {
				continue
			}
			seen[lowered] = struct{}{}
			files = append(files, normalized)
			if len(files) >= limit {
				break
			}
		}
	}
	return reverseStrings(files)
}

func looksLikeOpenQuestion(text string) bool {
	trimmed := strings.TrimSpace(strings.ToLower(text))
	if trimmed == "" {
		return false
	}
	if strings.ContainsAny(trimmed, "?？") {
		return true
	}
	return containsAnyFold(trimmed, "怎么", "如何", "为什么", "是否", "在哪", "what", "how", "why", "where", "which")
}

func formatWorkingMemoryContext(state *domain.WorkingMemoryState, workspaceRoot string) string {
	if state == nil {
		return ""
	}
	parts := make([]string, 0, 6)
	if strings.TrimSpace(workspaceRoot) != "" {
		parts = append(parts, "Workspace root: "+workspaceRoot)
	}
	if state.CurrentTask != "" {
		parts = append(parts, "Current task: "+domain.SummarizeText(state.CurrentTask, 180))
	}
	if state.TaskSummary != "" {
		parts = append(parts, "Task summary: "+state.TaskSummary)
	}
	if state.LastCompletedAction != "" {
		parts = append(parts, "Last completed action: "+state.LastCompletedAction)
	}
	if state.CurrentInProgress != "" {
		parts = append(parts, "Current in progress: "+state.CurrentInProgress)
	}
	if state.NextStep != "" {
		parts = append(parts, "Next step: "+state.NextStep)
	}
	if len(state.OpenQuestions) > 0 {
		parts = append(parts, "Open questions: "+strings.Join(state.OpenQuestions, " | "))
	}
	if len(state.RecentFiles) > 0 {
		parts = append(parts, "Recent file refs: "+strings.Join(state.RecentFiles, ", "))
	}
	if !state.UpdatedAt.IsZero() {
		parts = append(parts, "State updated at: "+state.UpdatedAt.Format(time.RFC3339))
	}
	if len(state.RecentTurns) > 0 {
		turnLines := make([]string, 0, len(state.RecentTurns))
		for idx, turn := range state.RecentTurns {
			user := domain.SummarizeText(turn.User, 100)
			assistant := domain.SummarizeText(turn.Assistant, 100)
			turnLines = append(turnLines, fmt.Sprintf("Turn %d user=%q assistant=%q", idx+1, user, assistant))
		}
		parts = append(parts, "Recent turns:\n"+strings.Join(turnLines, "\n"))
	}
	if len(parts) == 0 {
		return ""
	}
	return "Use the following working memory to stay consistent with the active task and recent context. Prefer it over reconstructing context from scratch.\n" + strings.Join(parts, "\n")
}

func reverseStrings(values []string) []string {
	for i, j := 0, len(values)-1; i < j; i, j = i+1, j-1 {
		values[i], values[j] = values[j], values[i]
	}
	return values
}
