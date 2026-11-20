// Package window opens a presentation in a terminal window of its own, with
// the font sized so the slides fill the screen.
//
// A terminal cannot scale text: the only way to make a slide bigger is a
// bigger font, and only the terminal emulator can set that. So this asks the
// emulator for a fullscreen window at a known font size, measures how many
// columns that gives, and opens the deck at the font size that gives the
// number of columns asked for.
package window

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const (
	// DefaultColumns is how wide the window is made, in cells, when the
	// caller does not say. Slides are usually written for 80 to 100 columns.
	DefaultColumns = 100

	// MinColumns and MaxColumns bound what a caller may ask for.
	MinColumns = 40
	MaxColumns = 400

	// SizeCommand is the hidden subcommand the probe window runs.
	SizeCommand = "__window-size"

	probeFont    = 20.0
	probeSettle  = 700 * time.Millisecond
	probeTimeout = 8 * time.Second
	minFont      = 6.0
	maxFont      = 96.0
)

// ErrUnsupported is returned when no terminal that can be driven is found.
var ErrUnsupported = errors.New("opening a sized window needs Ghostty on macOS; run lekture in a terminal about 100 columns wide instead")

// FontSize returns the font size that gives want columns, given that a font of
// probe points gave got columns on the same screen. Cell width is proportional
// to font size, so columns are inversely proportional to it.
func FontSize(probe float64, got, want int) float64 {
	if got < 1 || want < 1 {
		return probe
	}
	size := probe * float64(got) / float64(want)
	if size < minFont {
		return minFont
	}
	if size > maxFont {
		return maxFont
	}
	// One decimal is as fine as a terminal resolves a font size.
	return float64(int(size*10+0.5)) / 10
}

// shellQuote quotes s for a POSIX shell.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// ghosttyApp returns the Ghostty application bundle, if it is installed.
func ghosttyApp() (string, bool) {
	if runtime.GOOS != "darwin" {
		return "", false
	}
	candidates := []string{"/Applications/Ghostty.app"}
	if home, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates, filepath.Join(home, "Applications", "Ghostty.app"))
	}
	for _, c := range candidates {
		if st, err := os.Stat(c); err == nil && st.IsDir() {
			return c, true
		}
	}
	return "", false
}

// ghosttyArgs are the options shared by the probe and the real window: a
// separate instance that restores nothing, covers the screen at once, and
// quits when its command ends.
func ghosttyArgs(fontSize float64, command string) []string {
	return []string{
		"--window-save-state=never",
		"--confirm-close-surface=false",
		"--quit-after-last-window-closed=true",
		"--fullscreen=true",
		"--macos-non-native-fullscreen=true",
		"--font-size=" + strconv.FormatFloat(fontSize, 'f', 1, 64),
		"--command=" + command,
	}
}

func launchGhostty(app string, fontSize float64, command string) error {
	args := append([]string{"-na", app, "--args"}, ghosttyArgs(fontSize, command)...)
	out, err := exec.Command("open", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("opening Ghostty: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// parseSize reads the "rows columns" line the probe writes.
func parseSize(s string) (rows, cols int, err error) {
	fields := strings.Fields(s)
	if len(fields) != 2 {
		return 0, 0, fmt.Errorf("unexpected size %q", strings.TrimSpace(s))
	}
	if rows, err = strconv.Atoi(fields[0]); err != nil {
		return 0, 0, err
	}
	if cols, err = strconv.Atoi(fields[1]); err != nil {
		return 0, 0, err
	}
	if rows < 1 || cols < 1 {
		return 0, 0, fmt.Errorf("unexpected size %q", strings.TrimSpace(s))
	}
	return rows, cols, nil
}

// measure opens a window at the probe font size and returns its size in cells.
func measure(app, self string) (rows, cols int, err error) {
	f, err := os.CreateTemp("", "lekture-window-*")
	if err != nil {
		return 0, 0, err
	}
	path := f.Name()
	f.Close()
	defer os.Remove(path)

	command := shellQuote(self) + " " + SizeCommand + " " + shellQuote(path)
	if err := launchGhostty(app, probeFont, command); err != nil {
		return 0, 0, err
	}

	deadline := time.Now().Add(probeTimeout)
	for time.Now().Before(deadline) {
		if data, err := os.ReadFile(path); err == nil && len(data) > 0 {
			return parseSize(string(data))
		}
		time.Sleep(50 * time.Millisecond)
	}
	return 0, 0, errors.New("could not measure the screen: the terminal window did not report its size")
}

// Open shows deck in a fullscreen window that is about columns cells wide.
// self is the lekture binary to run in it, and dir the directory to run it in.
// It returns once the window has been asked for; the presentation then runs
// in that window, not in the caller's terminal.
func Open(self, dir, deck string, columns int) error {
	if columns < MinColumns || columns > MaxColumns {
		return fmt.Errorf("invalid --columns %d (must be %d-%d)", columns, MinColumns, MaxColumns)
	}
	app, ok := ghosttyApp()
	if !ok {
		return ErrUnsupported
	}

	_, cols, err := measure(app, self)
	if err != nil {
		return err
	}

	// A login shell, so code blocks run from the slides find the same
	// commands the presenter's own terminal does.
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}
	run := "cd " + shellQuote(dir) + " && exec " + shellQuote(self) + " " + shellQuote(deck)
	command := shellQuote(shell) + " -lic " + shellQuote(run)
	return launchGhostty(app, FontSize(probeFont, cols, columns), command)
}

// WriteSize waits for the window to settle, then writes the terminal size of
// fd to path as "rows columns". It is what SizeCommand runs.
func WriteSize(path string, size func() (cols, rows int, err error)) error {
	time.Sleep(probeSettle)
	cols, rows, err := size()
	if err != nil {
		return err
	}
	return os.WriteFile(path, []byte(fmt.Sprintf("%d %d\n", rows, cols)), 0o600)
}
