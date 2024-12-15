package ox

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/xo/ox/text"
)

// Command is a command.
type Command struct {
	// Parent is the command's parent.
	Parent *Command
	// Exec is the command's exec func.
	Exec ExecFunc
	// Name is the command's name.
	Name string
	// Usage is the command's usage.
	Usage string
	// Aliases are the command's aliases.
	Aliases []string
	// Suggested are the command's suggested names.
	Suggested []string
	// Flags are the command's flags.
	Flags *FlagSet
	// Commands are the command's sub commands.
	Commands []*Command
	// Args are the command's argument validation func's.
	Args []func([]string) error
	// OnErr indicates whether to continue, panic, or to error when
	// flag/argument parsing fails.
	OnErr OnErr
	// Help is the command's help emitter.
	Help io.WriterTo
	// Comp enables completion for the command.
	Comp bool
	// Section is the help section.
	Section int
	// Hidden indicates the command is hidden from help output.
	Hidden bool
	// Deprecated indicates the command is deprecated.
	Deprecated bool
	// Special is the special value.
	Special string
}

// NewCommand creates a new command.
func NewCommand(opts ...Option) (*Command, error) {
	cmd := &Command{
		Section: -1,
	}
	if err := Apply(cmd, opts...); err != nil {
		return nil, err
	}
	switch noName := cmd.Name == ""; {
	case noName && cmd.Parent == nil:
		cmd.Name = filepath.Base(os.Args[0])
	case noName:
		return nil, fmt.Errorf("command: %w", ErrUsageNotSet)
	}
	if err := ApplyPost(cmd, opts...); err != nil {
		return nil, err
	}
	return cmd, nil
}

// Sub creates a sub command.
func (cmd *Command) Sub(opts ...Option) error {
	sub, err := NewCommand(prepend(opts, Parent(cmd))...)
	if err != nil {
		return err
	}
	cmd.Commands = append(cmd.Commands, sub)
	return nil
}

// Tree returns the parents for the command.
func (cmd *Command) Tree() []string {
	var v []string
	for c := cmd; c != nil; c = c.Parent {
		v = append(v, c.Name)
	}
	slices.Reverse(v)
	return v
}

// Path returns the execution path for the command.
func (cmd *Command) Path() []string {
	return cmd.Tree()[1:]
}

// RootName returns the root name for the command.
func (cmd *Command) RootName() string {
	return cmd.Tree()[0]
}

// Populate populates vars with all the command's flags values, overwriting any
// set variables if applicable. When all is true, all flag values will be
// populated, otherwise only flags with default values will be. When overwrite
// is true, existing vars will be set to either to flag's empty or default
// value.
func (cmd *Command) Populate(ctx *Context, all, overwrite bool, vars Vars) error {
	if cmd.Flags == nil {
		return nil
	}
	for _, g := range cmd.Flags.Flags {
		if _, ok := vars[g.Name]; ok && overwrite {
			delete(vars, g.Name)
		}
		var value string
		switch {
		case g.Type == HookT, g.Def == nil && !all:
			continue
		case g.Def != nil:
			var err error
			if value, err = ctx.Expand(g.Def); err != nil {
				return err
			}
		}
		if err := vars.Set(ctx, g, value, false); err != nil {
			return fmt.Errorf("cannot populate %s with %q: %w", g.Name, value, err)
		}
	}
	return nil
}

// Suggest returns a [SuggestionError] if there is a matching sub command
// within the command's help distance.
func (cmd *Command) Suggest(args ...string) error {
	if len(args) == 0 {
		return nil
	}
	var minDist int
	if help, ok := cmd.Help.(*CommandHelp); ok {
		minDist = help.MinDist
	}
	if minDist <= 0 {
		minDist = DefaultMinDist
	}
	arg := []rune(strings.ToLower(args[0]))
	for _, c := range cmd.Commands {
		for _, name := range append(prepend(c.Aliases, c.Name), c.Suggested...) {
			if Ldist(arg, []rune(strings.ToLower(name))) <= minDist {
				return NewSuggestionError(cmd, args[0], c)
			}
		}
	}
	return fmt.Errorf(text.SuggestionErrorMessage, ErrUnknownCommand, args[0], cmd.Name)
}

