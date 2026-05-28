package exec

import (
	"bytes"
	"context"
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

// Run executes a code block in the given language and returns its combined
// stdout/stderr output. Execution is capped at 10 seconds.
func Run(language, code string) Result {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
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
		return Result{Err: fmt.Errorf("unsupported language: %s", language)}
	}
}

// runInterpreted passes code via stdin to an interpreter.
func runInterpreted(ctx context.Context, code, interpreter string) Result {
	cmd := exec.CommandContext(ctx, interpreter)
	cmd.Stdin = strings.NewReader(code)
	return capture(cmd)
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
	return capture(cmd)
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
	return capture(cmd)
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
	return capture(cmd)
}

// maxOutputBytes limits captured output to prevent OOM from runaway programs.
const maxOutputBytes = 1 << 20 // 1 MiB

func capture(cmd *exec.Cmd) Result {
	var buf bytes.Buffer
	limited := &limitedWriter{w: &buf, remaining: maxOutputBytes}
	cmd.Stdout = limited
	cmd.Stderr = limited
	err := cmd.Run()

	output := buf.String()
	if limited.truncated {
		output += "\n\n(output truncated)"
	}
	return Result{Output: output, Err: err}
}

// limitedWriter stops writing after a byte limit to prevent OOM.
type limitedWriter struct {
	w         io.Writer
	remaining int
	truncated bool
}

func (lw *limitedWriter) Write(p []byte) (int, error) {
	if lw.remaining <= 0 {
		lw.truncated = true
		return len(p), nil // discard but don't error so the process keeps running
	}
	if len(p) > lw.remaining {
		p = p[:lw.remaining]
		lw.truncated = true
	}
	n, err := lw.w.Write(p)
	lw.remaining -= n
	return n, err
}
