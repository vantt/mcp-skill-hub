# Bug ledger: curation CLI defects found during the independent evaluation

Date: 2026-10-01.

These defects were reproduced against a binary built from the working tree at commit `157507de`. The cited source files are unchanged from HEAD. Each entry gives the reproduction, what should happen, what actually happened, the code location, a suggested fix, and the acceptance test from the [evaluation report](independent-evaluation-261001-1651-curation-cli-redesign-report.md) §10.

## How to reproduce

Every reproduction starts from this setup. Change `S` to any disposable directory. Nothing touches a real workspace.

```bash
S=/tmp/skillhub-repro; rm -rf $S; mkdir -p $S/home
export HOME=$S/home                                 # isolate global agent config
go build -o $S/skillhub ./cmd/skillhub; B=$S/skillhub; W=$S/ws
$B init $W --yes >/dev/null
git -C $W add -A && git -C $W -c user.email=t@t -c user.name=t commit -qm init
# A draft skill used by several reproductions
$B skill create --workspace $W --id rr --collection software --name RR --description "Review reliability." \
  --trigger "review reliability" --not-for "design a service" --min-scope multi_step --yes
F=$W/skills/software/rr/SKILL.md
```

Severity scale:

- **P0:** silent data loss, hidden side effects, or a broken safety claim.
- **P1:** wrong or misleading behavior.
- **P2:** cosmetic issue or friction.

## Summary

| ID | Severity | Defect |
|---|---|---|
| BUG-01 | P0 | `--no-monitor` is ignored unless `--cadence` is also given. |
| BUG-02 | P0 | A concurrent edit is overwritten while `skill edit --editor` is open. |
| BUG-03 | P0 | Editor output is deleted when the preview fails. |
| BUG-04 | P0 | Importing a folder-scoped source silently drops companion files. |
| BUG-05 | P0 | `validate` and catalog publication apply different rules. |
| BUG-06 | P0 | A fresh clone of a workspace fails `validate` and `rebuild`. |
| BUG-07 | P1 | One invalid file disables every read and `resolve`. |
| BUG-08 | P1 | `status` says "No skills yet" when the workspace is invalid. |
| BUG-09 | P1 | An untouched scaffold can be activated. |
| BUG-10 | P1 | A GitHub tree URL fails with "repository not found:" and blank lines. |
| BUG-11 | P1 | No local folder outside the workspace can be imported, and capture accepts locators that triage rejects. |
| BUG-12 | P1 | The `--editor` preview hint "re-run with --yes" discards the edit. |
| BUG-13 | P1 | `ActiveLocally` is always `true`. |
| BUG-14 | P2 | The commit hint prints a literal `<ws>`. |
| BUG-15 | P2 | `diff` labels files as "active skills (N)". |
| BUG-16 | P2 | The source license is recorded as `unknown` although SKILL.md declares one. |
| BUG-17 | P2 | `status` suggests `skill create ...`, which cannot be run as printed. |

---

## BUG-01 (P0): `--no-monitor` is ignored unless `--cadence` is also given

**Reproduce:**

```bash
mkdir -p $W/runtime/fx/cr && printf -- '---\nname: cr\ndescription: d\n---\nbody\n' > $W/runtime/fx/cr/SKILL.md
C=$($B source capture runtime/fx --reason t --workspace $W | sed -n 's/^Candidate: //p')
$B source triage $C --decision accept --source-id fx --no-monitor --json --workspace $W \
  | python3 -c "import json,sys;print(json.load(sys.stdin)['source']['monitoring'])"
```

