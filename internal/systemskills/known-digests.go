package systemskills

// knownDigests lists bytes of system skill files that Skill Hub wrote in the
// past, keyed by skill id and by the file path inside the skill folder. A file
// on disk whose digest is listed here (or equals the current bundle) is owned by
// Skill Hub and may be upgraded or removed; any other content belongs to the
// user and is left alone.
//
// How the table was produced (from a full clone):
//
//	for p in internal/systemskills/curator/SKILL.md system-skills/curator/SKILL.md; do
//	  for c in $(git log --format=%H -- "$p"); do git show "$c:$p" | sha256sum; done
//	done | sort -u
//
// and the same loop over .claude/skills/distill-lab/SKILL.md, the old dev copy
// of distill-lab, keyed as SKILL.md so that an unedited hand copy of it in a hub
// is recognized. Owners copied that file by hand before distill-lab was bundled.
//
// There is no entry for the old dev copy's scripts/distill.py: Skill Hub never
// installed that file, so a copy of it in a hub was put there by its owner.
//
// Coverage differs by id. The curator list holds every committed version. The
// distill-lab list holds the old dev copies and, from now on, only versions that
// shipped in a release tag; a distill-lab file written by an untagged dev build
// is therefore treated as unknown.
//
// Rule for later edits: when a bundled file changes after a release, add the
// digest of the bytes that shipped. TestKnownDigestsCoverReleasedBundles checks
// every release tag and fails when a shipped file is missing here.
var knownDigests = map[string]map[string][]string{
	CuratorSkillID: {
		skillEntryFile: {
			"sha256:0a32b058a33bc1a3c33537b011d602c52215be5852f185d2d67492a53ca513db",
			"sha256:1abe16ef1b50228fbe08e2e2e764841b0b8a81305988416a98285a1b190d3f81",
			"sha256:1ea914eaffce9046d9595f0eb503169b6d7a37cb6694d4429478df45c1dae98e",
			"sha256:3a6c70cb1a80ee1107a67578800f76be9bbaffc281a64131859e275f6a5b5734",
			"sha256:76cd0217e43d993478b6f6befd78bc8edc30d0b547cb04f6e950a60f562ebc13",
			"sha256:85aacebfa59d60697ac75205a71f79e53cb63f7860df1e36ffa8bc78f62531de",
			"sha256:9a9c48059c43317ede428e6f07e75380c596449858f11b8166ef74f174ad5a8b",
			"sha256:b9c560bf0b3e4207ead20166028b77cb948d785c75af470c686ddbdf27a4b6a8",
			"sha256:ceb90d44b4cf39c83a8a3cfc8a9274090a1337406fafc17136f7f39a85a1c662",
			"sha256:db6ed8f7a2a237b4e43a1a2e06e05e010d927df6a5708f42323270e55641ca8b",
			"sha256:e0fb78303c882816908e73b95fb855648ce7800123857c0327e803a843e770e3",
			"sha256:e5624cf7fcf8e5fb00180315a17238bf0ff6883238668be96fb3b2eef61c39f1",
			"sha256:e865df620cf402945a3826605bcbacae83ae1530e9a9671d6e72c302a11eb4b1",
			"sha256:ea2c5b46141b3fc6155549373a6eef28877937ac0e01439d98838689ca0b4284",
			"sha256:f4c92c7f0a75ee1d46b629dfbb6a6de78a372ddf6d327cd074307a8f0be75d96",
		},
	},
	DistillSkillID: {
		skillEntryFile: {
			"sha256:048412a87f3ab55048f6826301a2328899708cb931ed1018d5a80ff42cdf9eca",
			"sha256:2efa56d2ef2d20f974b42de68072773837592d587767e454e4f20e31b2b6524e",
			"sha256:7e1486db9dc445c87a9e7fbdb29dba46c2a8cb37c4fe011800cd49c844819f0b",
		},
	},
}
