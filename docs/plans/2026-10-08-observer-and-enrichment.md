# Plan: Observer trước, làm giàu thông tin sau

**Ngày:** 2026-10-08
**Trạng thái:** D1–D8 đã duyệt 2026-10-08; Phase 5: câu 11–15 đã duyệt
**Phạm vi:** host mcp-skill-hub (agent ↔ hub, telemetry, resolver, curation). Không bao gồm skill `distill`.
**Nguồn:** deep-dive meta-skill (`docs/distillery/deep-dives/skill-recommendation.md`), thiết kế vòng enrichment trong chat 2026-10-08
**Liên quan:** [doc 04 telemetry/evaluation](../design/04-telemetry-reproducibility-evaluation.md), [doc 06 source learning](../design/06-source-learning-and-distillation.md), [doc 03 resolver](../design/03-resolver-design.md), PRD §30, §45

## 0. Nguyên tắc

1. **Đo trước, tối ưu sau.** Chưa đổi ranking, chưa thêm channel, chưa tự sinh metadata
   khi chưa có số liệu chuẩn hóa để so trước/sau. Mọi thay đổi resolver hoặc metadata
   về sau phải chỉ ra metric nào nó cải thiện.
2. **Server tự quan sát, không trông vào agent.** Agent hay quên gọi `skill_feedback`;
   tín hiệu chính phải là thứ hub thấy được (resolve, load, re-resolve, transcript).
3. **Ưu tiên của dự án:** (1) chất lượng gợi ý, (2) làm giàu thông tin để gợi ý đúng,
   (3) UX CLI/web giúp curate nhẹ nhàng.
4. **Usage không được âm thầm sửa nội dung Git** (PRD §45). Mọi thứ học từ usage đi qua
   preview/confirm.
5. Tối giản: dùng lại event model, rollup, funnel, insight/proposal lifecycle đã có.

---

## Phase 1 — Chuẩn hóa observer (làm trước)

Mục tiêu: một vòng resolve → load → kết quả được ghi **đầy đủ, nối được với nhau, và
đọc được** bằng một lệnh; có baseline metric trước khi tối ưu bất cứ thứ gì.

### 1.1 Hiện trạng đã xác minh trong code (2026-10-08)

Đã có:

- Event envelope + allowlist payload, content_mode `none`, retention 14 ngày raw, rollup
  180 ngày (`internal/telemetry`).
- Attribution phía server cho mỗi lần load: `recommended | supporting | override |
  after_no_skill | after_needs_context | unsolicited`
  (`internal/delivery/mcpserver/activation_tracker.go:131-163`), giữ resolution theo
  session trong RAM với `resolutionTTL`.
- `skill.loaded` server-observed có `SessionIDHash` (`mcpserver/server.go:303`).
- Funnel: `skillhub telemetry funnel [--since --until --skill]` với acceptance_rate,
  overrides, misses, unsolicited, negative_after_load, transcripts (`internal/app/usage.go`).
- Feedback (`skill_feedback`) → `task.outcome_reported`, `skill.utility_reported`,
  `skill.used/abandoned`, `activation.rejected`.
- Transcript scan Claude Code → `transcript.tool_observed` (bắt skill native dùng không qua hub).
- Promotion `resolution_id` → eval case template (người phải điền phần còn thiếu).
- Distill/curation events được phát đầy đủ.

Lỗ hổng (đây là việc của Phase 1):

