# Plan: Observer trước, làm giàu thông tin sau

**Ngày:** 2026-10-08
**Trạng thái:** D1–D8 đã duyệt 2026-10-08, sửa sau red-team (D2, D4, D5, D7, D8, §5.1 — xem §7 và Validation Log); Phase 5: câu 11–15 đã duyệt
**Phạm vi:** host mcp-skill-hub (agent ↔ hub, telemetry, resolver, curation). Không bao gồm skill `distill`.
**Nguồn:** deep-dive meta-skill (`docs/distillery/deep-dives/skill-recommendation.md`), thiết kế vòng enrichment trong chat 2026-10-08
**Liên quan:** [doc 04 telemetry/evaluation](../design/04-telemetry-reproducibility-evaluation.md), [doc 06 source learning](../design/06-source-learning-and-distillation.md), [doc 03 resolver](../design/03-resolver-design.md), PRD §30, §45
**Phụ thuộc plan khác:** [simplify-hub-model](../../plans/261008-1433-simplify-hub-model/plan.md). Observer Phase 3 **blockedBy** simplify Phase 3 (model lesson + preview/confirm pin); O7 baseline **blockedBy** simplify Phase 4 (đổi mọi `catalog_snapshot`). Lịch song song ở §9.

## 0. Nguyên tắc

1. **Đo trước, tối ưu sau.** Chưa đổi ranking, chưa thêm channel, chưa tự sinh metadata
   khi chưa có số liệu chuẩn hóa để so trước/sau. Mọi thay đổi resolver hoặc metadata
   về sau phải chỉ ra metric nào nó cải thiện.
2. **Server tự quan sát, không trông vào agent.** Agent hay quên gọi `skill_feedback`;
   tín hiệu chính phải là thứ hub thấy được (resolve, load, re-resolve, transcript).
3. **Ưu tiên của dự án:** (1) chất lượng gợi ý, (2) làm giàu thông tin để gợi ý đúng,
   (3) UX CLI/web giúp curate nhẹ nhàng.
4. **Usage không được âm thầm sửa nội dung Git** (PRD §45). Mọi thứ học từ usage đi qua
   preview/confirm, và confirm do **người** làm qua CLI/web (O5 red-team).
5. Tối giản: dùng lại event model, rollup, funnel; bài học từ usage theo model lesson gọn của
   plan simplify (`.meta/distill.yaml`), không dựng lifecycle riêng.
   <!-- Updated: Red Team 2026-10-08 - cross-plan: không dùng insight/proposal/incorporation lifecycle -->
6. **Không bao giờ xóa loại event cũ hoặc đổi `EventVersion`** — mọi đường đọc
   (`validateStored`) từ chối version khác "1" (`internal/telemetry/events.go:206`).
   <!-- Updated: Red Team 2026-10-08 - O1, B2 -->

---

## Phase 1 — Chuẩn hóa observer (làm trước)

Mục tiêu: một vòng resolve → load → kết quả được ghi **đầy đủ, nối được với nhau, và
đọc được** bằng một lệnh; có baseline metric trước khi tối ưu bất cứ thứ gì.

### 1.1 Hiện trạng đã xác minh trong code (2026-10-08)

Đã có:

- Event envelope + allowlist payload **phẳng** (token, tokens, count, millis, bool, stageMillis;
  `events.go:103-111`), content_mode `none`, retention 14 ngày raw + trim 100 MB, rollup 180 ngày
  keyed `(day, skill_id, metric)` (`internal/telemetry/rollup.go:17-24`).
- Attribution phía server cho mỗi lần load: `recommended | supporting | override |
  after_no_skill | after_needs_context | unsolicited`
  (`internal/delivery/mcpserver/activation_tracker.go:131-165`), giữ resolution theo
  session trong RAM (256 session × 32 resolution, `resolutionTTL` 2h, prune lười chỉ khi có lượt gọi tiếp).
- Session hash = 16 byte ngẫu nhiên mỗi `sessionState` (`activation_tracker.go:46-55`), **không phải HMAC**,
  đổi khi process restart; transcript dùng `sha256("claude-code:"+id)` (`internal/app/transcript_import.go:62`).
- `skill.loaded` server-observed có `SessionIDHash` (`mcpserver/server.go:303`).
- Funnel: `skillhub telemetry funnel [--since --until --skill]`, tính **chỉ từ rollup** (`internal/app/usage.go:401-444`).
- Feedback (`skill_feedback`) → `task.outcome_reported`, `skill.utility_reported`,
  `skill.used/abandoned`, `activation.rejected`.
- Transcript scan Claude Code → `transcript.tool_observed`.
- Promotion `resolution_id` → eval case template.
- Hub thật (2026-10-08): 3 resolution, 0 load — lưu lượng rất thấp.

Lỗ hổng (đây là việc của Phase 1):

