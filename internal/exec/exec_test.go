package exec

import (
	"bytes"
	"context"
	"io"
	"strings"
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