| # | Lỗ hổng | Bằng chứng | Hệ quả |
|---|---|---|---|
| G1 | Resolution event **không có `SessionIDHash`** | `internal/app/resolver.go:270-282` không set; chỉ load event có | Không nối được resolve với load/re-resolve ngoài RAM; không đếm được "số session khác nhau" |
| G2 | `Client` luôn là `"skillhub"` | `resolver.go:273`, `transcript_import.go:140` | Không so được chất lượng theo agent (Claude Code / Codex / Cursor…), dù MCP `initialize` có `clientInfo` |
| G3 | `candidate_count` thực ra là số skill **được gợi ý** (primary + supporting), không phải số ứng viên retrieval | `resolver.go:227,233` vs doc 04 §3.3 (`candidate_count: 17`) | Không biết resolver lọc từ bao nhiêu ứng viên; số liệu sai nghĩa |
| G4 | Không có `channels`, `stage_ms` trong resolution event | payload ở `resolver.go:223-241` chỉ có `duration_ms` | Không biết channel nào (fts/rules) tạo ra kết quả; không đo latency từng stage |
| G5 | Không ghi liên kết **re-resolve** (`prior.resolution_id`, `prior.kind`) | `addRequestTelemetry` chỉ ghi operation, artifact_kind | Mất tín hiệu quý nhất: agent phải viết lại câu mới tìm ra skill |
| G6 | `clarification.requested/answered` khai báo nhưng **không phát** | không có emitter ngoài `events.go` | `needs_context` không đo được tỷ lệ trả lời và kết quả sau đó |
| G7 | `index.rebuilt`, `catalog.changed`, `evaluation.run_completed`, `insight.reopened`, `incorporation.outcome_recorded`, `task.completed` khai báo nhưng không phát | như trên | Không gắn được metric với thay đổi catalog/eval; không đo outcome sau apply |
| G8 | "Recommended nhưng không load gì trong TTL" (bỏ qua lặng lẽ) không thành event | tracker chỉ prune | Acceptance rate chỉ tính phía có load; ignore rate vô hình |
| G9 | Không lưu top-k ứng viên và lý do khớp | chỉ `top_skill_id` | Khi sai, không biết skill đúng đứng hạng mấy, khớp trường nào |
| G10 | Hub quên nội dung task (content_mode khóa ở `none`, `recorder.go:146`) | | Có số đếm "sai" nhưng không có gì để học (xem Phase 2) |

### 1.2 Việc cần làm

**O1. Nối chuỗi sự kiện (G1, G2, G5)** — rẻ, giá trị cao nhất.

- Set `SessionIDHash` cho mọi resolution event (dùng cùng `tracker.sessionHash`).
- Ghi `Client{Name, Version}` từ MCP `clientInfo` (giá trị token-safe, đã có allowlist pattern).
- Thêm payload `prior_resolution_id` (token) và `prior_kind` (`clarification|rejected`).
- Kết quả: mỗi "lượt tìm skill" thành một chuỗi `res1 → (rejected) → res2 → load B` dựng
  lại được từ telemetry, không phụ thuộc RAM.

**O2. Sửa nghĩa số đo (G3, G4).**

- `candidate_count` = số ứng viên sau retrieval (đúng doc 04); thêm `recommended_count`
  nếu cần số cũ. Ghi rõ trong changelog event schema (đổi nghĩa → bump `event_version`).
- Thêm `channels` (`fts`, `rules`, …) và `stage_ms` (validation, retrieval, scoring, total).

**O3. Phát các event đã thiết kế (G6, G7, G8).**

- `clarification.requested` khi trả `needs_context` (field, reason_codes);
  `clarification.answered` khi request mới mang `prior.kind: clarification`.
- `catalog.changed` / `index.rebuilt` khi catalog generation đổi → mốc để so trước/sau.
- `evaluation.run_completed` cho routing eval gate.
- Bộ đếm cho câu 13 (Phase 5): đọc path không có trong `resources` của `skills/get`, gọi method skills hub không hỗ trợ.
- `recommendation.ignored` (mới, hoặc dùng `skill.abandoned` với attribution riêng —
  quyết định khi làm): khi resolution `resolved` hết TTL mà không có load nào.

**O4. Trace ứng viên có giới hạn (G9).**

- Với mỗi resolution, giữ top-k (k ≤ 5) `{skill_id, rank, matched_fields[], channel}`.
  Đây là ID + enum, không phải nội dung → hợp với content_mode `none`.
- Mặc định chỉ persist khi kết quả có **bất đồng** (override / after_no_skill /
  rejected / ignored); còn lại chỉ giữ RAM. Lý do: đủ để chẩn đoán, không phình DB.
- Doc 04 §4.1 có `persist_candidate_scores: false` → đổi thành chỉ persist rank +
  matched fields, không persist raw score.

**O5. Chuẩn hóa định nghĩa metric (một nguồn sự thật).**

