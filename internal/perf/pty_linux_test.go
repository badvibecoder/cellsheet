//go:build linux

package perf

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// The escape sequences that matter for leaving a terminal as we found it.
const (
	enterAltScreen = "\x1b[?1049h"
	leaveAltScreen = "\x1b[?1049l"
	showCursor     = "\x1b[?25h"
	disablePaste   = "\x1b[?2004l"
)

// openPTY allocates a pseudo-terminal without any third-party dependency. The
// project targets Linux (decision D5), so the Linux ioctls are enough.
func openPTY(t *testing.T) (master *os.File, slaveName string) {
	t.Helper()
	m, err := os.OpenFile("/dev/ptmx", os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		t.Skipf("no pseudo-terminal available: %v", err)
	}
	// Unlock the slave side, then ask for its name.
	if err := unix.IoctlSetPointerInt(int(m.Fd()), unix.TIOCSPTLCK, 0); err != nil {
		m.Close()
		t.Skipf("could not unlock the pty: %v", err)
	}
	n, err := unix.IoctlGetInt(int(m.Fd()), unix.TIOCGPTN)
	if err != nil {
		m.Close()
		t.Skipf("could not name the pty: %v", err)
	}
	return m, fmt.Sprintf("/dev/pts/%d", n)
}

// ptySession is a running program attached to a pseudo-terminal.
type ptySession struct {
	master *os.File
	cmd    *exec.Cmd
	out    bytes.Buffer
	start  time.Time
	first  time.Duration
}

// startPTY launches a command on a fresh terminal of the given size.
func startPTY(t *testing.T, name string, args []string, cols, rows int, env []string) *ptySession {
	t.Helper()
	master, slaveName := openPTY(t)
	slave, err := os.OpenFile(slaveName, os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		master.Close()
		t.Fatalf("opening the pty slave: %v", err)
	}
	defer slave.Close()

	if err := unix.IoctlSetWinsize(int(master.Fd()), unix.TIOCSWINSZ,
		&unix.Winsize{Col: uint16(cols), Row: uint16(rows)}); err != nil {
		master.Close()
		t.Fatalf("setting the window size: %v", err)
	}

	cmd := exec.Command(name, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = slave, slave, slave
	cmd.Env = append(os.Environ(), env...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true, Ctty: 0}

	s := &ptySession{master: master, cmd: cmd}
	if err := cmd.Start(); err != nil {
		master.Close()
		t.Fatalf("starting %s: %v", name, err)
	}
	s.start = time.Now()
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
		master.Close()
	})
	return s
}

// pump reads whatever the program has written, stopping when the deadline
// passes or the program exits.
func (s *ptySession) pump(t *testing.T, d time.Duration) {
	t.Helper()
	deadline := time.Now().Add(d)
	buf := make([]byte, 32*1024)
	for time.Now().Before(deadline) {
		n, err := unix.Read(int(s.master.Fd()), buf)
		if n > 0 {
			if s.first == 0 {
				s.first = time.Since(s.start)
			}
			s.out.Write(buf[:n])
		}
		if err != nil {
			if err == unix.EAGAIN || err == unix.EINTR {
				time.Sleep(5 * time.Millisecond)
				continue
			}
			return
		}
		if n == 0 {
			return
		}
	}
}

func (s *ptySession) send(t *testing.T, keys string) {
	t.Helper()
	if _, err := s.master.WriteString(keys); err != nil {
		t.Fatalf("writing to the pty: %v", err)
	}
}

// wait blocks until the program exits, reporting whether it did so in time.
func (s *ptySession) wait(t *testing.T, d time.Duration) bool {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- s.cmd.Wait() }()
	select {
	case <-done:
		return true
	case <-time.After(d):
		return false
	}
}

func (s *ptySession) output() string { return s.out.String() }

// TestTerminalIsRestoredOnCleanExit is the promise that matters most in daily
// use: when the program quits, the terminal must be usable again. A program
// that leaves the alternate screen or hides the cursor has broken the user's
// shell, and no amount of spreadsheet correctness makes up for it.
func TestTerminalIsRestoredOnCleanExit(t *testing.T) {
	if testing.Short() {
		t.Skip("pty integration tests are skipped in short mode")
	}
	bin := buildBinary(t)

	s := startPTY(t, bin, []string{"-color=none"}, 100, 30, nil)
	s.pump(t, 700*time.Millisecond)

	if !strings.Contains(s.output(), enterAltScreen) {
		t.Fatalf("the program never entered the alternate screen; output was %q",
			truncateForLog(s.output()))
	}
	if s.first == 0 {
		t.Fatal("the program produced no output at all")
	}
	// The §16 cold-start budget.
	if s.first > 100*time.Millisecond {
		t.Errorf("cold start took %v, budget is 100 ms", s.first.Round(time.Microsecond))
	}
	t.Logf("cold start to first frame: %v", s.first.Round(time.Microsecond))

	// Type something, then quit with Ctrl+Q.
	s.send(t, "123\r")
	s.pump(t, 300*time.Millisecond)
	s.send(t, "\x11") // Ctrl+Q
	s.pump(t, 700*time.Millisecond)

	if !s.wait(t, 3*time.Second) {
		t.Fatal("the program did not exit after Ctrl+Q")
	}
	out := s.output()
	for _, seq := range []struct{ name, code string }{
		{"leave the alternate screen", leaveAltScreen},
		{"show the cursor", showCursor},
		{"disable bracketed paste", disablePaste},
	} {
		if !strings.Contains(out, seq.code) {
			t.Errorf("on exit the program did not %s", seq.name)
		}
	}
}

