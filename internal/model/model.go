package model

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	codeexec "github.com/kontrolplane/lekture/internal/exec"
	"github.com/kontrolplane/lekture/internal/meta"
	"github.com/kontrolplane/lekture/internal/parser"
	"github.com/kontrolplane/lekture/internal/render"
)

// Styles are defined as package-level variables for reuse across renders.
var (
	statusStyle = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#666666", Dark: "#888888"}).
			Padding(0, 1)

	execBorderStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.AdaptiveColor{Light: "#555555", Dark: "#555555"}).
			Padding(1, 3)

	execTitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.AdaptiveColor{Light: "#333333", Dark: "#cccccc"})

	execDimStyle = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#999999", Dark: "#666666"})

	execCommandStyle = lipgloss.NewStyle().
				Foreground(lipgloss.AdaptiveColor{Light: "#333333", Dark: "#cccccc"})

	warningStyle = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#b36b00", Dark: "#e5a44c"}).
			Padding(0, 1)
)

// topPad is the number of blank rows kept above top-left aligned content.
const topPad = 2

// maxCountDigits caps the vim-style numeric prefix. Longer input would
// overflow strconv.Atoi and silently fall back to a count of 1.
const maxCountDigits = 7

// FileChangedMsg is sent when the watched file changes on disk.
type FileChangedMsg struct {
	Slides []parser.Slide
	Meta   meta.Meta
}

// WarningMsg reports a non-fatal problem to the user. Because the TUI owns the
// screen, stderr is invisible, so warnings surface in the status bar instead.
type WarningMsg struct {
	Text string
}

// execDoneMsg is sent when code execution completes. run identifies the
// execution it belongs to, so a block that was cancelled and replaced by a
// newer run cannot overwrite the newer run's panel.
type execDoneMsg struct {
	run      int
	output   string
	err      error
	duration time.Duration
}

// execTickMsg refreshes the output panel while a block is still running.
type execTickMsg struct {
	run int
}

// clockTickMsg redraws the status bar so its timer keeps moving.
type clockTickMsg struct{}

// clockTick schedules the next redraw of the status bar timer.
func clockTick() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg { return clockTickMsg{} })
}

// execTickInterval is how often a running block's output is redrawn. Output
// is collected as it is written and drawn on this beat, so a chatty program
// cannot flood the event loop with one message per write.
const execTickInterval = 100 * time.Millisecond

// liveOutput collects a running program's output. The program writes to it
// from its own goroutine while the model reads it on every tick.
type liveOutput struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (l *liveOutput) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.Write(p)
}

func (l *liveOutput) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.String()
}

// Model is the Bubble Tea model for the presentation.
type Model struct {
	slides  []parser.Slide
	current int
	// currentStep is the progressive reveal shown on the current slide.
	currentStep int
	width       int
	height      int
	renderer    *render.Renderer
	meta        meta.Meta

	// Numeric prefix buffer for vim-style commands
	numBuf   string
	pendingG bool

	// Search state
	searchMode    bool
	searchInput   string
	searchResults []int
	searchIdx     int
	searchActive  bool

	// Search error (shown briefly in status bar)
	searchError string

	// Help overlay state
	helpVisible bool

	// warning holds the most recent non-fatal problem, shown in the status
	// bar until the next keypress.
	warning string

	// allowExec gates code execution. It is disabled for served sessions
	// unless explicitly enabled, since a viewer could otherwise run the
	// deck's code blocks on the presenter's machine.
	allowExec bool

	// Code execution state
	execVisible  bool
	execRunning  bool
	execOutput   string
	execErr      error
	execBlockIdx int
	execScroll   int
	execDuration time.Duration
	execCancel   context.CancelFunc
	// execRun numbers the executions, execLive receives the output of the
	// one in flight, and execStart is when it began.
	execRun   int
	execLive  *liveOutput
	execStart time.Time
	// execFollow keeps the panel on the newest output. It is on while a
	// block runs and goes off when the user scrolls back.
	execFollow bool
	// execLang and execCode are the block the panel is showing, kept so the
	// panel can show the command above its output and run it again.
	execLang string
	execCode string

	// timerStart is when the elapsed timer began: the first time the
	// presenter moved off the opening slide. Zero until then.
	timerStart time.Time
}

// New creates a new presentation model.
func New(slides []parser.Slide, baseDir string, m meta.Meta) Model {
	r := render.New(baseDir, 80, 24, m.Theme, m.HeadingColor)
	mdl := Model{
		slides:    slides,
		renderer:  r,
		meta:      m,
		allowExec: true,
	}
	if err := r.ThemeError(); err != nil {
		mdl.warning = err.Error()
	}
	return mdl
}

// WithSize returns a copy of the model with the terminal size applied. It is
// used by the SSH server, which knows the size before the first render.
//
// This is a value method on purpose: bubbletea stores and copies the model by
// value, so a pointer method here would mutate a copy in some call paths and
// the original in others.
func (m Model) WithSize(w, h int) Model {
	m.width = w
	m.height = h
	m.renderer.SetSize(w, h)
	return m
}

// WithExec returns a copy of the model with code execution enabled or
// disabled.
func (m Model) WithExec(allow bool) Model {
	m.allowExec = allow
	return m
}

// Init satisfies tea.Model. No initial command is needed.
func (m Model) Init() tea.Cmd {
	if m.meta.Timer != meta.TimerOff {
		return clockTick()
	}
	return nil
}

