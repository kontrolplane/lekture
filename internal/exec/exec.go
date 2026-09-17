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

// Timeout caps how long a code block may run.
const Timeout = 10 * time.Second

// Languages lists the supported language identifiers, for error messages.
var Languages = []string{
	"bash", "c", "elixir", "go", "javascript", "lua", "python", "ruby", "rust",
}

// Run executes a code block in the given language and returns its combined
// stdout/stderr output. Execution is capped at Timeout.
func Run(language, code string) Result {
	return RunContext(context.Background(), language, code)
}

// RunContext is Run with a caller-supplied context, so an in-flight execution
// can be cancelled (e.g. when the user dismisses the output panel).
func RunContext(parent context.Context, language, code string) Result {
	ctx, cancel := context.WithTimeout(parent, Timeout)
	defer cancel()

	switch strings.ToLower(language) {
	case "go", "golang":
		return runFile(ctx, code, "main.go", "go", "run")
	case "python", "py", "python3":
		return runInterpreted(ctx, code, "python3")
	case "javascript", "js", "node":
		return runInterpreted(ctx, code, "node")
	case "ruby", "rb":
		return runInterpreted(ctx, code, "ruby")
	case "lua":
		return runInterpreted(ctx, code, "lua")
	case "elixir", "ex":
		return runInterpreted(ctx, code, "elixir")
	case "bash", "sh":
		return runInterpreted(ctx, code, "bash")
	case "c":
		return runC(ctx, code)
	case "rust", "rs":
		return runRust(ctx, code)
	default:
		if language == "" {
			return Result{Err: fmt.Errorf("code block has no language (supported: %s)", strings.Join(Languages, ", "))}
		}
		return Result{Err: fmt.Errorf("unsupported language %q (supported: %s)", language, strings.Join(Languages, ", "))}
	}
}

// runInterpreted passes code via stdin to an interpreter.
func runInterpreted(ctx context.Context, code, interpreter string) Result {
	dir, err := os.MkdirTemp("", "lekture-exec-*")
	if err != nil {
		return Result{Err: err}
	}
	defer os.RemoveAll(dir)

	cmd := exec.CommandContext(ctx, interpreter)
	cmd.Dir = dir
	cmd.Stdin = strings.NewReader(code)
	return capture(ctx, cmd)
}

// runFile writes code to a temp file and runs it with the given command.
func runFile(ctx context.Context, code, filename string, args ...string) Result {
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
	return capture(ctx, cmd)
}

// runC compiles and runs a C program.
func runC(ctx context.Context, code string) Result {
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
	return capture(ctx, cmd)
}

// runRust compiles and runs a Rust program via rustc.
func runRust(ctx context.Context, code string) Result {
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
	return capture(ctx, cmd)
}

// maxOutputBytes limits captured output to prevent OOM from runaway programs.
const maxOutputBytes = 1 << 20 // 1 MiB

func capture(ctx context.Context, cmd *exec.Cmd) Result {
	var buf bytes.Buffer
	limited := &limitedWriter{w: &buf, remaining: maxOutputBytes}
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
		err = fmt.Errorf("execution timed out after %s", Timeout)
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