Viết bảng định nghĩa trong doc 04 §9.1, mỗi metric có công thức, mẫu số, và event nguồn.
Metric chất lượng gợi ý tối thiểu:

| Metric | Công thức | Ý nghĩa |
|---|---|---|
| acceptance_rate | `activation:recommended / recommended:primary` | đã có |
| override_rate | `activation:override / resolution:resolved` | gợi ý sai skill |
| false_no_skill_rate | `activation:after_no_skill / resolution:no_skill` | nói "không có" nhưng thật ra có |
| true_no_skill | `no_skill` không có load nào trong TTL | catalog thật sự thiếu |
| reformulation_rate | chuỗi có `prior_kind: rejected` / tổng chuỗi | agent phải viết lại câu — chỉ số "đau" trực tiếp |
| ignore_rate | resolved không có load / resolved | gợi ý bị bỏ qua lặng lẽ |
| needs_context_answer_rate | `clarification.answered / clarification.requested` | câu hỏi có đáng hỏi không |
| bypass_rate | `native:no_resolve / transcript skill uses` | agent không hỏi hub |
| negative_after_load | đã có | load rồi mới thấy sai |

Mọi metric cắt được theo: `skill_id`, `client`, `operation`, `catalog_snapshot`,
`policy_revision`, khoảng ngày. Tách theo `catalog_snapshot` là điều kiện để đo trước/sau.

**O6. Một màn hình đọc được.**

- `skillhub telemetry funnel` thêm các metric O5 và `--by client|operation|snapshot`.
- `skillhub telemetry chains --since 7d [--kind override|after_no_skill|reformulation]`:
  liệt kê chuỗi bất đồng (ID, skill, rank skill đúng, matched fields) — đây là thứ người
  đọc để hiểu "vì sao gợi ý sai" trước khi có Phase 2.
- Web UI: thêm tab chất lượng gợi ý dùng cùng JSON.

**O7. Baseline.**

- Sau O1–O6, chạy dùng thật ≥ 2 tuần, chụp baseline (JSON funnel theo snapshot) vào
  `docs/brainstorm/` hoặc một report. Mọi đề xuất tối ưu sau phải so với baseline này.

**O8. Hướng dẫn agent (một dòng, không thêm field).**

- Trong `CLAUDE.md`/curator: khi không dùng skill được gợi ý, resolve lại với
  `prior.kind: rejected` thay vì tự chọn (PRD §30). Khi dùng skill khác, gọi
  `skill_feedback` với `skill_id` thật.

### 1.3 Tiêu chí xong Phase 1

- Từ telemetry (không cần RAM) dựng lại được chuỗi resolve → re-resolve → load cho một session.
- Funnel hiện đủ metric O5, cắt được theo client và snapshot.
- Event schema + doc 04 §3, §9 khớp code; test allowlist cho field mới.
- Có baseline ≥ 2 tuần.

---

## Phase 2 — Case journal (bắt đầu học được "vì sao")

Phụ thuộc: O1, O4.

- Giữ `task.description`, `operation`, `fact_keys` trong RAM của tracker cùng resolution.
- Khi có bất đồng (override, after_no_skill, reformulation, needs_context→resolved,
  rejected/scope_mismatch, gap lặp lại), ghi một **case** xuống store runtime riêng
  (không phải Git, không export). Resolution đồng thuận: không ghi nội dung.
- Redaction pipeline doc 04 §4.3 (allowlist, path normalization, secret/PII patterns,
  length limits) áp dụng trước khi ghi. Retention đề xuất 90 ngày, trần ~500 case.
- Đây là bản hẹp của content mode `redacted` đã thiết kế nhưng chưa làm.

Ví dụ case:

```yaml
case: cs_7f2a
kind: reformulation        # reformulation | override | false_no_skill | needs_context | rejected | gap
at: 2026-10-08T09:12Z
session: hmac:…
client: claude-code
catalog_snapshot: sha256:a180…
task: "triage why the nightly e2e job flakes only on arm runners"   # đã redact
operation: debug
resolver: {status: no_skill, top: [{id: ci-debug, rank: 4, matched: [operation]}]}
chosen: ci-debug
followup: {reformulated_to: "debug flaky CI test on arm"}
```

