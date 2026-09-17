package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/kontrolplane/lekture/internal/deck"
	"github.com/kontrolplane/lekture/internal/export"
	"github.com/kontrolplane/lekture/internal/meta"
	"github.com/kontrolplane/lekture/internal/model"
	"github.com/kontrolplane/lekture/internal/render"
	"github.com/kontrolplane/lekture/internal/server"
	"golang.org/x/term"
)

// Build information, overridable with -ldflags "-X main.version=...".
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

const usage = `lekture — present markdown slideshows in the terminal

usage:
  lekture <presentation.md>
  lekture serve <presentation.md> [--port PORT] [--host HOST] [--allow-exec]
  lekture dump <presentation.md> [--width N]
  lekture export <presentation.md> [--output FILE]
  cat presentation.md | lekture

flags:
  -h, --help       show this help
  -v, --version    show version information

serve flags:
  --port PORT      port to listen on (default 53531)
  --host HOST      address to bind (default localhost)
  --allow-exec     allow connected clients to execute code blocks

dump flags:
  --width N        render width (default: terminal width, or 100)

export flags:
  --output FILE    write to FILE instead of stdout
`

func main() {
	if err := run(); err != nil {
		if err != errSilent {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
		}
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) >= 2 {
		switch os.Args[1] {
		case "-h", "--help", "help":
			fmt.Print(usage)
			return nil
		case "-v", "--version":
			fmt.Printf("lekture %s (commit %s, built %s)\n", version, commit, date)
			return nil
		case "serve":
			return runServe()
		case "dump":
			return runDump()
		case "export":
			return runExport()
		}
	}
	return runPresent()
}

// deckPath returns the path to load from the top-level arguments, and whether
// input is arriving on stdin.
func deckPath() (string, error) {
	if len(os.Args) >= 2 {
		return os.Args[1], nil
	}
	// A failing Stat returns a nil FileInfo, so the error must be checked
	// before Mode() is called on it.
	stat, err := os.Stdin.Stat()
	if err != nil {
		return "", fmt.Errorf("checking stdin: %w", err)
	}
	if (stat.Mode() & os.ModeCharDevice) == 0 {
		return "-", nil
	}
	return "", errors.New(usage)
}

// userConfig loads the user configuration file, reporting but not failing on a
// broken one: a bad config must never stop a presentation.
func userConfig() meta.Meta {
	cfg, err := meta.LoadUserConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: %v\n", err)
		return meta.Meta{}
	}
	return cfg
}

// loadDeck loads a presentation with the user config applied underneath it.
func loadDeck(path string) (*deck.Deck, error) {
	return deck.Load(path, userConfig())
}

func runPresent() error {
	path, err := deckPath()
	if err != nil {
		return err
	}

	d, err := loadDeck(path)
	if err != nil {
		return err
	}
	if d.Warning != nil {
		fmt.Fprintf(os.Stderr, "warning: %v\n", d.Warning)
	}
	if d.Empty() {
		return d.EmptyError()
	}

	m := model.New(d.Slides, d.BaseDir, d.Meta)
	opts := []tea.ProgramOption{tea.WithAltScreen()}

	if d.Path == "" {
		// stdin was the deck, so reopen the terminal for key input.
		tty, err := os.Open("/dev/tty")
		if err != nil {
			return fmt.Errorf("opening terminal for input: %w", err)
		}
		defer tty.Close()
		opts = append(opts, tea.WithInput(tty))
	}

	p := tea.NewProgram(m, opts...)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if d.Path != "" {
		go deck.Watch(ctx, d.Path, userConfig(),
			func(next *deck.Deck) {
				if next.Warning != nil {
					p.Send(model.WarningMsg{Text: next.Warning.Error()})
				}
				p.Send(model.FileChangedMsg{Slides: next.Slides, Meta: next.Meta})
			},
			func(err error) {
				p.Send(model.WarningMsg{Text: fmt.Sprintf("live reload disabled: %v", err)})
			},
		)
	}

	_, err = p.Run()
	return err
}