| # | Lỗ hổng | Bằng chứng | Hệ quả |
|---|---|---|---|
| G1 | Resolution event **không có `SessionIDHash`**; `ResolverService.Resolve` không nhận session | `internal/app/resolver.go:35,270-282`; `mcpserver/resolver_tools.go:22-31` | Không nối được resolve với load/re-resolve ngoài RAM |
| G2 | `Client` luôn là `"skillhub"` (curation: `"skillhub-mcp"`) | `resolver.go:273`, `transcript_import.go:140`, `curation_telemetry.go:118` | Không so được chất lượng theo agent |
| G3 | `candidate_count` thực ra là số skill **được gợi ý** | `resolver.go:227,233` vs doc 04 §3.3 | Không biết resolver lọc từ bao nhiêu ứng viên |
| G4 | `channels`, `stage_ms` đã có trong allowlist nhưng **không được phát** | `events.go:117-118`; `resolver.go:223-241` | Không biết channel nào tạo kết quả; không đo latency stage |
| G5 | Không ghi liên kết re-resolve; `prior.kind: rejected` **không được kiểm** | `addRequestTelemetry`; `internal/resolver/resolver.go:105` chỉ replay `clarification` | Mất tín hiệu reformulation; nếu ghi thì giả mạo được |
| G6 | `clarification.requested/answered` khai báo nhưng không phát | chỉ có trong `events.go` | `needs_context` không đo được |
| G7 | `index.rebuilt`, `catalog.changed`, `evaluation.run_completed`, `task.completed` khai báo nhưng không phát | như trên | Không gắn metric với thay đổi catalog/eval |
| G8 | "Recommended nhưng không load" không bao giờ được quan sát (prune lười, không hook lúc tắt) | `activation_tracker.go:35-43,107,139` | Ignore rate vô hình |
| G9 | Không lưu top-k ứng viên và lý do khớp | chỉ `top_skill_id` | Khi sai, không biết skill đúng đứng hạng mấy |
| G10 | Hub quên nội dung task (`recorder.go:146`) | | Có số đếm "sai" nhưng không có gì để học (Phase 2) |
| G11 | `resolution_id` là **fingerprint nội dung request** — cùng task ở hai session ra cùng ID | `internal/resolver/resolver.go:336` (cacheKey bỏ `RequestID`) | Chuỗi bị gộp nhầm nếu key chỉ bằng resolution_id |
| G12 | Load event gắn snapshot **lúc load**, bị bỏ nếu catalog không mở được | `internal/app/curation_telemetry.go:17-40` | Metric theo snapshot vỡ đúng tại `catalog.changed` |
| G13 | Rollup không có chiều client/snapshot; raw chỉ 14 ngày | `rollup.go:17-24`, `recorder.go:18-20` | Không cắt được theo client/snapshot quá 14 ngày |

<!-- Updated: Red Team 2026-10-08 - thêm G11-G13, sửa G1/G4/G5/G8 theo bằng chứng -->

### 1.2 Việc cần làm

**O1. Nối chuỗi sự kiện (G1, G2, G5, G11).** Cần sửa API tầng app, không chỉ một dòng.

- Thêm `CallerContext{SessionHash, Client}` truyền qua `ctx` vào `ResolverService` và
  `FeedbackService`; CLI ghi rõ là không có session.
- `Client` chuẩn hóa thành enum `claude-code | codex | cursor | gemini | other` từ MCP
  `clientInfo` (tên lạ → `other`, không bao giờ làm event bị từ chối); version chỉ major.minor.
  Nhãn client là tham khảo, không dùng cho quyết định trust.
- Thêm payload `prior_resolution_id`, `prior_kind`, `prior_verified` (bool): `prior` chỉ
  `verified` khi `resolution_id` đó đã được cấp **cho chính session này** (tracker đã giữ).
- **Key của chuỗi:** `(session_hash, resolution_id, event_id)`, không dùng `resolution_id` một mình.
- Kết quả: chuỗi `res1 → (rejected, verified) → res2 → load B` dựng lại được từ telemetry.

**O2. Sửa nghĩa số đo (G3, G4).**

- **Giữ `event_version` "1".** Thêm field mới `retrieval_candidate_count` (allowlist + schema);
  `candidate_count` giữ nguyên, ghi nghĩa thật vào doc 04 §3.3.
- Phát `channels` (`fts`, `rules`, …) và `stage_ms` (đã có trong allowlist).
  <!-- Updated: Red Team 2026-10-08 - O1: đảo D2 -->

**O3. Phát các event đã thiết kế (G6, G7) và đo phần còn thiếu (G8, G12).**

- `clarification.requested` khi trả `needs_context`; `clarification.answered` khi request
  mới mang `prior.kind: clarification`.
- `catalog.changed` / `index.rebuilt` khi catalog generation đổi → mốc để so trước/sau.
- `evaluation.run_completed` cho routing eval gate.
- Bộ đếm cho câu 13 (Phase 5): đọc path không có trong `resources` của `skills/get`, gọi method skills hub không hỗ trợ.
- **Không thêm event "ignored".** `ignored` và `true_no_skill` tính **offline** từ event bền:
  resolution không có load cùng session trong TTL. Không đo được → báo `unknown`, không ghi 0.
- Load event gắn **snapshot của resolution** (tracker giữ snapshot + policy), và vẫn ghi
  `skill.loaded` khi catalog không mở được.
  <!-- Updated: Red Team 2026-10-08 - O10 (D8), O12 -->

**O4. Trace ứng viên có giới hạn (G9).**

- Top-k (k ≤ 5) mã hóa bằng **mảng token song song** (allowlist đã có `kindTokens`):
  `topk_skill_ids`, `topk_matched` (`<rank>:<field>`), `topk_channels`. Không dùng mảng object.
