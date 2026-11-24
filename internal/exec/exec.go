package exec

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Result holds the output of a code execution.
type Result struct {
	Output string
	Err    error
}

// DefaultTimeout caps how long a code block may run unless the caller asks
// for something else.
const DefaultTimeout = 10 * time.Second

// errTimedOut marks a run that was stopped by its deadline. RunWithTimeout
// turns it into a message that names the limit that applied.
var errTimedOut = errors.New("execution timed out")

// Languages lists the supported language identifiers, for error messages.
var Languages = []string{
	"bash", "c", "elixir", "go", "javascript", "lua", "python", "ruby", "rust",
}

// Run executes a code block in the given language and returns its combined
// stdout/stderr output. Execution is capped at DefaultTimeout.
func Run(language, code string) Result {
	return RunContext(context.Background(), language, code)
}

// RunContext is Run with a caller-supplied context, so an in-flight execution
// can be cancelled (e.g. when the user dismisses the output panel).
func RunContext(parent context.Context, language, code string) Result {
	return RunWithTimeout(parent, DefaultTimeout, language, code)
}

// RunWithTimeout is RunContext with a caller-supplied limit. A timeout that is
// zero or negative falls back to DefaultTimeout rather than running unbounded.
func RunWithTimeout(parent context.Context, timeout time.Duration, language, code string) Result {
	return RunStreaming(parent, timeout, language, code, nil)
}

// RunStreaming is RunWithTimeout that also copies the program's output to
// live as it is written, so a caller can show progress before the program
// exits. live is written to from another goroutine and receives the same
// bytes the Result ends up holding, up to the same size limit. A nil live
// gives RunWithTimeout.
func RunStreaming(parent context.Context, timeout time.Duration, language, code string, live io.Writer) Result {
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	r := run(parent, timeout, language, code, live)
	if errors.Is(r.Err, errTimedOut) {
		r.Err = fmt.Errorf("execution timed out after %s", timeout)
	}
	return r
}

// Supported reports whether a code block in the given language can be run.
// It must list exactly the languages run dispatches on.
func Supported(language string) bool {
	switch strings.ToLower(language) {
	case "go", "golang", "python", "py", "python3", "javascript", "js", "node",
		"ruby", "rb", "lua", "elixir", "ex", "bash", "sh", "c", "rust", "rs":
		return true
	}
	return false
}

func run(parent context.Context, timeout time.Duration, language, code string, live io.Writer) Result {
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()

	switch strings.ToLower(language) {
	case "go", "golang":
		return runFile(ctx, live, code, "main.go", "go", "run")
	case "python", "py", "python3":
		return runFile(ctx, live, code, "main.py", "python3")
	case "javascript", "js", "node":
		return runFile(ctx, live, code, "main.js", "node")
	case "ruby", "rb":
		return runFile(ctx, live, code, "main.rb", "ruby")
	case "lua":
		return runFile(ctx, live, code, "main.lua", "lua")
	case "elixir", "ex":
		return runFile(ctx, live, code, "main.exs", "elixir")
	case "bash", "sh":
		return runFile(ctx, live, code, "main.sh", "bash")
	case "c":
		return runC(ctx, live, code)
	case "rust", "rs":
		return runRust(ctx, live, code)
	default:
		if language == "" {
			return Result{Err: fmt.Errorf("code block has no language (supported: %s)", strings.Join(Languages, ", "))}
		}
		return Result{Err: fmt.Errorf("unsupported language %q (supported: %s)", language, strings.Join(Languages, ", "))}
	}
}

// runFile writes code to a temp file and runs it with the given command.
//
// The code is never passed on the interpreter's standard input. A shell that
// reads its script from stdin shares that stream with the commands it starts,
// so the first one that reads input ("read", "kubectl run -i", a prompt)
// swallows the lines that follow it and they never run. With the script in a
// file, standard input is empty and such a command sees end of input instead.
func runFile(ctx context.Context, live io.Writer, code, filename string, args ...string) Result {
	dir, err := os.MkdirTemp("", "lekture-exec-*")
	if err != nil {
		return Result{Err: err}
	}
	defer os.RemoveAll(dir)

	path := filepath.Join(dir, filename)
	if err := os.WriteFile(path, []byte(code), 0o600); err != nil {
		return Result{Err: err}
	}

	cmdArgs := append(args, path)
	cmd := exec.CommandContext(ctx, cmdArgs[0], cmdArgs[1:]...)
	cmd.Dir = dir
	return capture(ctx, cmd, live)
}

