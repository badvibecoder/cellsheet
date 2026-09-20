package functions_test

import (
	"sync"
	"testing"

	"github.com/badvibecoder/cellsheet/internal/cell"
	_ "github.com/badvibecoder/cellsheet/internal/formula/functions"
	"github.com/badvibecoder/cellsheet/internal/formula/registry"
)

// TestFunctionLibraryIsPure runs every shipped function concurrently, many
// times over. Under -race this is the check that no function holds mutable
// state, which is the contract that makes the calculation engine safe.
//
// It lives here rather than in the registry package so that it only ever sees
// real functions, never test scaffolding.
func TestFunctionLibraryIsPure(t *testing.T) {

	specs := registry.All()
	if len(specs) == 0 {
		t.Fatal("no functions registered")
	}

	num := cell.Single(cell.Number(cell.FromInt64(3)))
	cur := cell.Single(cell.Currency(cell.NewDec(1250, 2)))
	rng := cell.Range([]cell.Value{
		cell.Number(cell.FromInt64(1)),
		cell.Text("skip me"),
		cell.Empty(),
		cell.Number(cell.FromInt64(2)),
		cell.Currency(cell.NewDec(300, 2)),
	})
	errArg := cell.Single(cell.Error(cell.ErrDivZero))

	inputs := [][]cell.Arg{{num}, {cur}, {rng}, {num, rng}, {errArg}}

	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		for _, s := range specs {
			for _, args := range inputs {
				wg.Add(1)
				go func(name string, args []cell.Arg) {
					defer wg.Done()
					got := registry.Call(name, &registry.Ctx{}, args)
					// A function must always return something well-formed.
					if got.Kind == cell.KindError && got.Code == cell.ErrNone {
						t.Errorf("%s returned a malformed error value", name)
					}
				}(s.Name, args)
			}
		}
	}
	wg.Wait()
}

func TestSumIsRegistered(t *testing.T) {
	if _, ok := registry.Lookup("SUM"); !ok {
		t.Fatal("=SUM() must be registered")
	}
}

func TestSumBehaviour(t *testing.T) {
	cases := []struct {
		name string
		args []cell.Arg
		want string
	}{
		{
			name: "currency range",
			args: []cell.Arg{cell.Range([]cell.Value{
				cell.Currency(cell.NewDec(5000000, 2)),
				cell.Currency(cell.NewDec(125000, 2)),
			})},
			want: "$51,250.00",
		},
		{
			name: "text and blanks inside a range are skipped",
			args: []cell.Arg{cell.Range([]cell.Value{
				cell.Number(cell.FromInt64(1)),
				cell.Text("header"),
				cell.Empty(),
				cell.Number(cell.FromInt64(2)),
			})},
			want: "3",
		},
		{
			name: "mixed currency promotes",
			args: []cell.Arg{cell.Range([]cell.Value{
				cell.Currency(cell.NewDec(1000, 2)),
				cell.Number(cell.FromInt64(5)),
			})},
			want: "$15.00",
		},
		{
			name: "empty range sums to zero",
			args: []cell.Arg{cell.Range([]cell.Value{cell.Empty(), cell.Text("x")})},
			want: "0",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := registry.Call("SUM", &registry.Ctx{}, c.args)
			if got.Display() != c.want {
				t.Errorf("SUM = %q, want %q", got.Display(), c.want)
			}
		})
	}
}