- **Expected:** `{'enabled': False, ...}`.
- **Actual:** `{'enabled': True, 'cadence': 'weekly'}`. Adding `--cadence manual` makes it `False`.
- **Cause:** [`internal/app/source.go#L295-L297`](https://github.com/vantt/mcp-skill-hub/blob/157507de254a5005c32120dd318a4abf1c229da6/internal/app/source.go#L295-L297). The block `if !input.MonitoringEnabled && input.Cadence == "" { record.Monitoring.Enabled = true }` forces monitoring back on. MCP reaches the same code through [`source_tools.go#L84-L87`](https://github.com/vantt/mcp-skill-hub/blob/157507de254a5005c32120dd318a4abf1c229da6/internal/delivery/mcpserver/source_tools.go#L84-L87).
- **Fix:** delete the override. When monitoring is disabled and no cadence is given, set cadence to `manual`.
- **Test:** T1.

## BUG-02 (P0): A concurrent edit is overwritten while `skill edit --editor` is open

**Reproduce (managed writer):**

```bash
cat > $S/ed-a.sh <<EOF
#!/bin/sh
sed -i 's/1. First step./1. Change from A./' "\$1"; touch $S/open; while [ ! -f $S/go ]; do sleep 0.2; done
EOF
chmod +x $S/ed-a.sh; rm -f $S/open $S/go
( EDITOR=$S/ed-a.sh $B skill edit rr --editor --yes --workspace $W > $S/a.out 2>&1 ) &
while [ ! -f $S/open ]; do sleep 0.2; done
sed 's/## Examples/## From B\n\n## Examples/' $F > $S/b.md
$B skill edit rr --content-file $S/b.md --yes --workspace $W   # B: "saved", exit 0
touch $S/go; wait; cat $S/a.out                                # A: "saved", exit 0
grep -c "From B" $F                                            # 0: B's change is gone
```

The same loss happens if B edits the file directly and A then runs preview followed by `skill confirm` with the printed pins.

- **Expected:** A fails with a stale-base error, and B's change survives.
- **Actual:** both commands exit 0. B's content is lost, and B's operation receipt remains in `history/operations/` although its content is gone.
- **Cause:**
  - The original text is read before the editor opens ([`skill.go#L382`](https://github.com/vantt/mcp-skill-hub/blob/157507de254a5005c32120dd318a4abf1c229da6/internal/delivery/cli/skill.go#L382)).
  - The preview is built from the current state only after the editor exits ([`skill.go#L143`](https://github.com/vantt/mcp-skill-hub/blob/157507de254a5005c32120dd318a4abf1c229da6/internal/delivery/cli/skill.go#L143)).
  - `UpdateInput` has no base-digest field ([`internal/skill/lifecycle.go#L59-L67`](https://github.com/vantt/mcp-skill-hub/blob/157507de254a5005c32120dd318a4abf1c229da6/internal/skill/lifecycle.go#L59-L67)).
  - The MCP `skill_update_preview` input has none either ([`types.go#L115-L124`](https://github.com/vantt/mcp-skill-hub/blob/157507de254a5005c32120dd318a4abf1c229da6/internal/delivery/mcpserver/types.go#L115-L124)). The MCP variant was inferred from code, not executed.
- **Fix:**
  - Carry the digest of the original `SKILL.md` into the `SKILL.md` change as `BeforeDigest`. `PlanMutation` already rejects a mismatch ([`mutation.go#L154-L156`](https://github.com/vantt/mcp-skill-hub/blob/157507de254a5005c32120dd318a4abf1c229da6/internal/mutation/mutation.go#L154-L156)).
  - Add `expected_content_digest` to MCP `skill_update_preview`, and return `content_digest` from `skill_get`.
- **Tests:** T13, T14.

## BUG-03 (P0): Editor output is deleted when the preview fails

**Reproduce:**

```bash
printf '#!/bin/sh\nsed -i "s/^name: rr/name: wrong/" "$1"; echo VALUABLE >> "$1"; echo "$1" > %s/tmp\n' $S > $S/ed-bad.sh; chmod +x $S/ed-bad.sh
EDITOR=$S/ed-bad.sh $B skill edit rr --editor --yes --workspace $W; ls "$(cat $S/tmp)"
```

- **Expected:** an error that also prints the path where the edited text was kept.
- **Actual:** `WHY: SKILL.md frontmatter name must be "rr"` (exit 2), and `ls` reports `No such file`. The edits are lost.
- **Cause:** `defer os.Remove(name)` deletes the file as soon as `editContent` returns, before the preview runs ([`skill.go#L386-L391`](https://github.com/vantt/mcp-skill-hub/blob/157507de254a5005c32120dd318a4abf1c229da6/internal/delivery/cli/skill.go#L386-L391)).
- **Fix:** delete the temporary file only after a successful preview or confirmation. On failure, move it to `runtime/edits/<id>-<time>.md` with mode 0600 and print the path.
- **Test:** T15.

## BUG-04 (P0): Importing a folder-scoped source silently drops companion files

**Reproduce (network, bounded):**

```bash
C=$($B source capture https://github.com/anthropics/skills --reason t --workspace $W | sed -n 's/^Candidate: //p')
$B source triage $C --decision accept --source-id ap --path skills/pdf --workspace $W > $S/t.out
eval "$(grep 'skillhub source confirm' $S/t.out | sed "s|^ *skillhub|$B|") --workspace $W"
$B source import ap --yes --workspace $W
find $W/skills/default/pdf -type f        # only SKILL.md and skill.meta.yaml
grep -n "REFERENCE.md\|FORMS.md" $W/skills/default/pdf/SKILL.md   # still referenced
```

- **Expected:** all 12 upstream files are imported, or the preview names every file that will be skipped and warns.
- **Actual:** only 2 files are imported, with no warning. Upstream at commit `8a1541c` has `forms.md`, `reference.md`, `LICENSE.txt` and 8 `scripts/*.py` files.
- **Cause:**
  - Companion files are collected only when `skillDir != ""` ([`source_import.go#L209-L226`](https://github.com/vantt/mcp-skill-hub/blob/157507de254a5005c32120dd318a4abf1c229da6/internal/app/source_import.go#L209-L226)). When the scope is the skill folder itself, `skillDir` is empty.
  - Only `references/`, `scripts/` and `assets/` are kept ([L221](https://github.com/vantt/mcp-skill-hub/blob/157507de254a5005c32120dd318a4abf1c229da6/internal/app/source_import.go#L221)).
  - The canonical layout forbids any other file beside `SKILL.md` ([`canonical/skill.go#L248`](https://github.com/vantt/mcp-skill-hub/blob/157507de254a5005c32120dd318a4abf1c229da6/internal/canonical/skill.go#L248)).
- **Fix:** compute companion files relative to the skill folder, whether or not it is the scope root. Never drop a file without listing it in the preview. Whether to relax the layout is open decision D1 in the evaluation report.
- **Test:** T5.

## BUG-05 (P0): `validate` and catalog publication apply different rules

**Reproduce:**

```bash
$B skill activate rr --yes --workspace $W 2>/dev/null   # if routing allows; otherwise use a draft
sed -i 's/^name: rr/name: WRONG NAME/' $F
$B skill show rr --workspace $W | grep "^name"   # serves "WRONG NAME", exit 0
$B status --workspace $W | sed -n 3p             # "Workspace valid; search index current"
$B validate --workspace $W; echo $?              # frontmatter name mismatch, exit 2
```

- **Expected:** one validator. Content that `validate` rejects is never published or served.
- **Actual:** the content is published and served, and `status` reports the workspace as valid.
- **Cause:** the frontmatter-name rule exists only in `ValidateWorkspace` ([`app/operations.go#L31-L49`](https://github.com/vantt/mcp-skill-hub/blob/157507de254a5005c32120dd318a4abf1c229da6/internal/app/operations.go#L31-L49)). The catalog builder runs `canonical.Validate` ([`catalog/build.go#L141-L148`](https://github.com/vantt/mcp-skill-hub/blob/157507de254a5005c32120dd318a4abf1c229da6/internal/catalog/build.go#L141-L148)).
- **Fix:** move the rule into `canonical.Validate`. Add a test asserting that `ValidateWorkspace` adds no rule beyond `canonical.Validate`.
- **Test:** T17.

## BUG-06 (P0): A fresh clone of a workspace fails `validate` and `rebuild`

**Reproduce:**

```bash
git clone -q $W $S/clone
$B validate --workspace $S/clone; echo $?   # "Required canonical directory is missing" x10, exit 2
$B rebuild  --workspace $S/clone; echo $?   # exit 2
```

- **Expected:** the documented flow, `git clone` then `rebuild` ([`docs/user-guide.md#L119-L127`](https://github.com/vantt/mcp-skill-hub/blob/157507de254a5005c32120dd318a4abf1c229da6/docs/user-guide.md#L119-L127)), works. A CI `validate` on a fresh checkout passes.
- **Actual:** both commands fail. Only `doctor --fix --yes` repairs the clone, and it also rewrites `.mcp.json`, `.codex/config.toml` and `.gemini/settings.json`.
- **Cause:** required directories are empty, so Git does not track them, and the validator requires them to exist.
- **Fix:** treat a missing required directory that would be empty as empty. Alternatively, have `init` write tracked placeholder files that the layout accepts.
- **Test:** T20.

## BUG-07 (P1): One invalid file disables every read and `resolve`

**Reproduce:**

```bash
M=$W/skills/software/rr/skill.meta.yaml; cp $M $S/m.bak
sed -i 's/^status: .*/status: shiny/' $M
$B skill list --workspace $W     # error
$B skill show <another-skill> --workspace $W   # error
cp $S/m.bak $M
```

- **Expected:** reads keep serving the last published generation and print a warning (decision D3).
- **Actual:** every read fails, including MCP `skill_resolve`, which calls the same `EnsureFreshOrRebuild` ([`resolver.go#L59`](https://github.com/vantt/mcp-skill-hub/blob/157507de254a5005c32120dd318a4abf1c229da6/internal/app/resolver.go#L59)). Agents in every connected project lose all skills.
- **Cause:** [`catalog/open.go#L45-L52`](https://github.com/vantt/mcp-skill-hub/blob/157507de254a5005c32120dd318a4abf1c229da6/internal/catalog/open.go#L45-L52) returns an error instead of falling back to the published generation.
- **Test:** T18.

## BUG-08 (P1): `status` says "No skills yet" when the workspace is invalid

- **Reproduce:** the BUG-07 state, then run `$B status --workspace $W`.
- **Actual:** "Workspace needs repair…" followed by "No skills yet. Next: … `skill create ...`", although the workspace has active skills.
- **Cause:** [`delivery/cli/curation.go#L43-L46`](https://github.com/vantt/mcp-skill-hub/blob/157507de254a5005c32120dd318a4abf1c229da6/internal/delivery/cli/curation.go#L43-L46) prints the empty message whenever the counts are 0, including when they are 0 only because the health check failed.
- **Fix:** when health is not valid, name the invalid file and the repair command, and do not print counts.
- **Test:** T18.

## BUG-09 (P1): An untouched scaffold can be activated

**Reproduce:**

```bash
$B skill create --workspace $W --id ph --collection software --name PH --description d \
  --trigger "demo" --not-for "x" --min-scope single_step --yes
$B skill activate ph --yes --workspace $W   # "now active", exit 0
```

- **Expected:** activation is rejected while the generated placeholder lines remain, for example "Describe when your agent should choose this skill." or "1. First step."
- **Cause:** activation checks only routing fields ([`skill_lifecycle.go#L257-L284`](https://github.com/vantt/mcp-skill-hub/blob/157507de254a5005c32120dd318a4abf1c229da6/internal/app/skill_lifecycle.go#L257-L284)).
- **Test:** T12.

## BUG-10 (P1): A GitHub tree URL fails with an unhelpful error

**Reproduce:**

```bash
C=$($B source capture https://github.com/anthropics/skills/tree/main/skills/pdf --reason t --workspace $W | sed -n 's/^Candidate: //p')
$B source triage $C --decision accept --source-id x --workspace $W
```

- **Actual:** `WHY: Git HTTPS source operation failed: repository not found:` followed by several blank lines.
- **Cause:** the whole URL becomes `Locator.Repository` ([`source.go#L736-L739`](https://github.com/vantt/mcp-skill-hub/blob/157507de254a5005c32120dd318a4abf1c229da6/internal/app/source.go#L736-L739)).
- **Fix:** follow evaluation report §6.1. At minimum, detect `/tree/` and `/blob/` URLs and tell the user which repository URL and `--path` to use.
- **Tests:** T2, T3.

## BUG-11 (P1): No local folder outside the workspace can be imported, and capture accepts locators that triage rejects

**Reproduce:**

```bash
$B source capture /abs/folder --reason t --workspace $W                 # invalid source locator
$B source capture ./folder --reason t --workspace $W                    # captured
$B source triage <that id> --decision accept --source-id y --workspace $W   # invalid source locator
```

- **Additional findings:**
  - A quoted `"~/x"` is captured literally.
  - Placing a folder inside the workspace makes the workspace invalid ("outside the canonical layout").
  - Only a folder under gitignored `runtime/` works. The committed source record then points at a path that does not exist in any clone.
- **Cause:** [`source.go#L684-L703`](https://github.com/vantt/mcp-skill-hub/blob/157507de254a5005c32120dd318a4abf1c229da6/internal/app/source.go#L684-L703) and [`#L98-L104`](https://github.com/vantt/mcp-skill-hub/blob/157507de254a5005c32120dd318a4abf1c229da6/internal/app/source.go#L98-L104).
- **Fix:** follow evaluation report §6.2. Also make capture validate locators with the same rules as triage.
- **Test:** T8.

## BUG-12 (P1): The `--editor` preview hint "re-run with --yes" discards the edit

- **Reproduce:** `EDITOR=<script> $B skill edit rr --editor --workspace $W`. The output ends with "or re-run with --yes to apply directly." Re-running opens the original text again.
- **Cause:** a shared hint string ([`skill.go#L458`](https://github.com/vantt/mcp-skill-hub/blob/157507de254a5005c32120dd318a4abf1c229da6/internal/delivery/cli/skill.go#L458)). Also, the default preview shows only file counts, not the user's text diff.
- **Fix:** for `--editor`, show the `SKILL.md` diff and only the confirm command.
- **Test:** T16.

## BUG-13 (P1): `ActiveLocally` is always `true`

- **Reproduce:** `$B skill create … --yes --json` on a draft. The output contains `"active_locally": true`.
- **Cause:** [`skill_lifecycle.go#L207`](https://github.com/vantt/mcp-skill-hub/blob/157507de254a5005c32120dd318a4abf1c229da6/internal/app/skill_lifecycle.go#L207).
- **Fix:** set the field to `state == "active"`.
- **Test:** none in §10 covers this. Add an assertion that `active_locally` equals `state == "active"`.

## BUG-14 (P2): The commit hint prints a literal `<ws>`

- **Reproduce:** run `skill activate … --yes`. The output says "Commit with `git -C <ws> commit`."
- **Cause:** [`skill.go#L485-L497`](https://github.com/vantt/mcp-skill-hub/blob/157507de254a5005c32120dd318a4abf1c229da6/internal/delivery/cli/skill.go#L485-L497).
- **Fix:** print the actual quoted workspace path. `init` and `diff` already do this.
- **Test:** T21.

## BUG-15 (P2): `diff` labels files as "active skills (N)"

- **Reproduce:** with 2 new skills, `$B diff` prints `active skills (4):` and then lists 4 files, before the skills have been activated.
- **Fix:** group the output by skill, for example `rr: 2 files added`.
- **Test:** T22.

## BUG-16 (P2): The source license is recorded as `unknown` although SKILL.md declares one

- **Reproduce:** the BUG-04 flow. The triage preview says `license unknown`, while `skills/pdf/SKILL.md` contains `license: Proprietary. LICENSE.txt has complete terms`. `LICENSE.txt` is then dropped (BUG-04).
- **Fix:** show the license declared in frontmatter and any detected license file in the import preview, with a warning. Whether to block is decision D4.

## BUG-17 (P2): `status` suggests `skill create ...`, which cannot be run as printed

- **Reproduce:** run `$B status` in a new workspace. The hint is `run \`skillhub skill create ...\``, but `skill create` requires four flags and refuses a positional ID.
- **Cause:** [`delivery/cli/curation.go#L46`](https://github.com/vantt/mcp-skill-hub/blob/157507de254a5005c32120dd318a4abf1c229da6/internal/delivery/cli/curation.go#L46).
- **Fix:** print one complete example command.

---

## Not reproduced and still open

- Refs that contain `/`: no fixture repository was available.
- Windows paths.
- The MCP variant of BUG-02, which is inferred from code.
- Atomicity of `--watch`, because the feature does not exist yet.