// runC compiles and runs a C program.
func runC(ctx context.Context, live io.Writer, code string) Result {
	dir, err := os.MkdirTemp("", "lekture-exec-*")
	if err != nil {
		return Result{Err: err}
	}
	defer os.RemoveAll(dir)

	src := filepath.Join(dir, "main.c")
	bin := filepath.Join(dir, "main")
	if err := os.WriteFile(src, []byte(code), 0o600); err != nil {
		return Result{Err: err}
	}

	// Compile
	compile := exec.CommandContext(ctx, "cc", "-o", bin, src)
	compile.Dir = dir
	if out, err := compile.CombinedOutput(); err != nil {
		return Result{Output: string(out), Err: fmt.Errorf("compilation failed: %w", err)}
	}

	// Run
	cmd := exec.CommandContext(ctx, bin)
	cmd.Dir = dir
	return capture(ctx, cmd, live)
}

// runRust compiles and runs a Rust program via rustc.
func runRust(ctx context.Context, live io.Writer, code string) Result {
	dir, err := os.MkdirTemp("", "lekture-exec-*")
	if err != nil {
		return Result{Err: err}
	}
	defer os.RemoveAll(dir)

	src := filepath.Join(dir, "main.rs")
	bin := filepath.Join(dir, "main")
	if err := os.WriteFile(src, []byte(code), 0o600); err != nil {
		return Result{Err: err}
	}

	compile := exec.CommandContext(ctx, "rustc", "-o", bin, src)
	compile.Dir = dir
	if out, err := compile.CombinedOutput(); err != nil {
		return Result{Output: string(out), Err: fmt.Errorf("compilation failed: %w", err)}
	}

	cmd := exec.CommandContext(ctx, bin)
	cmd.Dir = dir
	return capture(ctx, cmd, live)
}

// maxOutputBytes limits captured output to prevent OOM from runaway programs.
const maxOutputBytes = 1 << 20 // 1 MiB

// capture runs cmd and collects its combined output. A non-nil live also
// receives each write as it happens, behind the same size limit.
func capture(ctx context.Context, cmd *exec.Cmd, live io.Writer) Result {
	var buf bytes.Buffer
	var sink io.Writer = &buf
	if live != nil {
		sink = io.MultiWriter(&buf, live)
	}
	limited := &limitedWriter{w: sink, remaining: maxOutputBytes}
	cmd.Stdout = limited
	cmd.Stderr = limited

	// Run the child in its own process group and kill the whole group on
	// cancellation. CommandContext otherwise kills only the direct child, so
	// a process it spawned ("go run" builds and runs a second binary) would
	// survive the timeout and keep the output pipe open indefinitely.
	setProcessGroup(cmd)
	cmd.Cancel = func() error { return killGroup(cmd) }
	cmd.WaitDelay = time.Second

	err := cmd.Run()

	output := buf.String()
	if limited.truncated {
		output += "\n\n(output truncated)"
	}
	if ctx.Err() != nil && errors.Is(ctx.Err(), context.DeadlineExceeded) {
		err = errTimedOut
	}
	return Result{Output: output, Err: err}
}

// limitedWriter stops writing after a byte limit to prevent OOM.
type limitedWriter struct {
	w         io.Writer
	remaining int
	truncated bool
}

// Write always reports the full length as consumed. Reporting a short write
// with a nil error violates the io.Writer contract: io.Copy turns it into
// io.ErrShortWrite and closes the pipe, which kills the child process.
func (lw *limitedWriter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if lw.remaining <= 0 {
		lw.truncated = true
		return len(p), nil // discard, but let the process keep running
	}
	keep := p
	if len(keep) > lw.remaining {
		keep = keep[:lw.remaining]
		lw.truncated = true
	}
	n, err := lw.w.Write(keep)
	lw.remaining -= n
	if err != nil {
		return n, err
	}
	return len(p), nil
}