- Mặc định chỉ persist khi kết quả có **bất đồng** (override / after_no_skill / rejected);
  còn lại chỉ giữ RAM.
- Giữ quy tắc "field sai → event bị từ chối" (đó là cửa chặn privacy); thay vào đó test rằng
  producer luôn phát field hợp lệ, và test funnel không mất mẫu số sau khi thêm field.
- Doc 04 §4.1 `persist_candidate_scores: false` → chỉ persist rank + matched fields.
  <!-- Updated: Red Team 2026-10-08 - O11 -->

**O5. Chuẩn hóa định nghĩa metric (một nguồn sự thật).**

Viết bảng định nghĩa trong doc 04 §9.1. **Đơn vị chung: một chuỗi** (theo key O1). Load chỉ
attribute cho resolution mới nhất trong chuỗi; `already_covered` có bucket riêng. Test mọi
rate nằm trong [0, 1].

| Metric | Công thức | Ý nghĩa |
|---|---|---|
| acceptance_rate | chuỗi có load đúng primary / chuỗi `resolved` | gợi ý được dùng |
| override_rate | chuỗi `resolved` có load skill khác / chuỗi `resolved` | gợi ý sai skill |
| false_no_skill_rate | chuỗi `no_skill` có load / chuỗi `no_skill` | nói "không có" nhưng thật ra có |
| true_no_skill | chuỗi `no_skill` không load trong TTL (offline) | catalog thật sự thiếu |
| reformulation_rate | chuỗi có `prior_kind: rejected` **verified** / tổng chuỗi | agent phải viết lại câu |
| ignore_rate | chuỗi `resolved` không load trong TTL (offline) / chuỗi `resolved` | gợi ý bị bỏ qua lặng lẽ |
| needs_context_answer_rate | `clarification.answered / clarification.requested` | câu hỏi có đáng hỏi không |
| bypass_rate | `native:no_resolve / transcript skill uses` | agent không hỏi hub (chỉ trong transcript) |
| negative_after_load | đã có | load rồi mới thấy sai |

Cắt theo `skill_id`, `client`, `operation`, `catalog_snapshot`, `policy_revision`:
**chỉ trong cửa sổ raw event**. Không thiết kế lại khóa rollup; thay vào đó tăng retention raw
(`defaultRetention`, `recorder.go:18`) đủ phủ cửa sổ baseline, và lưu snapshot funnel JSON hằng tuần.
Funnel ghi rõ ngày đầu tiên mỗi metric có dữ liệu hợp lệ.
<!-- Updated: Red Team 2026-10-08 - O2 (bản nhỏ của Opus), O14 -->

**O6. Một màn hình đọc được.**

- `skillhub telemetry funnel` thêm các metric O5 và `--by client|operation|snapshot`
  (trong cửa sổ raw).
- `skillhub telemetry chains --since 7d [--kind override|after_no_skill|reformulation]`:
  liệt kê chuỗi bất đồng (ID, skill, rank skill đúng, matched fields).
- Khi n nhỏ: hiện **số đếm thô** cạnh mọi rate.
- Web UI: thêm tab chất lượng gợi ý dùng cùng JSON, **không chứa câu task**.

**O7. Baseline.**

- Lấy **sau khi simplify Phase 4 merge** (migration đổi mọi `catalog_snapshot`); nếu cần sớm
  hơn thì phát `catalog.changed` lúc migration và bắt đầu baseline mới.
- Điều kiện đủ: **theo số event** (ví dụ ≥ N chuỗi `resolved` và ≥ N chuỗi `no_skill` mỗi client
  chính; N chốt khi làm), không theo ngày. Cửa sổ baseline phải nằm trong retention raw.
- Chụp baseline (JSON funnel theo snapshot) vào `docs/brainstorm/` hoặc một report.
  <!-- Updated: Red Team 2026-10-08 - O3, B3 -->

**O8. Hướng dẫn agent (một dòng, không thêm field).**

- Trong `CLAUDE.md`/curator: khi không dùng skill được gợi ý, resolve lại với
  `prior.kind: rejected` thay vì tự chọn (PRD §30). Khi dùng skill khác, gọi
  `skill_feedback` với `skill_id` thật.
- Làm **sau khi simplify Phase 1 merge** (chung file curator `SKILL.md` và template hostintegration).

### 1.3 Tiêu chí xong Phase 1

- Từ telemetry (không cần RAM) dựng lại được chuỗi resolve → re-resolve → load cho một session,
  với key `(session_hash, resolution_id, event_id)`.
- Funnel hiện đủ metric O5 (rate trong [0,1], kèm số đếm thô), cắt được theo client và snapshot
  trong cửa sổ raw.
- Event schema + doc 04 §3, §9 khớp code; `event_version` vẫn "1"; test allowlist cho field mới.
- Có baseline đủ số event (O7).

---

## Phase 2 — Case journal (bắt đầu học được "vì sao")

Phụ thuộc: O1, O4. **Opt-in** (D4): bật bằng một config flag, đúng doc 04 §4.1.

- Giữ `task.description`, `operation`, **request đã chuẩn hóa** (gồm fact values qua allowlist
  redaction) trong RAM của tracker cùng resolution.
