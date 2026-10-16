#!/usr/bin/env bash
# Run the documented happy path in a fresh demo sandbox and write one sanitized transcript per step.
#
#   video/demos/happy-path.sh [SANDBOX_DIR]
#
# Steps, in order (README "Quickstart", then docs/curating-skills.md):
#   version, Git identity, init, connect -g, doctor,
#   create your first skill (commit-style), write SKILL.md, publish it (draft + activate),
#   edit it (checkout, change SKILL.md, add a #! script), publish the change,
#   and with DEMO_NETWORK=1: add a skill from GitHub (read with --diff first), approve, route, activate.
# Left out on purpose: the curl install (the sandbox already has the binary under test), the agent
# prompts ("curate my Skill Hub", "use /commit-style"), which are filmed in Claude Code, and
# `serve --install-service`, which would install a real background service.
#
# Environment: DEMO_OUT (transcript folder), DEMO_NETWORK=1, DEMO_GITHUB_URL, DEMO_GITHUB_PATH,
# DEMO_GITHUB_SKILL, plus everything sandbox.sh reads (SKILLHUB_BIN, DEMO_REBUILD, ...).
# The sandbox is wiped and created again on every run; the web demo reuses it afterwards.
set -u

here=$(cd -P "$(dirname "${BASH_SOURCE[0]}")" && pwd) || exit 1
fixtures=$here/fixtures/commit-style
DEMO_FRESH=1
# shellcheck source=video/demos/sandbox.sh
. "$here/sandbox.sh" "${1:-${DEMO_SANDBOX:-${TMPDIR:-/tmp}/skillhub-demo}}" || exit 1
unset DEMO_FRESH
# shellcheck source=video/demos/lib.sh
. "$here/lib.sh"

command -v perl >/dev/null 2>&1 || demo_die "perl is required (it cleans the transcripts)"
command -v script >/dev/null 2>&1 || demo_die "script is required (it answers the y/N prompts in a terminal)"
mkdir -p "$DEMO_OUT" || demo_die "cannot create $DEMO_OUT"
rm -f "$DEMO_OUT"/[0-9][0-9]-*.txt "$DEMO_SANDBOX/run/sanitize.map"

# The answers of `skill create`, one per line; an empty line finishes a list.
create_answers=()
while IFS= read -r line || [ -n "$line" ]; do create_answers+=("$line"); done <"$fixtures/create-answers.txt"

skill_md=.claude/skills/commit-style/SKILL.md

# Step 2 of "Create your first skill": replace only the scaffold placeholder under Instructions (the
# marker comment and the paragraph under it), and add two requests under Examples.
write_instructions() {
	FIX_INSTRUCTIONS=$fixtures/instructions.md FIX_EXAMPLES=$fixtures/more-examples.md perl -0pi -e '
		sub slurp { open my $fh, "<", $_[0] or die "$_[0]: $!\n"; local $/; my $t = <$fh>; return $t }
		my $instr = slurp($ENV{FIX_INSTRUCTIONS});
		my $more = slurp($ENV{FIX_EXAMPLES});
		s/<!-- skillhub:scaffold -->\n(?:[^\n]+\n?)*/$instr/ or die "no scaffold placeholder in SKILL.md\n";
		s/(## Examples\n\n(?:- [^\n]*\n)+)/$1$more/ or die "no Examples list in SKILL.md\n";
	' "$skill_md"
}

# Step 2 of "Edit a skill": a longer summary line and a #! script, added without chmod.
edit_skill() {
	perl -pi -e 's/at most 50 characters/at most 72 characters/' "$skill_md" &&
		grep -q 'at most 72 characters' "$skill_md" &&
		mkdir -p .claude/skills/commit-style/scripts &&
		cp "$fixtures/scripts/trailer.sh" .claude/skills/commit-style/scripts/trailer.sh &&
		chmod -x .claude/skills/commit-style/scripts/trailer.sh
}

log_of() { printf '%s/logs/%02d-%s.log' "$DEMO_SANDBOX" "$DEMO_STEP_NO" "$1"; }

# --- README Quickstart -------------------------------------------------------------------------
demo_step version 'skillhub --version'
demo_step git-identity 'git config --global user.name "Demo User"
git config --global user.email "demo@example.com"
git config user.email'
demo_step init 'mkdir -p ~/skill-hub && cd ~/skill-hub && skillhub init'
demo_step connect 'skillhub connect -g'
demo_step doctor 'skillhub doctor'

# --- docs/curating-skills.md: Create your first skill ------------------------------------------
# <your-project> is a new Git repository; the docs ask to run curation commands there, not in the hub.
demo_step project 'mkdir -p ~/my-project && cd ~/my-project && git init'
demo_step skill-create 'skillhub skill create commit-style' "${create_answers[@]}"
write_instructions || demo_die "could not write the instructions into $skill_md"
demo_step skill-md "cat $skill_md"
demo_step skill-commit-new 'skillhub skill commit commit-style' y y
demo_step skill-list 'skillhub skill list --state active'
if ! grep -q 'commit-style' "$(log_of skill-list)"; then
	# A release whose first commit does not offer activation leaves a draft: activate it as its
	# output says, then check again.
	demo_step skill-activate 'skillhub skill activate commit-style' y
	demo_step skill-list 'skillhub skill list --state active'
	grep -q 'commit-style' "$(log_of skill-list)" || demo_die "commit-style is not active"
fi

# --- docs/curating-skills.md: Edit a skill and try it before others get it ---------------------
# The docs say to delete the folder once published; the edit then starts from a fresh checkout.
demo_step remove-folder 'rm -rf .claude/skills/commit-style'
demo_step skill-checkout 'skillhub skill checkout commit-style'
edit_skill || demo_die "could not edit the checkout of commit-style"
demo_step skill-edit "cat $skill_md .claude/skills/commit-style/scripts/trailer.sh"
demo_step skill-commit-change 'skillhub skill commit commit-style' y
mode=$(git -C "$HOME/skill-hub" ls-files -s | awk '/commit-style\/scripts\/trailer\.sh$/ {print $1}')
[ "$mode" = 100755 ] || demo_die "scripts/trailer.sh is not executable in the hub (mode '${mode:-missing}')"

# --- docs/curating-skills.md: Add a skill from GitHub (network) ---------------------------------
if [ "${DEMO_NETWORK:-0}" = 1 ]; then
	url=${DEMO_GITHUB_URL:-https://github.com/anthropics/skills}
	path=${DEMO_GITHUB_PATH:-skills/webapp-testing}
	id=${DEMO_GITHUB_SKILL:-webapp-testing}
	# The docs pipe the preview into `less`; without a terminal the command only shows it either way.
	demo_step add-read "skillhub skill add $url --path $path --diff"
	demo_step add "skillhub skill add $url --path $path" y y
	demo_step add-routing "skillhub skill edit $id --trigger \"test a local web app in a browser\" \\
  --not-for \"unit tests without a browser\" --example \"test my web app in a browser\" \\
  --example \"check the login page with Playwright\" --example \"take a screenshot of my local site\"" y
	demo_step add-activate "skillhub skill activate $id" y
	demo_step add-list 'skillhub skill list --state active'
	grep -q "$id" "$(log_of add-list)" || demo_die "$id is not active"
fi

demo_check_clean "$DEMO_OUT" || demo_die "a transcript still holds private data; see above"
printf 'Transcripts: %s (%d steps). Sandbox kept for the web demo: %s\n' \
	"$DEMO_OUT" "$DEMO_STEP_NO" "$DEMO_SANDBOX" >&2