func runServe() error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = func() { fmt.Fprint(os.Stderr, usage) }

	host := fs.String("host", "localhost", "address to bind")
	port := fs.Int("port", 53531, "port to listen on")
	allowExec := fs.Bool("allow-exec", false, "allow connected clients to execute code blocks")

	path, err := parseArgs(fs, os.Args[2:])
	if err != nil {
		return err
	}
	if *port < 1 || *port > 65535 {
		return fmt.Errorf("invalid --port %d (must be 1-65535)", *port)
	}

	d, err := loadDeck(path)
	if err != nil {
		return err
	}
	if d.Warning != nil {
		fmt.Fprintf(os.Stderr, "warning: %v\n", d.Warning)
	}
	if d.Empty() {
		return d.EmptyError()
	}

	return server.Serve(d, *host, *port, *allowExec)
}

func runDump() error {
	fs := flag.NewFlagSet("dump", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = func() { fmt.Fprint(os.Stderr, usage) }

	width := fs.Int("width", 0, "render width")

	path, err := parseArgs(fs, os.Args[2:])
	if err != nil {
		return err
	}

	d, err := loadDeck(path)
	if err != nil {
		return err
	}
	if d.Warning != nil {
		fmt.Fprintf(os.Stderr, "warning: %v\n", d.Warning)
	}
	if d.Empty() {
		return d.EmptyError()
	}

	w := *width
	if w <= 0 {
		w = terminalWidth()
	}

	r := render.New(d.BaseDir, w, 40, d.Meta.Theme, d.Meta.HeadingColor)
	if err := r.ThemeError(); err != nil {
		fmt.Fprintf(os.Stderr, "warning: %v\n", err)
	}

	out := os.Stdout
	for i, slide := range d.Slides {
		text, err := r.RenderSlide(slide)
		if err != nil {
			return fmt.Errorf("slide %d: %w", i+1, err)
		}
		if i > 0 {
			fmt.Fprintln(out)
			fmt.Fprintln(out, strings.Repeat("─", w))
			fmt.Fprintln(out)
		}
		fmt.Fprintln(out, text)
	}
	return nil
}

func runExport() error {
	fs := flag.NewFlagSet("export", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = func() { fmt.Fprint(os.Stderr, usage) }

	output := fs.String("output", "", "write to a file instead of stdout")

	path, err := parseArgs(fs, os.Args[2:])
	if err != nil {
		return err
	}

	d, err := loadDeck(path)
	if err != nil {
		return err
	}
	if d.Warning != nil {
		fmt.Fprintf(os.Stderr, "warning: %v\n", d.Warning)
	}
	if d.Empty() {
		return d.EmptyError()
	}

	// Render fully before touching the destination, so a failure cannot leave
	// a half-written file behind.
	var buf bytes.Buffer
	if err := export.HTML(d, &buf); err != nil {
		return err
	}

	if *output == "" {
		_, err := os.Stdout.Write(buf.Bytes())
		return err
	}
	if err := os.WriteFile(*output, buf.Bytes(), 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", *output, err)
	}
	fmt.Fprintf(os.Stderr, "wrote %s (%s)\n", *output, plural(len(d.Slides), "slide"))
	return nil
}

// parseArgs parses flags that may appear before or after the file argument and
// returns the file path.
func parseArgs(fs *flag.FlagSet, args []string) (string, error) {
	if err := fs.Parse(args); err != nil {
		return "", errSilent
	}
	rest := fs.Args()
	if len(rest) == 0 {
		fmt.Fprint(os.Stderr, usage)
		return "", errSilent
	}
	path := rest[0]
	if len(rest) > 1 {
		if err := fs.Parse(rest[1:]); err != nil {
			return "", errSilent
		}
	}
	return path, nil
}

// plural formats a count with a correctly pluralized noun.
func plural(n int, noun string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, noun)
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

// errSilent signals that a message has already been printed.
var errSilent = errors.New("")

func terminalWidth() int {
	const fallback = 100
	if w, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil && w > 0 {
		return w
	}
	return fallback
}