// Update handles all incoming messages and returns the updated model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case FileChangedMsg:
		timerWasOff := m.meta.Timer == meta.TimerOff
		m.slides = msg.Slides
		m.meta = msg.Meta
		if m.current >= len(m.slides) {
			m.current = len(m.slides) - 1
		}
		if m.current < 0 {
			m.current = 0
		}
		if len(m.slides) > 0 && m.currentStep >= m.slides[m.current].StepCount() {
			m.currentStep = m.slides[m.current].StepCount() - 1
		}
		// SetTheme also clears the cache, but only when the theme changed.
		m.renderer.SetTheme(msg.Meta.Theme, msg.Meta.HeadingColor)
		m.renderer.ClearCache()
		if err := m.renderer.ThemeError(); err != nil {
			m.warning = err.Error()
		}
		if timerWasOff && m.meta.Timer != meta.TimerOff {
			return m, clockTick()
		}
		return m, nil

	case WarningMsg:
		m.warning = msg.Text
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.renderer.SetSize(msg.Width, msg.Height)
		return m, nil

	case clockTickMsg:
		if m.meta.Timer == meta.TimerOff {
			return m, nil
		}
		return m, clockTick()

	case execTickMsg:
		if !m.execRunning || msg.run != m.execRun {
			return m, nil
		}
		if m.execLive != nil {
			m.execOutput = m.execLive.String()
		}
		return m, execTick(msg.run)

	case execDoneMsg:
		if msg.run != m.execRun {
			return m, nil
		}
		m.execRunning = false
		m.execOutput = msg.output
		m.execErr = msg.err
		m.execDuration = msg.duration
		m.execCancel = nil
		m.execLive = nil
		// Nobody watched a block that finished at once, so show its output
		// from the top. One that streamed stays on its last lines, where
		// the audience was already reading.
		if m.execFollow && msg.duration < time.Second {
			m.execFollow = false
			m.execScroll = 0
		}
		return m, nil

	case tea.KeyMsg:
		if m.helpVisible {
			return m.handleHelpKey(msg)
		}
		if m.execVisible {
			return m.handleExecKey(msg)
		}
		if m.searchMode {
			return m.handleSearchInput(msg)
		}
		next, cmd := m.handleNormalKey(msg)
		if nm, ok := next.(Model); ok {
			// The timer counts the talk, not the wait on the title slide
			// before it: it starts on the first move away from that slide.
			if nm.timerStart.IsZero() && nm.current > 0 {
				nm.timerStart = time.Now()
			}
			next = nm
		}
		return next, cmd

	}

	return m, nil
}

func (m Model) handleNormalKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	m.searchError = ""
	m.warning = ""

	// Accumulate digits into the numeric prefix buffer
	if len(key) == 1 && key[0] >= '0' && key[0] <= '9' {
		if len(m.numBuf) < maxCountDigits {
			m.numBuf += key
		}
		m.pendingG = false
		return m, nil
	}

	n := 1
	if m.numBuf != "" {
		parsed, err := strconv.Atoi(m.numBuf)
		if err == nil && parsed > 0 {
			n = parsed
		}
	}

	clearNum := key != "g"

	switch key {
	case "esc":
		if m.searchActive {
			m.searchActive = false
			m.searchResults = nil
			m.numBuf = ""
			return m, nil
		}
		return m, tea.Quit

	case "q", "ctrl+c":
		return m, tea.Quit

	case "right", "l", "n", " ", "enter", "pgdown", "down", "j":
		m.pendingG = false
		if m.numBuf == "" {
			m = m.advance()
		} else {
			m = m.navigateTo(m.current + n)
		}

	case "left", "h", "p", "backspace", "pgup", "up", "k", "N":
		m.pendingG = false
		if m.numBuf == "" {
			m = m.retreat()
		} else {
			m = m.navigateTo(m.current - n)
		}

	case "home":
		m.pendingG = false
		m = m.navigateTo(0)

	case "end":
		m.pendingG = false
		m = m.navigateTo(len(m.slides) - 1)

	case "g":
		if m.pendingG {
			m.pendingG = false
			m = m.navigateTo(0)
			clearNum = true
		} else {
			m.pendingG = true
		}

	case "G":
		m.pendingG = false
		if m.numBuf != "" {
			m = m.navigateTo(n - 1)
		} else {
			m = m.navigateTo(len(m.slides) - 1)
		}

	case "/":
		m.pendingG = false
		m.searchMode = true
		m.searchInput = ""

	case "ctrl+e":
		m.pendingG = false
		if !m.allowExec {
			m.searchError = "code execution is disabled"
			break
		}
		if len(m.slides) == 0 || m.current >= len(m.slides) {
			break
		}
		slide := m.slides[m.current]
		if len(slide.CodeBlocks) == 0 {
			m.searchError = "no code block on this slide"
			break
		}
		if m.execRunning {
			break
		}
		var cmd tea.Cmd
		m, cmd = m.startExec(slide.CodeBlocks[m.selectedBlock()])
		return m, cmd

	case "tab":
		// Choose which code block ctrl+e will run.
		m.pendingG = false
		if len(m.slides) == 0 || m.current >= len(m.slides) {
			break
		}
		// Step through the blocks that can run. A diagram or a config
		// sample is a code block too, but selecting it is never useful. On
		// a slide with nothing runnable, step through everything as before.
		blocks := m.slides[m.current].CodeBlocks
		choices := runnableBlocks(blocks)
		if len(choices) == 0 {
			for i := range blocks {
				choices = append(choices, i)
			}
		}
		if n := len(choices); n > 1 {
			pos := 0
			for i, idx := range choices {
				if idx == m.selectedBlock() {
					pos = i
				}
			}
			pos = (pos + 1) % n
			m.execBlockIdx = choices[pos]
			m.searchError = fmt.Sprintf("code block %d/%d selected", pos+1, n)
		}

	case "ctrl+n":
		m.pendingG = false
		if len(m.searchResults) > 0 {
			m.searchIdx = (m.searchIdx + 1) % len(m.searchResults)
			m = m.navigateTo(m.searchResults[m.searchIdx])
		}

	case "ctrl+p":
		m.pendingG = false
		if n := len(m.searchResults); n > 0 {
			m.searchIdx = (m.searchIdx - 1 + n) % n
			m = m.navigateTo(m.searchResults[m.searchIdx])
		}

	case "?":
		m.pendingG = false
		m.helpVisible = true

	default:
		m.pendingG = false
	}

	if clearNum {
		m.numBuf = ""
	}

	return m, nil
}

