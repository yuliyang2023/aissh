package policy

import (
	"path"
	"sort"
	"strings"

	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/syntax"
)

// The textual policy must not interpret an AWK expression comparison
// as a shell/print redirect. Keep this exception narrow: it is only available
// for a statically known AWK program, never arbitrary quoted shell arguments.
// The returned text is for policy inspection only; the collector is unchanged.
func maskMonitorAWKComparisons(command string, depth int) string {
	if depth >= 8 || len(command) > 128<<10 {
		return command
	}
	file, err := syntax.NewParser().Parse(strings.NewReader(command), "")
	if err != nil {
		return command // Uncertain syntax gets no exception to the existing policy.
	}
	type edit struct {
		start, end int
		text       string
	}
	var edits []edit
	syntax.Walk(file, func(node syntax.Node) bool {
		call, ok := node.(*syntax.CallExpr)
		if !ok || len(call.Args) < 2 {
			return true
		}
		name, ok := literalMonitorWord(call.Args[0])
		if !ok {
			return true
		}
		var word *syntax.Word
		var mask func(string) string
		switch path.Base(name) {
		case "awk", "gawk", "mawk", "nawk":
			word = monitorAWKProgram(call.Args[1:])
			mask = maskAWKComparisons
		case "sh", "bash", "dash", "ksh":
			for i := 1; i+1 < len(call.Args); i++ {
				flag, literal := literalMonitorWord(call.Args[i])
				if !literal || !strings.HasPrefix(flag, "-") || flag == "--" {
					break
				}
				if !strings.HasPrefix(flag, "--") && strings.Contains(flag, "c") {
					word = call.Args[i+1]
					break
				}
			}
			mask = func(script string) string { return maskMonitorAWKComparisons(script, depth+1) }
		}
		if word == nil {
			return true
		}
		value, literal := literalMonitorWord(word)
		if literal {
			if masked := mask(value); masked != value {
				quoted := "'" + strings.ReplaceAll(masked, "'", "'\"'\"'") + "'"
				edits = append(edits, edit{int(word.Pos().Offset()), int(word.End().Offset()), quoted})
			}
		}
		return true // Still visit command substitutions; never erase their checks.
	})
	sort.Slice(edits, func(i, j int) bool { return edits[i].start > edits[j].start })
	for _, e := range edits {
		command = command[:e.start] + e.text + command[e.end:]
	}
	return command
}

// Decode quoting only after proving there are no expansions. This never runs
// shell code, reads an environment variable, or expands a filesystem pattern.
func literalMonitorWord(word *syntax.Word) (string, bool) {
	static := true
	syntax.Walk(word, func(node syntax.Node) bool {
		switch n := node.(type) {
		case nil, *syntax.Word, *syntax.Lit:
		case *syntax.SglQuoted:
			static = static && !n.Dollar
		case *syntax.DblQuoted:
			static = static && !n.Dollar
		default:
			static = false
		}
		return static
	})
	if !static {
		return "", false
	}
	value, err := expand.Literal(&expand.Config{}, word)
	return value, err == nil
}

func monitorAWKProgram(args []*syntax.Word) *syntax.Word {
	for i := 0; i < len(args); i++ {
		value, ok := literalMonitorWord(args[i])
		if !ok {
			return nil
		}
		switch {
		case value == "--":
			if i+1 < len(args) {
				return args[i+1]
			}
			return nil
		case value == "-F" || value == "-v":
			i++
		case strings.HasPrefix(value, "-F") || strings.HasPrefix(value, "-v"):
		case strings.HasPrefix(value, "-"):
			return nil // Includes -f/-e and extensions whose source is not this word.
		default:
			return args[i]
		}
	}
	return nil
}

func maskAWKComparisons(program string) string {
	masked := []byte(program)
	// In AWK, > is a comparison except at the outer level of print/printf
	// arguments. Track that level separately, including print in a for header.
	// Strings and regexes are never masked: they may contain shell commands.
	printDepth := -1
	var parens []bool // true for control headers, whose closing ) starts a statement
	brackets := 0
	expectOperand := true
	control := false
	continued := false
	for i := 0; i < len(program); i++ {
		c := program[i]
		switch c {
		case ' ', '\t', '\r':
			continue
		case '\n':
			if !continued && len(parens) == 0 && brackets == 0 {
				printDepth = -1
				expectOperand = true
			}
			continue
		case '#':
			for i < len(program) && program[i] != '\n' {
				i++
			}
			i-- // Process the newline as a statement boundary.
			continue
		case '\\':
			// Do not guess how a dialect joins tokens across escaped newlines.
			return program
		case '"':
			end := awkLiteralEnd(program, i, false)
			if end < 0 {
				return program
			}
			i = end
			expectOperand = false
		case '/':
			if expectOperand {
				end := awkLiteralEnd(program, i, true)
				if end < 0 {
					return program
				}
				i = end
				expectOperand = false
			} else {
				expectOperand = true // Division, not the start of a regex.
			}
		case '(':
			parens = append(parens, control)
			expectOperand = true
		case ')':
			if len(parens) == 0 {
				return program
			}
			if printDepth == len(parens) {
				printDepth = -1
			}
			expectOperand = parens[len(parens)-1]
			parens = parens[:len(parens)-1]
		case '{', '}', ';':
			printDepth = -1
			expectOperand = true
		case '>':
			if i+1 < len(program) && program[i+1] == '>' {
				i++ // Append redirection is never a comparison.
			} else if printDepth < 0 || len(parens) > printDepth || brackets > 0 || i+1 < len(program) && program[i+1] == '=' {
				masked[i] = ' '
			}
			expectOperand = true
		case '[':
			brackets++
			expectOperand = true
		case ']':
			if brackets == 0 {
				return program
			}
			brackets--
			expectOperand = false
		case '+', '-':
			if i+1 < len(program) && program[i+1] == c {
				i++ // Prefix/postfix increment keeps its operand context.
			} else {
				expectOperand = true
			}
		case '$', '.', '=', '<', '!', '~', '*', '%', '^', ',', '|', '&', '?', ':':
			expectOperand = true
		default:
			if awkWordByte(c) {
				start := i
				for i+1 < len(program) && awkWordByte(program[i+1]) {
					i++
				}
				word := program[start : i+1]
				switch word {
				case "print", "printf":
					printDepth = len(parens)
					expectOperand = true
				case "if", "while", "for":
					control = true
					expectOperand = true
					continued = false
					continue
				case "return", "exit", "delete", "in", "else", "do":
					expectOperand = true
				default:
					expectOperand = false
				}
			} else {
				return program // Unknown dialect syntax gets no exception.
			}
		}
		control = false
		// AWK permits an unescaped newline after these expression separators.
		continued = strings.ContainsRune(",|&?:", rune(c))
	}
	if len(parens) != 0 || brackets != 0 {
		return program
	}
	return string(masked)
}

func awkWordByte(c byte) bool {
	return c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c >= 0x80
}

func awkLiteralEnd(program string, start int, regex bool) int {
	quote := program[start]
	brackets := 0
	for i := start + 1; i < len(program); i++ {
		switch program[i] {
		case '\\':
			i++
		case '[':
			if regex {
				brackets++
			}
		case ']':
			if regex && brackets > 0 {
				brackets--
			}
		default:
			if program[i] == quote && brackets == 0 {
				return i
			}
		}
	}
	return -1
}
