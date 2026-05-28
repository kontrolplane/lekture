# Lekture — terminal-based presentation tool
# Generates the main demo GIF for the README.

Output assets/cassette.gif

Require go

Set Shell "bash"
Set FontSize 16
Set Width 1200
Set Height 700
Set TypingSpeed 80ms
Set Theme "Catppuccin Mocha"
Set WindowBar Colorful
Set BorderRadius 8
Set Padding 20
Set LoopOffset 80%
Set Framerate 30

# Build and start the presentation
Hide
Type "go build -o vhs/lekture . && ./vhs/lekture example/presentation.md"
Enter
Sleep 2s
Show

# -- Slide 1: Title --
Sleep 3s

# -- Slide 2: Agenda --
Right
Sleep 3s

# -- Slide 3: What is a VPC? --
Right
Sleep 3s

# -- Slide 4: VPC Building Blocks --
Right
Sleep 3s

# -- Slide 5: Subnets in Practice --
Right
Sleep 3s

# -- Slide 6: CIDR Planning --
Right
Sleep 2s

# -- Slide 7: VPC-Attached Lambda --
Right
Sleep 2s

# Execute the Go Lambda handler
Ctrl+E
Sleep 4s

# Dismiss
Escape
Sleep 1s

# -- Slide 8: Security Groups --
Right
Sleep 3s

# -- Slide 9: NACLs --
Right
Sleep 3s

# -- Slide 10: Elastic Load Balancing --
Right
Sleep 3s

# -- Slide 11: CloudFront --
Right
Sleep 3s

# -- Slide 12: Route 53 --
Right
Sleep 3s

# -- Slide 13: DNS Records --
Right
Sleep 3s

# -- Demonstrate search --
Type "/"
Sleep 500ms
Type "subnet"
Sleep 500ms
Enter
Sleep 2s

# Next result
Ctrl+N
Sleep 2s

# Dismiss search
Escape
Sleep 1s

# -- Vim navigation --
Type "gg"
Sleep 2s

Shift+G
Sleep 2s

Type "10"
Sleep 300ms
Shift+G
Sleep 2s

# Exit
Type "q"
Sleep 1s
