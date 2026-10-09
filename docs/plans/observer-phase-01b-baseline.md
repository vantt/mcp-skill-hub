# Observer Phase 01b: Baseline Tooling & Tools Measurement

**Date:** 2026-10-09  
**Branch:** `wave3/observer-baseline`  
**Parent Plan:** [docs/plans/2026-10-08-observer-and-enrichment.md](2026-10-08-observer-and-enrichment.md) (§1.2 O7, §1.3, §5.1)  
**Related Design:** [docs/design/04-telemetry-reproducibility-evaluation.md](../design/04-telemetry-reproducibility-evaluation.md) (§4.1, §9.1)

---

## 1. Goal

Implement the baseline measurement tooling (O7) for telemetry observer evaluation before baseline capture begins (actual capture occurs after Simplify Phase 4 merges):
1. Add `skillhub telemetry baseline [--since <duration|date>] [--min-chains N] [--json] [--write <path>] [--workspace <path>]`.
2. Reuse existing `UsageService` chain reconstruction and O5 metrics calculations without creating a secondary metric engine.
3. Group metrics into buckets per `catalog_snapshot × client`.
4. Enforce sufficiency criteria per bucket: `sufficient` only when `resolved_chains >= N` AND `no_skill_chains >= N`. Otherwise mark `insufficient` and report missing counts.
5. Identify the newest `catalog_snapshot` as the active baseline candidate window; designate older snapshots as `superseded`.
6. Verify whether the baseline window lies within raw retention (30 days) and report if any portion was pruned.
7. Track the `unsolicited` load share per bucket to expose potential tracker RAM losses (e.g., across server restarts).
8. Ensure rates that cannot be measured are reported as `unknown` (nil in JSON) rather than `0`.
9. Ensure `--write` exports clean JSON without task descriptions, user conversations, or case text.
10. Provide an in-process `tools/list` measurement test to track payload bytes and token consumption.

---

## 2. Statistical Justification for Chosen Minimum Chains ($N$)

### Hub Traffic Characteristics
`mcp-skill-hub` is a local hub serving developer environments. As observed on 2026-10-08, production usage has low event velocity (3 resolutions, 0 loads). In such low-traffic deployments, collecting hundreds of events per snapshot requires months of calendar time, while too small a sample produces unacceptably wide confidence intervals.

### Margin of Error Analysis
For binomial routing metrics (such as `acceptance_rate`, `override_rate`, and `false_no_skill_rate`), the standard error is:
$$SE = \sqrt{\frac{p(1 - p)}{N}}$$

At a 95% confidence interval ($z = 1.96$), the margin of error ($MoE$) is:
$$MoE = 1.96 \cdot \sqrt{\frac{p(1 - p)}{N}}$$

| $N$ (min chains) | Worst-Case $MoE$ ($p = 0.50$) | Realistic $MoE$ ($p = 0.85$ acceptance) | Expected Time to Collect (at ~2 chains/day) |
|---|---|---|---|
| $N = 10$ | $\pm 31.0\%$ | $\pm 22.1\%$ | ~5 days |
| $N = 20$ | $\pm 21.9\%$ | $\pm 15.6\%$ | ~10 days |
| **$N = 30$ (Chosen Default)** | **$\pm 17.9\%$** | **$\pm 12.8\%$** | **~15 days** |
| $N = 50$ | $\pm 13.9\%$ | $\pm 9.9\%$ | ~25 days |
| $N = 100$ | $\pm 9.8\%$ | $\pm 7.0\%$ | ~50 days |

### Decision
- **Chosen default: $N = 30$** (`--min-chains 30`).
- **Reasoning:**
  1. $N = 30$ satisfies the standard central limit theorem heuristic for sampling distributions.
  2. For expected primary metrics where rates lie in the range $[0.80, 0.90]$ (e.g. acceptance rate), the 95% confidence margin of error is $\pm 12.8\%$, providing sufficient discriminative power to detect regressions $> 15\%$ without stalling baseline collection.
  3. Worst-case uncertainty is bounded below $\pm 18\%$.
  4. The parameter is CLI-configurable via `--min-chains <N>`, allowing high-traffic automated benchmark environments to set $N = 50$ or $N = 100$ when desired.

---

## 3. Files

- `docs/plans/observer-phase-01b-baseline.md`: This plan and measurement record.
- `internal/app/usage_baseline.go`: Core baseline reporting service, bucket grouping, and sufficiency evaluation.
- `internal/app/usage_baseline_test.go`: Unit tests for baseline computation, sufficiency transitions, and pruning detection.
- `internal/delivery/cli/telemetry_baseline.go`: CLI command handler for `skillhub telemetry baseline`.
- `internal/delivery/cli/telemetry_baseline_test.go`: CLI end-to-end tests including `--json`, `--write`, and absence of case text.
- `internal/delivery/mcpserver/tools_size_test.go`: In-process measurement harness for `tools/list` payload and outputSchema size.

---

## 4. Implementation Steps

