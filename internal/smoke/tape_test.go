package smoke

import (
	"os"
	"regexp"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/kontrolplane/lekture/internal/deck"
	"github.com/kontrolplane/lekture/internal/meta"
	"github.com/kontrolplane/lekture/internal/model"
)

// press replays a vhs key directive against the model.
func press(t *testing.T, m model.Model, directive string) model.Model {
	t.Helper()
	var msg tea.KeyMsg
	switch directive {
	case "Right":
		msg = tea.KeyMsg{Type: tea.KeyRight}
	case "Left":
		msg = tea.KeyMsg{Type: tea.KeyLeft}
	case "Tab":
		msg = tea.KeyMsg{Type: tea.KeyTab}
	case "Enter":
		msg = tea.KeyMsg{Type: tea.KeyEnter}
	case "Escape":
		msg = tea.KeyMsg{Type: tea.KeyEsc}
	case "Ctrl+E":
		msg = tea.KeyMsg{Type: tea.KeyCtrlE}
	case "Ctrl+N":
		msg = tea.KeyMsg{Type: tea.KeyCtrlN}
	case "Ctrl+P":
		msg = tea.KeyMsg{Type: tea.KeyCtrlP}
	case "Shift+G":
		msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("G")}
	default:
		t.Fatalf("unhandled directive %q", directive)
	}
	next, _ := m.Update(msg)
	return next.(model.Model)
}

func typeRunes(m model.Model, s string) model.Model {
	for _, r := range s {
		next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = next.(model.Model)
	}
	return m
}

func exampleModel(t *testing.T) (model.Model, *deck.Deck) {
	t.Helper()
	d, err := deck.Load("../../example/presentation.md", meta.Meta{})
	if err != nil {
		t.Skip("example deck not reachable")
	}
	return model.New(d.Slides, "../../example", d.Meta).WithSize(120, 40), d
}

func heading(md string) string {
	for _, l := range strings.Split(md, "\n") {
		l = strings.TrimSpace(l)
		if strings.HasPrefix(l, "#") {
			return strings.TrimSpace(strings.TrimLeft(l, "# "))
		}
	}
	return "(untitled)"
}

// TestScreenshotTapeLandsOnIntendedSlides replays the key directives from
// vhs/screenshots.tape so the capture points stay correct even though the tape
// itself can only be run where vhs is installed.
func TestScreenshotTapeLandsOnIntendedSlides(t *testing.T) {
	m, d := exampleModel(t)

	// Screenshot 1: the agenda, partly revealed.
	m = press(t, m, "Right")
	m = press(t, m, "Right")
	if got := heading(d.Slides[slideOf(t, m)].RawMarkdown); got != "agenda" {
		t.Errorf("reveal.png would capture %q, want the agenda", got)
	}
	if view := m.View(); !strings.Contains(view, "(2/3)") {
		t.Errorf("reveal.png should show a partial reveal counter")
	}
	if view := m.View(); strings.Contains(view, "reference architecture") {
		t.Error("reveal.png should not show the final reveal's content")
	}

	// Screenshot 2: slide overview.
	m = press(t, m, "Right")
	m = press(t, m, "Right")
	m = press(t, m, "Right")
	if got := heading(d.Slides[slideOf(t, m)].RawMarkdown); got != "what is a vpc?" {
		t.Errorf("overview.png would capture %q, want \"what is a vpc?\"", got)
	}

	// Screenshot 3: the code block slide.
	for i := 0; i < 4; i++ {
		m = press(t, m, "Right")
	}
	idx := slideOf(t, m)
	if got := heading(d.Slides[idx].RawMarkdown); got != "vpc-attached lambda" {
		t.Fatalf("code-block.png would capture %q, want \"vpc-attached lambda\"", got)
	}
	if n := len(d.Slides[idx].CodeBlocks); n != 2 {
		t.Errorf("the lambda slide has %d code blocks, want 2 so Tab has something to select", n)
	}
	if d.Slides[idx].CodeBlocks[0].Language != "go" {
		t.Errorf("first block is %q, want go — Ctrl+E runs it for code-execution.png",
			d.Slides[idx].CodeBlocks[0].Language)
	}

	// Screenshot 5: the keymap overlay.
	m = typeRunes(m, "?")
	if !strings.Contains(m.View(), "Keybindings") {
		t.Error("help.png would not capture the keymap overlay")
	}
}

// slideOf reads the slide number out of the rendered status bar.
var pagingRe = regexp.MustCompile(`(\d+) / \d+`)

func slideOf(t *testing.T, m model.Model) int {
	t.Helper()
	match := pagingRe.FindStringSubmatch(stripANSI(m.View()))
	if match == nil {
		t.Fatal("no slide counter in the status bar")
	}
	var n int
	for _, r := range match[1] {
		n = n*10 + int(r-'0')
	}
	return n - 1
}

var ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

func stripANSI(s string) string { return ansiRe.ReplaceAllString(s, "") }

// TestTapesReferenceExistingAssets keeps the tape output paths and the README
// image references from drifting apart.
func TestTapesReferenceExistingAssets(t *testing.T) {
	readme, err := os.ReadFile("../../README.md")
	if err != nil {
		t.Skip("README not reachable")
	}
	tape, err := os.ReadFile("../../vhs/screenshots.tape")
	if err != nil {
		t.Skip("tape not reachable")
	}

	shots := regexp.MustCompile(`(?m)^Screenshot (\S+)`).FindAllStringSubmatch(string(tape), -1)
	if len(shots) == 0 {
		t.Fatal("no screenshots declared in the tape")
	}
	for _, s := range shots {
		path := s[1]
		if !strings.Contains(string(readme), strings.TrimPrefix(path, "assets/")) {
			t.Errorf("tape writes %s but the README never shows it", path)
		}
	}
}
