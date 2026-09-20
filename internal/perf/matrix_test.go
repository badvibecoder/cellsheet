package perf

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// repoRoot walks up from the test's working directory to the module root.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find the module root")
		}
		dir = parent
	}
}

// TestBuildMatrix compiles the program for every platform it claims to support.
//
// It is opt-in because six cross-compilations take a while, and a test suite
// that is slow enough gets skipped. Run it with:
//
//	CELLSHEETS_MATRIX=1 go test ./internal/perf/
//
// Only Linux is supported in v1 (decision D5); the others are kept compiling on
// purpose, because keeping the code portable is nearly free now and porting it
// later is not.
func TestBuildMatrix(t *testing.T) {
	if os.Getenv("CELLSHEETS_MATRIX") == "" {
		t.Skip("set CELLSHEETS_MATRIX=1 to cross-compile the build matrix")
	}
	root := repoRoot(t)
	for _, goos := range []string{"linux", "darwin", "windows"} {
		for _, goarch := range []string{"amd64", "arm64"} {
			t.Run(goos+"_"+goarch, func(t *testing.T) {
				cmd := exec.Command("go", "build", "-o", os.DevNull, "./...")
				cmd.Dir = root
				cmd.Env = append(os.Environ(),
					"GOOS="+goos, "GOARCH="+goarch, "CGO_ENABLED=0", "GOFLAGS=-mod=vendor")
				if out, err := cmd.CombinedOutput(); err != nil {
					t.Errorf("building for %s/%s failed: %v\n%s", goos, goarch, err, out)
				}
			})
		}
	}
}

// TestTestSuiteCompilesEverywhere checks that the tests, not just the program,
// build on the other platforms. A test file that only compiles on Linux would
// silently reduce coverage for anyone who tries another target.
func TestTestSuiteCompilesEverywhere(t *testing.T) {
	if os.Getenv("CELLSHEETS_MATRIX") == "" {
		t.Skip("set CELLSHEETS_MATRIX=1 to check the matrix")
	}
	root := repoRoot(t)
	for _, goos := range []string{"darwin", "windows"} {
		t.Run(goos, func(t *testing.T) {
			cmd := exec.Command("go", "vet", "./...")
			cmd.Dir = root
			cmd.Env = append(os.Environ(), "GOOS="+goos, "GOARCH=amd64", "GOFLAGS=-mod=vendor")
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Errorf("the tests do not compile for %s: %v\n%s", goos, err, out)
			}
		})
	}
}
