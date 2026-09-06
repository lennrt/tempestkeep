# Real Go MCP client -> stdio -> TempestKeep; synthetic WeatherFlow data.
# Run make demo-agent. The script owns readiness, isolation, and cleanup.
Output docs/agent.gif
Output docs/agent.mp4

Set Shell bash
Set FontSize 18
Set FontFamily "monospace"
Set Width 1100
Set Height 700
Set Padding 24
Set Theme "Catppuccin Mocha"
Set WindowBar Colorful
Set BorderRadius 12
Set Framerate 15
Set TypingSpeed 35ms
Set WaitTimeout 120s

Hide
Type@0ms `export PS1='> ' COLORTERM=truecolor; unset NO_COLOR; clear`
Enter
Type@0ms `./scripts/demo-agent.sh -scenes -pace 350ms -hold 3s`
Enter
Wait+Screen /01.*DISCOVER/
Show
Wait+Screen /Temporary demo archive removed/
Sleep 3s
