# Scene: Install (README Quickstart step 1). The real install.sh into the sandbox HOME, then --version.
# Run through render.sh, which sets SKILLHUB_BIN and the output folder.
Output video/recordings/tapes/01-install.mp4
Source video/demos/tapes/settings.tape
# Machine label "Hub computer": drawn by Remotion (VHS has no window title setting).

Hide
Type "DEMO_FRESH=1 . video/demos/sandbox.sh /tmp/skillhub-tape-install 2>/dev/null && rm -f $DEMO_SANDBOX/bin/skillhub && clear"
Enter
Sleep 1s
Show

Type "curl -fsSL https://github.com/vantt/skillhub-mcp/releases/latest/download/install.sh | sh"
Sleep 600ms
Enter
Wait+Screen@90s /Next step/
Sleep 3s

# "Open a new terminal so it finds the command": the new terminal is simulated off camera.
Hide
Type 'export PATH="$HOME/.local/bin:$PATH" && clear'
Enter
Sleep 500ms
Show

Type "skillhub --version"
Sleep 400ms
Enter
Sleep 4s