- Khi có bất đồng (override, after_no_skill, reformulation **verified**, needs_context→resolved,
  rejected/scope_mismatch, gap lặp lại), ghi một **case** xuống store runtime riêng
  (không phải Git). Resolution đồng thuận: không ghi nội dung.
- **Redactor là deliverable riêng** (hiện chỉ có nhãn `redact-v1`, `events.go:15`): secret
  patterns, path normalization, length cap; test trước khi bật persistence.
- Retention 90 ngày, trần ~500 case **và trần theo ngày**; case text **chỉ CLI, loopback**;
  không có trong web JSON, không có trong `telemetry export` (test khẳng định).
- `skillhub telemetry cases` để đọc; `telemetry purge` xóa cả cases.
  <!-- Updated: Red Team 2026-10-08 - O9 (D4 opt-in), O13 (request đầy đủ) -->

Ví dụ case:

```yaml
case: cs_7f2a
kind: reformulation        # reformulation | override | false_no_skill | needs_context | rejected | gap
at: 2026-10-08T09:12Z
session: 9f3c…             # hash ngẫu nhiên theo process, không phải HMAC
client: claude-code
catalog_snapshot: sha256:a180…
prior_verified: true
task: "triage why the nightly e2e job flakes only on arm runners"   # đã redact
operation: debug
resolver: {status: no_skill, topk_skill_ids: [ci-debug], topk_matched: ["4:operation"]}
chosen: ci-debug
followup: {reformulated_to: "debug flaky CI test on arm"}
```

Bảng tín hiệu → bài học:

| Chuỗi | Bài học | Độ tin |
|---|---|---|
| resolve(d1) → rejected (**verified**) → resolve(d2) → load B | d1 nên là example của B | Cao |
| resolved A → load B | B thiếu trigger/example; A cần counter_example / distinguish_from B | Cao |
| no_skill → load B | B thiếu từ cho loại task này | Cao |
| needs_context → trả lời F → resolved B | F nên thành requirement/rule của B | Trung bình |
| recommended A → scope_mismatch / user_rejected | A cần not_for / counter_example | Trung bình (basis user: cao) |
| recommended A → không load (offline) | bị bỏ qua lặng lẽ | Thấp, chỉ đếm |
| no_skill → không load, lặp lại | catalog thiếu skill → intake/create, không phải metadata | Theo số lần |
| transcript: skill native X không resolve | hub bị bỏ qua; X chưa có → đề xuất import | Trung bình |
| recommended B → completed, helpful (user) | xác nhận; làm eval case chống regression | Cao |

Example rút từ case khớp phân phối query thật hơn example viết tay, vì đó chính là câu agent gửi.

---

## Phase 3 — Tổng hợp + kiểm chứng (vòng enrichment)

Phụ thuộc: Phase 2 có đủ case; **simplify Phase 3 đã merge** (model lesson + preview/confirm pin);
baseline O7 đã có.

### 3.1 Bài học từ usage theo model lesson gọn

<!-- Updated: Red Team 2026-10-08 - cross-plan decision (thay D7) -->

- Mỗi bài học là một lesson trong `.meta/distill.yaml` của skill đích (plan simplify D7), với
  `where: usage:<case_id>` khi còn ứng viên; nội dung case nằm ở runtime store.
- **Khi lesson được quyết** (confirm), `where` chuyển sang **eval case đã promote vào Git**
  (`evals/routing/…`), vì case runtime bị gitignore và hết hạn sau 90 ngày (B1).
- Một case có thể sinh lesson cho hai skill (trigger cho B, counter_example cho A).
- Dùng lại **preview/confirm pin** của hub (pin digest + base version, theo kết luận simplify
  Phase 3), không dùng insight/proposal/incorporation.

### 3.2 Tổng hợp

Hub (tất định), với mỗi skill B có **≥ 2 case trên ≥ 2 ngày khác nhau** (D5):

- trigger ứng viên: token (cùng tokenizer FTS) xuất hiện ≥ 2 case, chưa có trong metadata
  B, hiếm trên catalog, không thuộc `not_for` của B;
- example ứng viên: ≤ 3 câu task, khử gần trùng;
- skill thua A (nếu là primary): `counter_example`, `distinguish_from: B`.

Ngưỡng là giảm nhiễu, **không** chống giả mạo. Không có ngoại lệ 1 case cho `prior` chưa verified.

Agent soạn bản khái quát hóa câu task (bỏ tên project, path, khách hàng) trong phiên curate (D6);
người confirm qua CLI/web.

### 3.3 Replay-verify trước khi tới người

Cơ chế (O13): **API build-only** dựng catalog tạm có patch (kể cả FTS) ngoài
`runtime/catalog/generations`, không ghi pointer, snapshot ID riêng; replay qua `resolveWithin`
với `Telemetry: nil` và cache riêng (mẫu `internal/app/routing_eval.go:176`); test pointer live
và cache dùng chung không đổi sau replay. Case phải tự replay ra đúng kết quả gốc mới được tính.

1. cụm case của nó phải lật sang B (không lật → bỏ đề xuất);
2. mọi case khác: đếm lật đúng / lật sai;
3. routing eval gate (42 skill, 168 example, 126 counter-example, 34 no-skill) không tụt;
4. self-resolution lint: example của B vẫn về B, của A vẫn về A.