// Lookup returns the furthest matching command in the command tree.
func (cmd *Command) Lookup(names ...string) *Command {
	ret := cmd
	for _, name := range names {
		if c := ret.Command(name); c != nil {
			ret = c
			continue
		}
		break
	}
	return ret
}

// Command returns the sub command with the name.
func (cmd *Command) Command(name string) *Command {
	for _, c := range cmd.Commands {
		if c.Name == name || slices.Contains(c.Aliases, name) {
			return c
		}
	}
	return nil
}

// CommandSpecial returns the sub command with the special value.
func (cmd *Command) CommandSpecial(special string) *Command {
	for _, c := range cmd.Commands {
		if c.Special == special {
			return c
		}
	}
	return nil
}

// Flag finds a flag from the command or the command's parents.
func (cmd *Command) Flag(name string, parents, short bool) *Flag {
	if cmd.Flags != nil {
		if i := slices.IndexFunc(cmd.Flags.Flags, func(g *Flag) bool {
			return g.Name == name && !short ||
				g.Short == name && short ||
				slices.ContainsFunc(g.Aliases, func(s string) bool {
					return s == name && len(s) == 1 == short
				})
		}); i != -1 {
			return cmd.Flags.Flags[i]
		}
	}
	if parents && cmd.Parent != nil {
		return cmd.Parent.Flag(name, parents, short)
	}
	return nil
}

// FlagSpecial returns the flag with a special value.
func (cmd *Command) FlagSpecial(special string) *Flag {
	if cmd.Flags != nil && len(cmd.Flags.Flags) != 0 {
		for _, g := range cmd.Flags.Flags {
			if g.Special == special {
				return g
			}
		}
	}
	return nil
}

// Validate validates the passed args.
func (cmd *Command) Validate(args []string) error {
	// validate args
	for _, f := range cmd.Args {
		if err := f(args); err != nil {
			return newCommandError(cmd.Name, err)
		}
	}
	return nil
}

// HelpContext adds the context to the command's help.
func (cmd *Command) HelpContext(ctx *Context) io.WriterTo {
	help := cmd.Help
	if help == nil {
		help, _ = NewCommandHelp(cmd)
	}
	if v, ok := help.(interface{ SetContext(*Context) }); ok {
		v.SetContext(ctx)
	}
	return help
}

// WriteTo satisfies the [io.WriterTo] interface.
func (cmd *Command) WriteTo(w io.Writer) (int64, error) {
	help := cmd.Help
	if help == nil {
		var err error
		if help, err = NewCommandHelp(cmd); err != nil {
			return 0, err
		}
	}
	return help.WriteTo(w)
}

// Comps returns the completions for args.
func (cmd *Command) Comps(args ...string) ([]Completion, CompDirective, error) {
	ctx := &Context{
		Root:   cmd,
		Stdout: io.Discard,
		Stderr: io.Discard,
		Panic:  func(any) {},
		Exit:   func(int) {},
		Continue: func(cmd *Command, err error) bool {
			fmt.Fprintf(os.Stderr, "COMP CONTINUE ERR: %v\n", err)
			switch {
			case errors.Is(err, ErrUnknownFlag),
				errors.Is(err, ErrExit):
				return true
			}
			return false
		},
		Vars: make(Vars),
	}
	n := len(args)
	if n == 0 {
		return nil, CompError, ErrInvalidArgCount
	}
	fmt.Fprintf(ctx.Stderr, "COMP ARGS: %v\n", args)
	var err error
	if ctx.Exec, _, err = Parse(ctx, cmd, args[:n-1], ctx.Vars); err != nil {
		fmt.Fprintf(ctx.Stderr, "COMP PARSE ERR: %v\n", err)
		return nil, CompError, err
	}
	fmt.Fprintf(os.Stderr, "COMP COMMAND: %s\n", ctx.Exec.Name)
	var comps []Completion
	dir := CompNoFileComp
	// build completions
	switch prev, arg := prev(args, n), args[n-1]; {
	case strings.HasPrefix(arg, "--"):
		comps, dir = cmd.CompFlags(arg[2:], true, false)
	case strings.HasPrefix(arg, "-"):
		comps, dir = cmd.CompFlags(arg[1:], true, true)
	case strings.HasPrefix(prev, "-"):
		switch g := cmd.Flag(prev, true, len([]rune(prev)) == 2); {
		case g == nil, g.NoArg, strings.Contains(prev, "="):
			comps, dir = cmd.CompCommands(arg)
		default:
			// TODO: handle completion for certain flag types
		}
	default:
		comps, dir = cmd.CompCommands(arg)
	}
	return comps, dir, nil
}

