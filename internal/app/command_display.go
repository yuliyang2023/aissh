package app

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"git.example.com/ai/wrd-ai-cli/internal/terminalview"
)

type formattedCommand struct {
	Lines      []string
	TotalLines int
	Truncated  bool
}

func formatCommandForDisplay(command string, width, maximumLines int) formattedCommand {
	command = strings.Trim(command, "\r\n")
	if strings.TrimSpace(command) == "" || width <= 0 {
		return formattedCommand{}
	}
	if strings.Contains(command, "\n") {
		return finishFormattedCommand(formatMultilineCommand(command, width), width, maximumLines)
	}
	tokens := shellDisplayTokens(command)
	firstPrefix := "$ "
	continuationPrefix := "  ↳ "
	longCommand := textWidth(firstPrefix+command) > width || strings.Contains(command, "\n")
	var lines []string
	prefix := firstPrefix
	current := ""

	flush := func() {
		if current == "" {
			return
		}
		lines = append(lines, prefix+current)
		prefix = continuationPrefix
		current = ""
	}
	for _, token := range tokens {
		if token == "\n" {
			flush()
			continue
		}
		if longCommand && isShellChainOperator(token) && current != "" {
			flush()
		}
		separator := ""
		if current != "" {
			separator = " "
		}
		limit := width - textWidth(prefix)
		if limit < 1 {
			limit = 1
		}
		if current != "" && textWidth(current+separator+token) > limit {
			flush()
			separator = ""
			limit = width - textWidth(prefix)
			if limit < 1 {
				limit = 1
			}
		}
		if textWidth(token) <= limit {
			current += separator + token
			continue
		}
		if current != "" {
			flush()
			limit = width - textWidth(prefix)
			if limit < 1 {
				limit = 1
			}
		}
		parts := terminalview.Wrap(token, limit)
		for index, part := range parts {
			if index == len(parts)-1 {
				current = part
				break
			}
			lines = append(lines, prefix+part)
			prefix = continuationPrefix
		}
	}
	flush()
	if len(lines) == 0 {
		lines = []string{firstPrefix + command}
	}

	return finishFormattedCommand(lines, width, maximumLines)
}

func formatMultilineCommand(command string, width int) []string {
	prefix := "$ "
	continuationPrefix := "  ↳ "
	limit := width - textWidth(continuationPrefix)
	if limit < 1 {
		limit = 1
	}
	var lines []string
	for _, source := range strings.Split(command, "\n") {
		source = strings.TrimSuffix(source, "\r")
		wrapped := terminalview.Wrap(source, limit)
		if len(wrapped) == 0 {
			wrapped = []string{""}
		}
		for _, line := range wrapped {
			lines = append(lines, prefix+line)
			prefix = continuationPrefix
		}
	}
	return lines
}

func finishFormattedCommand(lines []string, width, maximumLines int) formattedCommand {
	result := formattedCommand{Lines: lines, TotalLines: len(lines)}
	if maximumLines <= 0 || len(lines) <= maximumLines {
		return result
	}
	result.Truncated = true
	if maximumLines == 1 {
		result.Lines = []string{terminalview.ClipPad("$ … [展开全部] · Ctrl+O", width)}
		return result
	}
	visible := append([]string(nil), lines[:maximumLines-1]...)
	summary := "  … [展开全部] · Ctrl+O · 余" + integerString(len(lines)-maximumLines+1) + "行"
	visible = append(visible, terminalview.ClipPad(summary, width))
	result.Lines = visible
	return result
}

