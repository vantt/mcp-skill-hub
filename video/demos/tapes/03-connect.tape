# Scene: Connect your agent and check (README steps 3 and 4). Needs 02-create-hub.tape's sandbox.
# doctor runs in a project folder: inside ~/skill-hub it adds "not used here" lines that confuse.
Output video/recordings/tapes/03-connect.mp4
Source video/demos/tapes/settings.tape
# Machine label "Hub computer": drawn by Remotion (VHS has no window title setting).

Hide
Type ". video/demos/sandbox.sh /tmp/skillhub-tape-a 2>/dev/null && cd ~/skill-hub && clear"
Enter
Sleep 1s
Show

Type "skillhub connect -g"
Sleep 400ms
Enter
Wait+Screen@30s /curate my Skill Hub/
Sleep 7s

Hide
Type "mkdir -p ~/my-project && cd ~/my-project && git init -q && clear"
Enter
Sleep 500ms
Show

Type "cd ~/my-project && skillhub doctor"
Sleep 400ms
Enter
Sleep 3s
Sleep 6s