// CompCommands returns completions for the sub commands with the provided
// name.
func (cmd *Command) CompCommands(name string) ([]Completion, CompDirective) {
	m := make(map[string]bool)
	var comps []Completion
	var ok bool
	dir := CompNoFileComp
	lower := strings.ToLower(name)
	// check exact or case insensitive name/alias
	for _, c := range cmd.Commands {
		if comps, ok = addComp(comps, c, m, name == c.Name); ok {
			continue
		}
		if comps, ok = addComp(comps, c, m, slices.ContainsFunc(c.Aliases, func(s string) bool {
			return name == s
		})); ok {
			continue
		}
		if comps, ok = addComp(comps, c, m, strings.ToLower(c.Name) == lower); ok {
			continue
		}
		if comps, ok = addComp(comps, c, m, slices.ContainsFunc(c.Aliases, func(s string) bool {
			return lower == strings.ToLower(s)
		})); ok {
			continue
		}
	}
	// check prefix
	for _, c := range cmd.Commands {
		if comps, ok = addComp(comps, c, m, strings.HasPrefix(strings.ToLower(c.Name), lower)); ok {
			continue
		}
		if comps, ok = addComp(comps, c, m, slices.ContainsFunc(c.Aliases, func(s string) bool {
			return strings.HasPrefix(strings.ToLower(s), lower)
		})); ok {
			continue
		}
	}
	var minDist int
	if help, ok := cmd.Help.(*CommandHelp); ok {
		minDist = help.MinDist
	}
	if minDist <= 0 {
		minDist = DefaultMinDist
	}
	if minDist != 0 {
		l := []rune(lower)
		// check distance
		for _, c := range cmd.Commands {
			if comps, ok = addComp(comps, c, m, Ldist([]rune(strings.ToLower(c.Name)), l) <= minDist); ok {
				continue
			}
			if comps, ok = addComp(comps, c, m, slices.ContainsFunc(c.Aliases, func(s string) bool {
				return Ldist([]rune(strings.ToLower(s)), l) <= minDist
			})); ok {
				continue
			}
		}
	}
	return comps, dir
}

// CompFlags returns completions for a flag.
func (cmd *Command) CompFlags(name string, parents, short bool) ([]Completion, CompDirective) {
	dir := CompNoFileComp
	return nil, dir
}

// addComp adds a completion to
func addComp(comps []Completion, cmd *Command, m map[string]bool, b bool) ([]Completion, bool) {
	switch {
	case m[cmd.Name]:
		return comps, true
	case b:
		m[cmd.Name] = true
		return append(comps, NewCompletion(cmd.Name, cmd.Usage)), true
	}
	return comps, false
}

// FlagSet is a set of command-line flag definitions.
type FlagSet struct {
	Flags []*Flag
}

// Flags creates a new flag set from the options.
func Flags() *FlagSet {
	return new(FlagSet)
}

// Option returns a [CommandOption] for the flag set.
func (fs *FlagSet) Option() option {
	return option{
		name: "FlagSet",
		cmd: func(cmd *Command) error {
			cmd.Flags = fs
			return nil
		},
	}
}

// Var adds a variable to the flag set.
func (fs *FlagSet) Var(name, desc string, opts ...Option) *FlagSet {
	if fs == nil {
		fs = Flags()
	}
	g, err := NewFlag(name, desc, opts...)
	if err != nil {
		panic(err)
	}
	fs.Flags = append(fs.Flags, g)
	return fs
}

// String adds a string variable to the flag set.
func (fs *FlagSet) String(name, desc string, opts ...Option) *FlagSet {
	return fs.Var(name, desc, prepend(opts, Option(StringT))...)
}

