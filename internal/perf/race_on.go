//go:build race

package perf

// raceEnabled reports whether the race detector is instrumenting this binary.
// It is set by a build tag because there is no runtime API for it.
const raceEnabled = true
