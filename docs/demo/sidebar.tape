# The README's screenshot of `go run ./docs/demo sidebar`, made-up data.
# Regenerate them all with `just screenshots`.
Output "docs/images/sidebar.gif"

Require ksesh-demo

Set Shell zsh
Set FontFamily "FiraCode Nerd Font Mono"
Set FontSize 16
Set LineHeight 1.15
Set Width 470
Set Height 700
Set Padding 18
Set Theme "Catppuccin Mocha"
Set WindowBar Colorful
Set BorderRadius 10

Hide
Type "ksesh-demo sidebar" Enter
Sleep 2.5s
Show
Sleep 0.5s
Screenshot "docs/images/sidebar.png"
Sleep 1s
