package calc

import (
	"sort"

	"github.com/badvibecoder/cellsheet/internal/grid"
)

// Graph is the dependency graph for one workbook in v1: a formula cell records
// what it reads, and each referenced cell records which formulas read it.
//
// v1 is deliberately single-sheet (decision D12). The key type is already
// (sheetID, ref) so that enabling cross-sheet references later is a change of
// resolution, not a rewrite — see PHASE-1-SPEC.md §18.3.
type Graph struct {
	// nodes holds formula cells only.
	nodes map[uint32]map[grid.Ref]*node

	// users maps a referenced cell to the formula cells that read it.
	users map[uint32]map[grid.Ref]map[grid.Ref]struct{}

	// broad holds formulas whose precedents were too numerous to index
	// individually. They are recomputed on every pass, like a volatile
	// function, because we cannot know cheaply whether they are affected.
	broad map[uint32]map[grid.Ref]struct{}
}

type node struct {
	src  string
	prec []grid.Ref
}

// NewGraph creates an empty graph.
func NewGraph() *Graph {
	return &Graph{
		nodes: make(map[uint32]map[grid.Ref]*node),
		users: make(map[uint32]map[grid.Ref]map[grid.Ref]struct{}),
		broad: make(map[uint32]map[grid.Ref]struct{}),
	}
}

// Reset forgets everything. It is used when the workbook's contents have
// changed wholesale, so that a formula which no longer exists cannot linger and
// be recalculated from a stale source.
func (g *Graph) Reset() {
	g.nodes = make(map[uint32]map[grid.Ref]*node)
	g.users = make(map[uint32]map[grid.Ref]map[grid.Ref]struct{})
	g.broad = make(map[uint32]map[grid.Ref]struct{})
}

// FormulaCount is how many formula cells are registered for a sheet.
func (g *Graph) FormulaCount(sheetID uint32) int { return len(g.nodes[sheetID]) }

// IsFormula reports whether a cell holds a registered formula.
func (g *Graph) IsFormula(sheetID uint32, ref grid.Ref) bool {
	_, ok := g.nodes[sheetID][ref]
	return ok
}

// Source returns a formula's text.
func (g *Graph) Source(sheetID uint32, ref grid.Ref) (string, bool) {
	n, ok := g.nodes[sheetID][ref]
	if !ok {
		return "", false
	}
	return n.src, true
}

// SetFormula registers or replaces a formula cell and its precedents.
func (g *Graph) SetFormula(sheetID uint32, ref grid.Ref, src string, prec []grid.Ref, broad bool) {
	g.ClearFormula(sheetID, ref)

	m := g.nodes[sheetID]
	if m == nil {
		m = make(map[grid.Ref]*node)
		g.nodes[sheetID] = m
	}
	n := &node{src: src, prec: dedupeRefs(prec)}
	m[ref] = n

	um := g.users[sheetID]
	if um == nil {
		um = make(map[grid.Ref]map[grid.Ref]struct{})
		g.users[sheetID] = um
	}
	for _, p := range n.prec {
		set := um[p]
		if set == nil {
			set = make(map[grid.Ref]struct{})
			um[p] = set
		}
		set[ref] = struct{}{}
	}
	if broad {
		bm := g.broad[sheetID]
		if bm == nil {
			bm = make(map[grid.Ref]struct{})
			g.broad[sheetID] = bm
		}
		bm[ref] = struct{}{}
	}
}

// ClearFormula removes a formula and its reverse edges.
func (g *Graph) ClearFormula(sheetID uint32, ref grid.Ref) {
	n, ok := g.nodes[sheetID][ref]
	if !ok {
		return
	}
	if um := g.users[sheetID]; um != nil {
		for _, p := range n.prec {
			if set := um[p]; set != nil {
				delete(set, ref)
				if len(set) == 0 {
					delete(um, p)
				}
			}
		}
	}
	delete(g.nodes[sheetID], ref)
	if bm := g.broad[sheetID]; bm != nil {
		delete(bm, ref)
	}
}

