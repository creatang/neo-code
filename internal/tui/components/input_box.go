package components

import "github.com/charmbracelet/lipgloss"

type InputBox struct {
	Body       string
	Generating bool
}

func (i InputBox) Render() string {
	helpText := "[Enter换行 F5/F8发送 PgUp/PgDn滚动]"
	if !i.Generating {
		helpText = "[Enter换行 F5/F8发送 Ctrl+V粘贴 PgUp/PgDn滚动]"
	}

	footer := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#5C6370")).
		Render(helpText)

	return i.Body + "\n" + footer
}
