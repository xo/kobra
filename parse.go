package ox

import (
	"fmt"
	"slices"
	"strings"
)

// Parse parses the command-line arguments into vars.
func Parse(root *Command, args []string, vars Vars) (*Command, []string, error) {
	if root.Parent != nil {
		return nil, nil, fmt.Errorf("Parse: %w", ErrCanOnlyBeUsedWithRootCommand)
	}
	if err := root.Populate(false, false, vars); err != nil {
		return nil, nil, newCommandError(root.Name(), err)
	}
	if len(args) == 0 {
		return root, nil, nil
	}
	return parse(root, args, vars)
}

// parse parses the args into m based on the flags on the command.
func parse(cmd *Command, args []string, vars Vars) (*Command, []string, error) {
	var v []string
	var s string
	var n int
	var err error
	for len(args) != 0 {
		switch s, n, args = args[0], len(args[0]), args[1:]; {
		case n == 0, n == 1, s[0] != '-':
			// lookup sub command
			var c *Command
			if len(v) == 0 {
				c = cmd.Command(s)
			}
			if c != nil {
				if err := c.Populate(false, false, vars); err != nil {
					return nil, nil, newCommandError(c.Name(), err)
				}
				cmd = c
			} else {
				v = append(v, s)
			}
		case s == "--":
			return cmd, append(v, args...), nil
		case s[1] == '-':
			if args, err = parseLong(cmd, s, args, vars); err != nil {
				return nil, nil, err
			}
		default:
			if args, err = parseShort(cmd, s, args, vars); err != nil {
				return nil, nil, err
			}
		}
	}
	return cmd, v, nil
}

// parseLong parses a long flag ('--arg' '--arg v' '--arg k=v' '--arg=' '--arg=v').
func parseLong(cmd *Command, s string, args []string, vars Vars) ([]string, error) {
	arg, value, ok := strings.Cut(strings.TrimPrefix(s, "--"), "=")
	g := cmd.Flag(arg)
	switch {
	case g == nil:
		return nil, newFlagError(arg, ErrUnknownFlag)
	case ok: // --arg=v
	case g.NoArg: // --arg
		value = toBoolString(g.Def)
	case len(args) != 0: // --arg v
		value, args = args[0], args[1:]
	default: // missing argument to --arg
		return nil, newFlagError(arg, ErrMissingArgument)
	}
	if err := vars.Set(g, value, true); err != nil {
		return nil, newFlagError(arg, err)
	}
	return args, nil
}

// parseShort parses short flags ('-a' '-aaa' '-av' '-a v' '-a=' '-a=v').
func parseShort(cmd *Command, s string, args []string, vars Vars) ([]string, error) {
	for v := []rune(s[1:]); len(v) != 0; v = v[1:] {
		arg := string(v[0])
		switch g, n := cmd.Flag(arg), len(v[1:]); {
		case g == nil:
			return nil, newFlagError(arg, ErrUnknownFlag)
		case g.NoArg: // -a
			var value string
			if slices.Index(v, '=') == 1 {
				value, v = string(v[2:]), v[len(v)-1:]
			} else {
				value = toBoolString(g.Def)
			}
			if err := vars.Set(g, value, true); err != nil {
				return nil, newFlagError(arg, err)
			}
		case n == 0 && len(args) == 0: // missing argument to -a
			return nil, newFlagError(arg, ErrMissingArgument)
		case n != 0: // -a=, -a=v, -av
			if slices.Index(v, '=') == 1 {
				v = v[1:]
			}
			if err := vars.Set(g, string(v[1:]), true); err != nil {
				return nil, newFlagError(arg, err)
			}
			return args, nil
		default: // -a v
			if err := vars.Set(g, args[0], true); err != nil {
				return nil, newFlagError(arg, err)
			}
			return args[1:], nil
		}
	}
	return args, nil
}