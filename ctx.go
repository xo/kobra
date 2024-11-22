package ox

import (
	"cmp"
	"context"
	"encoding/base64"
	"encoding/hex"
	"io"
	"math/big"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"time"
)

// contextKey is the context key types.
type contextKey int

// context keys.
const (
	rootKey contextKey = iota
	cmdKey
	stdoutKey
	stderrKey
	varsKey
)

// WithRoot sets the root command on the context.
func WithRoot(parent context.Context, root *Command) context.Context {
	return context.WithValue(parent, rootKey, root)
}

// WithCmd sets the active command on the context.
func WithCmd(parent context.Context, cmd *Command) context.Context {
	return context.WithValue(parent, cmdKey, cmd)
}

// WithStdout sets the standard out on the context.
func WithStdout(parent context.Context, stdout io.Writer) context.Context {
	return context.WithValue(parent, stdoutKey, stdout)
}

// WithStderr sets the standard error output on the context.
func WithStderr(parent context.Context, stderr io.Writer) context.Context {
	return context.WithValue(parent, stderrKey, stderr)
}

// WithVars sets the variables on the context.
func WithVars(parent context.Context, vars Vars) context.Context {
	return context.WithValue(parent, varsKey, vars)
}

// Root returns the root command from the context.
func Root(ctx context.Context) *Command {
	root, _ := ctx.Value(rootKey).(*Command)
	return root
}

// RootName returns the root command name from the context.
func RootName(ctx context.Context) string {
	if root := Root(ctx); root != nil {
		return root.Name()
	}
	return filepath.Base(os.Args[0])
}

// Cmd returns the active command from the context.
func Cmd(ctx context.Context) *Command {
	cmd, _ := ctx.Value(cmdKey).(*Command)
	return cmd
}

// Stdout returns the standard output from the context.
func Stdout(ctx context.Context) io.Writer {
	if w, _ := ctx.Value(stdoutKey).(io.Writer); w != nil {
		return w
	}
	return os.Stdout
}

// Stderr returns the standard error output from the context.
func Stderr(ctx context.Context) io.Writer {
	if w, _ := ctx.Value(stderrKey).(io.Writer); w != nil {
		return w
	}
	return os.Stderr
}

// VarsOK returns all variables from the context.
func VarsOK(ctx context.Context) (Vars, bool) {
	vars, ok := ctx.Value(varsKey).(Vars)
	return vars, ok
}

// AnyOK returns a variable, its set status, and if it was defined from the
// context.
func AnyOK(ctx context.Context, name string) (Value, bool) {
	if vars, ok := VarsOK(ctx); ok {
		if val, ok := vars[name]; ok {
			return val, true
		}
	}
	return nil, false
}

// GetOK returns a variable.
func GetOK[T any](ctx context.Context, name string) (T, bool) {
	if val, ok := AnyOK(ctx, name); ok {
		if v, err := As[T](val); err == nil {
			return v, true
		}
	}
	var v T
	return v, false
}

// Get returns the variable from the context.
func Get[T any](ctx context.Context, name string) T {
	v, _ := GetOK[T](ctx, name)
	return v
}

// Slice returns the slice variable from the context.
func Slice[T any](ctx context.Context, name string) []T {
	if val, ok := AnyOK(ctx, name); ok {
		if v, err := SliceAs[T](val); err == nil {
			return v
		}
	}
	return nil
}

// Map returns the map variable from the context.
func Map[K cmp.Ordered, T any](ctx context.Context, name string) map[K]T {
	if val, ok := AnyOK(ctx, name); ok {
		if m, err := MapAs[K, T](val); err == nil {
			return m
		}
	}
	return make(map[K]T)
}

// All returns all variables from the context.
func All[K cmp.Ordered, T any](ctx context.Context) map[K]T {
	if vars, ok := VarsOK(ctx); ok {
		m := make(map[K]T)
		for k, vs := range vars {
			/*
				if v, err := As[T](vs.Var); err == nil {
					m[k] = v
				}
			*/
			k, vs = k, vs
		}
		return m
	}
	return make(map[K]T)
}

// Bytes returns a variable as []byte from the context.
func Bytes(ctx context.Context, name string) []byte {
	return Get[[]byte](ctx, name)
}

// String returns the string variable from the context.
func String(ctx context.Context, name string) string {
	return Get[string](ctx, name)
}

// Runes returns a variable as []rune from the context.
func Runes(ctx context.Context, name string) []rune {
	return Get[[]rune](ctx, name)
}

