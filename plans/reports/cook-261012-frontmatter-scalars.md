# Frontmatter scalars keep their written form

Change: `ParseSkillFrontmatter` (internal/catalog/servable.go) still decodes through yaml.v3 and the JSON round-trip exactly as before, but now also decodes into a `yaml.Node` tree and walks it alongside the result (`keepWrittenScalars`). Only two kinds of scalar are replaced: timestamp-typed scalars become their original text, and integers beyond 2^53 (decimal, hex, or too large for uint64) become an exact `json.Number`. Aliases and `<<` merge keys are followed. Strings, bools, small numbers, lists and maps are untouched.

Consumers checked: name/description validation (string assertions, unaffected), the entry served by skills/list and skills/get (`DistributedSkill.Frontmatter`, JSON-encoded; json.Number emits the exact digits), the bundled curator frontmatter. Frontmatter feeds no digest, routing extraction or SQLite catalog row; those use file bytes and meta YAML.

Tests: catalog `frontmatter_scalars_test.go` (date, timestamp, quoted, 20-digit and 30-digit ints, hex, nested map/list dates, alias, merge key, byte-identical comparison against a copy of the old decoder on every repo SKILL.md plus an ordinary fixture); app `TestDistributionListedFrontmatterMatchesIndependentParseWithDate` (listed entry vs an independent yaml node parse). Verified the new tests fail with the walk disabled.

Doc: one sentence added to docs/design/02-agent-hub-protocol.md.

Gates: gofmt, go vet (linux/windows/darwin), make lint 0 issues, go test ./... all ok, -race on catalog and app ok.
