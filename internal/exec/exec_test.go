package exec

import (
	"bytes"
	"context"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestLimitedWriterIoCopyContract guards against returning a short write with
// a nil error: io.Copy turns that into io.ErrShortWrite and closes the pipe,
// which kills the running program.
func TestLimitedWriterIoCopyContract(t *testing.T) {
	var buf bytes.Buffer
	lw := &limitedWriter{w: &buf, remaining: 10}
	n, err := io.Copy(lw, strings.NewReader(strings.Repeat("x", 25)))
	if n != 25 || err != nil {
		t.Errorf("io.Copy = (%d, %v), want (25, nil)", n, err)
	}
	if buf.Len() != 10 || !lw.truncated {
		t.Errorf("buf=%d truncated=%v, want 10 and true", buf.Len(), lw.truncated)
	}
}

func TestLimitedWriterCases(t *testing.T) {
	cases := []struct {
		name      string
		limit     int
		writes    []string
		wantBuf   string
		wantNs    []int
		wantTrunc bool
	}{
		{"under limit", 5, []string{"abc"}, "abc", []int{3}, false},
		{"exact fit", 3, []string{"xyz"}, "xyz", []int{3}, false},
		{"crosses limit", 5, []string{"abc", "defgh"}, "abcde", []int{3, 5}, true},
		{"after exhausted", 5, []string{"abcde", "ijk"}, "abcde", []int{5, 3}, true},
		{"empty write", 0, []string{""}, "", []int{0}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			lw := &limitedWriter{w: &buf, remaining: tc.limit}
			for i, w := range tc.writes {
				n, err := lw.Write([]byte(w))
				if err != nil {
					t.Fatalf("write %d: %v", i, err)
				}
				if n != tc.wantNs[i] {
					t.Errorf("write %d returned %d, want %d", i, n, tc.wantNs[i])
				}
			}
			if buf.String() != tc.wantBuf {
				t.Errorf("buf = %q, want %q", buf.String(), tc.wantBuf)
			}
			if lw.truncated != tc.wantTrunc {
				t.Errorf("truncated = %v, want %v", lw.truncated, tc.wantTrunc)
			}
		})
	}
}

// TestLargeOutputDoesNotKillChild is the end-to-end form of the contract bug:
// a program producing more than the output cap used to die of a broken pipe.
func TestLargeOutputDoesNotKillChild(t *testing.T) {
	if testing.Short() {
		t.Skip("spawns a subprocess")
	}
	r := Run("bash", "for i in $(seq 1 12000); do printf '%0100d\n' $i; done")
	if r.Err != nil {
		t.Fatalf("err = %v, want nil", r.Err)
	}
	if !strings.Contains(r.Output, "(output truncated)") {
		t.Error("expected the truncation marker")
	}
}

func TestUnsupportedLanguageListsSupported(t *testing.T) {
	r := Run("kotlin", "x")
	if r.Err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(r.Err.Error(), "python") {
		t.Errorf("err = %v, want it to list the supported languages", r.Err)
	}
}

func TestEmptyLanguageIsExplained(t *testing.T) {
	r := Run("", "x")
	if r.Err == nil || !strings.Contains(r.Err.Error(), "no language") {
		t.Errorf("err = %v, want an explanation", r.Err)
	}
}

func TestRunWithTimeoutStopsAtTheLimitAndNamesIt(t *testing.T) {
	start := time.Now()
	r := RunWithTimeout(context.Background(), 300*time.Millisecond, "bash", "sleep 30")
	if r.Err == nil || !strings.Contains(r.Err.Error(), "timed out after 300ms") {
		t.Fatalf("err = %v, want a timeout that names 300ms", r.Err)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("took %s, the limit was not applied", elapsed)
	}
}

func TestRunWithTimeoutAllowsMoreThanTheDefault(t *testing.T) {
	// The limit is per call: a generous one must not cut a quick block short.
	r := RunWithTimeout(context.Background(), time.Minute, "bash", "echo ok")
	if r.Err != nil || strings.TrimSpace(r.Output) != "ok" {
		t.Fatalf("output = %q, err = %v", r.Output, r.Err)
	}
}

