package bootstrap

import (
	"go-llm-demo/internal/tui/core"
	"go-llm-demo/internal/tui/services"

	tea "github.com/charmbracelet/bubbletea"
)

func NewProgram(persona string, historyTurns int, configPath, workspaceRoot string) (*tea.Program, error) {
	client, err := services.NewLocalChatClient()
	if err != nil {
		return nil, err
	}

	model := core.NewModel(client, persona, historyTurns, configPath, workspaceRoot)
	return tea.NewProgram(model,
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	), nil
}
