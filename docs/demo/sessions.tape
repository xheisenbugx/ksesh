# The README's screenshot of `go run ./docs/demo sessions`, made-up data.
# Regenerate them all with `just screenshots`.
Output "docs/images/sessions.gif"

Require ksesh-demo

Set Shell zsh
Set FontFamily "FiraCode Nerd Font Mono"
Set FontSize 16
Set LineHeight 1.15
Set Width 1280
Set Height 620
Set Padding 18
Set Theme "Catppuccin Mocha"
Set WindowBar Colorful
Set BorderRadius 10

Hide
Type "ksesh-demo sessions" Enter
Sleep 2.5s
Show
Sleep 0.5s
Screenshot "docs/images/sessions.png"
Sleep 1s
