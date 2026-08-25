package sshwrap

import (
	"fmt"
	"strings"
)

// Parsed keeps the original OpenSSH connection options separate from an
// optional remote command. aissh uses the connection portion for read-only
// monitor channels while the interactive session receives the untouched args.
type Parsed struct {
	ConnectionArgs []string
	Target         string
	RemoteCommand  []string
}

var optionsWithValue = map[string]bool{
	"-B": true, "-b": true, "-c": true, "-D": true, "-E": true,
	"-e": true, "-F": true, "-I": true, "-i": true, "-J": true,
	"-L": true, "-l": true, "-m": true, "-O": true, "-o": true,
	"-p": true, "-Q": true, "-R": true, "-S": true, "-W": true,
	"-w": true,
}

func Parse(args []string) (Parsed, error) {
	var parsed Parsed
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if parsed.Target != "" {
			parsed.RemoteCommand = append(parsed.RemoteCommand, args[i:]...)
			break
		}
		if arg == "--" {
			if i+1 >= len(args) {
				return Parsed{}, fmt.Errorf("ssh target is required")
			}
			parsed.Target = args[i+1]
			parsed.ConnectionArgs = append(parsed.ConnectionArgs, "--")
			i++
			continue
		}
		if strings.HasPrefix(arg, "-") && arg != "-" {
			parsed.ConnectionArgs = append(parsed.ConnectionArgs, arg)
			name := arg
			if len(arg) > 2 {
				name = arg[:2]
			}
			if optionsWithValue[name] && len(arg) == 2 {
				if i+1 >= len(args) {
					return Parsed{}, fmt.Errorf("ssh option %s requires a value", arg)
				}
				i++
				parsed.ConnectionArgs = append(parsed.ConnectionArgs, args[i])
			}
			continue
		}
		parsed.Target = arg
	}
	if parsed.Target == "" {
		return Parsed{}, fmt.Errorf("ssh target is required")
	}
	return parsed, nil
}

func (p Parsed) MonitorArgs() []string {
	args := append([]string(nil), p.ConnectionArgs...)
	return append(args, p.Target)
}

func HasControlPath(args []string) bool {
	for index, arg := range args {
		if arg == "-S" && index+1 < len(args) {
			return true
		}
		if strings.HasPrefix(arg, "-S") && len(arg) > 2 {
			return true
		}
		if arg == "-o" && index+1 < len(args) && strings.HasPrefix(strings.ToLower(args[index+1]), "controlpath=") {
			return true
		}
		if strings.HasPrefix(strings.ToLower(arg), "-ocontrolpath=") {
			return true
		}
	}
	return false
}

func ControlOptions(path string) []string {
	return []string{
		"-o", "ControlMaster=auto",
		"-o", "ControlPersist=60",
		"-o", "ControlPath=" + path,
	}
}