// TestTerminalIsRestoredOnPanic checks the guarantee our top-level recover
// relies on. The model here is deliberately not ours: this asserts that the
// library restores the terminal when a model panics, which is the behaviour
// cellsheet inherits rather than implements.
func TestTerminalIsRestoredOnPanic(t *testing.T) {
	if testing.Short() {
		t.Skip("pty integration tests are skipped in short mode")
	}
	bin := buildHelper(t)

	s := startPTY(t, bin, nil, 80, 24, nil)
	s.pump(t, 900*time.Millisecond)
	_ = s.wait(t, 3*time.Second)

	if s.first == 0 {
		t.Fatal("the helper produced no output")
	}
	if !strings.Contains(s.output(), leaveAltScreen) {
		t.Errorf("the alternate screen was not left after a panic; output was %q",
			truncateForLog(s.output()))
	}
	if !strings.Contains(s.output(), showCursor) {
		t.Errorf("the cursor was not restored after a panic; output was %q",
			truncateForLog(s.output()))
	}
}

func truncateForLog(s string) string {
	s = strings.ReplaceAll(s, "\x1b", "ESC")
	if len(s) > 400 {
		return s[:400] + "..."
	}
	return s
}

var builtOnce struct {
	path string
	err  error
}

// buildBinary compiles the real program once per test run.
func buildBinary(t *testing.T) string {
	t.Helper()
	if builtOnce.path != "" || builtOnce.err != nil {
		if builtOnce.err != nil {
			t.Fatalf("building cellsheet: %v", builtOnce.err)
		}
		return builtOnce.path
	}
	dir, err := os.MkdirTemp("", "cellsheet-bin-")
	if err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(dir, "cellsheet")
	cmd := exec.Command("go", "build", "-o", bin, "./cmd/cellsheet")
	cmd.Dir = repoRoot(t)
	out, err := cmd.CombinedOutput()
	if err != nil {
		builtOnce.err = fmt.Errorf("%v: %s", err, out)
		t.Fatalf("building cellsheet: %v", builtOnce.err)
	}
	builtOnce.path = bin
	return bin
}

// buildHelper compiles a tiny program whose model panics on the first key,
// which is the only way to exercise the panic path without shipping test code
// in the real binary.
func buildHelper(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	src := filepath.Join(dir, "main.go")
	code := `package main

import (
	"os"

	tea "github.com/charmbracelet/bubbletea"
)

type panicky struct{}

func (panicky) Init() tea.Cmd { return nil }
func (panicky) View() string  { return "hello" }
func (p panicky) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	panic("deliberate panic to test terminal restoration")
}

func main() {
	p := tea.NewProgram(panicky{}, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		os.Exit(1)
	}
}
`
	if err := os.WriteFile(src, []byte(code), 0o644); err != nil {
		t.Fatal(err)
	}
	// Give the helper its own module so it does not disturb the project's.
	gomod := "module ptypanic\n\ngo 1.24.0\n\nrequire github.com/charmbracelet/bubbletea v1.3.10\n"
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(gomod), 0o644); err != nil {
		t.Fatal(err)
	}
	root := repoRoot(t)
	// Reuse the project's module cache and vendor tree so the helper builds
	// offline, exactly as the real binary does.
	for _, f := range []string{"go.sum", "vendor"} {
		src := filepath.Join(root, f)
		if _, err := os.Stat(src); err != nil {
			continue
		}
		if f == "go.sum" {
			data, err := os.ReadFile(src)
			if err == nil {
				_ = os.WriteFile(filepath.Join(dir, "go.sum"), data, 0o644)
			}
		}
	}
	bin := filepath.Join(dir, "ptyhelper")
	cmd := exec.Command("go", "build", "-mod=mod", "-o", bin, ".")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOFLAGS=", "GOWORK=off")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Skipf("could not build the panic helper (offline?): %v: %s", err, out)
	}
	return bin
}
