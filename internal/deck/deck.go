// Package deck loads and parses presentations. It is the single place that
// turns a path or a stream of markdown into the slides, metadata and base
// directory the renderer and server need.
package deck

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/kontrolplane/lekture/internal/meta"
	"github.com/kontrolplane/lekture/internal/parser"
)

// Deck is a parsed presentation.
type Deck struct {
	Slides  []parser.Slide
	Meta    meta.Meta
	BaseDir string
	// Path is the absolute path the deck was read from, or "" for stdin.
	Path string
	// Warning records a non-fatal problem, such as unparseable frontmatter.
	// The deck is still usable when it is set.
	Warning error
	// base holds lower-precedence configuration (the user config file), kept
	// so Reload applies the same precedence chain.
	base meta.Meta
}

// Base returns the lower-precedence configuration the deck was built with, so
// a reload can apply the same precedence chain.
func (d *Deck) Base() meta.Meta { return d.base }

// Empty reports whether the deck has no slides to show.
func (d *Deck) Empty() bool { return len(d.Slides) == 0 }

// Name is a short label for the deck, for use in messages.
func (d *Deck) Name() string {
	if d.Path == "" {
		return "input"
	}
	return filepath.Base(d.Path)
}

// EmptyError explains that a deck has no slides, and how slides are marked.
func (d *Deck) EmptyError() error {
	return fmt.Errorf("no slides found in %s — slides are separated by a line containing only ---", d.Name())
}

// Parse builds a deck from markdown content already in memory, applying the
// precedence chain: built-in defaults < base (user config) < frontmatter.
func Parse(content, baseDir string, base meta.Meta) *Deck {
	m, remaining, err := meta.Extract(content)
	merged := meta.Merge(base, m)
	merged.Defaults()
	return &Deck{
		Slides:  parser.ParseContent(remaining),
		Meta:    merged,
		BaseDir: baseDir,
		Warning: err,
		base:    base,
	}
}

// Load reads a presentation from path, layering it over base. An empty path or
// "-" reads stdin.
func Load(path string, base meta.Meta) (*Deck, error) {
	if path == "" || path == "-" {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return nil, fmt.Errorf("reading stdin: %w", err)
		}
		return Parse(string(data), workingDir(), base), nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading file: %w", err)
	}

	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolving path: %w", err)
	}

	d := Parse(string(data), filepath.Dir(abs), base)
	d.Path = abs
	return d, nil
}

// Reload re-reads the deck from disk. It returns an error for a deck that was
// read from stdin, which cannot be re-read.
func (d *Deck) Reload() (*Deck, error) {
	if d.Path == "" {
		return nil, fmt.Errorf("deck was read from stdin and cannot be reloaded")
	}
	return Load(d.Path, d.base)
}

// workingDir returns the current directory, falling back to "." on error.
func workingDir() string {
	cwd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return cwd
}