// Dependents returns every formula cell that must be recomputed because one of
// start changed, transitively, plus any "broad" formula. The result is sorted so
// that behaviour is reproducible.
func (g *Graph) Dependents(sheetID uint32, start []grid.Ref) []grid.Ref {
	seen := make(map[grid.Ref]struct{})
	var queue []grid.Ref

	push := func(r grid.Ref) {
		if _, ok := seen[r]; ok {
			return
		}
		seen[r] = struct{}{}
		queue = append(queue, r)
	}
	um := g.users[sheetID]
	for _, s := range start {
		for u := range um[s] {
			push(u)
		}
	}
	for i := 0; i < len(queue); i++ {
		for u := range um[queue[i]] {
			push(u)
		}
	}
	for r := range g.broad[sheetID] {
		push(r)
	}
	out := make([]grid.Ref, 0, len(seen))
	for r := range seen {
		out = append(out, r)
	}
	sortRefs(out)
	return out
}

// AllFormulas lists every registered formula for a sheet, sorted.
func (g *Graph) AllFormulas(sheetID uint32) []grid.Ref {
	m := g.nodes[sheetID]
	out := make([]grid.Ref, 0, len(m))
	for r := range m {
		out = append(out, r)
	}
	sortRefs(out)
	return out
}

// Waves sorts a dirty set into levels. Cells inside a level cannot affect one
// another, so they may be evaluated concurrently; levels must run in order.
//
// Anything left over after the levels are drained is part of a cycle, and is
// returned separately so the caller can mark it #CIRC! instead of hanging.
func (g *Graph) Waves(sheetID uint32, dirty []grid.Ref) (waves [][]grid.Ref, cyclic []grid.Ref) {
	dirty = dedupeRefs(dirty)
	inDirty := make(map[grid.Ref]struct{}, len(dirty))
	for _, r := range dirty {
		inDirty[r] = struct{}{}
	}

	indeg := make(map[grid.Ref]int, len(dirty))
	for _, r := range dirty {
		indeg[r] = 0
		n := g.nodes[sheetID][r]
		if n == nil {
			continue
		}
		for _, p := range n.prec {
			if _, ok := inDirty[p]; ok {
				indeg[r]++
			}
		}
	}

	ready := make([]grid.Ref, 0, len(dirty))
	for r, d := range indeg {
		if d == 0 {
			ready = append(ready, r)
		}
	}
	sortRefs(ready)

	remaining := len(dirty)
	um := g.users[sheetID]
	for len(ready) > 0 {
		waves = append(waves, ready)
		remaining -= len(ready)

		next := make([]grid.Ref, 0, len(ready))
		for _, r := range ready {
			for u := range um[r] {
				if _, ok := inDirty[u]; !ok {
					continue
				}
				indeg[u]--
				if indeg[u] == 0 {
					next = append(next, u)
				}
			}
		}
		sortRefs(next)
		ready = next
	}
	if remaining > 0 {
		cyclic = make([]grid.Ref, 0, remaining)
		for r, d := range indeg {
			if d > 0 {
				cyclic = append(cyclic, r)
			}
		}
		sortRefs(cyclic)
	}
	return waves, cyclic
}

func dedupeRefs(in []grid.Ref) []grid.Ref {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[grid.Ref]struct{}, len(in))
	out := make([]grid.Ref, 0, len(in))
	for _, r := range in {
		if _, ok := seen[r]; ok {
			continue
		}
		seen[r] = struct{}{}
		out = append(out, r)
	}
	sortRefs(out)
	return out
}

func sortRefs(rs []grid.Ref) {
	sort.Slice(rs, func(i, j int) bool {
		if rs[i].Row != rs[j].Row {
			return rs[i].Row < rs[j].Row
		}
		return rs[i].Col < rs[j].Col
	})
}