1. **`internal/app/usage_baseline.go`**:
   - Define `BaselineReport`, `BaselineBucket`, `BucketSufficiency`, and `BaselineQuery`.
   - Query raw events within the given window (`UsageService.buildChains`).
   - Group chains by `(catalog_snapshot, client)`.
   - Track unsolicited loads per bucket to measure server-restart attribution loss.
   - For each bucket:
     - Count `resolved_chains` and `no_skill_chains`.
     - Evaluate sufficiency against $N$: `resolved >= N && no_skill >= N`. If insufficient, report `missing_resolved = max(0, N - resolved)` and `missing_no_skill = max(0, N - no_skill)`.
     - Calculate O5 rate metrics via `calculateChainMetrics`.
   - Order catalog snapshots chronologically. The newest snapshot is marked `is_baseline_window: true` (or status `candidate`/`active`); older snapshots are marked `status: "superseded"`.
   - Verify if earliest event in raw retention indicates window truncation (`pruned: true/false`).

2. **`internal/delivery/cli/telemetry_baseline.go`**:
   - Parse `--since` (default 30d), `--min-chains` (default 30), `--json`, `--write <path>`, and `--workspace`.
   - Format human-readable terminal table showing status, sufficiency, chain counts, O5 rates, and unsolicited share.
   - If `--write` is specified, serialize the report to JSON and write to disk, ensuring directory creation.
   - Ensure the JSON payload contains no task descriptions or case records.

3. **`internal/delivery/mcpserver/tools_size_test.go`**:
   - Start in-process MCP server with in-memory transport.
   - Query `ListTools`.
   - Measure and report total bytes, tokens, per-tool breakdown, outputSchema share, and hypothetical runtime profile.

---

## 5. Verification & Tests

1. **Sufficiency Transition:** An initial set of events with $< N$ resolved or $< N$ no_skill reports `insufficient` with exact missing counts; adding events until both reach $N$ flips verdict to `sufficient`.
2. **Snapshot Reset & Supersession:** When a new `catalog_snapshot` appears, it becomes the new baseline window; the older snapshot becomes `superseded`.
3. **Unknown vs Zero:** Rates with 0 denominator report status `unknown` and `nil` rate, never misleading `0.0`.
4. **Unsolicited Attribution Tracking:** Unsolicited loads are tracked and reported as a percentage of total bucket loads.
5. **No Case Text Leakage:** Assert that JSON output written by `--write` or emitted via `--json` contains no task text, prompts, or case journal entries.
6. **Retention Pruning Detection:** Report accurately notes when the requested baseline window extends beyond retained raw events.

---

## 6. MCP tools/list Size Measurements

Measurement captured via `go test -v ./internal/delivery/mcpserver -run TestToolsListSize`:

### Comparison: 2026-10-08 (§5.1) vs 2026-10-09 (Today)

| Metric | 2026-10-08 Baseline (§5.1) | 2026-10-09 Measured Today | Delta |
|---|---|---|---|
| **Total registered tools** | 43 | 43 | 0 |
| **Total `tools/list` payload** | 223,674 bytes (~56k tokens) | 223,867 bytes (~55,966 tokens) | +193 bytes (+0.09%) |
| **Total `outputSchema` bytes** | ~178,000 bytes (~80%) | 177,068 bytes (79.1% of payload) | -932 bytes |
| **Largest individual tool** | `source_triage` (15,624 bytes) | `source_triage` (14,293 bytes, 88.6% output) | -1,331 bytes |
| **Runtime profile (3 tools)** | 18,642 bytes (~4.7k tokens) | 18,681 bytes (~4,670 tokens) | +39 bytes (+0.2%) |
| **Runtime profile (no `outputSchema`)** | ~7,400 bytes (~1.8k tokens) | 7,432 bytes (~1,858 tokens) | +32 bytes (+0.4%) |
| **Runtime `outputSchema` share** | ~60% | 60.2% (11,249 bytes) | +0.2% |

### Top 5 Heaviest Tools Today
1. `source_triage`: 14,293 bytes (~3,573 tokens), `outputSchema`: 12,666 bytes (88.6%)
2. `curation_run_start`: 10,735 bytes (~2,683 tokens), `outputSchema`: 10,089 bytes (94.0%)
3. `skill_resolve`: 9,720 bytes (~2,430 tokens), `outputSchema`: 5,028 bytes (51.7%)
4. `routing_evaluate`: 9,026 bytes (~2,256 tokens), `outputSchema`: 4,802 bytes (53.2%)
5. `curation_run_submit`: 8,946 bytes (~2,236 tokens), `outputSchema`: 4,803 bytes (53.7%)

### Runtime Profile Tools Breakdown
- `skill_resolve`: 9,720 bytes (Output: 5,028 bytes / 51.7%)
- `skill_get`: 5,680 bytes (Output: 4,178 bytes / 73.6%)
- `skill_feedback`: 3,267 bytes (Output: 1,995 bytes / 61.1%)
- **Total Runtime Set:** 18,681 bytes (~4,670 tokens). Without `outputSchema`: 7,432 bytes (~1,858 tokens).
