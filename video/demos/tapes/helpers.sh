# shellcheck shell=bash
# Sourced off camera by the tapes, after sandbox.sh. Off-camera steps that the video shows another way.

# Step 2 of "Create your first skill", from the same fixtures and in the same way as happy-path.sh:
# replace the scaffold placeholder under Instructions and add two requests under Examples.
video_write_instructions() {
	FIX_INSTRUCTIONS=$DEMO_DIR/fixtures/commit-style/instructions.md \
		FIX_EXAMPLES=$DEMO_DIR/fixtures/commit-style/more-examples.md perl -0pi -e '
		sub slurp { open my $fh, "<", $_[0] or die "$_[0]: $!\n"; local $/; my $t = <$fh>; return $t }
		my $instr = slurp($ENV{FIX_INSTRUCTIONS});
		my $more = slurp($ENV{FIX_EXAMPLES});
		s/<!-- skillhub:scaffold -->\n(?:[^\n]+\n?)*/$instr/ or die "no scaffold placeholder in SKILL.md\n";
		s/(## Examples\n\n(?:- [^\n]*\n)+)/$1$more/ or die "no Examples list in SKILL.md\n";
	' .claude/skills/commit-style/SKILL.md
}
