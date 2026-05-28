# Lekture — screenshot captures for README.
# Generates individual PNG screenshots of key slides.

Require go

Set Shell "bash"
Set FontSize 16
Set Width 1200
Set Height 700
Set Theme "Catppuccin Mocha"
Set WindowBar Colorful
Set BorderRadius 8
Set Padding 20

# Build and start the presentation
Hide
Type "go build -o vhs/lekture . && ./vhs/lekture example/presentation.md"
Enter
Sleep 2s
Show

# -- Screenshot 1: Title slide --
Sleep 1s
Screenshot assets/pages/slide/overview.png
Sleep 500ms

# Navigate to slide 7 (VPC-Attached Lambda)
Hide
Right
Right
Right
Right
Right
Right
Show
Sleep 1s

# Execute code block
Ctrl+E
Sleep 4s

# -- Screenshot 2: Code execution --
Screenshot assets/pages/slide/code-execution.png
Sleep 500ms

# Exit
Escape
Sleep 500ms
Type "q"
Sleep 500ms
