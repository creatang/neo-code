package components

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var codeBlockStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color("#ABB2BF")).
	Background(lipgloss.Color("#282C34")).
	Padding(0, 1)

func RenderContent(content string, width int) string {
	if content == "" {
		return "..."
	}
	if width <= 0 {
		width = 80
	}

	lines := strings.Split(content, "\n")
	var b strings.Builder

	inCodeBlock := false
	codeLang := ""
	var codeLines []string

	for _, line := range lines {
		trimmedLine := strings.TrimSpace(line)
		if isFenceLine(trimmedLine) {
			if !inCodeBlock {
				inCodeBlock = true
				codeLang = parseFenceLanguage(trimmedLine)
				codeLines = []string{}
				b.WriteString("\n")
			} else {
				// 围栏闭合时才统一高亮，避免逐行渲染打断终端样式。
				inCodeBlock = false
				highlighted := HighlightCodeBlock(codeLines, codeLang, width, true)
				b.WriteString(highlighted)
				codeLines = nil
			}
			continue
		}

		if inCodeBlock {
			codeLines = append(codeLines, line)
		} else {
			b.WriteString(lipgloss.NewStyle().MaxWidth(width).Render(line))
			b.WriteString("\n")
		}
	}

	if inCodeBlock {
		// 模型流式输出时经常会出现“代码块还没闭合”的中间态，这里也尽量渲染已有内容。
		highlighted := HighlightCodeBlock(codeLines, codeLang, width, false)
		b.WriteString(highlighted)
	}

	return b.String()
}

func HighlightCodeBlock(lines []string, lang string, width int, closed bool) string {
	var b strings.Builder
	code := strings.Join(lines, "\n")
	resolvedLang := strings.TrimSpace(lang)
	if resolvedLang == "" {
		resolvedLang = DetectLanguage(code)
	}

	highlighted := HighlightCode(code, resolvedLang)
	b.WriteString("```")
	b.WriteString(resolvedLang)
	b.WriteString("\n")
	b.WriteString(highlighted)
	if !strings.HasSuffix(highlighted, "\n") {
		b.WriteString("\n")
	}
	if closed {
		b.WriteString("```\n")
	}

	blockStyle := codeBlockStyle.MaxWidth(width)
	return blockStyle.Render(b.String()) + "\n"
}

func isFenceLine(line string) bool {
	return strings.HasPrefix(line, "```")
}

func parseFenceLanguage(line string) string {
	return strings.TrimSpace(strings.TrimPrefix(line, "```"))
}
