# Lekture — terminal-based presentation tool
# Generates the main demo GIF for the README.

Output assets/cassette.gif

Require go

Set FontSize 16
Set Width 1280
Set Height 800
Set Padding 20

# Build and start the presentation
Hide
Type "go build -o vhs/lekture . && ./vhs/lekture example/presentation.md"
Enter
Sleep 2s
Show

# -- Slide 1: Title --
Sleep 5s

# -- Slide 2: Agenda --
Right
Sleep 5s

# -- Slide 3: What is a VPC? --
Right
Sleep 5s

# -- Slide 4: VPC Building Blocks --
Right
Sleep 5s

# -- Slide 5: Subnets in Practice --
Right
Sleep 5s

# -- Slide 6: CIDR Planning --
Right
Sleep 4s

# -- Slide 7: VPC-Attached Lambda --
Right
Sleep 4s

# Execute the Go Lambda handler
Ctrl+E
Sleep 6s

# Dismiss
Escape
Sleep 2s

# -- Slide 8: Security Groups --
Right
Sleep 5s

# -- Slide 9: NACLs --
Right
Sleep 5s

# -- Slide 10: Elastic Load Balancing --
Right
Sleep 5s

# -- Slide 11: CloudFront --
Right
Sleep 5s

# -- Slide 12: Route 53 --
Right
Sleep 5s

# -- Slide 13: DNS Records --
Right
Sleep 5s

# -- Demonstrate search --
Type "/"
Sleep 1s
Type "subnet"
Sleep 1s
Enter
Sleep 3s

# Next result
Ctrl+N
Sleep 3s

# Dismiss search
Escape
Sleep 2s

# -- Vim navigation --
Type "gg"
Sleep 3s

Shift+G
Sleep 3s

Type "10"
Sleep 500ms
Shift+G
Sleep 3s

# Exit
Type "q"
Sleep 2s
