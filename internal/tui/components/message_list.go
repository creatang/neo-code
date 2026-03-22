package components

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

type Message struct {
	Role      string
	Content   string
	Timestamp time.Time
	Streaming bool
}

type MessageList struct {
	Messages []Message
	Width    int
}

func (ml MessageList) Render() string {
	if len(ml.Messages) == 0 {
		return ""
	}
	contentWidth := ml.Width - 4
	if contentWidth < 20 {
		contentWidth = ml.Width
	}

	userMsgStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#98C379")).
		Bold(true)

	assistantMsgStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#E5C07B"))

	systemMsgStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#C678DD"))

	var b strings.Builder

	wrapStyle := lipgloss.NewStyle().MaxWidth(contentWidth)

	for i, msg := range ml.Messages {
		idx := i + 1
		switch msg.Role {
		case "user":
			b.WriteString(userMsgStyle.Render(fmt.Sprintf("你 [%d]:", idx)))
			b.WriteString(" ")
			b.WriteString(wrapStyle.Render(msg.Content))
			b.WriteString("\n\n")

		case "assistant":
			b.WriteString(assistantMsgStyle.Render(fmt.Sprintf("Neo [%d]:", idx)))
			b.WriteString("\n")
			b.WriteString(RenderContent(msg.Content, contentWidth))
			b.WriteString("\n\n")

		case "system":
			b.WriteString(systemMsgStyle.Render("[系统]"))
			b.WriteString(" ")
			b.WriteString(wrapStyle.Render(msg.Content))
			b.WriteString("\n\n")
		}
	}

	return b.String()
}
