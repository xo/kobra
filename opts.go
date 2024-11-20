package kobra

import (
	"context"
	"fmt"
	"slices"
)

// parent is a command option to set the parent for the command.
func parent(parent *Command) Option {
	return option{
		command: func(c *Command) error {
			c.Parent, c.OnErr = parent, parent.OnErr
			return nil
		},
	}
}

// reflectTo is a flag set option to set various options on each flag based on
// val.
func reflectTo[T *E, E any](val T) Option {
	return option{
		set: func(fs *FlagSet) error {
			if val == nil {
				return ErrValueCannotBeNil
			}
			return nil
		},
	}
}

// RunArgs is a [Run] option to set the command-line arguments to use.
func RunArgs(args []string) Option {
	return option{
		run: func(opts *runOpts) error {
			opts.args = args
			return nil
		},
	}
}

// Version is a command option to hook --version with version output.
func Version(version string, opts ...Option) Option {
	return Hook(func(ctx context.Context) error {
		_, _ = fmt.Fprintf(Stdout(ctx), "%s %s\n", RootName(ctx), version)
		return ErrExit
	},
		prepend(opts, Usage("version", "display version and exit"))...,
	)
}

// Help is a command option to hook --help with help output.
func Help(opts ...Option) Option {
	return Hook(func(ctx context.Context) error {
		_, _ = fmt.Fprintf(Stdout(ctx), "%s help!\n", RootName(ctx))
		return ErrExit
	},
		prepend(opts, Usage("help", "display help and exit"))...,
	)
}

// Comp is a command option to enable command completion.
func Comp() Option {
	return option{
		command: func(c *Command) error {
			if c.Parent != nil {
				return ErrOptionCanOnlyBeUsedWithRootCommand
			}
			return nil
		},
	}
}

// Usage is a command and flag option to set the command/flag's name and usage.
func Usage(name, usage string) Option {
	return option{
		command: func(c *Command) error {
			c.Descs[0].Name, c.Descs[0].Usage = name, usage
			return nil
		},
		flag: func(g *Flag) error {
			g.Descs[0].Name, g.Descs[0].Usage = name, usage
			return nil
		},
	}
}

// Short is a flag option to set the short name for a flag.
func Short(name string) Option {
	return option{
		flag: func(g *Flag) error {
			if len(name) != 1 {
				return ErrInvalidShortName
			}
			g.Descs = append(g.Descs, Desc{Name: name})
			return nil
		},
	}
}

// Alias is a command/flag option to set the command/flag's alias.
func Alias(name, usage string) Option {
	return option{
		command: func(c *Command) error {
			c.Descs = append(c.Descs, Desc{
				Name:  name,
				Usage: usage,
			})
			return nil
		},
		flag: func(g *Flag) error {
			g.Descs = append(g.Descs, Desc{
				Name:  name,
				Usage: usage,
			})
			return nil
		},
	}
}

// ArgsFunc is a command option to set the command's argument validation funcs.
func ArgsFunc(funcs ...func([]string) error) Option {
	return option{
		command: func(c *Command) error {
			c.Args = append(c.Args, funcs...)
			return nil
		},
	}
}

// Args is a command option to the set the range of a command's minimum/maximum
// arg count and allowed arg values. A minimum/maximum < 0 means no
// minimum/maximum.
func Args(minimum, maximum int, values ...string) Option {
	return ArgsFunc(func(args []string) error {
		switch n := len(args); {
		case minimum < 0 && maximum < 0:
		case minimum == 0 && maximum == 0 && n != 0:
			return fmt.Errorf("%w: takes no args", ErrInvalidArgCount)
		case minimum <= 0 && maximum < n:
			return fmt.Errorf("%w: takes max %d arg(s)", ErrInvalidArgCount, maximum)
		case maximum <= 0 && n < minimum:
			return fmt.Errorf("%w: takes min %d arg(s)", ErrInvalidArgCount, minimum)
		case 0 <= minimum && 1 <= maximum && (n < minimum || maximum < n):
			return fmt.Errorf("%w: takes %d-%d args", ErrInvalidArgCount, minimum, maximum)
		}
		if len(values) != 0 {
			for i, arg := range args {
				if !slices.Contains(values, arg) {
					return fmt.Errorf("%w: arg %d (%q) is not an allowed value", ErrInvalidArgValue, i, arg)
				}
			}
		}
		return nil
	})
}

