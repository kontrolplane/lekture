package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strconv"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/fsnotify/fsnotify"
	"github.com/kontrolplane/lekture/internal/meta"
	"github.com/kontrolplane/lekture/internal/model"
	"github.com/kontrolplane/lekture/internal/parser"
	"github.com/kontrolplane/lekture/internal/server"
)

func main() {
	if len(os.Args) >= 2 && os.Args[1] == "serve" {
		handleServe()
		return
	}

	content, filePath, baseDir, err := loadContent()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}

	m, slides := parseAndBuild(content, baseDir)
	if len(slides) == 0 {
		fmt.Fprintf(os.Stderr, "No slides found\n")
		os.Exit(1)
	}

	opts := []tea.ProgramOption{tea.WithAltScreen()}

	if filePath == "" {
		tty, err := os.Open("/dev/tty")
		if err == nil {
			defer tty.Close()
			opts = append(opts, tea.WithInput(tty))
		}
	}

	p := tea.NewProgram(m, opts...)

	// Start file watcher with cancellation so it shuts down cleanly.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if filePath != "" {
		go watchFile(ctx, filePath, baseDir, p)
	}

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func loadContent() (content string, filePath string, baseDir string, err error) {
	if len(os.Args) >= 2 {
		path := os.Args[1]
		if path == "-" {
			data, err := io.ReadAll(os.Stdin)
			if err != nil {
				return "", "", "", fmt.Errorf("reading stdin: %w", err)
			}
			return string(data), "", getwd(), nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return "", "", "", fmt.Errorf("reading file: %w", err)
		}

		absPath, err := filepath.Abs(path)
		if err != nil {
			return "", "", "", fmt.Errorf("resolving path: %w", err)
		}

		return string(data), absPath, filepath.Dir(absPath), nil
	}

	stat, _ := os.Stdin.Stat()
	if (stat.Mode() & os.ModeCharDevice) == 0 {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", "", "", fmt.Errorf("reading stdin: %w", err)
		}
		return string(data), "", getwd(), nil
	}

	return "", "", "", fmt.Errorf("Usage: lekture <presentation.md>\n       lekture serve <presentation.md> [--port PORT] [--host HOST]\n       cat presentation.md | lekture")
}

// getwd returns the current working directory, falling back to "." on error.
func getwd() string {
	cwd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return cwd
}

func parseAndBuild(content string, baseDir string) (model.Model, []parser.Slide) {
	m, remaining := meta.Extract(content)
	slides := parser.ParseContent(remaining)
	mdl := model.New(slides, baseDir, m)
	return mdl, slides
}

func watchFile(ctx context.Context, path string, baseDir string, p *tea.Program) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return
	}
	defer watcher.Close()

	if err := watcher.Add(path); err != nil {
		return
	}

	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			if event.Has(fsnotify.Write) {
				data, err := os.ReadFile(path)
				if err != nil {
					continue
				}
				m, remaining := meta.Extract(string(data))
				slides := parser.ParseContent(remaining)
				if len(slides) > 0 {
					p.Send(model.FileChangedMsg{
						Slides: slides,
						Meta:   m,
					})
				}
			}
		case _, ok := <-watcher.Errors:
			if !ok {
				return
			}
		}
	}
}

func handleServe() {
	if len(os.Args) < 3 {
		fmt.Fprintf(os.Stderr, "Usage: lekture serve <presentation.md> [--port PORT] [--host HOST]\n")
		os.Exit(1)
	}

	path := os.Args[2]
	host := "localhost"
	port := 53531

	for i := 3; i < len(os.Args); i++ {
		switch os.Args[i] {
		case "--port", "-p":
			if i+1 < len(os.Args) {
				i++
				p, err := strconv.Atoi(os.Args[i])
				if err == nil {
					port = p
				}
			}
		case "--host", "-h":
			if i+1 < len(os.Args) {
				i++
				host = os.Args[i]
			}
		}
	}

	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading file: %v\n", err)
		os.Exit(1)
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error resolving path: %v\n", err)
		os.Exit(1)
	}
	baseDir := filepath.Dir(absPath)

	m, remaining := meta.Extract(string(data))
	slides := parser.ParseContent(remaining)

	if len(slides) == 0 {
		fmt.Fprintf(os.Stderr, "No slides found in %s\n", path)
		os.Exit(1)
	}

	if err := server.Serve(slides, m, baseDir, host, port); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