Bảng tín hiệu → bài học:

| Chuỗi | Bài học | Độ tin |
|---|---|---|
| resolve(d1) → rejected → resolve(d2) → load B | d1 nên là example của B | Cao |
| resolved A → load B | B thiếu trigger/example; A cần counter_example / distinguish_from B | Cao |
| no_skill → load B | B thiếu từ cho loại task này | Cao |
| needs_context → trả lời F → resolved B | F nên thành requirement/rule của B | Trung bình |
| recommended A → scope_mismatch / user_rejected | A cần not_for / counter_example | Trung bình (basis user: cao) |
| recommended A → không load | bị bỏ qua lặng lẽ | Thấp, chỉ đếm |
| no_skill → không load, lặp lại | catalog thiếu skill → intake/create, không phải metadata | Theo số lần |
| transcript: skill native X không resolve | hub bị bỏ qua; X chưa có → đề xuất import | Trung bình |
| recommended B → completed, helpful (user) | xác nhận; làm eval case chống regression | Cao |

Example rút từ case khớp phân phối query thật hơn example viết tay, vì đó chính là câu
agent gửi.

`skillhub telemetry cases` để đọc; xóa được (`telemetry purge` bao cả cases).

---

## Phase 3 — Tổng hợp + kiểm chứng (vòng enrichment)

Phụ thuộc: Phase 2 có đủ case.

### 3.1 Coi usage là một source trong lifecycle có sẵn

| Lifecycle (doc 06) | Usage |
|---|---|
| Observation | case |
| Comparison | cụm case của skill B so với metadata hiện tại |
| Insight | "B thiếu trigger X / A cần counter_example" |
| Application Proposal | patch metadata + kết quả replay |
| Incorporation outcome | metric trước/sau của skill đó |

Dùng lại inbox, preview/confirm, web UI, staleness theo `base_catalog_version`.

### 3.2 Tổng hợp

Hub (tất định), với mỗi skill B có ≥ 2 case từ ≥ 2 session:

- trigger ứng viên: token (cùng tokenizer FTS) xuất hiện ≥ 2 case, chưa có trong metadata
  B, hiếm trên catalog, không thuộc `not_for` của B;
- example ứng viên: ≤ 3 câu task, khử gần trùng;
- skill thua A (nếu là primary): `counter_example`, `distinguish_from: B`.

Agent (qua `curation_run`, hub không có LLM): khái quát hóa câu task — bỏ tên project,
path, khách hàng — vì nó sẽ vào thư viện Git.

### 3.3 Replay-verify trước khi tới người

Dựng catalog generation tạm có patch, chạy:

1. cụm case của nó phải lật sang B (không lật → bỏ đề xuất);
2. mọi case khác: đếm lật đúng / lật sai;
3. routing eval gate (42 skill, 168 example, 126 counter-example, 34 no-skill) không tụt;
4. self-resolution lint: example của B vẫn về B, của A vẫn về A.

Preview: `ci-debug: +2 trigger, +1 example — sửa 4/5 case, 0 regression; test-runner: +1 counter_example`.
**Một confirm ghi cả patch metadata lẫn case (đã khái quát) vào golden corpus.**

### 3.4 Sau khi apply

- `incorporation.outcome_recorded`: so override/miss của skill trước/sau theo snapshot;
  xấu đi → `insight.reopened`.
- Case cũ được replay trên catalog mới; đã tự đúng → đóng, ghi "fixed by <commit>".

### 3.5 Rủi ro và chặn

- Agent sai cũng tạo override → ngưỡng session khác nhau + replay + người confirm.
- Skill phổ biến phình rộng → điều kiện "hiếm trên catalog", counter-example và no-skill trong gate.
- **Prompt injection**: câu task là dữ liệu không tin cậy sẽ vào metadata phục vụ mọi agent →
  giới hạn độ dài/ký tự trigger, không auto-apply, preview hiện nguyên văn.
- Overfit vào chính case → giữ ~1/3 case đã promote làm held-out, không dùng sinh đề xuất.

---

## Phase 4 — Cải tiến resolver dựa trên số liệu