// Bytes adds a []byte variable to the flag set.
func (fs *FlagSet) Bytes(name, desc string, opts ...Option) *FlagSet {
	return fs.Var(name, desc, prepend(opts, Option(BytesT))...)
}

// Base64 adds a base64 encoded []byte variable to the flag set.
func (fs *FlagSet) Base64(name, desc string, opts ...Option) *FlagSet {
	return fs.Var(name, desc, prepend(opts, Option(Base64T))...)
}

// Hex adds a hex encoded []byte variable to the flag set.
func (fs *FlagSet) Hex(name, desc string, opts ...Option) *FlagSet {
	return fs.Var(name, desc, prepend(opts, Option(HexT))...)
}

// Bool adds a bool variable to the flag set.
func (fs *FlagSet) Bool(name, desc string, opts ...Option) *FlagSet {
	return fs.Var(name, desc, prepend(opts, Option(BoolT))...)
}

// Byte adds a byte variable to the flag set.
func (fs *FlagSet) Byte(name, desc string, opts ...Option) *FlagSet {
	return fs.Var(name, desc, prepend(opts, Option(ByteT))...)
}

// Rune adds a rune variable to the flag set.
func (fs *FlagSet) Rune(name, desc string, opts ...Option) *FlagSet {
	return fs.Var(name, desc, prepend(opts, Option(RuneT))...)
}

// Int64 adds a int64 variable to the flag set.
func (fs *FlagSet) Int64(name, desc string, opts ...Option) *FlagSet {
	return fs.Var(name, desc, prepend(opts, Option(Int64T))...)
}

// Int32 adds a int32 variable to the flag set.
func (fs *FlagSet) Int32(name, desc string, opts ...Option) *FlagSet {
	return fs.Var(name, desc, prepend(opts, Option(Int32T))...)
}

// Int16 adds a int16 variable to the flag set.
func (fs *FlagSet) Int16(name, desc string, opts ...Option) *FlagSet {
	return fs.Var(name, desc, prepend(opts, Option(Int16T))...)
}

// Int8 adds a int8 variable to the flag set.
func (fs *FlagSet) Int8(name, desc string, opts ...Option) *FlagSet {
	return fs.Var(name, desc, prepend(opts, Option(Int8T))...)
}

// Int adds a int variable to the flag set.
func (fs *FlagSet) Int(name, desc string, opts ...Option) *FlagSet {
	return fs.Var(name, desc, prepend(opts, Option(IntT))...)
}

// Uint64 adds a uint64 variable to the flag set.
func (fs *FlagSet) Uint64(name, desc string, opts ...Option) *FlagSet {
	return fs.Var(name, desc, prepend(opts, Option(Uint64T))...)
}

// Uint32 adds a uint32 variable to the flag set.
func (fs *FlagSet) Uint32(name, desc string, opts ...Option) *FlagSet {
	return fs.Var(name, desc, prepend(opts, Option(Uint32T))...)
}

// Uint16 adds a uint16 variable to the flag set.
func (fs *FlagSet) Uint16(name, desc string, opts ...Option) *FlagSet {
	return fs.Var(name, desc, prepend(opts, Option(Uint16T))...)
}

// Uint8 adds a uint8 variable to the flag set.
func (fs *FlagSet) Uint8(name, desc string, opts ...Option) *FlagSet {
	return fs.Var(name, desc, prepend(opts, Option(Uint8T))...)
}

// Uint adds a uint variable to the flag set.
func (fs *FlagSet) Uint(name, desc string, opts ...Option) *FlagSet {
	return fs.Var(name, desc, prepend(opts, Option(UintT))...)
}

// Float64 adds a float64 variable to the flag set.
func (fs *FlagSet) Float64(name, desc string, opts ...Option) *FlagSet {
	return fs.Var(name, desc, prepend(opts, Option(Float64T))...)
}

// Float32 adds a float32 variable to the flag set.
func (fs *FlagSet) Float32(name, desc string, opts ...Option) *FlagSet {
	return fs.Var(name, desc, prepend(opts, Option(Float32T))...)
}