Preview: `ci-debug: +2 trigger, +1 example — sửa 4/5 case, 0 regression; test-runner: +1 counter_example`.
**Một confirm ghi cả patch metadata lẫn case (đã khái quát) vào golden corpus.**

### 3.4 Sau khi apply

- Outcome là **bản ghi người xác nhận** trên decision của lesson (định dạng theo simplify Phase 3)
  kèm metric override/miss của skill trước/sau theo snapshot. Không tự động phát
  `incorporation.outcome_recorded` / `insight.reopened`; xấu đi → báo để người mở lại lesson.
- Case cũ được replay trên catalog mới; đã tự đúng → đóng, ghi "fixed by <commit>".

### 3.5 Rủi ro và chặn

- Agent sai cũng tạo override → ngưỡng ngày khác nhau + replay + người confirm.
- Skill phổ biến phình rộng → điều kiện "hiếm trên catalog", counter-example và no-skill trong gate.
- **Prompt injection** (O5): câu task là dữ liệu không tin cậy sẽ vào metadata phục vụ mọi agent →
  patch từ usage **chỉ confirm qua CLI/web** (như content approval: tool MCP `skill_review` chỉ đọc,
  `skill_review_tools.go:19`), giới hạn độ dài và bộ ký tự cho trigger/example, không auto-apply,
  preview hiện nguyên văn.
- Overfit vào chính case → giữ ~1/3 case đã promote làm held-out, không dùng sinh đề xuất.
- Lưu lượng thấp (hub thật: 3 resolution, 0 load) → Phase 3 có thể hiếm khi đủ ngưỡng; chấp nhận.

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
| URI skill chứa digest **của từng skill** (`internal/app/distribution.go:355-360`) → URI cũ trả `snapshot_expired` sau khi skill đó đổi | **Đã quyết 2026-10-08 — (a):** giữ URI khóa phiên bản; lỗi `snapshot_expired` kèm `current_uri` và thông báo dễ hiểu ("skill đã cập nhật, gọi `skills/get <current_uri>`"); thêm một dòng vào `CLAUDE.md`. Không làm alias `current` vì: đọc nửa cũ nửa mới âm thầm, phá cache client theo URI, mất digest cho observer. Phase 1 đếm `snapshot_expired`; nếu nhiều và agent xử lý sai thì xét lại alias + `resources/updated`. Lesson `stable-skill-uri-for-recovery` |
| `directoryRead: false` (`server.go:75`) | **Đã quyết 2026-10-08 — hoãn và đo:** host đã cho agent thấy đủ file qua `resources` trong `skills/get` (`distribution.go:40`) và `local.path` của `skill_get`; chỉ client thuần MCP, bỏ qua `resources`, không có shell mới thiếu. Phase 1 (O3) thêm bộ đếm: đọc path không có trong `resources`, gọi method skills không hỗ trợ; > 0 thì làm. Lesson `directory-read-for-deferred-files` đã sửa (fact a sai, impact 0, rejected-deferred) |
| `system-curator` vừa native vừa MCP cùng tên (`hostintegration/integration.go:33-35`) | **Đã port 2026-10-10 — (c), sửa round 2:** mỗi client chỉ một nguồn, theo `skills_extension` trong `docs/mcp-compatibility-matrix.json`. Commits sau rebase: `51cb26b` matrix + sync test; `352c063` native cutover; `9041f52` lọc `skills/list`/`skills/get` theo client chuẩn hóa; `7c64527` test doctor version skew; `06f8aab` bỏ receipt files, chỉ xóa native khi bytes trùng bundled curator, bản khác giữ nguyên và báo người dùng tự xem, xóa rồi chạy lại `connect` (không gợi ý `doctor --fix`, vì nó đi qua cùng cutover và cũng gặp conflict); `7660bf5` in lệnh fix trong doctor text như JSON. Chưa stock client nào verified; connect giữ đúng file set của main, không tạo sidecar. Unknown, Cursor và Claude Desktop vẫn nhận MCP. CLI là `connect` (alias `integrate`); cleanup native chạy qua hostintegration `Plan`/`Apply`. Lesson `native-and-mcp-same-name` |
| `skill_list` model-callable, không `limit`/`cursor` (`mcpserver/types.go:229-231`) | **Đã quyết 2026-10-08:** phân trang (mặc định ~50, dùng gói `paging` như `skills/list`) + sửa mô tả: dùng cho curate, chọn skill cho task thì gọi `skill_resolve`. Không có trong profile runtime (§5.1). Lesson `model-callable-list-needs-guard` |

Đã sửa: curator frontmatter phục vụ nguyên văn (commit `3746e3e`).

Phase 1 nên đo luôn `payload bytes/tokens` của tools/list theo client (doc 04 §9.3).

### 5.1 Quyết định: tách profile MCP (duyệt 2026-10-08, sửa sau red-team)

Lý do: agent trong luồng làm việc cần gợi ý skill phải cực tiết kiệm token; curate là luồng
riêng, người dùng chủ động mở. Context bị chiếm làm agent kém đi, không chỉ tốn tiền.

Số đo 2026-10-08 (`skillhub mcp serve`, `tools/list`):

