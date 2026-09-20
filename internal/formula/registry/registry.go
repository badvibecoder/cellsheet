// Package registry is the function table.
//
// Every formula function registers itself from an init() in the functions
// package. Because all the files there are in one Go package, adding a file
// adds a function with no other edit anywhere — the modularity promise in
// PHASE-1-SPEC.md §14.
package registry

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/badvibecoder/cellsheet/internal/cell"
)

// Ctx is scratch space passed to every function call. It carries no mutable
// state, which is what makes functions safe to run on any goroutine.
type Ctx struct{}

// Spec describes one function.
type Spec struct {
	Name    string
	Summary string

	// MinArgs and MaxArgs bound the argument count. MaxArgs of -1 means
	// variadic.
	MinArgs int
	MaxArgs int

	// Numeric says that a scalar text argument is an error. Functions that
	// handle text themselves (LEN, UPPER) leave it false.
	Numeric bool

	// Volatile marks functions that must be recomputed whenever anything
	// changes, such as NOW or RAND. None exist in v1.
	Volatile bool

	Fn func(*Ctx, []cell.Arg) cell.Value
}

var (
	mu    sync.RWMutex
	funcs = map[string]Spec{}
)

// Register adds a function. It panics on a specification error, because that is
// a programming mistake and should be caught the moment the program starts
// rather than by a user typing a formula.
func Register(s Spec) {
	name := strings.ToUpper(strings.TrimSpace(s.Name))
	if name == "" {
		panic("registry: function name must not be empty")
	}
	if s.Fn == nil {
		panic("registry: function " + name + " has no implementation")
	}
	if s.MinArgs < 0 {
		panic("registry: function " + name + " has a negative MinArgs")
	}
	if s.MaxArgs != -1 && s.MaxArgs < s.MinArgs {
		panic("registry: function " + name + " has MaxArgs below MinArgs")
	}
	if _, dup := funcs[name]; dup {
		panic("registry: function " + name + " is registered twice")
	}
	s.Name = name
	funcs[name] = s
}

// Lookup finds a function by name, case-insensitively.
func Lookup(name string) (Spec, bool) {
	mu.RLock()
	defer mu.RUnlock()
	s, ok := funcs[strings.ToUpper(strings.TrimSpace(name))]
	return s, ok
}

// Names lists every registered function in alphabetical order.
func Names() []string {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]string, 0, len(funcs))
	for n := range funcs {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// All returns a copy of every specification, for help screens and tests.
func All() []Spec {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]Spec, 0, len(funcs))
	for _, s := range funcs {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Count is how many functions are registered.
func Count() int {
	mu.RLock()
	defer mu.RUnlock()
	return len(funcs)
}

// Call validates the arguments, invokes the function, and contains any panic.
//
// Every function therefore gets argument checking, kind checking, error
// propagation and crash isolation without writing a line of it — which is what
// makes one bad module degrade one cell instead of the program.
func Call(name string, ctx *Ctx, args []cell.Arg) cell.Value {
	spec, ok := Lookup(name)
	if !ok {
		return cell.Error(cell.ErrName)
	}
	if len(args) < spec.MinArgs {
		return cell.Error(cell.ErrValue)
	}
	if spec.MaxArgs >= 0 && len(args) > spec.MaxArgs {
		return cell.Error(cell.ErrValue)
	}
	// Errors propagate from any scalar argument.
	for _, a := range args {
		if !a.IsRange() && a.Value().IsError() {
			return a.Value()
		}
	}
	// A scalar text argument to a numeric function is #VALUE!.
	if spec.Numeric {
		for _, a := range args {
			if !a.IsRange() && a.Value().Kind == cell.KindText {
				return cell.Error(cell.ErrValue)
			}
		}
	}
	return safeCall(spec, ctx, args)
}

func safeCall(spec Spec, ctx *Ctx, args []cell.Arg) (result cell.Value) {
	defer func() {
		if r := recover(); r != nil {
			// A broken module costs one cell, never the program.
			result = cell.Error(cell.ErrInternal)
		}
	}()
	result = spec.Fn(ctx, args)
	if result.Kind == cell.KindError && result.Code == cell.ErrNone {
		result = cell.Error(cell.ErrInternal)
	}
	return result
}

// String renders a function's signature for help text.
func (s Spec) String() string {
	arity := ""
	switch {
	case s.MaxArgs == -1:
		arity = fmt.Sprintf("%d or more arguments", s.MinArgs)
	case s.MinArgs == s.MaxArgs:
		arity = fmt.Sprintf("%d arguments", s.MinArgs)
	default:
		arity = fmt.Sprintf("%d to %d arguments", s.MinArgs, s.MaxArgs)
	}
	return fmt.Sprintf("%s(%s)", s.Name, arity)
}
