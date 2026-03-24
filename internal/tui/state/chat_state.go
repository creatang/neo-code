package state

import (
	"time"

	"go-llm-demo/internal/tui/services"
)

type Message struct {
	Role      string
	Content   string
	Timestamp time.Time
	Streaming bool
}

type ChatState struct {
	Messages       []Message
	HistoryTurns   int
	Generating     bool
	ActiveModel    string
	MemoryStats    services.MemoryStats
	CommandHistory []string
	CmdHistIndex   int
	WorkspaceRoot  string
	ToolExecuting  bool
	APIKeyReady    bool
	ConfigPath     string
}
