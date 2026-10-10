# Wave 3 · Worktree F round 2: lead review of 420e862..3b9673a

```text
Lead review of your six commits. make check is green and there is no conflict
with main or with worktree C. Five problems remain. Same rules as before:
your worktree only, absolute paths, test-first, one commit per item, make check
green before each commit, no push, no merge. Do not touch server.go or anything
C owns.

1. HIGH, the redactor now over-redacts ordinary prose (redactor.go:35). I ran it:
   "Write a basic example for the API" -> "Write a [TOKEN] for the API"
   "Basic usage of the tool"           -> "[TOKEN] of the tool"
   "use sk-learn-pipeline for ML"      -> "use [TOKEN] for ML"
   "the bearer of bad news"            -> "the [TOKEN] bad news" (this one is older)
   Match Basic and Bearer only as credentials: after `Authorization:` (any case),
   or followed by a credential-shaped token (Basic: base64 of 8 or more
   characters; Bearer: 16 or more token characters including a digit).
   Without the `(?i)` flag, `basic` and `bearer` in prose must stay unchanged.
   For sk-, require at least 20 characters after `sk-` and at least one digit.
   Add all four strings above to TestRedactorDoNotOverRedactOrdinaryWords, and
   keep every positive case from round 1 passing.

2. MEDIUM, tools_list_bytes now writes the client name into skill_id
   (rollup.go:98-104). aggregateRollups (usage.go:330-340) then makes a skill
   accumulator for every non-empty skill_id, so the funnel lists "claude-code"
   as a skill. Use a metric key that carries the client instead, for example
   metric `tools_list_bytes:<client>` with an empty skill_id, and keep the max
   per day. The accumulator keeps the max overall. Add a test: the Skills list
   in the funnel contains no client names.

3. MEDIUM, per-bucket counts send transcript events to the wrong bucket.
   transcript_import.go:139 always writes Client{Name: "skillhub"}, so
   parseBaselineBucketCounts puts every transcript_skill_uses /
   native_no_resolve count in a (snapshot, "skillhub") bucket with no chains.
   The real client buckets then always show bypass_rate unknown, and an extra
   empty bucket appears (usage_baseline.go:315-319 adds buckets from counts
   alone). Attribute transcript events by payload source: source=claude goes to
   the bucket of the Claude Code client name that the MCP server records
   (check the real name in the tracker or initialize handling; do not guess).
   Do not create a bucket from counts alone. Write a test with chains from the
   Claude Code client and transcript events from transcript_import: bypass_rate
   is known in that bucket, and no "skillhub" bucket exists.

4. MEDIUM, the caseEventIDs sync.Map (cases.go:104-108, recorder.go:254,587)
   has three problems.
   - LoadOrStore runs before gate.RLock while Purge and resetCounters reassign
     the map, which is a data race.
   - It grows without limit for the life of the process.
   - It keeps the ID even when the queue is full and the case is dropped.
   recordCaseStore already checks event_id inside the BEGIN IMMEDIATE
   transaction, so delete the in-memory map. If a caller needs ErrCaseConflict
   synchronously, say which caller and why. Add a -race test that runs Purge
   concurrently with RecordCase.

5. LOW
   - internal/app TestUsageServiceFunnel fails under `go test -race`. The race
     is between the test's `clock` variable (usage_test.go:50-81) and
     maintainStore on the worker. This also happens on main and is not your
     regression, but fix it: give the test clock an atomic or a mutex. After
     that, `go test -race ./internal/app ./internal/telemetry
     ./internal/delivery/mcpserver` must be green. I will run it.
   - funnel --json still returns 0 for UnlistedResourceReads and
     UnsupportedMethodCalls. Make the JSON match the text output: null or
     omitted, plus a status of "unmeasured" (additive only, and document it).

Final report: hashes, what changed and the test that proves it for each item,
the actual output of the four redactor strings after the fix, the output of the
race command, and make check. End with "Status: DONE | DONE_WITH_CONCERNS |
BLOCKED" and one sentence.
```
