package parser

import (
	"regexp"
	"strings"
)

var (
	// imageRegex matches a markdown image. The destination allows one level of
	// balanced parentheses so filenames like "Screenshot (1).png" work; an
	// optional title is stripped afterwards by trimTitle.
	imageRegex = regexp.MustCompile(`!\[([^\]]*)\]\(([^()]*(?:\([^()]*\)[^()]*)*)\)`)

	// titleRegex matches a trailing link title, e.g. ` "a caption"`.
	titleRegex = regexp.MustCompile(`\s+("[^"]*"|'[^']*')\s*$`)

	// separatorRegex matches a slide separator: a line of exactly three
	// dashes, with optional trailing whitespace.
	separatorRegex = regexp.MustCompile(`^---[ \t]*$`)

	// fenceRegex matches an opening or closing code fence, capturing the
	// indent, the fence characters, and the info string.
	fenceRegex = regexp.MustCompile("^([ \t]{0,3})(`{3,}|~{3,})[ \t]*(.*)$")

	// pauseRegex matches a reveal marker on a line of its own.
	pauseRegex = regexp.MustCompile(`(?i)^[ \t]*<!--[ \t]*pause[ \t]*-->[ \t]*$`)
)

// ImageRef represents a markdown image reference found in a slide.
type ImageRef struct {
	Alt         string
	Path        string
	Placeholder string // the full markdown text, e.g. "![alt](path)"
}

// CodeBlock represents a fenced code block found in a slide.
type CodeBlock struct {
	Language string
	Code     string
}

// Slide represents a single presentation slide.
type Slide struct {
	// RawMarkdown is the whole slide, equal to the last entry of Steps.
	RawMarkdown string
	// Steps holds the progressive reveals of the slide, each a cumulative
	// prefix of the content. A slide without "<!-- pause -->" markers has
	// exactly one step.
	Steps      []string
	Images     []ImageRef
	CodeBlocks []CodeBlock
	Index      int
}

// StepCount is the number of reveals on the slide, always at least one.
func (s Slide) StepCount() int {
	if len(s.Steps) == 0 {
		return 1
	}
	return len(s.Steps)
}

// Step returns the markdown to render for the given reveal, clamped into
// range.
func (s Slide) Step(i int) string {
	if len(s.Steps) == 0 {
		return s.RawMarkdown
	}
	if i < 0 {
		i = 0
	}
	if i >= len(s.Steps) {
		i = len(s.Steps) - 1
	}
	return s.Steps[i]
}

// ParseContent splits markdown content into slides on "---" separators
// and extracts image references from each slide.
//
// Splitting is fence-aware: a "---" inside a fenced code block is content, not
// a separator, so a slide can show YAML frontmatter or a markdown snippet
// without being torn in two.
func ParseContent(content string) []Slide {
	// Normalize line endings
	content = strings.ReplaceAll(content, "\r\n", "\n")

	var slides []Slide
	var buf []string

	flush := func() {
		md := strings.TrimSpace(strings.Join(buf, "\n"))
		buf = buf[:0]
		if md == "" {
			return
		}
		steps := splitSteps(md)
		full := steps[len(steps)-1]
		slides = append(slides, Slide{
			RawMarkdown: full,
			Steps:       steps,
			Images:      extractImages(full),
			CodeBlocks:  extractCodeBlocks(full),
			Index:       len(slides),
		})
	}

	var fence string // the open fence marker, empty when not in a code block
	for _, line := range strings.Split(content, "\n") {
		if fence == "" {
			if m := fenceRegex.FindStringSubmatch(line); m != nil {
				fence = m[2]
				buf = append(buf, line)
				continue
			}
			if separatorRegex.MatchString(line) {
				flush()
				continue
			}
		} else if closesFence(line, fence) {
			fence = ""
		}
		buf = append(buf, line)
	}
	flush()

	return slides
}

// splitSteps divides a slide at "<!-- pause -->" markers into cumulative
// reveals. Splitting is fence-aware and only ever cuts at a marker line, so
// every step is a complete, independently renderable markdown document — no
// step can end inside an unclosed code fence.
func splitSteps(md string) []string {
	var (
		steps []string
		buf   []string
		fence string
	)
	for _, line := range strings.Split(md, "\n") {
		if fence == "" {
			if m := fenceRegex.FindStringSubmatch(line); m != nil {
				fence = m[2]
			} else if pauseRegex.MatchString(line) {
				steps = append(steps, strings.TrimRight(strings.Join(buf, "\n"), "\n"))
				continue
			}
		} else if closesFence(line, fence) {
			fence = ""
		}
		buf = append(buf, line)
	}
	steps = append(steps, strings.TrimRight(strings.Join(buf, "\n"), "\n"))

	// Drop leading empty reveals, which a marker before any content produces.
	for len(steps) > 1 && strings.TrimSpace(steps[0]) == "" {
		steps = steps[1:]
	}
	return steps
}

// closesFence reports whether line closes a code block opened with open.
// A closing fence uses the same character and is at least as long.
func closesFence(line, open string) bool {
	m := fenceRegex.FindStringSubmatch(line)
	if m == nil {
		return false
	}
	// A closing fence carries no info string.
	if strings.TrimSpace(m[3]) != "" {
		return false
	}
	return m[2][0] == open[0] && len(m[2]) >= len(open)
}

// extractCodeBlocks returns the fenced code blocks in a slide. The language is
// the first word of the info string, so attributes such as
// "```go {highlight=1}" still resolve to "go".
func extractCodeBlocks(markdown string) []CodeBlock {
	var blocks []CodeBlock

	var (
		fence string
		lang  string
		body  []string
	)
	for _, line := range strings.Split(markdown, "\n") {
		if fence == "" {
			if m := fenceRegex.FindStringSubmatch(line); m != nil {
				fence = m[2]
				lang = strings.TrimSpace(m[3])
				if i := strings.IndexAny(lang, " \t{"); i >= 0 {
					lang = lang[:i]
				}
				body = nil
			}
			continue
		}
		if closesFence(line, fence) {
			blocks = append(blocks, CodeBlock{
				Language: lang,
				Code:     strings.TrimRight(strings.Join(body, "\n"), "\n"),
			})
			fence = ""
			continue
		}
		body = append(body, line)
	}

	return blocks
}

func extractImages(markdown string) []ImageRef {
	matches := imageRegex.FindAllStringSubmatch(markdown, -1)
	var refs []ImageRef
	for _, m := range matches {
		refs = append(refs, ImageRef{
			Alt:         m[1],
			Path:        strings.TrimSpace(titleRegex.ReplaceAllString(m[2], "")),
			Placeholder: m[0],
		})
	}
	return refs
}