// Complex128 adds a complex128 variable to the flag set.
func (fs *FlagSet) Complex128(name, desc string, opts ...Option) *FlagSet {
	return fs.Var(name, desc, prepend(opts, Option(Complex128T))...)
}

// Complex64 adds a complex64 variable to the flag set.
func (fs *FlagSet) Complex64(name, desc string, opts ...Option) *FlagSet {
	return fs.Var(name, desc, prepend(opts, Option(Complex64T))...)
}

// Timestamp adds a [time.Time] variable to the flag set in the expected format of
// [time.RFC3339].
func (fs *FlagSet) Timestamp(name, desc string, opts ...Option) *FlagSet {
	return fs.Var(name, desc, prepend(opts, Option(TimestampT))...)
}

// DateTime adds a [time.Time] variable to the flag set in the expected format of
// [time.DateTime].
func (fs *FlagSet) DateTime(name, desc string, opts ...Option) *FlagSet {
	return fs.Var(name, desc, prepend(opts, Option(DateTimeT))...)
}

// Date adds a [time.Time] variable to the flag set in the expected format of
// [time.DateOnly].
func (fs *FlagSet) Date(name, desc string, opts ...Option) *FlagSet {
	return fs.Var(name, desc, prepend(opts, Option(DateT))...)
}

// Duration adds a [time.Duration] variable to the flag set.
func (fs *FlagSet) Duration(name, desc string, opts ...Option) *FlagSet {
	return fs.Var(name, desc, prepend(opts, Option(DurationT))...)
}

// Path adds a path variable to the flag set.
func (fs *FlagSet) Path(name, desc string, opts ...Option) *FlagSet {
	return fs.Var(name, desc, prepend(opts, Option(PathT))...)
}

// Count adds a count variable to the flag set.
func (fs *FlagSet) Count(name, desc string, opts ...Option) *FlagSet {
	return fs.Var(name, desc, prepend(opts, Option(CountT))...)
}

// BigInt adds a [math/big.Int] variable to the flag set.
func (fs *FlagSet) BigInt(name, desc string, opts ...Option) *FlagSet {
	return fs.Var(name, desc, prepend(opts, Option(BigIntT))...)
}

// BigFloat adds a [math/big.Float] variable to the flag set.
func (fs *FlagSet) BigFloat(name, desc string, opts ...Option) *FlagSet {
	return fs.Var(name, desc, prepend(opts, Option(BigFloatT))...)
}

// BigRat adds a [math/big.Rat] variable to the flag set.
func (fs *FlagSet) BigRat(name, desc string, opts ...Option) *FlagSet {
	return fs.Var(name, desc, prepend(opts, Option(BigRatT))...)
}

// Addr adds a [netip.Addr] variable to the flag set.
func (fs *FlagSet) Addr(name, desc string, opts ...Option) *FlagSet {
	return fs.Var(name, desc, prepend(opts, Option(AddrT))...)
}

// AddrPort adds a [netip.AddrPort] variable to the flag set.
func (fs *FlagSet) AddrPort(name, desc string, opts ...Option) *FlagSet {
	return fs.Var(name, desc, prepend(opts, Option(AddrPortT))...)
}

// CIDR adds a [netip.Prefix] variable to the flag set.
func (fs *FlagSet) CIDR(name, desc string, opts ...Option) *FlagSet {
	return fs.Var(name, desc, prepend(opts, Option(CIDRT))...)
}

// URL adds a [url.URL] variable to the flag set.
func (fs *FlagSet) URL(name, desc string, opts ...Option) *FlagSet {
	return fs.Var(name, desc, prepend(opts, Option(URLT))...)
}

// UUID adds a uuid variable to the flag set.
func (fs *FlagSet) UUID(name, desc string, opts ...Option) *FlagSet {
	return fs.Var(name, desc, prepend(opts, Option(UUIDT))...)
}

// Color adds a color variable to the flag set.
func (fs *FlagSet) Color(name, desc string, opts ...Option) *FlagSet {
	return fs.Var(name, desc, prepend(opts, Option(ColorT))...)
}

// Slice adds a slice variable to the flag set.
func (fs *FlagSet) Slice(name, desc string, opts ...Option) *FlagSet {
	return fs.Var(name, desc, prepend(opts, Option(SliceT))...)
}