// startExec opens the output panel and runs a code block in it.
func (m Model) startExec(block parser.CodeBlock) (Model, tea.Cmd) {
	m.execVisible = true
	m.execRunning = true
	m.execOutput = ""
	m.execErr = nil
	m.execScroll = 0
	m.execDuration = 0
	m.execFollow = true
	m.execRun++
	m.execLive = &liveOutput{}
	m.execStart = time.Now()
	m.execLang = block.Language
	m.execCode = block.Code

	ctx, cancel := context.WithCancel(context.Background())
	m.execCancel = cancel
	return m, tea.Batch(
		m.execCmd(ctx, m.execRun, m.execLive, block.Language, block.Code),
		execTick(m.execRun),
	)
}

// execCmd returns a tea.Cmd that runs a code block asynchronously. The context
// lets the user cancel a long-running block by dismissing the panel.
//
// The block's output is copied to live while it runs; execTick redraws the
// panel from it until the execDoneMsg for the same run arrives.
func (m Model) execCmd(ctx context.Context, run int, live *liveOutput, language, code string) tea.Cmd {
	timeout := m.meta.ExecTimeoutDuration()
	return func() tea.Msg {
		start := time.Now()
		r := codeexec.RunStreaming(ctx, timeout, language, code, live)
		return execDoneMsg{run: run, output: r.Output, err: r.Err, duration: time.Since(start)}
	}
}

// execTick schedules the next redraw of a running block's output.
func execTick(run int) tea.Cmd {
	return tea.Tick(execTickInterval, func(time.Time) tea.Msg {
		return execTickMsg{run: run}
	})
}

// advance moves to the next reveal, or to the next slide once the current one
// is fully revealed.
func (m Model) advance() Model {
	if len(m.slides) == 0 {
		return m
	}
	if m.currentStep < m.slides[m.current].StepCount()-1 {
		m.currentStep++
		return m
	}
	if m.current >= len(m.slides)-1 {
		// Already at the end of the deck; stay put rather than letting
		// navigateTo clamp and reset the reveal.
		return m
	}
	return m.navigateTo(m.current + 1)
}

// retreat moves to the previous reveal, or back to the previous slide. It
// lands on that slide's final reveal: arriving at its first would hide content
// the audience has already seen.
func (m Model) retreat() Model {
	if len(m.slides) == 0 {
		return m
	}
	if m.currentStep > 0 {
		m.currentStep--
		return m
	}
	if m.current == 0 {
		return m
	}
	m = m.navigateTo(m.current - 1)
	m.currentStep = m.slides[m.current].StepCount() - 1
	return m
}

// navigateTo jumps to a slide, showing it from its first reveal. Counted
// motions, gg/G, home/end and search all land here, so they operate on slides
// rather than reveals.
func (m Model) navigateTo(target int) Model {
	if len(m.slides) == 0 {
		m.current = 0
		m.currentStep = 0
		return m
	}
	if target < 0 {
		target = 0
	}
	if target >= len(m.slides) {
		target = len(m.slides) - 1
	}
	if target != m.current {
		m.execBlockIdx = 0
	}
	m.current = target
	m.currentStep = 0
	return m
}

func (m Model) handleExecKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		if m.execCancel != nil {
			m.execCancel()
		}
		return m, tea.Quit

	case "esc", "q":
		// Cancel rather than just hide: otherwise a hung program keeps the
		// feature locked until its timeout expires.
		if m.execCancel != nil {
			m.execCancel()
			m.execCancel = nil
		}
		m.execVisible = false
		m.execRunning = false
		m.execOutput = ""
		m.execErr = nil
		m.execScroll = 0
		m.execFollow = false
		m.execLive = nil
		m.execLang = ""
		m.execCode = ""
		// Retire the run, so its tick and its result are both ignored.
		m.execRun++

	case "r", "ctrl+e":
		// Run the same block again, without going back to the slide. A
		// block that is still running is left alone.
		if !m.execRunning && m.execCode != "" {
			return m.startExec(parser.CodeBlock{Language: m.execLang, Code: m.execCode})
		}

	case "down", "j":
		m.execScroll++
	case "up", "k":
		m = m.stopFollowing()
		if m.execScroll > 0 {
			m.execScroll--
		}
	case "pgdown", " ":
		m.execScroll += m.execBodyHeight()
	case "pgup":
		m = m.stopFollowing()
		m.execScroll -= m.execBodyHeight()
		if m.execScroll < 0 {
			m.execScroll = 0
		}
	case "home", "g":
		m.execFollow = false
		m.execScroll = 0
	case "end", "G":
		m.execFollow = true
	}
	// Scrolling down to the last line picks the newest output up again.
	if !m.execFollow && m.execRunning {
		if maxScroll := m.execMaxScroll(); maxScroll > 0 && m.execScroll >= maxScroll {
			m.execFollow = true
		}
	}
	return m, nil
}

