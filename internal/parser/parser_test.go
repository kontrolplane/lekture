package parser

import (
	"testing"
)

func TestParseContent_BasicSplit(t *testing.T) {
	content := `# Slide 1

Hello world

---

# Slide 2

Goodbye world`

	slides := ParseContent(content)
	if len(slides) != 2 {
		t.Fatalf("expected 2 slides, got %d", len(slides))
	}
	if slides[0].RawMarkdown != "# Slide 1\n\nHello world" {
		t.Errorf("slide 0 content = %q", slides[0].RawMarkdown)
	}
	if slides[1].RawMarkdown != "# Slide 2\n\nGoodbye world" {
		t.Errorf("slide 1 content = %q", slides[1].RawMarkdown)
	}
}

func TestParseContent_EmptySlides(t *testing.T) {
	content := `# Only slide

---

---

# Last slide`

	slides := ParseContent(content)
	if len(slides) != 2 {
		t.Fatalf("expected 2 slides (empty ones filtered), got %d", len(slides))
	}
}

func TestParseContent_ImageExtraction(t *testing.T) {
	content := `# Slide with image

![logo](images/logo.png)

Some text

![diagram](./arch.png)`

	slides := ParseContent(content)
	if len(slides) != 1 {
		t.Fatalf("expected 1 slide, got %d", len(slides))
	}
	if len(slides[0].Images) != 2 {
		t.Fatalf("expected 2 images, got %d", len(slides[0].Images))
	}

	img := slides[0].Images[0]
	if img.Alt != "logo" {
		t.Errorf("image 0 alt = %q, want %q", img.Alt, "logo")
	}
	if img.Path != "images/logo.png" {
		t.Errorf("image 0 path = %q, want %q", img.Path, "images/logo.png")
	}

	img2 := slides[0].Images[1]
	if img2.Alt != "diagram" {
		t.Errorf("image 1 alt = %q, want %q", img2.Alt, "diagram")
	}
}

func TestParseContent_NoImages(t *testing.T) {
	content := `# Plain slide

No images here.`

	slides := ParseContent(content)
	if len(slides[0].Images) != 0 {
		t.Errorf("expected 0 images, got %d", len(slides[0].Images))
	}
}

func TestParseContent_WindowsLineEndings(t *testing.T) {
	content := "# Slide 1\r\n\r\nHello\r\n---\r\n# Slide 2"

	slides := ParseContent(content)
	if len(slides) != 2 {
		t.Fatalf("expected 2 slides, got %d", len(slides))
	}
}

func TestParseContent_Indexes(t *testing.T) {
	content := `A

---

B

---

C`

	slides := ParseContent(content)
	for i, s := range slides {
		if s.Index != i {
			t.Errorf("slide %d has Index %d", i, s.Index)
		}
	}
}