func shellDisplayTokens(command string) []string {
	runes := []rune(command)
	var tokens []string
	var token strings.Builder
	var quote rune
	escaped := false
	flush := func() {
		if token.Len() == 0 {
			return
		}
		tokens = append(tokens, token.String())
		token.Reset()
	}
	for index := 0; index < len(runes); index++ {
		r := runes[index]
		if quote == '\'' {
			token.WriteRune(r)
			if r == quote {
				quote = 0
			}
			continue
		}
		if escaped {
			token.WriteRune(r)
			escaped = false
			continue
		}
		if r == '\\' {
			token.WriteRune(r)
			escaped = true
			continue
		}
		if quote != 0 {
			token.WriteRune(r)
			if r == quote {
				quote = 0
			}
			continue
		}
		if r == '\'' || r == '"' || r == '`' {
			token.WriteRune(r)
			quote = r
			continue
		}
		if r == '\n' {
			flush()
			tokens = append(tokens, "\n")
			continue
		}
		if unicode.IsSpace(r) {
			flush()
			continue
		}
		if r == '|' {
			flush()
			op := string(r)
			if index+1 < len(runes) && (runes[index+1] == r || runes[index+1] == '&') {
				op += string(runes[index+1])
				index++
			}
			tokens = append(tokens, op)
			continue
		}
		if r == '&' {
			// An ampersand is also part of shell redirections such as 2>&1,
			// >&2 and &>/dev/null. Keeping it in the surrounding token avoids
			// presenting a harmless file-descriptor redirect as a background
			// command operator on its own display line.
			current := token.String()
			previousIsRedirect := strings.HasSuffix(current, ">") || strings.HasSuffix(current, "<")
			nextIsRedirect := index+1 < len(runes) && runes[index+1] == '>'
			if previousIsRedirect || nextIsRedirect {
				token.WriteRune(r)
				continue
			}
			flush()
			op := string(r)
			if index+1 < len(runes) && runes[index+1] == r {
				op += string(r)
				index++
			}
			tokens = append(tokens, op)
			continue
		}
		if r == ';' {
			flush()
			tokens = append(tokens, string(r))
			continue
		}
		token.WriteRune(r)
	}
	flush()
	return tokens
}

func isShellChainOperator(token string) bool {
	switch token {
	case "|", "||", "&&", "&", ";":
		return true
	default:
		return false
	}
}

func riskDisplay(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "read_only", "readonly", "safe":
		return "只读"
	case "low":
		return "低风险"
	case "medium":
		return "中风险"
	case "high", "critical", "destructive", "dangerous":
		return "高风险"
	case "":
		return "风险未知"
	default:
		return value
	}
}

func (a *App) riskDisplay(value string) string {
	return a.t(riskDisplay(value))
}

func (a *App) formatCommandForDisplay(command string, width, maximumLines int) formattedCommand {
	return formatCommandForDisplayWithLabels(command, width, maximumLines, a.t("$ … [展开全部] · Ctrl+O"), a.t("  … [展开全部] · Ctrl+O · 余%d行"))
}

func (a *App) formatExpandedCommandForDisplay(command string, width, maximumLines int) formattedCommand {
	return formatCommandForDisplayWithLabels(command, width, maximumLines, a.t("$ … 命令超出当前屏幕"), a.t("  … 命令超出当前屏幕 · 余%d行"))
}

func (a *App) commandFoldLine(line string) bool {
	return strings.Contains(line, a.t("[展开全部]"))
}

func formatCommandForDisplayWithLabels(command string, width, maximumLines int, singleSummary, multiSummary string) formattedCommand {
	result := formatCommandForDisplay(command, width, 0)
	if maximumLines <= 0 || len(result.Lines) <= maximumLines {
		return result
	}
	result.Truncated = true
	if maximumLines == 1 {
		result.Lines = []string{terminalview.ClipPad(singleSummary, width)}
		return result
	}
	visible := append([]string(nil), result.Lines[:maximumLines-1]...)
	summary := fmt.Sprintf(multiSummary, len(result.Lines)-maximumLines+1)
	visible = append(visible, terminalview.ClipPad(summary, width))
	result.Lines = visible
	return result
}

func commandCharacterCount(command string) int {
	return utf8.RuneCountInString(strings.TrimSpace(command))
}

func integerString(value int) string {
	if value == 0 {
		return "0"
	}
	var digits [20]byte
	index := len(digits)
	for value > 0 {
		index--
		digits[index] = byte('0' + value%10)
		value /= 10
	}
	return string(digits[index:])
}