// stopFollowing pins the panel where it is, so scrolling back from the
// newest output starts from the lines on screen rather than from the top.
func (m Model) stopFollowing() Model {
	if m.execFollow {
		m.execFollow = false
		m.execScroll = m.execMaxScroll()
	}
	return m
}

// execMaxScroll is the scroll offset that shows the last line of output.
func (m Model) execMaxScroll() int {
	if over := len(m.execLines()) - m.execBodyHeight(); over > 0 {
		return over
	}
	return 0
}

// execBodyHeight is how many lines of output the panel can show.
func (m Model) execBodyHeight() int {
	// Terminal height, less the status bar, the panel border and padding,
	// and the title and hint lines.
	h := m.height - 1 - execBorderStyle.GetVerticalFrameSize() - 4
	// The command shown above the output, and the blank line under it.
	if n := len(m.execCommandLines()); n > 0 {
		h -= n + 1
	}
	if h < 1 {
		h = 1
	}
	return h
}

func (m Model) handleHelpKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc", "q", "?", "enter":
		m.helpVisible = false
	}
	return m, nil
}

func (m Model) handleSearchInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	switch key {
	case "enter":
		m.searchMode = false
		query := m.searchInput
		m.searchResults = nil
		m.searchIdx = 0
		// Clear the previous search outright; leaving it set would keep a
		// stale "[i/n]" indicator and make ctrl+n a silent no-op.
		m.searchActive = false

		if query == "" {
			return m, nil
		}

		if strings.HasSuffix(query, "/i") {
			query = "(?i)" + strings.TrimSuffix(query, "/i")
		}

		re, err := regexp.Compile(query)
		if err != nil {
			m.searchError = fmt.Sprintf("invalid regex: %v", err)
			return m, nil
		}
		m.searchError = ""

		for i, slide := range m.slides {
			if re.MatchString(slide.RawMarkdown) {
				m.searchResults = append(m.searchResults, i)
			}
		}

		if len(m.searchResults) == 0 {
			m.searchError = fmt.Sprintf("no matches: %s", m.searchInput)
			return m, nil
		}

		if len(m.searchResults) > 0 {
			m.searchActive = true
			jumped := false
			for i, idx := range m.searchResults {
				if idx >= m.current {
					m.searchIdx = i
					m = m.navigateTo(idx)
					jumped = true
					break
				}
			}
			if !jumped {
				m.searchIdx = 0
				m = m.navigateTo(m.searchResults[0])
			}
		}

	case "esc", "ctrl+c":
		m.searchMode = false
		m.searchInput = ""

	case "backspace":
		if n := len(m.searchInput); n > 0 {
			_, size := utf8.DecodeLastRuneInString(m.searchInput)
			m.searchInput = m.searchInput[:n-size]
		}

	default:
		// Match on the key type rather than its name: the previous string
		// check appended key names like "up" or "tab" into the query.
		switch msg.Type {
		case tea.KeyRunes:
			if !msg.Alt {
				m.searchInput += string(msg.Runes)
			}
		case tea.KeySpace:
			m.searchInput += " "
		}
	}

	return m, nil
}

// overlayMargin is the space left free on each side of the panel.
const overlayMargin = 2

// overlay wraps body in the shared bordered panel used by the exec output and
// help views.
func (m Model) overlay(body string) string {
	return execBorderStyle.Width(m.overlayInnerWidth()).Render(body)
}

// overlayInnerWidth is the panel width inside its border, padding included.
func (m Model) overlayInnerWidth() int {
	// Leave a two-cell margin on each side and use the rest. Command output
	// is tabular and usually 70 to 90 cells wide: a narrower panel wraps
	// every row of a table and makes it unreadable.
	panelW := m.width - 2*overlayMargin
	if panelW < 40 {
		panelW = 40
	}
	// Width() is the padded content width, so only the border is subtracted.
	// Deriving it from the style keeps this correct if the padding changes.
	innerW := panelW - execBorderStyle.GetHorizontalBorderSize()
	if innerW < 10 {
		innerW = 10
	}
	return innerW
}

// overlayTextWidth is how many cells of text fit on one line of the panel.
func (m Model) overlayTextWidth() int {
	w := m.overlayInnerWidth() - execBorderStyle.GetHorizontalPadding()
	if w < 1 {
		w = 1
	}
	return w
}

// renderHelpOverlay lists the keybindings.
func (m Model) renderHelpOverlay() string {
	rows := [][2]string{
		{"→ l j n space enter", "next reveal, then next slide"},
		{"← h k p backspace", "previous reveal, then previous slide"},
		{"[n]j / [n]k", "move n slides"},
		{"gg / home", "first slide"},
		{"G / end", "last slide"},
		{"[n]G", "jump to slide n"},
		{"/", "search (regex, /i for case-insensitive)"},
		{"ctrl+n / ctrl+p", "next / previous match"},
		{"esc", "clear search"},
		{"ctrl+e", "execute the selected code block"},
		{"r", "run the block again, from its output"},
		{"tab", "select another code block"},
		{"?", "toggle this help"},
		{"q ctrl+c", "quit"},
	}

	width := 0
	for _, r := range rows {
		if n := lipgloss.Width(r[0]); n > width {
			width = n
		}
	}

	var b strings.Builder
	b.WriteString(execTitleStyle.Render("Keybindings"))
	b.WriteString("\n\n")
	for i, r := range rows {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(r[0])
		b.WriteString(strings.Repeat(" ", width-lipgloss.Width(r[0])+3))
		b.WriteString(execDimStyle.Render(r[1]))
	}
	b.WriteString("\n\n")
	b.WriteString(execDimStyle.Render("press esc to dismiss"))
	return m.overlay(b.String())
}

