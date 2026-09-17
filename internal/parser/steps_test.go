package parser

import (
	"strings"
	"testing"
)

func TestSlideWithoutPauseHasOneStep(t *testing.T) {
	slides := ParseContent("# one\n\nbody")
	if got := slides[0].StepCount(); got != 1 {
		t.Errorf("StepCount = %d, want 1", got)
	}
	if slides[0].Step(0) != slides[0].RawMarkdown {
		t.Error("the only step should be the whole slide")
	}
}

func TestPauseSplitsIntoCumulativeSteps(t *testing.T) {
	md := "# title\n\n- one\n\n<!-- pause -->\n\n- two\n\n<!-- pause -->\n\n- three"
	slides := ParseContent(md)
	s := slides[0]

	if s.StepCount() != 3 {
		t.Fatalf("StepCount = %d, want 3", s.StepCount())
	}
	for i, want := range []string{"one", "two", "three"} {
		step := s.Step(i)
		if !strings.Contains(step, want) {
			t.Errorf("step %d is missing %q:\n%s", i, want, step)
		}
	}
	// Steps are cumulative: later steps keep earlier content.
	if strings.Contains(s.Step(0), "two") {
		t.Error("step 0 should not contain later content")
	}
	if !strings.Contains(s.Step(2), "one") {
		t.Error("the final step should contain everything")
	}
	if s.Step(s.StepCount()-1) != s.RawMarkdown {
		t.Error("the last step should equal the whole slide")
	}
}

// TestPauseInsideCodeFenceIsContent guards the same class of bug as the slide
// separator: a marker shown inside a code sample must not split the slide.
func TestPauseInsideCodeFenceIsContent(t *testing.T) {
	md := "# demo\n\n```html\n<!-- pause -->\n```\n"
	s := ParseContent(md)[0]
	if s.StepCount() != 1 {
		t.Errorf("StepCount = %d, want 1 — the marker was inside a fence", s.StepCount())
	}
}

func TestEveryStepIsRenderableMarkdown(t *testing.T) {
	// A pause after a fence must not leave an unclosed fence in an earlier step.
	md := "# demo\n\n```go\nx := 1\n```\n\n<!-- pause -->\n\ndone"
	s := ParseContent(md)[0]
	for i := 0; i < s.StepCount(); i++ {
		if n := strings.Count(s.Step(i), "```"); n%2 != 0 {
			t.Errorf("step %d has an unbalanced code fence:\n%s", i, s.Step(i))
		}
	}
}

func TestStepClampsOutOfRange(t *testing.T) {
	s := ParseContent("# a\n\n<!-- pause -->\n\nb")[0]
	if s.Step(-5) != s.Step(0) {
		t.Error("negative step should clamp to the first")
	}
	if s.Step(99) != s.Step(s.StepCount()-1) {
		t.Error("large step should clamp to the last")
	}
}

func TestPauseVariants(t *testing.T) {
	for _, marker := range []string{"<!-- pause -->", "<!--pause-->", "<!--  PAUSE  -->", "  <!-- pause -->  "} {
		md := "a\n\n" + marker + "\n\nb"
		if got := ParseContent(md)[0].StepCount(); got != 2 {
			t.Errorf("marker %q gave %d steps, want 2", marker, got)
		}
	}
}

func TestPauseIsStrippedFromRenderedContent(t *testing.T) {
	s := ParseContent("a\n\n<!-- pause -->\n\nb")[0]
	for i := 0; i < s.StepCount(); i++ {
		if strings.Contains(s.Step(i), "pause") {
			t.Errorf("step %d still contains the marker:\n%s", i, s.Step(i))
		}
	}
}
