package registry

import (
	"testing"

	"github.com/badvibecoder/cellsheet/internal/cell"
)

func TestRegisterRejectsBadSpecs(t *testing.T) {
	cases := []struct {
		name string
		spec Spec
	}{
		{"empty name", Spec{Fn: func(*Ctx, []cell.Arg) cell.Value { return cell.Empty() }}},
		{"nil implementation", Spec{Name: "TESTNILFN"}},
		{"negative MinArgs", Spec{Name: "TESTNEG", MinArgs: -1, Fn: noop}},
		{"MaxArgs below MinArgs", Spec{Name: "TESTARITY", MinArgs: 3, MaxArgs: 1, Fn: noop}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Errorf("Register(%s) should panic", c.name)
				}
			}()
			Register(c.spec)
		})
	}
}

func TestRegisterRejectsDuplicates(t *testing.T) {
	Register(Spec{Name: "TESTDUP", MinArgs: 0, MaxArgs: -1, Fn: noop})
	defer func() {
		if recover() == nil {
			t.Error("registering the same name twice should panic")
		}
	}()
	Register(Spec{Name: "testdup", MinArgs: 0, MaxArgs: -1, Fn: noop})
}

func TestLookupIsCaseInsensitive(t *testing.T) {
	Register(Spec{Name: "TESTCASE", MinArgs: 0, MaxArgs: -1, Fn: noop})
	for _, n := range []string{"TESTCASE", "testcase", "  TestCase  "} {
		if _, ok := Lookup(n); !ok {
			t.Errorf("Lookup(%q) failed", n)
		}
	}
	if _, ok := Lookup("NOSUCHFUNCTION"); ok {
		t.Error("Lookup should fail for an unknown name")
	}
}

func TestArgumentCountIsEnforced(t *testing.T) {
	Register(Spec{Name: "TESTARITY2", MinArgs: 2, MaxArgs: 3, Fn: first})
	if got := Call("TESTARITY2", &Ctx{}, nil); got.Code != cell.ErrValue {
		t.Errorf("too few arguments should be #VALUE!, got %s", got.Display())
	}
	args := []cell.Arg{cell.Single(cell.Number(cell.FromInt64(1)))}
	if got := Call("TESTARITY2", &Ctx{}, args); got.Code != cell.ErrValue {
		t.Errorf("one argument should be #VALUE!, got %s", got.Display())
	}
	if got := Call("TESTARITY2", &Ctx{}, append(args, args...)); got.Display() != "1" {
		t.Errorf("two arguments should work, got %s", got.Display())
	}
	if got := Call("TESTARITY2", &Ctx{}, append(args, args...)); got.IsError() {
		t.Errorf("variadic upper bound not respected: %s", got.Display())
	}
}

// TestPanicIsContained is the guarantee that lets functions be drop-in files:
// a broken one costs a single cell, never the program.
func TestPanicIsContained(t *testing.T) {
	Register(Spec{
		Name: "TESTPANIC", MinArgs: 0, MaxArgs: -1,
		Fn: func(*Ctx, []cell.Arg) cell.Value { panic("deliberate") },
	})
	got := Call("TESTPANIC", &Ctx{}, nil)
	if got.Code != cell.ErrInternal {
		t.Errorf("a panicking function should yield #ERR!, got %s", got.Display())
	}
	// The registry must still be usable afterwards.
	if _, ok := Lookup("TESTPANIC"); !ok {
		t.Error("the registry should be intact after a contained panic")
	}
}

func TestUnknownFunctionIsNameError(t *testing.T) {
	if got := Call("NOSUCHFUNCTION", &Ctx{}, nil); got.Code != cell.ErrName {
		t.Errorf("unknown function should be #NAME?, got %s", got.Display())
	}
}

func TestScalarTextIsRejectedForNumericFunctions(t *testing.T) {
	Register(Spec{
		Name: "TESTNUMERIC", MinArgs: 1, MaxArgs: -1, Numeric: true,
		Fn: func(_ *Ctx, args []cell.Arg) cell.Value { return args[0].Value() },
	})
	Register(Spec{
		Name: "TESTTEXTY", MinArgs: 1, MaxArgs: -1, Numeric: false,
		Fn: func(_ *Ctx, args []cell.Arg) cell.Value { return args[0].Value() },
	})
	txt := cell.Single(cell.Text("hello"))

	if got := Call("TESTNUMERIC", &Ctx{}, []cell.Arg{txt}); got.Code != cell.ErrValue {
		t.Errorf("a numeric function must reject scalar text, got %s", got.Display())
	}
	// The same text inside a range is the function's business, not the
	// registry's: =SUM(A1:A3) must tolerate a text header.
	if got := Call("TESTNUMERIC", &Ctx{}, []cell.Arg{cell.Range([]cell.Value{cell.Text("x")})}); got.IsError() {
		t.Errorf("a range containing text must reach the function, got %s", got.Display())
	}
	if got := Call("TESTTEXTY", &Ctx{}, []cell.Arg{txt}); got.Display() != "hello" {
		t.Errorf("a text-handling function should receive text, got %s", got.Display())
	}
}

func TestErrorsPropagateBeforeTheFunctionRuns(t *testing.T) {
	ran := false
	Register(Spec{
		Name: "TESTPROP", MinArgs: 1, MaxArgs: -1,
		Fn: func(*Ctx, []cell.Arg) cell.Value { ran = true; return cell.ZeroNumber() },
	})
	got := Call("TESTPROP", &Ctx{}, []cell.Arg{cell.Single(cell.Error(cell.ErrDivZero))})
	if got.Code != cell.ErrDivZero {
		t.Errorf("the error should propagate, got %s", got.Display())
	}
	if ran {
		t.Error("the function should not have been called")
	}
}

func first(_ *Ctx, args []cell.Arg) cell.Value { return args[0].Value() }

func noop(*Ctx, []cell.Arg) cell.Value { return cell.Empty() }