// execStatus summarizes how the run finished.
func (m Model) execStatus() string {
	switch {
	case m.execRunning:
		return "running"
	case m.execErr == nil:
		return "exit 0"
	}
	var ee *exec.ExitError
	if errors.As(m.execErr, &ee) {
		return fmt.Sprintf("exit %d", ee.ExitCode())
	}
	return "failed"
}

// execLines is the text of the output panel, wrapped to its width: what the
// block has written so far while it runs, and all of it once it has exited.
//
// Wrapping happens here, before lines are counted. The panel wraps long
// lines anyway, and counting first lets a few long ones make the panel
// taller than the terminal, which cuts off its bottom border and hint.
func (m Model) execLines() []string {
	var body string
	switch {
	case m.execRunning && m.execOutput == "":
		body = execDimStyle.Render("Running...")
	case m.execRunning:
		body = strings.TrimRight(m.execOutput, "\n")
	case m.execErr != nil && m.execOutput == "":
		body = m.execErr.Error()
	default:
		body = strings.TrimRight(m.execOutput, "\n")
		if m.execErr != nil {
			body += "\n\n" + execDimStyle.Render(m.execErr.Error())
		}
	}
	return strings.Split(ansi.Wrap(body, m.overlayTextWidth(), ""), "\n")
}

// execCommandMaxLines caps how much of a block is repeated above its output.
// A command is a line or a few; a long block (a manifest in a heredoc) would
// otherwise leave no room for what it printed.
const execCommandMaxLines = 5

// execCommandLines is the block being run, as shown at the top of the output
// panel. The panel covers the slide, so without it the audience reads output
// with no sign of the command that produced it.
func (m Model) execCommandLines() []string {
	code := strings.Trim(strings.ReplaceAll(m.execCode, "\t", "    "), "\n")
	if strings.TrimSpace(code) == "" {
		return nil
	}
	// A shell block reads as something typed at a prompt. Other languages
	// are a program, not a command, and are shown as they are.
	first, rest := "", ""
	switch strings.ToLower(m.execLang) {
	case "bash", "sh":
		first, rest = "$ ", "  "
	}

	var out []string
	for i, line := range strings.Split(code, "\n") {
		prefix := rest
		if i == 0 {
			prefix = first
		}
		out = append(out, strings.Split(ansi.Wrap(prefix+line, m.overlayTextWidth(), ""), "\n")...)
	}
	if len(out) > execCommandMaxLines {
		hidden := len(out) - (execCommandMaxLines - 1)
		out = append(out[:execCommandMaxLines-1], fmt.Sprintf("%s… %d more lines", rest, hidden))
	}
	for i := range out {
		out[i] = execCommandStyle.Render(out[i])
	}
	return out
}

// renderExecOverlay builds the code execution output panel.
func (m Model) renderExecOverlay() string {
	title := "Output · " + m.execStatus()
	if m.execRunning {
		title = "Running"
		if !m.execStart.IsZero() {
			title += " · " + time.Since(m.execStart).Truncate(time.Second).String()
		}
	} else if m.execDuration > 0 {
		title += " · " + m.execDuration.Round(time.Millisecond).String()
	}

	// Scroll rather than letting View truncate the panel, which would cut off
	// the border and the dismiss hint before any of the output.
	lines := m.execLines()
	avail := m.execBodyHeight()
	var hints []string
	if len(lines) > avail {
		maxScroll := len(lines) - avail
		scroll := min(m.execScroll, maxScroll)
		if m.execFollow {
			scroll = maxScroll
		}
		lines = lines[scroll : scroll+avail]
		hints = append(hints,
			fmt.Sprintf("lines %d-%d of %d", scroll+1, scroll+avail, len(lines)+maxScroll),
			"↑/↓ scroll")
	}
	switch {
	case m.execRunning:
		hints = append(hints, "esc to cancel")
	case m.execCode != "":
		hints = append(hints, "r to run again", "esc to dismiss")
	default:
		hints = append(hints, "esc to dismiss")
	}

	out := execTitleStyle.Render(title) + "\n\n"
	if cmd := m.execCommandLines(); len(cmd) > 0 {
		out += strings.Join(cmd, "\n") + "\n\n"
	}
	out += strings.Join(lines, "\n") + "\n\n" +
		execDimStyle.Render(strings.Join(hints, " · "))

	return m.overlay(out)
}

// renderContent returns the rendered string for the current view state.
// This is called from View() on every render; the Renderer's internal cache
// avoids redundant glamour/image work.
func (m Model) renderContent() string {
	if m.helpVisible {
		return m.renderHelpOverlay()
	}
	if m.execVisible {
		return m.renderExecOverlay()
	}
	if len(m.slides) == 0 {
		return "No slides"
	}
	if m.current >= len(m.slides) {
		return "No slides"
	}
	return m.renderSlideStep(m.currentStep)
}

// renderSlideStep renders one reveal of the current slide, with the marker on
// the block ctrl+e runs.
func (m Model) renderSlideStep(step int) string {
	output, err := m.renderer.RenderStep(m.slides[m.current], step)
	if err != nil {
		return fmt.Sprintf("Error rendering slide: %v", err)
	}
	// Show which block ctrl+e will run on every slide that has one, also
	// when it is the only one: a marker that comes and goes between slides
	// reads as a glitch. Blocks that cannot run are never marked.
	if blocks := m.slides[m.current].CodeBlocks; m.allowExec && len(runnableBlocks(blocks)) > 0 {
		marker := lipgloss.NewStyle().Foreground(lipgloss.Color(m.meta.HeadingColor)).Render(selectionMarker)
		output = markSelectedBlock(output, blocks, m.selectedBlock(), marker)
	}
	return output
}

