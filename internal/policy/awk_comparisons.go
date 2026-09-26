package policy

import (
	"path"
	"regexp"
	"sort"
	"strings"

	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/syntax"
)

// The textual policy must not interpret a parenthesized AWK scalar comparison
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
			mask = maskAWKScalarComparisons
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

const awkScalar = `(?:\$?[A-Za-z_][A-Za-z_0-9]*|\$[0-9]+|[0-9]+(?:\.[0-9]+)?(?:[eE][+-]?[0-9]+)?)`

var awkScalarComparison = regexp.MustCompile(`^\([ \t]*` + awkScalar + `(?:[ \t]*[+*-][ \t]*` + awkScalar + `)*[ \t]*(>=?)[ \t]*` + awkScalar + `(?:[ \t]*[+*-][ \t]*` + awkScalar + `)*[ \t]*\)`)

func maskAWKScalarComparisons(program string) string {
	masked := []byte(program)
	previous := byte(0)
	for i := 0; i < len(program); i++ {
		c := program[i]
		switch c {
		case ' ', '\t', '\r', '\n':
			continue
		case '#':
			for i < len(program) && program[i] != '\n' {
				i++
			}
			continue
		case '"':
			end := awkLiteralEnd(program, i, false)
			if end < 0 {
				return program
			}
			i = end
		case '/':
			// Only recognize an unambiguous regex context. Division and dialect
			// extensions retain the original conservative check for this program.
			if previous != 0 && !strings.ContainsRune("~(,=!:;{", rune(previous)) {
				return program
			}
			end := awkLiteralEnd(program, i, true)
			if end < 0 {
				return program
			}
			i = end
		case '(':
			if match := awkScalarComparison.FindStringSubmatchIndex(program[i:]); match != nil {
				// Mask just the comparison operator, never strings, regexes,
				// print destinations or the rest of the program.
				for j := match[2]; j < match[3]; j++ {
					masked[i+j] = ' '
				}
			}
		}
		previous = c
	}
	return string(masked)
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