// Base64 returns the base64 encoded string variable from the context.
func Base64(ctx context.Context, name string) string {
	return base64.StdEncoding.EncodeToString(Get[[]byte](ctx, name))
}

// Hex returns the hex encoded string variable from the context.
func Hex(ctx context.Context, name string) string {
	return hex.EncodeToString(Get[[]byte](ctx, name))
}

// Bool returns the bool variable from the context.
func Bool(ctx context.Context, name string) bool {
	return Get[bool](ctx, name)
}

// Byte returns the byte variable from the context.
func Byte(ctx context.Context, name string) byte {
	return Get[byte](ctx, name)
}

// Rune returns the rune variable from the context.
func Rune(ctx context.Context, name string) rune {
	return Get[rune](ctx, name)
}

// Int64 returns the int64 variable from the context.
func Int64(ctx context.Context, name string) int64 {
	return Get[int64](ctx, name)
}

// Int32 returns the int32 variable from the context.
func Int32(ctx context.Context, name string) int32 {
	return Get[int32](ctx, name)
}

// Int16 returns the int16 variable from the context.
func Int16(ctx context.Context, name string) int16 {
	return Get[int16](ctx, name)
}

// Int returns the int variable from the context.
func Int(ctx context.Context, name string) int {
	return Get[int](ctx, name)
}

// Uint64 returns the uint64 variable from the context.
func Uint64(ctx context.Context, name string) uint64 {
	return Get[uint64](ctx, name)
}

// Uint32 returns the uint32 variable from the context.
func Uint32(ctx context.Context, name string) uint32 {
	return Get[uint32](ctx, name)
}

// Uint16 returns the uint16 variable from the context.
func Uint16(ctx context.Context, name string) uint16 {
	return Get[uint16](ctx, name)
}

// Uint8 returns the uint8 variable from the context.
func Uint8(ctx context.Context, name string) uint8 {
	return Get[uint8](ctx, name)
}

// Uint returns the uint variable from the context.
func Uint(ctx context.Context, name string) uint {
	return Get[uint](ctx, name)
}

// Float64 returns the float64 variable from the context.
func Float64(ctx context.Context, name string) float64 {
	return Get[float64](ctx, name)
}

// Float32 returns the float32 variable from the context.
func Float32(ctx context.Context, name string) float32 {
	return Get[float32](ctx, name)
}

// Complex128 returns the complex128 variable from the context.
func Complex128(ctx context.Context, name string) complex128 {
	return Get[complex128](ctx, name)
}

// Complex64 returns the complex64 variable from the context.
func Complex64(ctx context.Context, name string) complex64 {
	return Get[complex64](ctx, name)
}

// BigInt returns the [big.Int] variable from the context.
func BigInt(ctx context.Context, name string) *big.Int {
	return Get[*big.Int](ctx, name)
}

// BigFloat returns the [big.Float] variable from the context.
func BigFloat(ctx context.Context, name string) *big.Float {
	return Get[*big.Float](ctx, name)
}

// BigRat returns the [big.Rat] variable from the context.
func BigRat(ctx context.Context, name string) *big.Rat {
	return Get[*big.Rat](ctx, name)
}

// Time returns the [time.Time] variable from the context.
func Time(ctx context.Context, name string) time.Time {
	return Get[time.Time](ctx, name)
}

// Duration returns the [time.Duration] variable from the context.
func Duration(ctx context.Context, name string) time.Duration {
	return Get[time.Duration](ctx, name)
}

// URL returns the [url.URL] variable from the context.
func URL(ctx context.Context, name string) *url.URL {
	return Get[*url.URL](ctx, name)
}

// Addr returns the [netip.Addr] variable from the context.
func Addr(ctx context.Context, name string) *netip.Addr {
	return Get[*netip.Addr](ctx, name)
}

// AddrPort returns the [netip.AddrPort] variable from the context.
func AddrPort(ctx context.Context, name string) *netip.AddrPort {
	return Get[*netip.AddrPort](ctx, name)
}

// CIDR returns the [netip.Prefix] variable from the context.
func CIDR(ctx context.Context, name string) *netip.Prefix {
	return Get[*netip.Prefix](ctx, name)
}

// Path returns the path variable from the context.
func Path(ctx context.Context, name string) string {
	return Get[string](ctx, name)
}

// Count returns the count variable from the context.
func Count(ctx context.Context, name string) int {
	return Get[int](ctx, name)
}
