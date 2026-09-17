package model

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/kontrolplane/lekture/internal/meta"
	"github.com/kontrolplane/lekture/internal/parser"
)

func newModel(t *testing.T, markdowns ...string) Model {
	t.Helper()
	var m meta.Meta
	content := strings.Join(markdowns, "\n---\n")
	mm, rest, err := meta.Extract(content)
	if err != nil {
		t.Fatal(err)
	}
	m = mm
	m.Defaults()
	return New(parser.ParseContent(rest), t.TempDir(), m)
}

func key(s string) tea.KeyMsg {
	switch s {
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "backspace":
		return tea.KeyMsg{Type: tea.KeyBackspace}
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	case "space":
		return tea.KeyMsg{Type: tea.KeySpace}
	case "ctrl+e":
		return tea.KeyMsg{Type: tea.KeyCtrlE}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func send(m Model, keys ...string) Model {
	for _, k := range keys {
		next, _ := m.Update(key(k))
		m = next.(Model)
	}
	return m
}

func TestNavigateToClampsEmptyDeck(t *testing.T) {
	m := New(nil, t.TempDir(), meta.Meta{})
	for _, target := range []int{-3, 0, 5} {
		if got := m.navigateTo(target).current; got != 0 {
			t.Errorf("navigateTo(%d) on empty deck = %d, want 0", target, got)
		}
	}
}

func TestExecOnEmptyDeckDoesNotPanic(t *testing.T) {
	m := New(nil, t.TempDir(), meta.Meta{})
	for _, k := range []string{"ctrl+e", "end", "G", "g", "g", "l"} {
		m = send(m, k)
	}
	if m.current != 0 {
		t.Errorf("current = %d, want 0", m.current)
	}
}

func TestSearchBackspaceIsRuneAware(t *testing.T) {
	m := newModel(t, "# 日本語")
	m = send(m, "/", "日", "本", "語", "backspace")
	if m.searchInput != "日本" {
		t.Errorf("searchInput = %q, want %q", m.searchInput, "日本")
	}
}

func TestSearchIgnoresNamedKeys(t *testing.T) {
	m := newModel(t, "# one")
	m = send(m, "/", "b", "up", "e", "tab", "t")
	if m.searchInput != "bet" {
		t.Errorf("searchInput = %q, want %q", m.searchInput, "bet")
	}
}

func TestSearchNoMatchReportsAndClearsState(t *testing.T) {
	m := newModel(t, "# alpha", "# beta")
	m = send(m, "/", "b", "e", "t", "a", "enter")
	if !m.searchActive || len(m.searchResults) != 1 {
		t.Fatalf("expected an active search with 1 result, got active=%v results=%v", m.searchActive, m.searchResults)
	}
	m = send(m, "/", "z", "z", "z", "enter")
	if m.searchActive {
		t.Errorf("searchActive should be false after a zero-result search")
	}
	if m.searchError == "" {
		t.Errorf("expected a no-matches message")
	}
}

func TestQuitAlwaysQuitsAfterSearch(t *testing.T) {
	m := newModel(t, "# alpha", "# beta")
	m = send(m, "/", "b", "e", "t", "a", "enter")
	_, cmd := m.Update(key("q"))
	if cmd == nil {
		t.Fatal("q should quit even while a search is active")
	}
	if msg := cmd(); msg == nil {
		t.Fatal("expected a quit message")
	}
}

func TestEscClearsSearchThenQuits(t *testing.T) {
	m := newModel(t, "# alpha", "# beta")
	m = send(m, "/", "b", "e", "t", "a", "enter")
	next, cmd := m.Update(key("esc"))
	m = next.(Model)
	if cmd != nil {
		t.Fatal("first esc should clear the search, not quit")
	}
	if m.searchActive {
		t.Error("search should be cleared")
	}
	if _, cmd = m.Update(key("esc")); cmd == nil {
		t.Fatal("second esc should quit")
	}
}

func TestSearchCyclesBothDirections(t *testing.T) {
	m := newModel(t, "# hit", "# miss", "# hit again", "# hit three")
	m = send(m, "/", "h", "i", "t", "enter")
	if len(m.searchResults) != 3 {
		t.Fatalf("results = %v, want 3", m.searchResults)
	}
	start := m.searchIdx
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlN})
	m = next.(Model)
	if m.searchIdx == start {
		t.Error("ctrl+n did not advance")
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
	m = next.(Model)
	if m.searchIdx != start {
		t.Errorf("ctrl+p did not return to %d, got %d", start, m.searchIdx)
	}
}

func TestNumericPrefixIsCapped(t *testing.T) {
	m := newModel(t, "# a", "# b", "# c")
	m = send(m, strings.Split(strings.Repeat("9", 21), "")...)
	if len(m.numBuf) > maxCountDigits {
		t.Errorf("numBuf grew to %d digits, want <= %d", len(m.numBuf), maxCountDigits)
	}
	m = send(m, "G")
	if m.current != 2 {
		t.Errorf("current = %d, want last slide (2)", m.current)
	}
}

func TestExecDisabledIsReported(t *testing.T) {
	m := newModel(t, "# a\n\n```bash\necho hi\n```").WithExec(false)
	m = send(m, "ctrl+e")
	if m.execVisible {
		t.Error("exec should not open when disabled")
	}
	if m.searchError == "" {
		t.Error("expected a message explaining exec is disabled")
	}
}

func TestHelpOverlayToggles(t *testing.T) {
	m := newModel(t, "# a")
	m = send(m, "?")
	if !m.helpVisible {
		t.Fatal("? should open help")
	}
	m = send(m, "esc")
	if m.helpVisible {
		t.Fatal("esc should close help")
	}
}

func TestViewIsExactlyTerminalSize(t *testing.T) {
	m := newModel(t, "# a\n\nsome body text", "# b")
	m = m.WithSize(80, 24)
	out := m.View()
	lines := strings.Split(out, "\n")
	if len(lines) != 24 {
		t.Errorf("View produced %d lines, want 24", len(lines))
	}
}

func TestPagingFormatCannotEmitGarbage(t *testing.T) {
	mm, rest, err := meta.Extract("---\npaging: \"%s\"\n---\n# a")
	if err != nil {
		t.Fatal(err)
	}
	mm.Defaults()
	m := New(parser.ParseContent(rest), t.TempDir(), mm).WithSize(80, 24)
	if strings.Contains(m.View(), "%!") {
		t.Errorf("status bar rendered a format error:\n%s", m.View())
	}
}

// TestBadThemeSurfacesAsWarning checks that a mistyped theme path is reported
// in the status bar instead of silently rendering with the default style.
func TestBadThemeSurfacesAsWarning(t *testing.T) {
	m := New(parser.ParseContent("# a"), t.TempDir(), meta.Meta{Theme: "nope.json", Paging: "%d/%d", Align: "top-left", HeadingColor: "#a6da95"})
	if m.warning == "" {
		t.Fatal("expected a warning about the theme")
	}
	m = m.WithSize(80, 24)
	if !strings.Contains(m.View(), "theme") {
		t.Errorf("status bar did not mention the theme problem:\n%s", m.View())
	}
}

func deckWithBlocks(t *testing.T) Model {
	t.Helper()
	md := "# demo\n\n```bash\necho one\n```\n\n```python\nprint(2)\n```"
	return New(parser.ParseContent(md), t.TempDir(), meta.Meta{Paging: "%d/%d", Align: "top-left", HeadingColor: "#a6da95"}).WithSize(80, 24)
}

func TestTabCyclesCodeBlockSelection(t *testing.T) {
	m := deckWithBlocks(t)
	if len(m.slides[0].CodeBlocks) != 2 {
		t.Fatalf("expected 2 code blocks, got %d", len(m.slides[0].CodeBlocks))
	}
	if m.execBlockIdx != 0 {
		t.Fatalf("initial selection = %d, want 0", m.execBlockIdx)
	}
	m = send(m, "tab")
	if m.execBlockIdx != 1 {
		t.Errorf("after tab = %d, want 1", m.execBlockIdx)
	}
	m = send(m, "tab")
	if m.execBlockIdx != 0 {
		t.Errorf("tab should wrap around, got %d", m.execBlockIdx)
	}
}

func TestBlockSelectionResetsOnNavigation(t *testing.T) {
	md := "# a\n\n```bash\necho 1\n```\n\n```bash\necho 2\n```\n---\n# b"
	m := New(parser.ParseContent(md), t.TempDir(), meta.Meta{Paging: "%d/%d", Align: "top-left", HeadingColor: "#a6da95"}).WithSize(80, 24)
	m = send(m, "tab")
	if m.execBlockIdx != 1 {
		t.Fatalf("selection = %d, want 1", m.execBlockIdx)
	}
	m = send(m, "l")
	if m.execBlockIdx != 0 {
		t.Errorf("selection should reset on slide change, got %d", m.execBlockIdx)
	}
}

func TestExecOverlayScrollsInsteadOfBeingCut(t *testing.T) {
	m := deckWithBlocks(t)
	m.execVisible = true
	m.execOutput = strings.TrimRight(strings.Repeat("line\n", 200), "\n")

	out := m.renderExecOverlay()
	if strings.Count(out, "\n")+1 > 24 {
		t.Errorf("overlay is %d lines, taller than the terminal", strings.Count(out, "\n")+1)
	}
	if !strings.Contains(out, "esc to dismiss") {
		t.Error("the dismiss hint was cut off")
	}
	if !strings.Contains(out, "of 200") {
		t.Errorf("expected a scroll position indicator:\n%s", out)
	}

	scrolled := send(m, "down", "down").renderExecOverlay()
	if scrolled == out {
		t.Error("scrolling did not change the visible window")
	}
}

func TestExecScrollClampsAtEnd(t *testing.T) {
	m := deckWithBlocks(t)
	m.execVisible = true
	m.execOutput = strings.TrimRight(strings.Repeat("x\n", 50), "\n")
	for i := 0; i < 500; i++ {
		m = send(m, "down")
	}
	// Must not panic or slice out of range.
	if out := m.renderExecOverlay(); out == "" {
		t.Error("expected output")
	}
}

func TestEscCancelsRunningExec(t *testing.T) {
	m := deckWithBlocks(t)
	cancelled := false
	m.execVisible = true
	m.execRunning = true
	m.execCancel = func() { cancelled = true }
	m = send(m, "esc")
	if !cancelled {
		t.Error("esc should cancel the running program, not just hide the panel")
	}
	if m.execRunning || m.execVisible {
		t.Error("exec state should be cleared")
	}
}

func TestExecOverlayShowsExitStatus(t *testing.T) {
	m := deckWithBlocks(t)
	m.execVisible = true
	m.execOutput = "hi"
	m.execDuration = 1234 * 1000000
	if got := m.renderExecOverlay(); !strings.Contains(got, "exit 0") {
		t.Errorf("expected exit status in the panel:\n%s", got)
	}
}

func TestNoCodeBlockIsReported(t *testing.T) {
	m := newModel(t, "# nothing here")
	m = send(m, "ctrl+e")
	if m.execVisible {
		t.Error("panel should not open when there is no code block")
	}
	if m.searchError == "" {
		t.Error("expected a message explaining there is no code block")
	}
}

func TestStatusBarKeepsPositionOnNarrowTerminal(t *testing.T) {
	mm := meta.Meta{
		Author:       "a presenter with a rather long name indeed",
		Date:         "2026-09-15",
		Paging:       "%d/%d",
		Align:        "top-left",
		HeadingColor: "#a6da95",
	}
	m := New(parser.ParseContent("# a\n---\n# b"), t.TempDir(), mm).WithSize(30, 10)
	lines := strings.Split(m.View(), "\n")
	status := lines[len(lines)-1]
	if !strings.Contains(status, "1/2") {
		t.Errorf("slide position was dropped from a narrow status bar: %q", status)
	}
	if w := lipgloss.Width(status); w > 30 {
		t.Errorf("status bar is %d columns wide, want <= 30", w)
	}
}

func revealDeck(t *testing.T) Model {
	t.Helper()
	md := "# one\n\na\n\n<!-- pause -->\n\nb\n\n<!-- pause -->\n\nc\n---\n# two\n\nx\n\n<!-- pause -->\n\ny"
	return New(parser.ParseContent(md), t.TempDir(),
		meta.Meta{Paging: "%d/%d", Align: "top-left", HeadingColor: "#a6da95"}).WithSize(80, 24)
}

func TestForwardWalksRevealsThenSlides(t *testing.T) {
	m := revealDeck(t)
	want := []struct{ slide, step int }{
		{0, 1}, {0, 2}, // remaining reveals on slide one
		{1, 0}, {1, 1}, // then slide two and its reveal
		{1, 1}, // clamped at the end
	}
	for i, w := range want {
		m = send(m, "l")
		if m.current != w.slide || m.currentStep != w.step {
			t.Fatalf("press %d: slide=%d step=%d, want slide=%d step=%d", i+1, m.current, m.currentStep, w.slide, w.step)
		}
	}
}

// TestBackwardLandsOnLastReveal is the detail that makes reverse navigation
// feel right: stepping back into a slide must show it fully revealed.
func TestBackwardLandsOnLastReveal(t *testing.T) {
	m := revealDeck(t)
	m = send(m, "l", "l", "l") // slide two, first reveal
	if m.current != 1 || m.currentStep != 0 {
		t.Fatalf("setup: slide=%d step=%d", m.current, m.currentStep)
	}
	m = send(m, "h")
	if m.current != 0 || m.currentStep != 2 {
		t.Errorf("stepping back gave slide=%d step=%d, want slide=0 step=2 (fully revealed)", m.current, m.currentStep)
	}
}

func TestCountedMotionsOperateOnSlides(t *testing.T) {
	m := revealDeck(t)
	m = send(m, "2", "l")
	if m.current != 1 {
		t.Errorf("2l moved to slide %d, want 1 — counts should skip slides, not reveals", m.current)
	}
	if m.currentStep != 0 {
		t.Errorf("a counted jump should land on the first reveal, got %d", m.currentStep)
	}
}

func TestJumpsResetReveal(t *testing.T) {
	m := revealDeck(t)
	m = send(m, "l") // reveal 1 of slide one
	m = send(m, "G")
	if m.currentStep != 0 {
		t.Errorf("G left step at %d, want 0", m.currentStep)
	}
	m = send(m, "l")
	m = send(m, "g", "g")
	if m.current != 0 || m.currentStep != 0 {
		t.Errorf("gg gave slide=%d step=%d, want 0/0", m.current, m.currentStep)
	}
}

func TestRevealIndicatorInStatusBar(t *testing.T) {
	m := revealDeck(t)
	if got := m.View(); !strings.Contains(got, "(1/3)") {
		t.Errorf("expected a reveal indicator in the status bar")
	}
	m = send(m, "l")
	if got := m.View(); !strings.Contains(got, "(2/3)") {
		t.Errorf("reveal indicator did not advance")
	}
}

func TestRevealShowsOnlyRevealedContent(t *testing.T) {
	m := revealDeck(t)
	first := m.View()
	if !strings.Contains(first, "a") {
		t.Error("first reveal should show its own content")
	}
	if strings.Contains(first, "c") {
		t.Error("first reveal leaked later content")
	}
	m = send(m, "l", "l")
	if !strings.Contains(m.View(), "c") {
		t.Error("final reveal should show all content")
	}
}

func TestRevealClampedAfterReload(t *testing.T) {
	m := revealDeck(t)
	m = send(m, "l", "l") // step 2
	next, _ := m.Update(FileChangedMsg{
		Slides: parser.ParseContent("# one\n\nonly one step now"),
		Meta:   m.meta,
	})
	m = next.(Model)
	if m.currentStep != 0 {
		t.Errorf("step = %d after reload to a shorter slide, want 0", m.currentStep)
	}
	if m.View() == "" {
		t.Error("expected a rendered view")
	}
}
