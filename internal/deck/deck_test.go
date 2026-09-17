package deck

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kontrolplane/lekture/internal/meta"
)

func write(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadResolvesBaseDirAndPath(t *testing.T) {
	dir := t.TempDir()
	p := write(t, dir, "deck.md", "---\nauthor: levi\n---\n# one\n\n---\n\n# two\n")

	d, err := Load(p, meta.Meta{})
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Slides) != 2 {
		t.Errorf("slides = %d, want 2", len(d.Slides))
	}
	if d.Meta.Author != "levi" {
		t.Errorf("author = %q, want levi", d.Meta.Author)
	}
	if !filepath.IsAbs(d.Path) {
		t.Errorf("path %q is not absolute", d.Path)
	}
	if d.BaseDir != filepath.Dir(d.Path) {
		t.Errorf("baseDir = %q, want %q", d.BaseDir, filepath.Dir(d.Path))
	}
	if d.Name() != "deck.md" {
		t.Errorf("name = %q, want deck.md", d.Name())
	}
}

func TestLoadMissingFile(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "nope.md"), meta.Meta{}); err == nil {
		t.Error("expected an error")
	}
}

func TestEmptyDeckExplainsSeparators(t *testing.T) {
	d := Parse("", t.TempDir(), meta.Meta{})
	if !d.Empty() {
		t.Fatal("expected an empty deck")
	}
	if !strings.Contains(d.EmptyError().Error(), "---") {
		t.Errorf("error should explain the separator: %v", d.EmptyError())
	}
}

func TestReloadPicksUpChanges(t *testing.T) {
	dir := t.TempDir()
	p := write(t, dir, "deck.md", "# one")

	d, err := Load(p, meta.Meta{})
	if err != nil {
		t.Fatal(err)
	}
	write(t, dir, "deck.md", "# one\n\n---\n\n# two")

	next, err := d.Reload()
	if err != nil {
		t.Fatal(err)
	}
	if len(next.Slides) != 2 {
		t.Errorf("slides after reload = %d, want 2", len(next.Slides))
	}
}

func TestReloadRejectsStdinDeck(t *testing.T) {
	d := Parse("# one", t.TempDir(), meta.Meta{})
	if _, err := d.Reload(); err == nil {
		t.Error("a stdin deck cannot be reloaded")
	}
}

// TestWatchSurvivesAtomicSave is the regression guard for editors that save by
// writing a temporary file and renaming it over the original, which destroys
// the watched inode. Watching the file directly silently stopped reloading.
func TestWatchSurvivesAtomicSave(t *testing.T) {
	dir := t.TempDir()
	p := write(t, dir, "deck.md", "# one")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	changes := make(chan *Deck, 16)
	errs := make(chan error, 4)
	go Watch(ctx, p, meta.Meta{}, func(d *Deck) { changes <- d }, func(e error) { errs <- e })

	// Give the watcher time to register before the first save.
	time.Sleep(150 * time.Millisecond)

	atomicSave := func(content string) {
		tmp := filepath.Join(dir, ".deck.md.swp")
		if err := os.WriteFile(tmp, []byte(content), 0o600); err != nil {
			t.Error(err)
			return
		}
		if err := os.Rename(tmp, p); err != nil {
			t.Error(err)
		}
	}

	for i := 0; i < 3; i++ {
		atomicSave(strings.Repeat("# slide\n\n---\n\n", i+1) + "# last")
		select {
		case d := <-changes:
			if d.Empty() {
				t.Fatalf("save %d produced an empty deck", i+1)
			}
		case err := <-errs:
			t.Fatalf("save %d: watch error %v", i+1, err)
		case <-time.After(3 * time.Second):
			t.Fatalf("save %d produced no reload — the watch did not survive an atomic save", i+1)
		}
	}
}

func TestWatchDebouncesBurst(t *testing.T) {
	dir := t.TempDir()
	p := write(t, dir, "deck.md", "# one")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	changes := make(chan *Deck, 32)
	go Watch(ctx, p, meta.Meta{}, func(d *Deck) { changes <- d }, func(error) {})
	time.Sleep(150 * time.Millisecond)

	for i := 0; i < 5; i++ {
		write(t, dir, "deck.md", "# one\n\nedit")
		time.Sleep(5 * time.Millisecond)
	}

	time.Sleep(500 * time.Millisecond)
	if n := len(changes); n == 0 {
		t.Fatal("no reload at all")
	} else if n > 2 {
		t.Errorf("a burst of writes produced %d reloads, want them coalesced", n)
	}
}

// TestParseAppliesDefaults is the guarantee callers rely on: a deck built
// through this package always has usable, sanitized metadata.
func TestParseAppliesDefaults(t *testing.T) {
	d := Parse("---\npaging: \"%s\"\n---\n# a", t.TempDir(), meta.Meta{})
	if d.Meta.Paging != "Slide %d / %d" {
		t.Errorf("paging = %q, want the default to replace an unsafe format", d.Meta.Paging)
	}
	if d.Meta.Align == "" || d.Meta.HeadingColor == "" {
		t.Errorf("defaults were not applied: %+v", d.Meta)
	}
}

func TestParseAppliesConfigPrecedence(t *testing.T) {
	config := meta.Meta{Align: "center", Author: "config author", Theme: "kontrolplane"}
	d := Parse("---\nauthor: deck author\n---\n# a", t.TempDir(), config)

	if d.Meta.Author != "deck author" {
		t.Errorf("author = %q, want the frontmatter to win", d.Meta.Author)
	}
	if d.Meta.Align != "center" {
		t.Errorf("align = %q, want the config value where the deck is silent", d.Meta.Align)
	}
	if d.Meta.Theme != "kontrolplane" {
		t.Errorf("theme = %q, want the config value", d.Meta.Theme)
	}
}

func TestReloadKeepsConfigPrecedence(t *testing.T) {
	dir := t.TempDir()
	p := write(t, dir, "deck.md", "# one")
	config := meta.Meta{Align: "center"}

	d, err := Load(p, config)
	if err != nil {
		t.Fatal(err)
	}
	next, err := d.Reload()
	if err != nil {
		t.Fatal(err)
	}
	if next.Meta.Align != "center" {
		t.Errorf("align = %q after reload, want the config value to persist", next.Meta.Align)
	}
}
