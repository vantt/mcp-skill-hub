# CLI Conventions Evidence for Skill Import / Source Watch Redesign

Date: 2026-10-01. Read-only research. Source code citations are pinned to commit SHAs fetched on this date, so their line numbers stay stable.
Credibility tiers: **[O]** official docs or man page, **[S]** the tool's own source code, **[3P]** third-party tool (relevant prior art, not a standard).

## Bottom line

1. **Split "register a source" from "use one item."** helm (`repo add` / `install`), brew (`tap` / `install`), asdf (`plugin add` / `plugin update`) and gh (`extension install` / `extension upgrade`) all keep these as separate verbs. Mapping for skillhub: `source add|update` corresponds to `helm repo add|update`, and `skill add <locator>` corresponds to `helm install <ref|dir|url>`.
2. **Accept one positional locator in many forms.** gh, npm, helm and vercel `skills add` each accept shorthand, URL, and local dir through the same argument. Refs travel either as flags (cargo `--branch/--tag/--rev`) or as `#ref` fragments (npm, degit, giget, vercel skills).
3. **No surveyed tool does longest-prefix ref matching** for `tree/<ref>/<path>`. All of them take the single segment after `tree/` as the ref, and offer `#ref` as the escape hatch for refs that contain `/`. Longest-prefix matching is possible with `git ls-remote` or the REST `matching-refs` endpoint, but you would be the first to adopt it, not following a convention.
4. **Validating staged content.** pre-commit and lint-staged both hide unstaged changes in the live worktree using a patch or stash. Exporting the index elsewhere (`git checkout-index --prefix`) is cleaner and has no restore risk.
5. **Deprecation.** Keep the old name working, hide it from help, and print a warning that names the replacement (cobra/pflag, gh, and kubectl rule #6).

---

## Q1. gh (GitHub CLI)

| Claim | URL | Quote | Relevance |
|---|---|---|---|
| [O] One repo arg accepts `OWNER/REPO`, a bare name, an HTTPS URL, or an SSH URL | https://cli.github.com/manual/gh_repo_clone | "If the OWNER/ portion of the OWNER/REPO repository argument is omitted, it defaults to the name of the authenticating user"; examples `gh repo clone https://github.com/cli/cli`, `git@github.com:cli/cli.git` | Precedent for a single positional locator with several syntaxes |
| [S] gh parses only `host/owner/repo` from a URL and **rejects extra path segments**, so tree URLs are not resolved at all | https://github.com/cli/go-gh/blob/859e813416733b742d6fc21df59a2058e04f0f38/internal/git/url.go#L68-L79 | `parts := strings.SplitN(strings.Trim(u.Path, "/"), "/", 3); if len(parts) != 2 { ... "invalid path: %s"` | gh is no precedent for parsing tree URLs. The reverse direction does exist: `gh browse` *generates* `tree/<ref>/<path>` URLs (https://github.com/cli/cli/blob/fc4b137cdef0a6bd28fd461b7cf9c84a5812a8cd/pkg/cmd/browse/browse.go#L294) |
| [O] `gh pr view` accepts a number, URL, or branch, and defaults to the current branch | https://cli.github.com/manual/gh_pr_view | "Without an argument, the pull request that belongs to the current branch is displayed." | Infers the target from context, so the user does not need an ID |
| [O] `--json` takes a field list; `--jq` and `--template` are layered on top of it | https://cli.github.com/manual/gh_help_formatting | "Some commands support passing the `--json` flag, which converts the output to JSON format"; "`jq` utility does not need to be installed" | Machine output is opt-in and has the same shape on every command |
| [O] Human output depends on the TTY | https://cli.github.com/manual/gh_help_formatting | "When connected to a terminal, the output is automatically pretty-printed." | Auto-detect the TTY, and also provide a way to override it |
| [O] Env overrides for TTY, color and prompts | https://cli.github.com/manual/gh_help_environment | `GH_FORCE_TTY` "force terminal-style output even when the output is redirected"; `GH_PROMPT_DISABLED` "disable interactive prompting"; `NO_COLOR`; `CLICOLOR=0` | Scripts and agents need a no-prompt switch |
| [O] `extension install` accepts `OWNER/REPO`, a URL, or `.` for a local dir; `--pin` pins to a ref | https://cli.github.com/manual/gh_extension_install | `--pin` "Pin extension to a release tag or commit ref"; `--force` "Force upgrade extension, or ignore if latest already installed" | Precedent for local and remote sources in one verb, and for pinning at install time |
| [S] Install on an already-installed extension **warns and exits 0**; it only upgrades when `--force` is passed | https://github.com/cli/cli/blob/fc4b137cdef0a6bd28fd461b7cf9c84a5812a8cd/pkg/cmd/extension/command.go#L376-L386 | `if forceFlag && ext != nil { return upgradeFunc(...) }` ... `"Extension %s is already installed"` | Install is idempotent, and update is a separate, explicit action |
| [O] `extension upgrade {<name> \| --all}` has `--dry-run` | https://cli.github.com/manual/gh_extension_upgrade | `--dry-run` "Only display upgrades"; `--all` "Upgrade all extensions" | The update verb carries dry-run and all-or-named targeting |
| [S] Pinned extensions refuse to upgrade unless `--force` is passed | https://github.com/cli/cli/blob/fc4b137cdef0a6bd28fd461b7cf9c84a5812a8cd/pkg/cmd/extension/manager.go#L525-L526 | `if !force && ext.IsPinned() { return pinnedExtensionUpgradeError }` ("pinned extensions can not be upgraded") | A pinned skill should be skipped by the watch/update flow |

## Q2. Tree URL ref ambiguity (refs containing `/`)

| Claim | URL | Quote | Relevance |
|---|---|---|---|
| [O] Refs may contain `/` | https://git-scm.com/docs/git-check-ref-format | Refs may include "slash `/` for hierarchical (directory) grouping" | `tree/feature/x/skills/foo` is genuinely ambiguous |
| [O] Git's own resolution order for short names | https://git-scm.com/docs/gitrevisions#_specifying_revisions | "When ambiguous, a <refname> is disambiguated by taking the first match ... refs/tags/<refname> ... refs/heads/<refname>" | Tags win over branches when names collide; mirror this if you resolve refs yourself |
| [O] `ls-remote` patterns tail-match at slash boundaries | https://git-scm.com/docs/git-ls-remote | "matched against the 'tail' of a ref ... (so `bar` matches `refs/heads/bar` but not `refs/heads/foobar`)" | One `ls-remote --heads --tags <url>` call returns every candidate, which is enough for longest-prefix matching |
| [O] REST `matching-refs` does prefix listing; `git/ref` is exact-match | https://docs.github.com/en/rest/git/refs#list-matching-references | "If the :ref doesn't exist in the repository, but existing refs start with :ref, they will be returned as an array." / get-a-reference: "a 404 is returned" | An API-only alternative to ls-remote for longest-prefix matching |
| [3P][S] **vercel-labs/skills** (`npx skills add`, about 32.9k stars) takes the **first segment** after `tree/` as the ref, with no ls-remote | https://github.com/vercel-labs/skills/blob/3694740352eeef5cdd689af694c485f1ff62eec3/src/source-parser.ts#L431-L441 | `input.match(/github\.com\/([^/]+)\/([^/]+)\/tree\/([^/]+)\/(.+)/)` | This is the de-facto agent-skills CLI. Its behavior is first-segment matching, with a fragment as the escape hatch |
| [3P][S] vercel skills accepts a `#ref` or `#ref@skill` fragment on git-like sources | same file, L284-L314 | `const atIndex = fragment.indexOf('@'); ... ref: ..., skillFilter: ...` | A cheap disambiguation syntax that users already know |
| [3P][O] vercel skills README lists its locator forms | https://github.com/vercel-labs/skills#source-formats | `owner/repo`, full URL, `.../tree/main/skills/web-design-guidelines`, GitLab, Azure, `git@...`, `./my-local-skills` | This is the closest analog to skillhub's `skill add` |
| [3P][S] vercel skills lock entry records source, ref, subpath and folder tree SHA | https://github.com/vercel-labs/skills/blob/3694740352eeef5cdd689af694c485f1ff62eec3/src/skill-lock.ts#L13-L33 | `sourceUrl` "original URL used to install the skill (for re-fetching updates)"; `ref?`; `skillPath?`; `skillFolderHash` "GitHub tree SHA for the entire skill folder. This hash changes when ANY file in the skill folder changes." | Provenance model: use the folder tree SHA, not the commit SHA, to detect upstream changes |
| [3P][S] degit takes the first segment after `tree`/`blob` as the ref, and uses `ls-remote --symref` only to resolve HEAD and refs | https://github.com/Rich-Harris/degit/blob/abb913a13706cc00f86f79ca81d54e104c3dd75c/src/domain/repo.ts#L108-L118 ; client.ts#L31 | `return { ref: segments[i + 1], subdir: segments.slice(i + 2) }`; `spawn('git', ['ls-remote', '--symref', ...])` | No longest-prefix matching here either |
| [3P][O] degit's documented ref syntax | https://github.com/Rich-Harris/degit/blob/master/docs/USAGE.md | "Append `#ref` to any source: `degit user/repo#dev`" | Same fragment convention |
| [3P][S] giget allows `/` inside the `#ref` fragment and defaults to `main` | https://github.com/unjs/giget/blob/f1dad453055b97a81d5227edce7262c8dcaf53d2/src/_utils.ts#L42-L56 | `(?<ref>#[-\w./@]+)?` ... `ref: m.ref ? m.ref.slice(1) : "main"` | The fragment is the established place for slashed refs |

**Verdict:** I found no surveyed tool that does longest-prefix matching against ls-remote. The ranked options are below.
1. Keep the first-segment rule and add an explicit `--ref` flag (or `#ref`). This matches existing practice.
2. Additionally, when the first-segment ref does not resolve, fall back to longest-prefix matching via a single `ls-remote --heads --tags`. This is an improvement over existing tools rather than a convention, and it costs one network call.

## Q3. Single-locator `add`

| Claim | URL | Quote | Relevance |
|---|---|---|---|
| [O] `cargo add` sources are `--git`, `--branch`, `--tag`, `--rev`, `--path` and `--registry` | https://doc.rust-lang.org/cargo/commands/cargo-add.html#source-options | `--rev` "Specific commit to use when adding from git."; `--path` "Filesystem path to local crate to add." | Refs as flags avoid URL ambiguity entirely |
| [O] `cargo add --dry-run` | https://doc.rust-lang.org/cargo/commands/cargo-add.html#dependency-options | "Don't actually write the manifest" | Dry-run on the add verb |
| [O] Cargo searches the whole git repo for the crate | https://doc.rust-lang.org/cargo/reference/specifying-dependencies.html#specifying-dependencies-from-git-repositories | "traverses the file tree to find `Cargo.toml` file for the requested crate anywhere inside the `git` repository" | Precedent for "repo URL plus name" discovery, so the user does not have to supply the subpath |
| [O] Cargo locks the git rev and only moves it on `cargo update` | https://doc.rust-lang.org/cargo/reference/specifying-dependencies.html#choice-of-commit | "Cargo locks the commits of `git` dependencies in `Cargo.lock` file at the time of their addition and checks for updates only when you run `cargo update`" | add = pin, update = an explicit separate verb |
| [O] npm accepts a folder, tarball file, tarball URL, git URL `#<commit-ish>` or `#semver:`, or `user/repo` | https://docs.npmjs.com/cli/v10/commands/npm-install#description | Tarball URL "must start with 'http://' or 'https://'"; `npm install <githubname>/<githubrepo>[#<commit-ish>]` | Dispatches on locator shape; ref goes in the fragment |
| [O] npm symlinks folders that are outside the project | same page | External folders are symlinked unless `--install-links` is passed | Local-dir import has to choose between copy and link |
| [O] npm records the git commit in the lockfile | https://docs.npmjs.com/cli/v10/configuring-npm/package-lock-json#packages | `resolved`: "In the case of git dependencies, this will be the full git url with commit sha." | Record the resolved SHA, not only the ref |
| [O] pip VCS: `@ref` plus `#subdirectory=` | https://pip.pypa.io/en/stable/topics/vcs-support/#url-fragments | "Pip looks at the `subdirectory` fragments of VCS URLs for specifying the path to the Python package, when it is not in the root" | A subpath fragment is a recognized alternative to tree URLs |
| [O] pip `-e` editable VCS installs | https://pip.pypa.io/en/stable/topics/vcs-support/#editable-vcs-installs | "VCS projects can be installed in editable mode (using the --editable option) or not." | The link-vs-copy choice for local sources, again |
| [O] `brew tap` registers a source repo, `brew update` refreshes it, and a fully qualified install trusts one item | https://docs.brew.sh/Taps | "`brew tap <user>/<repository>` clones ... Homebrew updates the repository during `brew update`."; "Install a fully qualified item to trust only that item: `brew install user/repository/formula`" | A direct analog of "source watch" vs "skill add" |

## Q4. Staged vs working tree

| Claim | URL | Quote | Relevance |
|---|---|---|---|
| [O] `git diff --cached` compares index to HEAD; plain `git diff` compares worktree to index | https://git-scm.com/docs/git-diff#_description | "view the changes you staged for the next commit relative to the named <commit>" | Basic primitive for detecting staged changes |
| [O] pre-commit validates **staged content only** by hiding unstaged changes | https://pre-commit.com/#pre-commit | "pre-commit only runs on the staged contents of files by temporarily stashing the unstaged changes while running hooks." | Established precedent for validating the index |
| [S] pre-commit's mechanism is a binary diff to a patch file, then `git checkout -- .`, then `git apply` to restore. On conflict it rolls back hook fixes | https://github.com/pre-commit/pre-commit/blob/main/pre_commit/staged_files_only.py | `logger.warning('Unstaged files detected.')`; `logger.info(f'Stashing unstaged files to {patch_filename}.')` | Caveat: it uses `diff-index` over tracked files, so **untracked files remain visible** to hooks, and it mutates the user's worktree |
| [O] lint-staged backs up state with `git stash create/store`, hides partially staged hunks through a patch, and resets on error | https://github.com/lint-staged/lint-staged#how-it-works | "the unstaged changes to these files are removed from the worktree so that tasks only see the staged changes"; "In case of any errors, the state is reset with `git reset` and the original state restored from the git stash." | Same approach with more safety machinery; `--no-stash` opts out |
| [O] `stash push --keep-index` leaves the index intact; documented use case is "Testing partial commits" | https://git-scm.com/docs/git-stash#Documentation/git-stash.txt-Testingpartialcommits | "All changes already added to the index are left intact." | Caveat from the man page: push stashes local modifications "in the working tree and in the index", so the stash also holds staged hunks, and `pop` can conflict. Untracked files need `-u` |
| [O] `checkout-index --prefix -a` exports the index to another directory | https://git-scm.com/docs/git-checkout-index#_examples | "`$ git checkout-index --prefix=git-export-dir/ -a` ... The final "/" is important." | **Recommended:** materializes exactly the staged snapshot without touching the worktree |
| [O] `write-tree` turns the index into a tree object, which `git archive <tree>` can export | https://git-scm.com/docs/git-write-tree ; https://git-scm.com/docs/git-archive | "Creates a tree object using the current index."; archive "containing the tree structure for the named tree" | Alternative export path that also gives a content hash of the staged state |
| [O] `git worktree add -d` checks out a **commit**, not the index | https://git-scm.com/docs/git-worktree#_description | "creates a new working tree with a detached HEAD at the same commit as the current branch" | Not suitable for snapshotting uncommitted staged content |

## Q5. Install vs update/watch

| Claim | URL | Quote | Relevance |
|---|---|---|---|
| [O] `helm repo add` registers a repository | https://helm.sh/docs/helm/helm_repo_add/ | "add a chart repository"; `--force-update` "replace (overwrite) the repo if it already exists" | `source add`: idempotence plus an explicit overwrite flag |
| [O] `helm repo update` refreshes the local index only | https://helm.sh/docs/helm/helm_repo_update/ | "update information of available charts locally from chart repositories" | `source update/check` refreshes metadata and does not install |
| [O] `helm install` takes one chart as a repo ref, `.tgz`, unpacked dir, URL, `--repo` or OCI | https://helm.sh/docs/helm/helm_install/ | "helm install mymaria example/mariadb" / "./nginx" / "https://example.com/charts/nginx-1.2.3.tgz" / "--repo https://example.com/charts/ mynginx nginx"; `--dry-run` | **Strongest analog:** `skill add` accepts either a registered-source ref or an ad hoc locator |
| [O] asdf `plugin add <name> <git-url>` vs `plugin update` | https://asdf-vm.com/manage/plugins.html | update "will fetch the _latest commit_ on the _default branch_ of the _origin_"; `asdf plugin list --urls --refs` | Lists show the source URL and ref, which is useful for `source list` |
| [O] VS Code installs by ID; updates happen automatically in the background | https://code.visualstudio.com/docs/configure/extensions/extension-marketplace#_extension-auto-update | "VS Code checks for extension updates and installs them automatically." (`extensions.autoUpdate`) | Auto-apply is a GUI default; CLIs (gh, cargo, helm, asdf) all make update explicit |

## Q6. Deprecation and aliases

| Claim | URL | Quote | Relevance |
|---|---|---|---|
| [O] cobra `Command.Deprecated`, `Aliases`, `Hidden`, `SuggestFor` | https://pkg.go.dev/github.com/spf13/cobra#Command | "Deprecated defines, if this command is deprecated and should print this string when used."; "Hidden ... should NOT show up in the list of available commands." | Standard Go vocabulary |
| [S] A deprecated command prints `Command %q is deprecated, %s` and is excluded from help listings | https://github.com/spf13/cobra/blob/adbc8813901bba65827259daa8e22ff94ec1f30e/command.go#L910-L911 , #L1606-L1610 | `if len(c.Deprecated) != 0 \|\| c.Hidden { return false }` | Keep the command working, hide it, and warn |
| [S] Caveat: cobra prints that warning via `OutOrStderr()`, so it lands on **stdout** if the app called `SetOut(os.Stdout)` | same file #L1435-L1437 | `fmt.Fprint(c.OutOrStderr(), i...)` | Warnings must not corrupt `--json` stdout |
| [S] pflag `MarkDeprecated` hides the flag and prints "Flag --%s has been deprecated, %s" | https://github.com/spf13/pflag/blob/c966cfef47379dcb01e7929504d66d94b540945b/flag.go#L488-L500 | "It will continue to function but will not show up in help or usage messages." | Same pattern applied to flags |
| [S] gh uses this for renames | https://github.com/cli/cli/blob/fc4b137cdef0a6bd28fd461b7cf9c84a5812a8cd/pkg/cmd/repo/delete/delete.go | `cmd.Flags().MarkDeprecated("confirm", "use `--yes` instead")` | The message names the replacement |
| [O] Kubernetes policy: kubectl CLI elements must keep working for 12 months or 2 releases (GA) and must warn | https://kubernetes.io/docs/reference/deprecation-policy/#deprecating-a-flag-or-cli | "Rule #5a: CLI elements of user-facing components (e.g. kubectl) must function after their announced deprecation for no less than: GA: 12 months or 2 releases"; "Rule #6: Deprecated CLI elements must emit warnings (optionally disable) when used." | Concrete deprecation window to borrow |

Repo fit: skillhub's `go.mod` does not import cobra, so these fields do not apply directly. Replicate the pattern by hand: keep the old verb as an alias, leave it out of help, and write one stderr warning that names the replacement.

## Q7. clig.dev (https://clig.dev, source: github.com/cli-guidelines/cli-guidelines `content/_index.md`)

| Guideline | Anchor | Quote | Relevance |
|---|---|---|---|
| Dry run | #arguments-and-flags | "`-n`, `--dry-run`: Dry run. Do not run the command, but describe the changes that would occur" | `skill add` and `source update` should preview |
| Confirmation levels | #arguments-and-flags | "Moderate: ... a complex bulk modification that can't be easily undone. You usually want to prompt ... Consider giving the user a way to 'dry run'"; "Let them alternatively pass a flag such as `--confirm="name-of-thing"`, so it's still scriptable." | Bulk updates from a watched source fall under "moderate" |
| Never require a prompt | #interactivity | "Never _require_ a prompt ... If `stdin` is not an interactive terminal, skip prompting and just require those flags/args."; "If the command requires input, fail and tell the user how to pass the information as a flag." | Agents run without a TTY |
| `--json` | #output | "Display output as formatted JSON if `--json` is passed." | Same as gh |
| stdout/stderr | #the-basics | "Send messaging to `stderr`. Log messages, errors, and so on should all be sent to `stderr`." | Deprecation warnings and progress go to stderr |
| TTY heuristic | #output | "whether a particular output stream ... is being read by a human is _whether or not it's a TTY_" | Same as gh |
| Tell the user about state changes | #output | "If you change state, tell the user." | Report what was imported and where |
| Suggest next commands | #output | "Suggest commands the user should run. When several commands form a workflow, suggesting ... commands they can run next" | e.g. after `source add`, print the `skill add <source>/<name>` hint |
| Avoid update/upgrade confusion | #subcommands | "having two subcommands called 'update' and 'upgrade' is quite confusing." | Do not ship both `source update` and `skill upgrade` |
| Noun-verb consistency | #subcommands | "Use the same flag names for the same things"; "`noun verb` seems to be more common" | `source add/list/update/remove`, `skill add/list/remove` |
| Deprecate in-program | #future-proofing | "forewarn your users in the program itself: when they pass the flag you're looking to deprecate, tell them it's going to change soon." | Same as kubectl rule #6 |
| No catch-all, no implicit abbreviations | #future-proofing | "Don't have a catch-all subcommand."; aliases "should be explicit and remain stable." | Do not let `skillhub <url>` mean add |
| Not requiring IDs | none | **Not found** as a clig.dev guideline. The closest evidence is gh's context inference (`gh pr view` defaults to the current branch) | Treat this as a design choice, not a cited convention |

## Trade-off: how to locate a ref (ranked)

| Option | Correctness on slashed refs | Network cost | Precedent | Rank |
|---|---|---|---|---|
| First segment after `tree/` plus `--ref`/`#ref` override | Wrong by default for slashed refs; user can fix it | 0 extra calls | vercel skills, degit, giget | 1 (baseline) |
| Option 1 plus a longest-prefix fallback via `ls-remote --heads --tags` | Correct | +1 call, only on a miss | git primitives only | 2 (worth adding) |
| REST `matching-refs` prefix probe | Correct | 1+ API calls, rate-limited, GitHub only | GitHub API | 3 |

## Limitations

- I did not open vercel skills `update.ts` in detail. The claim that it compares `skillFolderHash` for updates rests on the lock-file comment, not on reading the update loop.
- I did not verify whether npm has any git subdirectory syntax. The v10 docs list none.
- The `pre-commit.com` quote comes from the docs page. The untracked-files caveat is my inference from `diff-index` usage in the source.
- Kubernetes and clig.dev are guidance documents, not enforced standards.

## Unresolved questions

1. Should skillhub treat a slashed-ref miss as a hard error (explicit is better than implicit) or fall back to longest-prefix matching automatically?
2. Should imported provenance record the folder tree SHA (as vercel does), the commit SHA (as cargo and npm do), or both?

Status: DONE_WITH_CONCERNS
Summary: Every question is answered with pinned official and source citations. The surveyed tools (vercel skills, degit, giget) use the first segment after `tree/` as the ref plus a `#ref` override; none does longest-prefix ref matching. helm `repo add`/`install` and brew `tap`/`install` are the clearest precedents for a source-vs-skill split.
Concerns: clig.dev has no "don't require IDs" guideline, and the vercel update-loop internals were inferred from lock-file comments.