// Map adds a map variable to the flag set.
func (fs *FlagSet) Map(name, desc string, opts ...Option) *FlagSet {
	return fs.Var(name, desc, prepend(opts, Option(MapT))...)
}

// Hook adds a hook to the flag set.
func (fs *FlagSet) Hook(name, desc string, f any, opts ...Option) *FlagSet {
	return fs.Var(name, desc, prepend(opts, Option(HookT), NoArg(true, ""), Default(f))...)
}

// Flag is a command-line flag variable definition.
type Flag struct {
	// Type is the flag's [Type].
	Type Type
	// MapKey is the flag's map key type when the flag is a [MapT].
	MapKey Type
	// Elem is the flag's element type when the flag is a [SliceT] or [MapT].
	Elem Type
	// Name is the flag's long name (`--arg`).
	Name string
	// Usage is the flag's usage.
	Usage string
	// Short is the flag's short, single letter name (`-a`).
	Short string
	// Aliases are the flag's long and short aliases.
	Aliases []string
	// Spec is the flag's help spec.
	Spec string
	// Def is the flag's default value.
	Def any
	// NoArg when true, indicates that the flag does not require an argument.
	NoArg bool
	// NoArgDef is the flag's default value when no argument was passed for the
	// flag.
	NoArgDef any
	// Binds are the bound variables that will be additionally set when the
	// flag is encountered on the command-line.
	Binds []Binder
	// Keys are the flag's config look up keys.
	Keys map[string]string
	// Section is the flag's help section.
	Section int
	// Hidden indicates the flag is hidden from help output.
	Hidden bool
	// Deprecated indicates the flag is deprecated.
	Deprecated bool
	// Special is the flag's special value.
	Special string
}

// NewFlag creates a new command-line flag.
func NewFlag(name, usage string, opts ...Option) (*Flag, error) {
	g := &Flag{
		Type:    StringT,
		MapKey:  StringT,
		Elem:    StringT,
		Name:    name,
		Usage:   usage,
		Section: -1,
	}
	if err := Apply(g, opts...); err != nil {
		return nil, err
	}
	if extra, ok := typeFlagOpts[g.Type]; ok {
		if err := Apply(g, extra...); err != nil {
			return nil, err
		}
	}
	switch {
	case g.Name == "":
		return nil, ErrInvalidFlagName
	case g.NoArg && g.NoArgDef == nil:
		return nil, fmt.Errorf("flag %s: %w: NoArgDef cannot be nil", g.Name, ErrInvalidValue)
	case g.Type == HookT && g.Def == nil:
		return nil, fmt.Errorf("flag %s: %w: Def cannot be nil", g.Name, ErrInvalidValue)
	}
	return g, nil
}

