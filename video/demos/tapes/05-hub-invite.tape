# Scene: Hub and agent on different computers, hub side (README): a one-time code for the agent
# computer. render.sh prepares the hub in /tmp/skillhub-tape-b and starts `skillhub serve` there
# off camera (`serve --install-service` would install a real background service).
Output video/recordings/tapes/05-hub-invite.mp4
Output video/recordings/tapes/05-hub-invite.txt
Source video/demos/tapes/settings.tape
# Machine label "Hub computer": drawn by Remotion (VHS has no window title setting).

Hide
Type ". video/demos/sandbox.sh /tmp/skillhub-tape-b 2>/dev/null && cd ~/skill-hub && clear"
Enter
Sleep 1s
Show

Type "skillhub token invite"
Sleep 400ms
Enter
Sleep 3s
Sleep 7s