Chỉ bắt đầu khi Phase 1 có baseline; mỗi mục phải chỉ ra metric mục tiêu.
Nguồn: deep-dive meta-skill (`docs/distillery/deep-dives/skill-recommendation.md`).

| Ý tưởng | Metric mục tiêu | Ghi chú |
|---|---|---|
| FactProvider tối thiểu: marker file project, giới hạn cwd, có trần ambient | false_no_skill_rate, needs_context rate | lesson `project-signal-detector` |
| Cửa lọc (admission gate) cho kết quả vector trước khi bật channel vector | override_rate khi bật vector | ghi vào doc 03 §13 trước; lesson `semantic-admission-gate` |
| Ẩn/ghim skill theo người dùng, dạng fact có giải thích | override_rate theo user | lesson `user-skill-preferences` |
| Không dùng bandit; thay đổi trọng số đi qua eval + promotion từ telemetry | — | lesson `eval-before-ranking-changes`; meta-skill bandit không có giá trị thực |
| Findability lint với self-resolution khi thêm skill | dead skills, false_no_skill | lesson `findability-lint` |
| `lint --explain` / `--fix`, màn hình "vì sao skill này khó được chọn" | curation UX (doc 04 §9.6) | lesson `lint-explain-and-fix` |

---

## Phase 5 — Các vấn đề spec/host (đã quyết 2026-10-08)

Phát hiện trong lúc distill; quyết định thiết kế đã chốt, chưa implement:

| Vấn đề | Đề xuất |
|---|---|
| `tools/list` (danh sách tool MCP, không phải danh sách skill) | **Đã quyết 2026-10-08 — tách profile, xem §5.1** |
| URI skill chứa digest **của từng skill** (`distribution.go:355-360`) → URI cũ trả `snapshot_expired` sau khi skill đó đổi | **Đã quyết 2026-10-08 — (a):** giữ URI khóa phiên bản; lỗi `snapshot_expired` kèm `current_uri` và thông báo dễ hiểu ("skill đã cập nhật, gọi `skills/get <current_uri>`"); thêm một dòng vào `CLAUDE.md`. Không làm alias `current` vì: đọc nửa cũ nửa mới âm thầm, phá cache client theo URI, mất digest cho observer. Phase 1 đếm `snapshot_expired`; nếu nhiều và agent xử lý sai thì xét lại alias + `resources/updated`. Lesson `stable-skill-uri-for-recovery` |
| `directoryRead: false` (`server.go:75`) | **Đã quyết 2026-10-08 — hoãn và đo:** host đã cho agent thấy đủ file qua `resources` trong `skills/get` (`distribution.go:40`) và `local.path` của `skill_get`; chỉ client thuần MCP, bỏ qua `resources`, không có shell mới thiếu. Phase 1 (O3) thêm bộ đếm: đọc path không có trong `resources`, gọi method skills không hỗ trợ; > 0 thì làm. Lesson `directory-read-for-deferred-files` đã sửa (fact a sai, impact 0, rejected-deferred) |
| `system-curator` vừa native vừa MCP cùng tên (`hostintegration/integration.go:33-35`) | **Đã quyết 2026-10-08 — (c):** mỗi client chỉ một nguồn; client hỗ trợ MCP skills extension dùng MCP, còn lại dùng native, theo `docs/mcp-compatibility-matrix.json`. Lesson `native-and-mcp-same-name` |
| `skill_list` model-callable, không `limit`/`cursor` (`mcpserver/types.go:229-231`) | **Đã quyết 2026-10-08:** phân trang (mặc định ~50, dùng gói `paging` như `skills/list`) + sửa mô tả: dùng cho curate, chọn skill cho task thì gọi `skill_resolve`. Không có trong profile runtime (§5.1). Lesson `model-callable-list-needs-guard` |

Đã sửa: curator frontmatter phục vụ nguyên văn (commit `3746e3e`).

Phase 1 nên đo luôn `payload bytes/tokens` của tools/list theo client (doc 04 §9.3).

### 5.1 Quyết định: tách profile MCP (duyệt 2026-10-08)

Lý do: agent trong luồng làm việc cần gợi ý skill phải cực tiết kiệm token; curate là luồng
riêng, người dùng chủ động mở. Context bị chiếm làm agent kém đi, không chỉ tốn tiền.