// FlagsFrom creates flags for a value of type *struct using reflection. Builds
// flags for all exported fields with a `ox` tag, and a non-empty description.
//
// A `ox` tag starts with the flag's description, followed by one or more
// options separated by `,`. If a flag description must contain a comma (`,`)
// it can be escaped with a double `\\`. The flag's name is determined by the
// result of calling [DefaultFlagNameMapper], or it can be set with the `name:`
// option (see below).
//
// Example:
//
//	args := struct{
//		MyFlag      string   `ox:"my flag,short:f,default:$USER"`
//		MyVerbosity int      `ox:"my verbosity,type:count,short:v"`
//		MyURL       *url.URL `ox:"my url,set:MyURLSet"`
//		MyURLSet    bool
//		MyOtherFlag string   `ox:"a long\\, long description,short:F"`
//		MyFloat     float64  `ox:"my float,hidden,name:MYF"`
//	}{}
//
//	ox.FromFlags(&args)
//
// Recognized options:
//
//	type - sets the flag's field type
//	mapkey - sets the flag's map key type
//	elem - sets the flag's map/slice element type
//	name - sets the flag's name
//	short - sets the flag's short (single character) name
//	alias - adds a alias to the flag
//	aliases - adds multiple aliases, separated by `|` to the flag
//	spec - sets the flag's use spec
//	default - sets the flag's default value
//	noarg - set's the flag as requiring no argument, and the default value for the flag when toggled
//	key - set's the flag's lookup config key
//	hook - set's the flag's special value to `hook:<type>`, and can be used to hook [Defaults]'s flags
//	section - set's the flag's section
//	hidden - marks the flag as hidden
//	deprecated - marks the flag as deprecated
//	set - bind's the flag's set value to a bool field in the *struct of the name
//
// A `default:` option value will be expanded by [Context.Expand].
//
// The reflect tag name (`ox`) can be changed by setting the [DefaultTagName]
// variable if necessary.
func FlagsFrom[T *E, E any](val T) ([]*Flag, error) {
	v := reflect.ValueOf(val).Elem()
	if v.Kind() != reflect.Struct {
		return nil, fmt.Errorf("%w: %T is not a *struct", ErrInvalidType, val)
	}
	typ := v.Type()
	var flags []*Flag
	for i := range typ.NumField() {
		// check field is exported
		f := typ.Field(i)
		s, ok := f.Tag.Lookup(DefaultTagName)
		if !ok {
			continue
		}
		if r := []rune(f.Name); !unicode.IsUpper(r[0]) {
			return nil, fmt.Errorf("%w: field %q is not exported but has tag `%s`", ErrInvalidType, f.Name, DefaultTagName)
		}
		tag := split(s, ',')
		if len(tag) == 0 || tag[0] == "-" {
			continue
		}
		// build flag opts
		opts, err := buildFlagOpts(v, v.Field(i).Addr(), tag[1:])
		if err != nil {
			return nil, fmt.Errorf("field %s: %w", f.Name, err)
		}
		// create flag
		g, err := NewFlag(DefaultFlagNameMapper(f.Name), tag[0], opts...)
		if err != nil {
			return nil, fmt.Errorf("field %s: cannot create flag: %w", f.Name, err)
		}
		flags = append(flags, g)
	}
	return flags, nil
}

// New creates a new value for the flag's type.
func (g *Flag) New(ctx *Context) (Value, error) {
	switch g.Type {
	case SliceT:
		return NewSlice(g.Elem), nil
	case MapT:
		return NewMap(g.MapKey, g.Elem)
	case HookT:
		return newHook(ctx, g.Def)
	}
	return g.Type.New()
}

// SpecString returns the spec string for the flag.
func (g *Flag) SpecString() string {
	switch {
	case g.NoArg || g.Type == HookT:
		return g.Name
	case g.Spec != "":
		return g.Name + text.FlagSpecSpacer + g.Spec
	case g.Type == SliceT:
		return g.Name + text.FlagSpecSpacer + g.Elem.String()
	case g.Type == MapT:
		return g.Name + text.FlagSpecSpacer + g.MapKey.String() + "=" + g.Elem.String()
	}
	return g.Name + text.FlagSpecSpacer + g.Type.String()
}

// ExecType is the interface for func's that can be used with [Run],
// [NewCommand], and [Exec].
type ExecType interface {
	func(context.Context, []string) error |
		func(context.Context, []string) |
		func(context.Context) error |
		func(context.Context) |
		func([]string) error |
		func([]string) |
		func() error |
		func()
}

// ExecFunc wraps a exec func.
type ExecFunc func(context.Context, []string) error

// NewExec creates a [ExecFunc] func.
func NewExec[T ExecType](f T) (ExecFunc, error) {
	var v any = f
	switch f := v.(type) {
	case func(context.Context, []string) error:
		return f, nil
	case func(context.Context, []string):
		return func(ctx context.Context, args []string) error {
			f(ctx, args)
			return nil
		}, nil
	case func(context.Context) error:
		return func(ctx context.Context, _ []string) error {
			return f(ctx)
		}, nil
	case func(context.Context):
		return func(ctx context.Context, _ []string) error {
			f(ctx)
			return nil
		}, nil
	case func([]string) error:
		return func(_ context.Context, args []string) error {
			return f(args)
		}, nil
	case func([]string):
		return func(_ context.Context, args []string) error {
			f(args)
			return nil
		}, nil
	case func() error:
		return func(context.Context, []string) error {
			return f()
		}, nil
	case func():
		return func(context.Context, []string) error {
			f()
			return nil
		}, nil
	}
	return nil, fmt.Errorf("%w: invalid exec func %T", ErrInvalidType, f)
}

