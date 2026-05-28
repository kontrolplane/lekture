package model

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

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
)

// FileChangedMsg is sent when the watched file changes on disk.
type FileChangedMsg struct {
	Slides []parser.Slide
	Meta   meta.Meta
}

// execDoneMsg is sent when code execution completes.
type execDoneMsg struct {
	output string
	err    error
}

// Model is the Bubble Tea model for the presentation.
type Model struct {
	slides   []parser.Slide
	current  int
	width    int
	height   int
	renderer *render.Renderer
	meta     meta.Meta

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

	// Code execution state
	execVisible bool
	execRunning bool
	execOutput  string
	execErr     error
}

// New creates a new presentation model.
func New(slides []parser.Slide, baseDir string, m meta.Meta) Model {
	return Model{
		slides:   slides,
		renderer: render.New(baseDir, 80, 24, m.Theme, m.HeadingColor),
		meta:     m,
	}
}

// SetSize sets the initial terminal size (used by SSH server).
func (m *Model) SetSize(w, h int) {
	m.width = w
	m.height = h
	m.renderer.SetSize(w, h)
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
		m.renderer.ClearCache()
		return m, nil

	case execDoneMsg:
		m.execRunning = false
		m.execOutput = msg.output
		m.execErr = msg.err
		return m, nil

	case tea.KeyMsg:
		if m.execVisible {
			return m.handleExecKey(msg)
		}
		if m.searchMode {
			return m.handleSearchInput(msg)
		}
		return m.handleNormalKey(msg)

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.renderer.SetSize(msg.Width, msg.Height)
	}

	return m, nil
}

func (m Model) handleNormalKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	m.searchError = ""

	// Accumulate digits into the numeric prefix buffer
	if len(key) == 1 && key[0] >= '0' && key[0] <= '9' {
		m.numBuf += key
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
	case "q", "esc", "ctrl+c":
		if m.searchActive {
			m.searchActive = false
			m.searchResults = nil
			m.numBuf = ""
			return m, nil
		}
		return m, tea.Quit

	case "right", "l", "n", " ", "enter", "pgdown", "down", "j":
		m.pendingG = false
		m = m.navigateTo(m.current + n)

	case "left", "h", "p", "backspace", "pgup", "up", "k", "N":
		m.pendingG = false
		m = m.navigateTo(m.current - n)

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
		slide := m.slides[m.current]
		if len(slide.CodeBlocks) > 0 && !m.execRunning {
			block := slide.CodeBlocks[0]
			m.execVisible = true
			m.execRunning = true
			m.execOutput = ""
			m.execErr = nil
			return m, m.execCmd(block.Language, block.Code)
		}

	case "ctrl+n":
		m.pendingG = false
		if len(m.searchResults) > 0 {
			m.searchIdx = (m.searchIdx + 1) % len(m.searchResults)
			m = m.navigateTo(m.searchResults[m.searchIdx])
		}

	default:
		m.pendingG = false
	}

	if clearNum {
		m.numBuf = ""
	}

	return m, nil
}

// execCmd returns a tea.Cmd that runs a code block asynchronously.
func (m Model) execCmd(language, code string) tea.Cmd {
	return func() tea.Msg {
		r := codeexec.Run(language, code)
		return execDoneMsg{output: r.Output, err: r.Err}
	}
}

func (m Model) navigateTo(target int) Model {
	if target < 0 {
		target = 0
	}
	if target >= len(m.slides) {
		target = len(m.slides) - 1
	}
	m.current = target
	return m
}

func (m Model) handleExecKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.execVisible = false
		m.execOutput = ""
		m.execErr = nil
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
		if len(m.searchInput) > 0 {
			m.searchInput = m.searchInput[:len(m.searchInput)-1]
		}

	default:
		if len(key) == 1 || (len(key) > 1 && !strings.HasPrefix(key, "ctrl+")) {
			m.searchInput += msg.String()
		}
	}

	return m, nil
}

// renderExecOverlay builds the code execution output panel.
func (m Model) renderExecOverlay() string {
	var body string
	if m.execRunning {
		body = execDimStyle.Render("Running...")
	} else if m.execErr != nil && m.execOutput == "" {
		body = execTitleStyle.Render("Error") + "\n\n" + m.execErr.Error()
	} else {
		output := strings.TrimRight(m.execOutput, "\n")
		if m.execErr != nil {
			output += "\n\n" + execDimStyle.Render(m.execErr.Error())
		}
		body = execTitleStyle.Render("Output") + "\n\n" + output
	}

	hint := execDimStyle.Render("press esc to dismiss")
	body += "\n\n" + hint

	panelW := m.width * 4 / 5
	if panelW < 40 {
		panelW = 40
	}
	innerW := panelW - 14 // border (2) + padding (12)
	if innerW < 10 {
		innerW = 10
	}

	return execBorderStyle.Width(innerW).Render(body)
}

// renderContent returns the rendered string for the current view state.
// This is called from View() on every render; the Renderer's internal cache
// avoids redundant glamour/image work.
func (m Model) renderContent() string {
	if m.execVisible {
		return m.renderExecOverlay()
	}
	if len(m.slides) == 0 {
		return "No slides"
	}
	output, err := m.renderer.RenderSlide(m.slides[m.current])
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
	if m.searchError != "" {
		rightStatus = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#cc0000", Dark: "#ff6666"}).
			Padding(0, 1).
			Render(m.searchError)
	} else if m.searchMode {
		rightStatus = statusStyle.Render(fmt.Sprintf("/%s", m.searchInput))
	} else {
		paging := fmt.Sprintf(m.meta.Paging, m.current+1, len(m.slides))
		if m.searchActive && len(m.searchResults) > 0 {
			paging += fmt.Sprintf(" [%d/%d]", m.searchIdx+1, len(m.searchResults))
		}
		rightStatus = statusStyle.Render(paging)
	}

	leftWidth := lipgloss.Width(leftStatus)
	rightWidth := lipgloss.Width(rightStatus)
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
	if !m.execVisible && m.meta.Align == "top-left" {
		hPos = lipgloss.Left
		vPos = lipgloss.Top
		// Add vertical padding so content doesn't stick to the very top
		rendered = "\n\n" + rendered
	}

	content := lipgloss.Place(m.width, contentHeight, hPos, vPos, rendered)

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
		if lipgloss.Width(lines[i]) > m.width {
			lines[i] = ansi.Truncate(lines[i], m.width, "")
		}
	}
	content = strings.Join(lines, "\n")

	if lipgloss.Width(statusLine) > m.width {
		statusLine = ansi.Truncate(statusLine, m.width, "")
	}

	return content + "\n" + statusLine
}