Số đo 2026-10-08 (`skillhub mcp serve`, `tools/list`):

| Bộ tool | Số tool | Byte | ~Token |
|---|---|---|---|
| Đủ bộ hiện tại | 43 | 223,674 | ~56k |
| Runtime (`skill_resolve`, `skill_get`, `skill_feedback`) | 3 | 18,642 | ~4.7k |
| Runtime, bỏ `outputSchema` | 3 | ~7,400 | ~1.8k |

(Lần đo trước trong phiên: 243,573 byte; outputSchema chiếm ~80%; lớn nhất `source_triage` 15,624 byte.)

Quyết định:

1. **Profile `runtime`:** `skill_resolve`, `skill_get`, `skill_feedback` + `skills/list`,
   `resources/read` của extension skills. Không có `skill_list` (agent không tự liệt kê
   rồi bỏ qua resolver — giải một nửa câu 15). `skill_feedback` bắt buộc ở đây vì Phase 1–2 cần nó.
2. **Profile `curation`:** mọi tool còn lại, gồm `outcome_record` (ghi outcome incorporation).
3. **`skillhub integrate`** ghi hai server entry: `skillhub` (runtime, luôn bật) và
   `skillhub-curation` (tắt sẵn ở client cho phép bật/tắt, ví dụ Claude Code `/mcp`).
4. **Curator skill**: bước đầu kiểm tra có tool curation không; không có thì hướng dẫn bật
   `skillhub-curation` hoặc dùng lệnh CLI tương ứng. Không nhét tool curate lại vào runtime.
5. **Tương thích ngược:** `skillhub mcp serve` không cờ vẫn phơi đủ bộ như cũ; profile chỉ áp
   dụng cho cấu hình mới do `integrate` ghi (hoặc chạy lại `integrate`), kèm ghi chú migrate.
6. **Sau đó:** thu gọn `outputSchema` và rút mô tả các tool nặng (`source_triage`…) cho cả hai profile.

Cần xác minh khi làm: client nào (Codex, Gemini, Cursor) hỗ trợ bật/tắt từng server; cập nhật
`docs/mcp-compatibility-matrix.json`.

---

## 7. Quyết định cần duyệt

| # | Câu hỏi | Đề xuất | Trạng thái |
|---|---|---|---|
| D1 | Thứ tự: Phase 1 trước mọi tối ưu? | Có | duyệt 2026-10-08 |
| D2 | O2 đổi nghĩa `candidate_count` (bump event_version) hay thêm field mới giữ field cũ? | Đổi nghĩa + bump, vì số cũ sai so với doc | duyệt 2026-10-08 |
| D3 | O4 persist top-k chỉ khi bất đồng? | Có | duyệt 2026-10-08 |
| D4 | Phase 2 lưu câu task cục bộ cho case bất đồng (90 ngày, ~500 case, redact, không export)? | Có, mặc định bật chỉ cho case bất đồng; không có nó Phase 3 không chạy | duyệt 2026-10-08 |
| D5 | Ngưỡng tạo đề xuất (chỉ để tạo đề xuất; vẫn qua replay-verify + confirm; chỉnh lại theo số liệu Phase 1) | 2 case từ 2 session khác nhau; reformulation hoặc feedback basis user: 1 case | duyệt 2026-10-08 |
| D6 | Khái quát hóa câu task | Agent soạn trong curation_run, người confirm | duyệt 2026-10-08 |
| D7 | Usage là source trong lifecycle insight | Có, không dựng hệ riêng | duyệt 2026-10-08 |
| D8 | `recommendation.ignored` là event mới hay `skill.abandoned` + attribution | Quyết khi làm O3 | duyệt 2026-10-08 |

## 8. Thứ tự thực hiện gợi ý

1. O1 (nối chuỗi) → O2 (sửa nghĩa) → O5 (định nghĩa metric) → O6 (funnel + chains)
2. O3 (event còn thiếu) → O4 (trace top-k) → O8 (hướng dẫn agent)
3. O7 baseline 2 tuần — trong lúc chờ: Phase 5 (quyết định spec)
4. Phase 2 → Phase 3 → Phase 4 theo số liệu