// runnableBlocks returns the indexes of the blocks ctrl+e can run.
func runnableBlocks(blocks []parser.CodeBlock) []int {
	var out []int
	for i, b := range blocks {
		if codeexec.Supported(b.Language) {
			out = append(out, i)
		}
	}
	return out
}

// selectedBlock is the index of the block ctrl+e runs on the current slide:
// the chosen one when it can run, otherwise the first that can. On a slide
// with nothing runnable it is the chosen block, so ctrl+e can still explain
// why that one cannot run.
func (m Model) selectedBlock() int {
	if len(m.slides) == 0 || m.current >= len(m.slides) {
		return 0
	}
	blocks := m.slides[m.current].CodeBlocks
	idx := m.execBlockIdx
	if idx >= len(blocks) {
		idx = 0
	}
	runnable := runnableBlocks(blocks)
	if len(runnable) == 0 {
		return idx
	}
	for _, r := range runnable {
		if r == idx {
			return idx
		}
	}
	return runnable[0]
}

// selectionMarker is drawn in the margin beside the selected code block.
const selectionMarker = "▌"

// markSelectedBlock draws marker in the left margin of every rendered line of
// the selected code block. It finds the blocks by walking the rendered slide
// in order and matching each source line of each block, so identical blocks
// on one slide are told apart by position. A block that cannot be found (it
// is behind a pause, or the renderer reshaped it) is left unmarked: the
// status bar still reports the selection.
func markSelectedBlock(rendered string, blocks []parser.CodeBlock, selected int, marker string) string {
	if selected < 0 || selected >= len(blocks) {
		return rendered
	}
	lines := strings.Split(rendered, "\n")
	plain := make([]string, len(lines))
	for i, line := range lines {
		plain[i] = ansi.Strip(line)
	}

	cursor := 0
	first, last := -1, -1
	for b, block := range blocks {
		start, end := -1, -1
		for _, src := range strings.Split(block.Code, "\n") {
			want := strings.TrimSpace(strings.ReplaceAll(src, "\t", "    "))
			if want == "" {
				continue
			}
			at := -1
			for i := cursor; i < len(plain); i++ {
				got := strings.TrimSpace(plain[i])
				// A long line is hard-wrapped by the renderer, so the
				// rendered line may hold only the start of the source line.
				if got != "" && (got == want || strings.HasPrefix(want, got)) {
					at = i
					break
				}
			}
			if at < 0 {
				return rendered
			}
			if start < 0 {
				start = at
			}
			end = at
			cursor = at + 1
		}
		if b == selected {
			first, last = start, end
			break
		}
	}
	if first < 0 {
		return rendered
	}

	// The marker goes in the code block's own margin, two cells left of its
	// least indented line, so the code itself does not move.
	indent := -1
	for i := first; i <= last; i++ {
		if strings.TrimSpace(plain[i]) == "" {
			continue
		}
		if lead := len(plain[i]) - len(strings.TrimLeft(plain[i], " ")); indent < 0 || lead < indent {
			indent = lead
		}
	}
	col := indent - 2
	if col < 0 {
		col = 0
	}
	for i := first; i <= last; i++ {
		line := lines[i]
		if lipgloss.Width(line) <= col {
			line += strings.Repeat(" ", col+1-lipgloss.Width(line))
		}
		lines[i] = ansi.Truncate(line, col, "") + marker + ansi.TruncateLeft(line, col+1, "")
	}
	return strings.Join(lines, "\n")
}

