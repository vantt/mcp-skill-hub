# shellcheck shell=bash
# Helpers shared by the demo scripts. Source it after sandbox.sh:
#
#   demo_step <slug> <command> [answer...]   run one step, write NN-<slug>.txt, stop on failure
#   demo_pty <command> [answer...]           run a command in a pseudo-terminal, typing the answers
#   demo_sanitize                            filter stdin to stdout: no paths, tokens, hosts or hashes
#   demo_check_clean <dir>                   fail when a file in <dir> still holds a private string
#
# Transcripts go to $DEMO_OUT (default: <repo>/video/recordings/transcripts). Raw output stays in
# $DEMO_SANDBOX/logs, which is never committed or shown.

DEMO_OUT=${DEMO_OUT:-$DEMO_REPO/video/recordings/transcripts}
DEMO_STEP_TIMEOUT=${DEMO_STEP_TIMEOUT:-180}
DEMO_STEP_NO=0

demo_die() {
	printf 'demo: %s\n' "$*" >&2
	exit 1
}

# demo_pty runs the command in a pseudo-terminal so it asks its questions as it would for a person.
# Each answer is typed once the output has gone quiet (the command waits at a prompt), so the
# transcript shows every answer right after its question. The command's exit status is returned.
demo_pty() {
	local cmd=$1
	shift
	local run=$DEMO_SANDBOX/run
	local fifo=$run/pty.in out=$run/pty.out rcfile=$run/pty.rc script_file=$run/pty.sh
	rm -f "$fifo" "$out" "$rcfile"
	mkfifo "$fifo" || return 1
	# The folder the command ends in is kept in run/step.cwd, so a step may `cd`.
	printf 'cd %q || exit 1\n%s\n_rc=$?\npwd >%q\nprintf "%%s" "$_rc" >%q\n' \
		"$PWD" "$cmd" "$run/step.cwd" "$rcfile" >"$script_file"

	if script --version 2>/dev/null | grep -q util-linux; then
		script -q -e -c "bash $(printf %q "$script_file")" /dev/null <"$fifo" >"$out" 2>&1 &
	else
		script -q /dev/null bash "$script_file" <"$fifo" >"$out" 2>&1 &
	fi
	local pid=$!
	exec 3>"$fifo"

	local answer size prev last=0 still waited
	for answer in "$@"; do
		# Wait for new output, then for 0.6 s without more: the command waits at a prompt.
		prev=-1 still=0 waited=0
		while kill -0 "$pid" 2>/dev/null && [ "$waited" -le $((DEMO_STEP_TIMEOUT * 5)) ]; do
			size=$(($(wc -c <"$out")))
			if [ "$size" -gt "$last" ] && [ "$size" -eq "$prev" ]; then
				still=$((still + 1))
				[ "$still" -ge 3 ] && break
			else
				still=0
			fi
			prev=$size
			sleep 0.2
			waited=$((waited + 1))
		done
		kill -0 "$pid" 2>/dev/null || break
		last=$(($(wc -c <"$out")))
		printf '%s\n' "$answer" >&3
	done

	# Wait for the command; one that asks more than the script answers is stopped.
	waited=0
	while kill -0 "$pid" 2>/dev/null; do
		sleep 0.2
		waited=$((waited + 1))
		if [ "$waited" -gt $((DEMO_STEP_TIMEOUT * 5)) ]; then
			kill "$pid" 2>/dev/null
			printf '\n[demo: stopped after %ss: the command asked for input the script does not answer]\n' "$DEMO_STEP_TIMEOUT" >>"$out"
			break
		fi
	done
	exec 3>&-
	wait "$pid" 2>/dev/null
	cat "$out"
	rm -f "$fifo"
	[ -s "$rcfile" ] || return 1
	return "$(cat "$rcfile")"
}