| Bộ tool | Số tool | Byte | ~Token |
|---|---|---|---|
| Đủ bộ hiện tại | 43 | 223,674 | ~56k |
| Runtime (`skill_resolve`, `skill_get`, `skill_feedback`) | 3 | 18,642 | ~4.7k |
| Runtime, bỏ `outputSchema` | 3 | ~7,400 | ~1.8k |

(Lần đo trước trong phiên: 243,573 byte; outputSchema chiếm ~80%; lớn nhất `source_triage` 15,624 byte.)
**Đo lại sau simplify Phase 3** (bỏ ~20 tool) trước khi tách — có thể bớt cần thiết (B5).

Quyết định:

1. **Profile `runtime`:** `skill_resolve`, `skill_get`, `skill_feedback` + `skills/list`,
   `resources/read` của extension skills. Không có `skill_list`. `skill_feedback` bắt buộc ở đây vì Phase 1–2 cần nó.
2. **Profile `curation`:** mọi tool còn lại (danh sách theo contract table của simplify Phase 3).
3. **Chỉ tách ở host đã kiểm chứng** có bật/tắt từng server (ghi vào `docs/mcp-compatibility-matrix.json`);
   host khác giữ đủ bộ. `skillhub integrate` **inspect, plan và remove cả hai entry**
   (`skillhub`, `skillhub-curation`), không chỉ `mcpServers.skillhub`
   (`hostintegration/integration.go:280,288`, `preview.go:71`).
4. **Curator skill**: bước đầu kiểm tra có tool curation không; không có thì hướng dẫn bật
   `skillhub-curation` hoặc dùng lệnh CLI tương ứng. Test server-boundary: `compatible-tools`
   của curator ⊆ tool của profile curation.
5. **Tương thích ngược:** `skillhub mcp serve` không cờ vẫn phơi đủ bộ như cũ; profile chỉ áp
   dụng cho cấu hình mới do `integrate` ghi (hoặc chạy lại `integrate`), kèm ghi chú migrate.
6. **Sau đó:** thu gọn `outputSchema` và rút mô tả các tool nặng (`source_triage`…) cho cả hai profile.
   <!-- Updated: Red Team 2026-10-08 - O15, B5 -->

---

## 7. Quyết định

| # | Câu hỏi | Quyết định | Trạng thái |
|---|---|---|---|
| D1 | Thứ tự: Phase 1 trước mọi tối ưu? | Có | duyệt 2026-10-08 |
| D2 | `candidate_count` | **Giữ `event_version` "1"; thêm `retrieval_candidate_count`** (bump làm mọi event đã lưu không đọc được) | sửa sau red-team 2026-10-08 |
| D3 | O4 persist top-k chỉ khi bất đồng? | Có (mã hóa mảng token song song) | duyệt 2026-10-08 |
| D4 | Phase 2 lưu câu task cục bộ cho case bất đồng | **Opt-in bằng một config flag** (đúng doc 04 §4.1); 90 ngày, ~500 case + trần theo ngày, redactor riêng, chỉ CLI | sửa sau red-team 2026-10-08 |
| D5 | Ngưỡng tạo đề xuất | **2 case trên 2 ngày khác nhau**; không ngoại lệ 1 case khi `prior` chưa verified; chỉnh theo số liệu | sửa sau red-team 2026-10-08 |
| D6 | Khái quát hóa câu task | Agent soạn trong phiên curate, người confirm qua CLI/web | duyệt 2026-10-08 (chỉnh: không gắn `curation_run`) |
| D7 | Bài học từ usage đi đâu | **Lesson trong `.meta/distill.yaml`** (`where: usage:<case_id>` → eval case Git khi quyết), preview/confirm pin của hub; không dùng insight lifecycle | sửa sau red-team 2026-10-08 (quyết định liên plan) |
| D8 | "Ignored" | **Không thêm event**; tính offline từ event bền | sửa sau red-team 2026-10-08 |

## 8. Thứ tự thực hiện trong plan

1. O1 (nối chuỗi) → O2 (field mới) → O5 (định nghĩa metric) → O6 (funnel + chains)
2. O3 (event còn thiếu, đo offline) → O4 (trace top-k) → O8 (sau simplify Phase 1)
3. O7 baseline sau simplify Phase 4, đủ số event
4. Phase 2 → Phase 3 (sau simplify Phase 3) → Phase 4 theo số liệu
5. Phase 5 / §5.1 sau simplify Phase 3 (đo lại tools/list trước)

### Trạng thái (2026-10-10, main 7c8b279)

