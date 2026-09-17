# Lekture — screenshot captures for the README.
#
# Slide map (example/presentation.md):
#   1 title           4 what is a vpc?     8 vpc-attached lambda (2 code blocks)
#   2 agenda (3 reveals)                  18 thank you
#   3 how this deck is written
#
# Right advances a reveal before it advances a slide, so the agenda needs
# three presses to pass.

Require go

Set FontSize 16
Set Width 1280
Set Height 800
Set Padding 20

Hide
Type "go build -o vhs/lekture . && ./vhs/lekture example/presentation.md"
Enter
Sleep 2s
Show

# -- Screenshot 1: a slide revealed in stages --
# slide 1 -> 2 (first reveal), then one more to reveal the middle pair.
Right
Sleep 1s
Right
Sleep 1500ms
Screenshot assets/pages/slide/reveal.png
Sleep 500ms

# -- Screenshot 2: slide overview --
# finish the agenda (1 press), past "how this deck is written" (1), to slide 4.
Hide
Right
Right
Show
Right
Sleep 1500ms
Screenshot assets/pages/slide/overview.png
Sleep 500ms

# -- Screenshot 3: code block with syntax highlighting --
# slides 5, 6, 7, then 8.
Hide
Right
Right
Right
Right
Show
Sleep 1500ms
Screenshot assets/pages/slide/code-block.png
Sleep 500ms

# -- Screenshot 4: code execution --
Ctrl+E
Sleep 4s
Screenshot assets/pages/slide/code-execution.png
Sleep 500ms
Escape
Sleep 500ms

# -- Screenshot 5: the keymap overlay --
Type "?"
Sleep 1500ms
Screenshot assets/pages/slide/help.png
Sleep 500ms
Escape
Sleep 500ms

Type "q"
Sleep 500ms
