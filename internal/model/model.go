package model

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
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
			Padding(1, 6)

	execTitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.AdaptiveColor{Light: "#333333", Dark: "#cccccc"})

	execDimStyle = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#999999", Dark: "#666666"})

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

// execDoneMsg is sent when code execution completes.
type execDoneMsg struct {
	output   string
	err      error
	duration time.Duration
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
	// deck'''s code blocks on the presenter'''s machine.
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
	return nil
}

// Update handles all incoming messages and returns the updated model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case FileChangedMsg:
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
		return m, nil

	case WarningMsg:
		m.warning = msg.Text
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.renderer.SetSize(msg.Width, msg.Height)
		return m, nil

	case execDoneMsg:
		m.execRunning = false
		m.execOutput = msg.output
		m.execErr = msg.err
		m.execDuration = msg.duration
		m.execCancel = nil
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
		return m.handleNormalKey(msg)

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
		if m.execBlockIdx >= len(slide.CodeBlocks) {
			m.execBlockIdx = 0
		}
		block := slide.CodeBlocks[m.execBlockIdx]
		m.execVisible = true
		m.execRunning = true
		m.execOutput = ""
		m.execErr = nil
		m.execScroll = 0
		m.execDuration = 0

		ctx, cancel := context.WithCancel(context.Background())
		m.execCancel = cancel
		return m, m.execCmd(ctx, block.Language, block.Code)

	case "tab":
		// Choose which code block ctrl+e will run.
		m.pendingG = false
		if len(m.slides) == 0 || m.current >= len(m.slides) {
			break
		}
		if n := len(m.slides[m.current].CodeBlocks); n > 1 {
			m.execBlockIdx = (m.execBlockIdx + 1) % n
			m.searchError = fmt.Sprintf("code block %d/%d selected", m.execBlockIdx+1, n)
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

// execCmd returns a tea.Cmd that runs a code block asynchronously. The context
// lets the user cancel a long-running block by dismissing the panel.
func (m Model) execCmd(ctx context.Context, language, code string) tea.Cmd {
	return func() tea.Msg {
		start := time.Now()
		r := codeexec.RunContext(ctx, language, code)
		return execDoneMsg{output: r.Output, err: r.Err, duration: time.Since(start)}
	}
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

	case "down", "j":
		m.execScroll++
	case "up", "k":
		if m.execScroll > 0 {
			m.execScroll--
		}
	case "pgdown", " ":
		m.execScroll += m.execBodyHeight()
	case "pgup":
		m.execScroll -= m.execBodyHeight()
		if m.execScroll < 0 {
			m.execScroll = 0
		}
	case "home", "g":
		m.execScroll = 0
	}
	return m, nil
}

// execBodyHeight is how many lines of output the panel can show.
func (m Model) execBodyHeight() int {
	// Terminal height, less the status bar, the panel border and padding,
	// and the title and hint lines.
	h := m.height - 1 - execBorderStyle.GetVerticalFrameSize() - 4
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

// overlay wraps body in the shared bordered panel used by the exec output and
// help views.
func (m Model) overlay(body string) string {
	panelW := m.width * 4 / 5
	if panelW < 40 {
		panelW = 40
	}
	// Width() is the padded content width, so only the border is subtracted.
	// Deriving it from the style keeps this correct if the padding changes.
	innerW := panelW - execBorderStyle.GetHorizontalBorderSize()
	if innerW < 10 {
		innerW = 10
	}
	return execBorderStyle.Width(innerW).Render(body)
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

// renderExecOverlay builds the code execution output panel.
func (m Model) renderExecOverlay() string {
	title := "Output"
	if m.execRunning {
		title = "Running"
	}
	title += " · " + m.execStatus()
	if m.execDuration > 0 {
		title += " · " + m.execDuration.Round(time.Millisecond).String()
	}

	var body string
	switch {
	case m.execRunning:
		body = execDimStyle.Render("Running...")
	case m.execErr != nil && m.execOutput == "":
		body = m.execErr.Error()
	default:
		body = strings.TrimRight(m.execOutput, "\n")
		if m.execErr != nil {
			body += "\n\n" + execDimStyle.Render(m.execErr.Error())
		}
	}

	// Scroll rather than letting View truncate the panel, which would cut off
	// the border and the dismiss hint before any of the output.
	lines := strings.Split(body, "\n")
	avail := m.execBodyHeight()
	hint := "press esc to dismiss"
	if len(lines) > avail {
		maxScroll := len(lines) - avail
		scroll := min(m.execScroll, maxScroll)
		lines = lines[scroll : scroll+avail]
		hint = fmt.Sprintf("lines %d-%d of %d · ↑/↓ scroll · esc to dismiss",
			scroll+1, scroll+avail, len(lines)+maxScroll)
	}

	out := execTitleStyle.Render(title) + "\n\n" +
		strings.Join(lines, "\n") + "\n\n" +
		execDimStyle.Render(hint)

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
	output, err := m.renderer.RenderStep(m.slides[m.current], m.currentStep)
	if err != nil {
		return fmt.Sprintf("Error rendering slide: %v", err)
	}
	return output
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