| Việc | Trạng thái |
|---|---|
| Phase 1 (O1–O6, O3, O4, O8) | xong (worktree A, D, E, F) |
| O7 baseline | lệnh có; hub thật migrate lên v4 ngày 2026-10-10, bắt đầu đếm từ đó |
| Phase 2 case journal | xong (D, F) |
| Phase 3 | **đã quyết hướng (2026-10-10): hướng 1** — case đã xác nhận được đưa thành eval case trong Git (`evals/routing/<case_id>.json`) và lesson trích dẫn `repo@commit:path`; không sửa distill-lab. Chờ có cases (0 case ngày 2026-10-10) |
| Phase 4 | chờ baseline |
| §5.1 tách profile | xong (G, I `7cd3fb0`). Claude Code verified 2026-10-10: `connect` ghi `skillhub` (runtime) + `skillhub-curation` (curation) và nhắc tắt curation trong `/mcp`; Codex/Gemini vẫn một entry. Hướng tiếp: [curator qua CLI](2026-10-10-curator-via-cli.md) |
| 5 (a) `current_uri`, phân trang `skill_list` | xong (G) |
| 5 `directoryRead` | **hoãn hẳn** (người dùng quyết 2026-10-10). Bộ đếm O3 đã bỏ vì không đo được (C round 3). Giữ `directoryRead: false`; chỉ mở lại khi có client thuần MCP thật sự thiếu file |
| 5 (c) `system-curator` native/MCP trùng tên | xong (H round 2): `51cb26b`, `352c063`, `9041f52`, `7c64527`, `06f8aab`, `7660bf5`; tests `TestConnectPreviewThenApplyWritesProjectFilesOnly`, `TestNativeCuratorMatrixCutover`, `TestCuratorSourcePerSessionClient`, `TestDoctorReportsNativeCuratorVersionSkew`, `TestDoctorTextReportsNativeCuratorFixCommand`; temp HOME/project smoke connect đúng 10 files, không có `*.skillhub-sha256`; doctor text in cùng lệnh fix như JSON và không sửa file; `make check` xanh sau rebase lên main `4834408` (đã có C Phase 5/schema v5) |

## 9. Thực thi song song với plan simplify (worktree riêng)

Mỗi phần việc do **một agent trong một git worktree riêng** (`isolation: worktree`).

| Đợt | Worktree | Việc | Ghi chú |
|---|---|---|---|
| 1 | A | Observer O1, O2, O5, O6 (+ O3 phần event) | `internal/telemetry/**`, `internal/app/{resolver,usage,curation_telemetry,skill_load_telemetry,telemetry}.go`, `internal/resolver` (chỉ emit), `mcpserver/{activation_tracker,resolver_tools}.go`, CLI/web telemetry, `schemas/telemetry-event-v1.schema.json`, doc 04 |
| 1 | B | Simplify Phase 1 → Phase 2 | merge **B trước**, A rebase lên B |
| 2 | C | Simplify Phase 4 → Phase 3 (tuần tự, một worktree) | |
| 2 | D | Observer O3/O4 phần còn lại → Phase 2 (case journal); O8 sau khi B merge | D **không** sửa `mcpserver/server.go` `registerTools`, `insight_tools.go`, `distribution.go` |
| 3 | tuần tự | Observer §5.1 (đo lại) → O7 baseline → Phase 3 → Phase 4 | sau khi C merge |

Quy tắc cho mỗi agent worktree:

1. Chỉ sửa path phase mình sở hữu. File dùng chung (curator `SKILL.md`, template hostintegration,
   `mcpserver/server.go` `registerTools`, `telemetry/events.go`, doc 04/06/07) có **một chủ mỗi đợt**;
   người khác ghi follow-up thay vì sửa.
2. Không xóa loại event telemetry, không đổi `EventVersion`.
3. Rebase lên `main` ngay trước khi merge; `make check` (fmt-check, vet, lint, test) xanh sau rebase.
4. Merge từng worktree một, theo thứ tự đợt. Thay đổi schema/public contract đi cùng commit với code.
5. Không commit vào hub thật `/home/vantt/skill-hub` từ worktree code.

---

## Red Team Review

### Session — 2026-10-08
**Reviewers:** Security Adversary, Assumption Destroyer (Scope Auditor), Failure Mode Analyst (Flow Tracer); adjudication thứ hai bởi agent Opus.
**Findings:** 15 (15 accepted, 0 rejected; 6 accepted với cách sửa nhỏ hơn theo Opus)
**Severity breakdown:** 5 Critical, 10 High

| # | Finding | Severity | Disposition | Applied To |
|---|---|---|---|---|
| O1 | Bump `event_version` làm mọi event đã lưu không đọc được | Critical | Accept | O2, §0.6, D2 |
| O2 | Rollup không có chiều client/snapshot; raw 14 ngày | Critical | Accept (modified: tăng retention raw, không đổi khóa rollup) | O5, O6, O7 |
| O3 | Lưu lượng thấp; baseline theo ngày vô nghĩa | Critical | Accept | O6, O7, §3.5 |
| O4 | `prior.kind: rejected` không được kiểm | Critical | Accept (modified: kiểm theo session, bỏ suy luận phía server) | O1, Phase 2, D5 |
| O5 | Câu task không tin cậy vào routing metadata; confirm do model gọi | Critical | Accept (modified: confirm CLI/web như content approval) | §0.4, §3.5 |
| O6 | `resolution_id` là fingerprint nội dung | High | Accept | G11, O1 |
| O7 | Session hash ngẫu nhiên theo process, không phải HMAC | High | Accept | §1.1, Phase 2 ví dụ, D5 |
| O8 | Session/client không có ở tầng app; `clientInfo` lạ làm mất event | High | Accept | O1 |
| O9 | Case journal mặc định bật trái doc 04; chưa có redactor | High | Accept (modified: opt-in một flag, trần theo ngày, chỉ CLI) | Phase 2, D4 |
| O10 | "Ignored"/"true no_skill" không bao giờ quan sát được | High | Accept | O3, O5, D8 |
| O11 | Top-k dạng mảng object vi phạm allowlist phẳng | High | Accept (modified: giữ quy tắc từ chối, test producer) | O4 |
| O12 | Load gắn snapshot lúc load, bị bỏ khi catalog không mở | High | Accept | O3, G12 |
| O13 | Replay không có API catalog tạm; case thiếu fact values | High | Accept | §3.3, Phase 2 |
| O14 | Định nghĩa metric lệch đơn vị | High | Accept | O5 |
| O15 | Tách profile: `integrate` chỉ biết một entry; toggle chưa kiểm chứng | High | Accept (modified: bỏ phần tranh lock) | §5.1 |
| X | Xung đột liên plan: simplify Phase 3 xóa lifecycle mà D7 dựa vào | Critical | Accept — người dùng chọn "observer theo model gọn" | §0.5, Phase 3, D7, §9 |
| B1–B5 | Opus bổ sung: `where: usage` trỏ store hết hạn; không xóa event cũ; baseline sau simplify Phase 4; file dùng chung; đo lại tools/list | — | Accept | §3.1, §0.6, O7, §9, §5.1 |