// View renders the full terminal output. It is a pure function of model state
// and must not mutate the receiver (value receiver by Bubble Tea convention).
func (m Model) View() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}

	rendered := m.renderContent()

	// Build status bar
	var leftParts []string
	if m.meta.Author != "" {
		leftParts = append(leftParts, m.meta.Author)
	}
	if m.meta.Date != "" {
		leftParts = append(leftParts, m.meta.Date)
	}
	if t := m.timerText(time.Now()); t != "" {
		leftParts = append(leftParts, t)
	}
	leftStatus := statusStyle.Render(strings.Join(leftParts, " - "))

	var rightStatus string
	if m.warning != "" {
		rightStatus = warningStyle.Render(m.warning)
	} else if m.searchError != "" {
		rightStatus = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#cc0000", Dark: "#ff6666"}).
			Padding(0, 1).
			Render(m.searchError)
	} else if m.searchMode {
		rightStatus = statusStyle.Render(fmt.Sprintf("/%s", m.searchInput))
	} else {
		paging := fmt.Sprintf(m.meta.Paging, m.current+1, len(m.slides))
		if len(m.slides) > 0 {
			if steps := m.slides[m.current].StepCount(); steps > 1 {
				paging += fmt.Sprintf(" (%d/%d)", m.currentStep+1, steps)
			}
		}
		if m.searchActive && len(m.searchResults) > 0 {
			paging += fmt.Sprintf(" [%d/%d]", m.searchIdx+1, len(m.searchResults))
		}
		rightStatus = statusStyle.Render(paging)
		// A slide that is cut off looks finished from the presenter's
		// seat. Say so, rather than dropping its last rows unannounced.
		if !m.execVisible && !m.helpVisible {
			if cut := m.rowsCutOff(); cut > 0 {
				rightStatus = warningStyle.Render(plural(cut, "row")+" cut off") + rightStatus
			}
		}
	}

	leftWidth := lipgloss.Width(leftStatus)
	rightWidth := lipgloss.Width(rightStatus)
	// On a narrow terminal the author/date is what gives way: the right-hand
	// segment carries position and messages, which matter more while
	// presenting. Truncating the whole line instead would drop it first.
	if leftWidth+rightWidth > m.width {
		room := m.width - rightWidth
		if room < 0 {
			room = 0
		}
		leftStatus = ansi.Truncate(leftStatus, room, "")
		leftWidth = lipgloss.Width(leftStatus)
	}
	gap := m.width - leftWidth - rightWidth
	if gap < 0 {
		gap = 0
	}
	statusLine := leftStatus + strings.Repeat(" ", gap) + rightStatus

	// Content area (leave 1 row for status bar)
	contentHeight := m.height - 1
	if contentHeight < 1 {
		contentHeight = 1
	}

	hPos := lipgloss.Center
	vPos := lipgloss.Center
	pad := 0
	if !m.execVisible && !m.helpVisible && m.meta.Align == "top-left" {
		hPos = lipgloss.Left
		vPos = lipgloss.Top
		// Add vertical padding so content doesn't stick to the very top.
		// The rows are reserved out of the content area rather than prepended
		// afterwards, which would push the last lines off the bottom.
		if contentHeight > topPad {
			pad = topPad
		}
	}

	if hPos == lipgloss.Center && !m.execVisible && !m.helpVisible {
		// The renderer pads every line to the wrap width, which is a fraction
		// of the terminal. Centering that padded block leaves a slide with
		// short lines visibly left of center on a wide terminal, so center
		// what is actually drawn instead.
		rendered = m.centeredSlide(rendered)
	}

	content := lipgloss.Place(m.width, contentHeight-pad, hPos, vPos, rendered)
	if pad > 0 {
		content = strings.Repeat("\n", pad) + content
	}

	// Ensure the output is always exactly m.height lines so Bubble Tea's
	// renderer overwrites every line and doesn't leave stale content.
	// Truncate any lines wider than the terminal to prevent wrapping.
	lines := strings.Split(content, "\n")
	if len(lines) > contentHeight {
		lines = lines[:contentHeight]
	}
	emptyLine := strings.Repeat(" ", m.width)
	for len(lines) < contentHeight {
		lines = append(lines, emptyLine)
	}
	for i := range lines {
		// len() is a cheap lower bound on display width: a line can only be
		// too wide if it also has more bytes than the terminal has columns.
		if len(lines[i]) <= m.width || lipgloss.Width(lines[i]) <= m.width {
			continue
		}
		lines[i] = ansi.Truncate(lines[i], m.width, "")
		// Truncation will not split a double-width glyph, so the result can
		// land one column short. Pad it back so no stale cell survives.
		if w := lipgloss.Width(lines[i]); w < m.width {
			lines[i] += strings.Repeat(" ", m.width-w)
		}
	}
	content = strings.Join(lines, "\n")

	if lipgloss.Width(statusLine) > m.width {
		statusLine = ansi.Truncate(statusLine, m.width, "")
	}

	return content + "\n" + statusLine
}

// timerText is the status bar timer at the given moment, or "" when the
// deck asks for none.
func (m Model) timerText(now time.Time) string {
	switch m.meta.Timer {
	case meta.TimerClock:
		return now.Format("15:04")
	case meta.TimerElapsed:
		var d time.Duration
		if !m.timerStart.IsZero() {
			d = now.Sub(m.timerStart)
		}
		if d < 0 {
			d = 0
		}
		sec := int(d.Seconds())
		if sec >= 3600 {
			return fmt.Sprintf("%d:%02d:%02d", sec/3600, sec/60%60, sec%60)
		}
		return fmt.Sprintf("%d:%02d", sec/60, sec%60)
	}
	return ""
}

// plural formats a count with a correctly pluralized noun.
func plural(n int, noun string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, noun)
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

// slideRows is how many rows a slide may use: the terminal, less the status
// bar and the padding kept above top-left aligned content.
func (m Model) slideRows() int {
	h := m.height - 1
	if h < 1 {
		h = 1
	}
	if m.meta.Align == "top-left" && h > topPad {
		h -= topPad
	}
	return h
}

// slideExtent is the space the current slide needs once fully revealed.
func (m Model) slideExtent() (rows, cols int) {
	if len(m.slides) == 0 || m.current >= len(m.slides) {
		return 0, 0
	}
	b, ok := contentBox(m.renderSlideStep(m.slides[m.current].StepCount() - 1))
	if !ok {
		return 0, 0
	}
	if m.meta.Align == "top-left" {
		// Nothing is trimmed in this layout, so the rows above the content
		// and the indentation left of it take up room as well.
		return b.first + b.height, b.indent + b.width
	}
	return b.height, b.width
}

// rowsCutOff is how many rows of the current slide fall below the terminal.
func (m Model) rowsCutOff() int {
	if m.width == 0 || m.height == 0 {
		return 0
	}
	rows, _ := m.slideExtent()
	if over := rows - m.slideRows(); over > 0 {
		return over
	}
	return 0
}

// centeredSlide prepares a rendered reveal for centering.
//
// A reveal is centered in the box the finished slide will occupy, not in its
// own. Centering each reveal by itself moves the heading up a little with
// every step, because the content under it keeps growing; the audience then
// watches the slide jump instead of watching the new line appear.
func (m Model) centeredSlide(rendered string) string {
	if len(m.slides) == 0 || m.current >= len(m.slides) {
		return trimToContent(rendered)
	}
	last := m.slides[m.current].StepCount() - 1
	if m.currentStep >= last {
		return trimToContent(rendered)
	}
	b, ok := contentBox(m.renderSlideStep(last))
	if !ok {
		return trimToContent(rendered)
	}
	return cropTo(rendered, b, true)
}