func TestRunWithTimeoutNonPositiveUsesDefault(t *testing.T) {
	// Zero must not mean "no deadline": context.WithTimeout(0) would cancel
	// at once, and an unbounded run is what the limit exists to prevent.
	r := RunWithTimeout(context.Background(), 0, "bash", "echo ok")
	if r.Err != nil || strings.TrimSpace(r.Output) != "ok" {
		t.Fatalf("output = %q, err = %v", r.Output, r.Err)
	}
}

// Supported must agree with what Run actually dispatches on, or a block could
// be offered for selection and then refused, or the other way round.
func TestSupportedMatchesRun(t *testing.T) {
	for _, lang := range []string{"go", "golang", "python", "py", "python3", "javascript", "js", "node",
		"ruby", "rb", "lua", "elixir", "ex", "bash", "sh", "BASH", "c", "rust", "rs"} {
		if !Supported(lang) {
			t.Errorf("Supported(%q) = false, but Run handles it", lang)
		}
	}
	for _, lang := range []string{"", "text", "yaml", "csv", "mermaid"} {
		if Supported(lang) {
			t.Errorf("Supported(%q) = true", lang)
		}
		if r := Run(lang, "x"); r.Err == nil || !strings.Contains(r.Err.Error(), "supported:") {
			t.Errorf("Run(%q) should refuse the language, got %v", lang, r.Err)
		}
	}
}

// syncWriter records writes made from the program's goroutine.
type syncWriter struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (w *syncWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.Write(p)
}

func (w *syncWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.String()
}

// TestRunStreamingDeliversOutputBeforeExit guards the point of streaming: the
// first line must reach the live writer while the program is still running.
func TestRunStreamingDeliversOutputBeforeExit(t *testing.T) {
	live := &syncWriter{}
	done := make(chan Result, 1)
	go func() {
		done <- RunStreaming(context.Background(), 10*time.Second, "bash", "echo first; sleep 2; echo second", live)
	}()

	deadline := time.After(1500 * time.Millisecond)
	for live.String() != "first\n" {
		select {
		case r := <-done:
			t.Fatalf("program exited before its first line was streamed: %+v, live=%q", r, live.String())
		case <-deadline:
			t.Fatalf("first line was not streamed while the program ran, live=%q", live.String())
		case <-time.After(10 * time.Millisecond):
		}
	}

	r := <-done
	if r.Err != nil {
		t.Fatal(r.Err)
	}
	if want := "first\nsecond\n"; r.Output != want || live.String() != want {
		t.Errorf("output = %q, live = %q, want both %q", r.Output, live.String(), want)
	}
}

// TestRunStreamingRespectsTheOutputLimit keeps a runaway program from growing
// the live copy past what the result is allowed to hold.
func TestRunStreamingRespectsTheOutputLimit(t *testing.T) {
	live := &syncWriter{}
	r := RunStreaming(context.Background(), 10*time.Second, "bash", "head -c 2000000 /dev/zero | tr '\\0' x", live)
	if r.Err != nil {
		t.Fatal(r.Err)
	}
	if got := len(live.String()); got != maxOutputBytes {
		t.Errorf("live copy holds %d bytes, want %d", got, maxOutputBytes)
	}
}

// TestCommandReadingStdinDoesNotEatTheScript guards against passing the block
// on the shell's standard input: a command that reads input would then
// consume the lines after it, and they would silently never run.
func TestCommandReadingStdinDoesNotEatTheScript(t *testing.T) {
	r := Run("bash", "read line\necho \"read got: [$line]\"\necho after")
	if r.Err != nil {
		t.Fatal(r.Err)
	}
	if want := "read got: []\nafter\n"; r.Output != want {
		t.Errorf("output = %q, want %q", r.Output, want)
	}
}