### Whole-Plan Consistency Sweep
- Files reread: plan này (toàn bộ), simplify plan (phụ thuộc, §9)
- Decision deltas checked: 9 (D2, D4, D5, D6, D7, D8, §5.1, cross-plan, B1–B5)
- Reconciled stale references: `bump event_version`, `recommendation.ignored`, `≥ 2 session`, `hmac:`, `insight lifecycle`/`Incorporation outcome`/`insight.reopened`, `curation_run`, `outcome_record`, `mặc định bật`, `baseline ≥ 2 tuần`
- Unresolved contradictions: 0 (định dạng outcome và preview pin chờ simplify Phase 3 — đã ghi là phụ thuộc, không phải mâu thuẫn)

## Validation Log

### Session 1 — 2026-10-08
**Trigger:** `/ak:plan validate` + `red-team` sau khi tạo plan; verification bằng 3 reviewer red-team (Flow Tracer, Scope Auditor, Fact Checker).
**Questions asked:** 4

#### Verification Results
- Tier: Full (5 phase)
- Claims checked: ~45 | Verified: ~40 | Failed: 3 | Unverified: 2
- Failures:
  1. Ví dụ case `session: hmac:…` — session hash là 16 byte ngẫu nhiên (`activation_tracker.go:46-51`). Đã sửa.
  2. "Redaction pipeline doc 04 §4.3 áp dụng" ngụ ý đã có — chỉ có nhãn `redact-v1` (`events.go:15`). Đã sửa (redactor là deliverable).
  3. "Cắt metric theo client/snapshot theo khoảng ngày" — rollup không có các chiều đó (`rollup.go:17-24`). Đã sửa (chỉ trong cửa sổ raw).
- Unverified: byte count tools/list (cần chạy server; đo trong phiên 2026-10-08), kích thước eval gate.

#### Questions & Answers
1. **[Tradeoffs]** Áp dụng kết quả red-team thế nào?
   - Options: Áp dụng tất cả | Xem từng finding | Chỉ áp dụng Critical
   - **Answer:** Other — "đưa qua một opus agent để tư vấn thêm 1 lần, nếu ok thì accept hết"
   - **Rationale:** cần ý kiến thứ hai trước khi đổi quyết định đã duyệt.
2. **[Architecture]** Giải xung đột liên plan (simplify Phase 3 xóa lifecycle mà observer D7 dựng lên)?
   - Options: Observer theo model gọn | Giữ lifecycle insight | Hoãn quyết định
   - **Answer:** Observer theo model gọn
   - **Rationale:** hợp hướng Git-native tối giản; observer Phase 3 phụ thuộc simplify Phase 3.
3. **[Tradeoffs]** Opus đề xuất "Accept with changes". Áp dụng bản nào?
   - Options: Bản Opus | Bản reviewer gốc | Bản Opus, trừ D4
   - **Answer:** Bản Opus
   - **Rationale:** cách sửa nhỏ hơn; D4 thành opt-in.
4. **[Scope]** simplify D3 giữ hay đổi? (thuộc plan simplify)
   - Options: Giữ D3 | Thêm danh sách cấp hub
   - **Answer:** Giữ D3

#### Confirmed Decisions
- D2: giữ v1, thêm field — bump phá mọi đường đọc.
- D4: opt-in — đúng doc 04, redactor chưa có.
- D5: 2 ngày khác nhau, không ngoại lệ khi prior chưa verified — session hash không chống giả mạo.
- D7: lesson trong `.meta/distill.yaml` — theo model gọn.
- D8: không thêm event — đo offline.
- §5.1: chỉ tách ở host đã kiểm chứng; `integrate` quản cả hai entry.
- Thực thi: song song theo §9, mỗi agent một worktree.

#### Action Items
- [ ] Khi bắt đầu mỗi phase: viết phase file chi tiết từ plan này.
- [ ] Chốt N (số event tối thiểu) cho O7 khi làm.

#### Impact on Phases
- Phase 1: O1 cần `CallerContext`; O2 không bump; O3 đo offline; O4 mảng token; O5 đơn vị chuỗi.
- Phase 2: opt-in, redactor riêng, request đầy đủ.
- Phase 3: model lesson gọn, phụ thuộc simplify Phase 3, confirm CLI/web, replay build-only.
- Phase 5: tách profile có điều kiện, đo lại.