// buildFlagOpts builds flag options for value from the passed struct tags.
func buildFlagOpts(parent, value reflect.Value, tags []string) ([]Option, error) {
	var set *bool
	typ, mapKey, elem, err := defaultType(value.Elem().Type())
	if err != nil {
		return nil, fmt.Errorf("%s: %w", value.Type().String(), err)
	}
	opts := []Option{typ, MapKey(mapKey), Elem(elem)}
	for _, opt := range tags {
		key, val, _ := strings.Cut(opt, ":")
		switch key {
		case "type":
			opts = append(opts, Type(val))
		case "mapkey":
			opts = append(opts, MapKey(Type(val)))
		case "elem":
			opts = append(opts, Elem(Type(val)))
		case "name":
			opts = append(opts, Name(val))
		case "short":
			opts = append(opts, Short(val))
		case "alias":
			opts = append(opts, Aliases(val))
		case "aliases":
			opts = append(opts, Aliases(strings.Split(val, "|")...))
		case "spec":
			opts = append(opts, Spec(val))
		case "default":
			opts = append(opts, Default(val))
		case "noarg":
			opts = append(opts, NoArg(true, val))
		case "key":
			typ, key, _ := strings.Cut(val, "|")
			if key == "" {
				typ, key = key, typ
			}
			opts = append(opts, Key(typ, key))
		case "hook":
			opts = append(opts, Special("hook:"+val))
		case "section":
			section, err := strconv.Atoi(val)
			if err != nil {
				return nil, fmt.Errorf("%w: section %s: %w", ErrInvalidConversion, val, err)
			}
			opts = append(opts, Section(section))
		case "hidden":
			opts = append(opts, Hidden(true))
		case "deprecated":
			opts = append(opts, Deprecated(true))
		case "set":
			if set, err = setField(parent, val); err != nil {
				return nil, err
			}
		default:
			return nil, fmt.Errorf("%w: %q", ErrUnknownTagOption, key)
		}
	}
	// prepend bind to opt
	return prepend(opts, BindRef(value, set)), nil
}

// setField returns the pointer to the bool for name.
func setField(value reflect.Value, name string) (*bool, error) {
	if r := []rune(name); !unicode.IsUpper(r[0]) {
		return nil, fmt.Errorf("%w: set: %q is not an exported field", ErrInvalidType, name)
	}
	refType := value.Type()
	for i := range refType.NumField() {
		switch f := refType.Field(i); {
		case f.Name != name:
			continue
		case f.Type.Kind() != reflect.Bool:
			return nil, fmt.Errorf("%w: set: field %q is not bool", ErrInvalidType, name)
		}
		return value.Field(i).Addr().Interface().(*bool), nil
	}
	return nil, fmt.Errorf("%w: set: field %q was not found", ErrInvalidType, name)
}

// newCommandError creates a command error.
func newCommandError(name string, err error) error {
	return fmt.Errorf("command %s: %w", name, err)
}

// newFlagError creates a flag error.
func newFlagError(name string, err error) error {
	if utf8.RuneCountInString(name) == 1 {
		return fmt.Errorf("-%s: %w", name, err)
	}
	return fmt.Errorf("--%s: %w", name, err)
}

// split splits a string by the cut rune, skipping escaped runes.
func split(str string, cut rune) []string {
	r := []rune(str)
	v := make([][]rune, 1)
	var c, next rune
	for i, j, n := 0, 0, len(r); i < n; i++ {
		c = r[i]
		switch next = peek(r, i+1, n); {
		case r[i] == '\\' && next != 0:
			i, c = i+1, next
		case r[i] == cut:
			j, v = j+1, append(v, nil)
			continue
		}
		v[j] = append(v[j], c)
	}
	s := make([]string, len(v))
	for i, r := range v {
		s[i] = string(r)
	}
	return s
}

// peek peeks at the next rune in r.
func peek(r []rune, i, n int) rune {
	if i < n {
		return r[i]
	}
	return 0
}

// prev
func prev(s []string, n int) string {
	if n != 0 {
		return s[n-1]
	}
	return ""
}