# demo_sanitize removes everything personal or run-specific from a transcript:
# terminal control codes, sandbox and repository paths (-> ~ and <repo>), the real HOME, user name,
# hostname and Git identity, session tokens, invite codes, content digests, submission IDs, LAN
# addresses, and commit hashes (-> <commit-1>, <commit-2>, ... the same hash keeps its number across
# the transcripts of one run).
demo_sanitize() {
	SAN_PRIVATE=$DEMO_SANDBOX/.private SAN_HOME=$HOME SAN_ROOT=$DEMO_SANDBOX SAN_REPO=$DEMO_REPO \
		SAN_MAP=$DEMO_SANDBOX/run/sanitize.map perl -e '
use strict; use warnings;
my (%priv, @hosts);
if (open my $fh, "<", $ENV{SAN_PRIVATE}) {
  while (<$fh>) { chomp; my ($k, $v) = split /=/, $_, 2; next unless defined $v && length $v;
    if ($k eq "REAL_HOST") { push @hosts, $v } else { $priv{$k} = $v } }
}
my %map; my %n = (commit => 0, ip => 0);
if (open my $fh, "<", $ENV{SAN_MAP}) { while (<$fh>) { chomp; my ($k, $v) = split / /, $_, 2; $map{$k} = $v;
  $n{commit}++ if $v =~ /^<commit-/; $n{ip}++ if $v =~ /^192\.0\.2\./ } }
my $text = do { local $/; <STDIN> } // "";
$text =~ s/\e\][^\a\e]*(?:\a|\e\\)//g;           # OSC sequences (titles, links)
$text =~ s/\e\[[0-9;?]*[ -\/]*[@-~]//g;          # CSI sequences (colours, cursor)
$text =~ s/\e[()][A-Za-z0-9]//g; $text =~ s/\e[=>]//g;
$text =~ s/\r\n/\n/g; $text =~ s/[^\n]*\r(?!\n)//g; $text =~ s/[\x00-\x08\x0b\x0c\x0e-\x1f\x7f]//g;
# Paths: the sandbox HOME (also its /private/var form on macOS) reads as ~, other sandbox files as <demo>.
for my $p (grep { defined && length } ($ENV{SAN_HOME}, $ENV{SAN_ROOT}, $ENV{SAN_REPO})) {
  my @forms = ($p); push @forms, "/private$p" if $p =~ m{^/(?:var|tmp)/};
  for my $f (@forms) {
    my $to = $p eq $ENV{SAN_HOME} ? "~" : $p eq $ENV{SAN_REPO} ? "<repo>" : "<demo>";
    $text =~ s/\Q$f\E(?=\/|\b|$)/$to/g;
  }
}
$text =~ s/\Q$ENV{SAN_ROOT}\E/<demo>/g if $ENV{SAN_ROOT};
if (my $h = $priv{REAL_HOME}) { $text =~ s/\Q$h\E/<home>/g }
if (my $e = $priv{REAL_GIT_EMAIL}) { $text =~ s/\Q$e\E/demo\@example.com/gi }
if (my $g = $priv{REAL_GIT_NAME}) { $text =~ s/\Q$g\E/Demo User/g if length $g >= 3 }
for my $h (sort { length $b <=> length $a } @hosts) { $text =~ s/(?<![\w-])\Q$h\E(?![\w-])/demo-host/gi if length $h >= 2 }
if (my $u = $priv{REAL_USER}) { $text =~ s/(?<![\w.-])\Q$u\E(?![\w-])/demo/g if length $u >= 3 }
# Secrets and per-run identifiers.
$text =~ s/(#token=)[^\s"\x27<>]+/$1<token>/g;
$text =~ s/([?&](?:token|invite|code)=)[^\s&"\x27<>]+/$1<token>/gi;
$text =~ s/(\b[Bb]earer\s+)\S+/$1<token>/g;
$text =~ s/\b[0-9A-Z]{4}-[0-9A-Z]{4}-[0-9A-Z]{4}\b/XXXX-XXXX-XXXX/g;
$text =~ s/\bsha256:[0-9a-f]{64}\b/sha256:<digest>/g;
$text =~ s/\bSUB-[0-9A-Z]{26}\b/SUB-<id>/g;
$text =~ s{(?<![\w./-])((?=[0-9a-f]*[0-9])(?=[0-9a-f]*[a-f])[0-9a-f]{7,40})(?![\w-])}{
  $map{$1} //= "<commit-" . ++$n{commit} . ">"
}ge;
$text =~ s/(?<![\w:])[0-9a-fA-F]{32,}(?![\w])/<secret>/g;
$text =~ s/[A-Za-z0-9._%+-]+\@(?!example\.(?:com|org|invalid)\b)[A-Za-z0-9-]+(?:\.[A-Za-z0-9-]+)+/demo\@example.com/g;
$text =~ s{\b((?:\d{1,3}\.){3}\d{1,3})\b}{
  my $ip = $1;
  ($ip =~ /^(?:127\.|0\.0\.0\.0$)/) ? $ip : ($map{$ip} //= "192.0.2." . (++$n{ip} + 9))
}ge;
print $text;
if (open my $fh, ">", $ENV{SAN_MAP}) { print {$fh} "$_ $map{$_}\n" for sort keys %map }
'
}

# demo_step runs one step from the current folder. With answers it runs in a pseudo-terminal and
# types them; without, stdin is closed, as for an agent or a script. The working folder after the
# step carries over, so a step may `cd`. A failed step stops the whole run and names itself.
demo_step() {
	local slug=$1 cmd=$2
	shift 2
	DEMO_STEP_NO=$((DEMO_STEP_NO + 1))
	local name
	name=$(printf '%02d-%s' "$DEMO_STEP_NO" "$slug")
	local raw=$DEMO_SANDBOX/logs/$name.log cwdfile=$DEMO_SANDBOX/run/step.cwd rc=0
	printf '==> %s\n' "$name" >&2
	if [ $# -gt 0 ]; then
		demo_pty "$cmd" "$@" >"$raw" 2>&1 || rc=$?
	else
		(
			eval "$cmd"
			_rc=$?
			pwd >"$cwdfile"
			exit "$_rc"
		) </dev/null >"$raw" 2>&1 || rc=$?
	fi
	[ -s "$cwdfile" ] && cd "$(cat "$cwdfile")" && rm -f "$cwdfile"
	{
		# Every command line gets the prompt; a line after a trailing backslash continues the command.
		printf '%s\n' "$cmd" | awk '{ print (cont ? "" : "$ ") $0; cont = /\\$/ }'
		cat "$raw"
	} | demo_sanitize >"$DEMO_OUT/$name.txt"
	if [ "$rc" -ne 0 ]; then
		sed 's/^/    /' "$DEMO_OUT/$name.txt" >&2
		demo_die "step $name failed (exit $rc): $cmd"
	fi
}

# demo_check_clean fails when a transcript still holds the real HOME, user name, hostname or Git
# identity, a session token, or the address of a machine on the network.
demo_check_clean() {
	local dir=$1 key value found=0
	while IFS='=' read -r key value; do
		[ -n "$value" ] || continue
		case "$key" in REAL_USER | REAL_GIT_NAME) [ "${#value}" -ge 3 ] || continue ;; esac
		if grep -rqiwF -- "$value" "$dir"; then
			printf 'demo: %s of the real machine appears in %s:\n' "$key" "$dir" >&2
			grep -rliwF -- "$value" "$dir" >&2
			found=1
		fi
	done <"$DEMO_SANDBOX/.private"
	if grep -rhoE '#token=[0-9a-fA-F]+|[0-9A-Z]{4}-[0-9A-Z]{4}-[0-9A-Z]{4}' "$dir" | grep -qv '^XXXX-XXXX-XXXX$'; then
		printf 'demo: a session token or invite code remains in %s\n' "$dir" >&2
		found=1
	fi
	if grep -rhoE '\b([0-9]{1,3}\.){3}[0-9]{1,3}\b' "$dir" | grep -qvE '^(127\.|0\.0\.0\.0$|192\.0\.2\.)'; then
		printf 'demo: a network address remains in %s\n' "$dir" >&2
		found=1
	fi
	return "$found"
}