// box is the part of a rendered slide its visible text occupies.
type box struct {
	first  int // index of the first line with text
	height int // lines from the first to the last with text
	indent int // indentation every line with text shares
	width  int // cells from that indentation to the end of the widest line
}

// contentBox measures the box of a rendered slide. It reports false for a
// slide with no visible text.
func contentBox(s string) (box, bool) {
	lines := strings.Split(s, "\n")
	first, last := -1, -1
	indent, right := -1, 0
	for i, line := range lines {
		p := strings.TrimRight(ansi.Strip(line), " ")
		if p == "" {
			continue
		}
		if first < 0 {
			first = i
		}
		last = i
		if lead := len(p) - len(strings.TrimLeft(p, " ")); indent < 0 || lead < indent {
			indent = lead
		}
		if w := lipgloss.Width(p); w > right {
			right = w
		}
	}
	if first < 0 {
		return box{}, false
	}
	return box{first: first, height: last - first + 1, indent: indent, width: right - indent}, true
}

// cropTo cuts a rendered slide down to a box: the lines above and below it,
// the indentation left of it and the padding right of it are dropped. Lines
// keep their position relative to each other.
//
// With hold set the result always has the box's full height and width, even
// when s has less text than the slide the box was measured on, so that a
// partly revealed slide is placed exactly where the finished one will be.
func cropTo(s string, b box, hold bool) string {
	lines := strings.Split(s, "\n")
	out := make([]string, 0, b.height)
	for i := b.first; i < b.first+b.height; i++ {
		if i >= len(lines) {
			if !hold {
				break
			}
			out = append(out, "")
			continue
		}
		line := lines[i]
		if strings.TrimSpace(ansi.Strip(line)) == "" {
			out = append(out, "")
			continue
		}
		line = ansi.Truncate(line, b.indent+b.width, "")
		line = strings.TrimRight(ansi.TruncateLeft(line, b.indent, ""), " ")
		out = append(out, line)
	}
	if hold && len(out) > 0 {
		// One line of the full width is enough to keep the block as wide
		// as the finished slide when it is centered.
		if w := lipgloss.Width(out[0]); w < b.width {
			out[0] += strings.Repeat(" ", b.width-w)
		}
	}
	return strings.Join(out, "\n")
}

// Misfit describes a slide that does not fit a terminal of a given size.
type Misfit struct {
	Slide   int    // 1-based slide number
	Title   string // the slide's first line of text
	Rows    int    // rows the slide needs
	MaxRows int    // rows available for a slide
	Columns int    // columns the slide needs
	Wrapped int    // lines of code that are too long and get wrapped
}

// Fit lays out every slide for a terminal of the given size, exactly as the
// presentation would, and returns the slides that do not fit: too tall, too
// wide, or holding a line of code long enough to be wrapped.
func Fit(slides []parser.Slide, baseDir string, mt meta.Meta, width, height int) []Misfit {
	// Without execution no selection marker is drawn, so code lines can be
	// compared with their source as they are.
	m := New(slides, baseDir, mt).WithSize(width, height).WithExec(false)
	var out []Misfit
	for i, slide := range slides {
		m.current = i
		rows, cols := m.slideExtent()
		rendered := m.renderSlideStep(slide.StepCount() - 1)
		f := Misfit{
			Slide:   i + 1,
			Title:   slideTitle(slide),
			Rows:    rows,
			MaxRows: m.slideRows(),
			Columns: cols,
			Wrapped: wrappedCodeLines(slide, rendered),
		}
		if f.Rows > f.MaxRows || f.Columns > width || f.Wrapped > 0 {
			out = append(out, f)
		}
	}
	return out
}

// slideTitle is the first line of text on a slide, without heading markers.
func slideTitle(slide parser.Slide) string {
	fallback := ""
	for _, line := range strings.Split(slide.RawMarkdown, "\n") {
		t := strings.TrimSpace(line)
		if t == "" {
			continue
		}
		// Prefer a real title over a small label above it.
		if strings.HasPrefix(t, "#") && !strings.HasPrefix(t, "#####") {
			return strings.TrimSpace(strings.TrimLeft(t, "#"))
		}
		if fallback == "" {
			fallback = strings.TrimSpace(strings.TrimLeft(t, "#"))
		}
	}
	return fallback
}

// wrappedCodeLines counts the lines of a slide's code blocks that no longer
// appear whole in the rendered slide, which is what wrapping does to them. A
// wrapped line breaks a diagram or a table drawn in a code block.
func wrappedCodeLines(slide parser.Slide, rendered string) int {
	whole := make(map[string]bool)
	for _, line := range strings.Split(rendered, "\n") {
		whole[strings.TrimSpace(ansi.Strip(line))] = true
	}
	n := 0
	for _, b := range slide.CodeBlocks {
		for _, line := range strings.Split(b.Code, "\n") {
			t := strings.TrimSpace(strings.ReplaceAll(line, "\t", "    "))
			if t != "" && !whole[t] {
				n++
			}
		}
	}
	return n
}

// trimToContent shrinks a rendered slide to the box its visible text occupies:
// it drops blank lines above and below, the indentation every line shares,
// and the trailing padding. Lines keep their position relative to each other,
// so left-aligned text stays left-aligned inside the box.
func trimToContent(s string) string {
	b, ok := contentBox(s)
	if !ok {
		return s
	}
	return cropTo(s, b, false)
}