// UserConfigFile is a command option to load a config file from the user's
// config directory.
func UserConfigFile() Option {
	return option{
		command: func(c *Command) error {
			if c.Parent != nil {
				return ErrUserConfigFileCannotBeUsedWithSubCommand
			}
			dir, err := userConfigDir()
			if err != nil {
				return err
			}
			dir = dir
			return nil
		},
	}
}

// Sub is a command option to create a sub command.
func Sub(f func(context.Context, []string) error, opts ...Option) Option {
	return option{
		command: func(c *Command) error {
			return c.Sub(f, opts...)
		},
	}
}

// MapKey is a flag option to set the map key type.
func MapKey(opts ...Option) Option {
	return option{
		flag: func(g *Flag) error {
			if g.Type == MapT {
			}
			return nil
		},
	}
}

// BindSet is a flag option to set a binding variable and a set flag.
func BindSet[T *E, E any](v T, b *bool) Option {
	return option{
		flag: func(g *Flag) error {
			val, err := newBind(v, b)
			if err != nil {
				return err
			}
			g.Binds = append(g.Binds, val)
			return nil
		},
	}
}

// Bind is a flag option to set a binding variable.
func Bind[T *E, E any](v T) Option {
	return BindSet(v, nil)
}

// Default is a flag option to set the flag's default value.
func Default(def any) Option {
	return option{
		flag: func(g *Flag) error {
			g.Def = def
			return nil
		},
	}
}

// NoArg is a flag option to set that the flag expects no argument.
func NoArg(noArg bool) Option {
	return option{
		flag: func(g *Flag) error {
			g.NoArg = noArg
			return nil
		},
	}
}

// Key is a flag option to set the flag's lookup key in a config file.
func Key(typ, key string) Option {
	return option{
		flag: func(g *Flag) error {
			if g.Keys == nil {
				g.Keys = make(map[string]string)
			}
			g.Keys[typ] = key
			return nil
		},
	}
}

// Hook is a option to set a hook for a flag, that exits normally.
func Hook(f func(context.Context) error, opts ...Option) Option {
	return option{
		command: func(c *Command) error {
			_ = c.Flags.Hook("", "", f, opts...)
			return nil
		},
		flag: func(g *Flag) error {
			g.Type, g.Def = HookT, f
			return nil
		},
	}
}

// HookDump is an option to set a hook for a flag that Fprint's s and v to the
// set standard out and then exits normally.
func HookDump(s string, v ...any) Option {
	return Hook(func(ctx context.Context) error {
		_, _ = fmt.Fprintf(Stdout(ctx), s, v...)
		return ErrExit
	})
}

// Layout is a option to set the parsing layout for a time value.
func Layout(layout string) Option {
	return option{
		time: func(t *timeVal) error {
			t.layout = layout
			return nil
		},
	}
}

/*
// MustExist is a option to indicate that a path value must exist on disk.
func MustExist(mustExist bool) Option {
	return option{
		flag: func(g *Flag) error {
			return nil
		},
	}
}

// Relative is a option to indacet that a path value is relative to the base
// path.
func Relative(dir string) Option {
	return option{
		flag: func(g *Flag) error {
			return nil
		},
	}
}
*/

// Option is a option.
type Option interface {
	apply(any) error
}

// option wraps an option.
type option struct {
	command func(*Command) error
	set     func(*FlagSet) error
	flag    func(*Flag) error
	time    func(*timeVal) error
	run     func(*runOpts) error
}

// apply satisfies the [Option] interface.
func (opt option) apply(val any) error {
	switch v := val.(type) {
	case *Command:
		if opt.command != nil {
			return opt.command(v)
		}
	case *Flag:
		if opt.flag != nil {
			return opt.flag(v)
		}
	case *timeVal:
		if opt.time != nil {
			return opt.time(v)
		}
	case *runOpts:
		if opt.run != nil {
			return opt.run(v)
		}
		return nil
	}
	return ErrOptionAppliedToInvalidType
}
