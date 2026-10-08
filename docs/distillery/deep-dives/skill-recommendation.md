---
topic: skill-recommendation
date: 2026-10-08
based_on: [meta-skill@e999668e8d8c]
entries: [project-signal-detector, bandit-adaptive-ranking, graded-implicit-rewards, semantic-admission-gate, rrf-hybrid-fusion, prune-proposals-human-approved, test-the-claim-not-the-doc, feedback-learns-without-context, user-skill-preferences, session-mining-drafts-only, eval-before-ranking-changes, findability-lint, lint-explain-and-fix]
---

# Deep-dive: skill suggestion và recommendation trong meta_skill

**Bottom Line:** meta_skill *mô tả* một hệ gợi ý giàu tính năng: hybrid search, bandit theo ngữ cảnh, phát hiện project, cá nhân hoá. Nhưng code ở HEAD cho thấy phần lớn chưa được nối. Tool `suggest` qua MCP trả về skill sửa gần nhất. `ms suggest` là bandit có trọng số bằng 0 lúc khởi đầu, nên thứ tự là ngẫu nhiên. Bandit không được lưu sau khi chạy, và feedback học với ngữ cảnh rỗng. Không có tập eval nào. Ba lỗi chất lượng gần nhất đều do người dùng phát hiện (#144, #192, #216). Với ưu tiên số 1 (chất lượng gợi ý), host **không nên chép cách xếp hạng** của meta_skill: resolver của host (doc 03) đã vượt xa, có trigger/example/not_for, `no_skill`, tính tất định và eval gate. Có 5 điều đáng lấy:
1. Tự suy ra ngữ cảnh project ở phía server, có giới hạn trần (cap).
2. Cửa lọc trước khi gộp kênh vector.
3. Tuỳ chọn ẩn/ghim skill theo người dùng.
4. Đào phiên làm việc của agent để làm giàu **routing metadata của skill đã có**. Đây là việc meta_skill chưa làm được.
5. Lint chỉ ra skill khó được tìm thấy, có `--explain` và `--fix`.

Ưu tiên số 2 (làm giàu thông tin) là chỗ host có khoảng trống lớn nhất: metadata routing hiện hoàn toàn do người viết tay.

## Câu hỏi

Theo thứ tự quan trọng mà người dùng đặt ra:
1. **Chất lượng gợi ý:** agent gửi gì lên server, server suy ra gì, xếp hạng qua những tầng nào, đo chất lượng ra sao.
2. **Làm giàu thông tin:** dữ liệu nào giúp một skill được tìm thấy, ai viết nó, cái gì được tự động trích ra hoặc học từ việc dùng.
3. **UX curate:** CLI/web giúp vòng đời curate nhẹ nhàng tới đâu.

## Cách meta_skill giải quyết

### 1. Chất lượng gợi ý: ba đường xếp hạng gần như không dùng chung gì

**Agent gửi lên rất ít.** Qua MCP, agent chỉ gửi được:
- một `query` cho `search`;
- một `cwd` cho `suggest`;
- `skill_id`, `helpful`, `comment` cho `feedback`. Trường `context` được khai báo nhưng không ai đọc (`mcp.rs#L1341-1356`).

Không có trường nào cho mô tả task, file đang mở hay session id (`meta-skill@e999668e8d8c:src/cli/commands/mcp.rs`). Biến môi trường `MS_OPEN_FILES` chỉ dùng cho fingerprint của cooldown.

**Server tự suy ra khá nhiều:**
- file sửa trong 24 giờ qua (tối đa 20);
- 29 binary trên PATH;
- trạng thái git (branch, thay đổi chưa commit, 5 commit gần nhất);
- 6 biến môi trường;
- loại project theo file đánh dấu, với độ tin cố định: `Cargo.toml` 1.0, `package.json` 0.9 (`src/context/collector.rs`, `src/context/detector.rs#L297-L312`).

Nhưng phần lớn dữ liệu này chỉ đi vào fingerprint và vector đặc trưng của bandit.

**Ba đường xếp hạng:**

| Đường | Cách xếp | Thực tế ở HEAD |
|---|---|---|
| MCP `suggest` | `ctx.db.list_skills(limit, 0)` | `ORDER BY modified_at DESC`. `explain` và `threshold` bị bỏ qua (`mcp.rs#L1288-1339`). Commit thêm tool này gọi nó là "context-aware". |
| `ms suggest` | Thompson bandit theo ngữ cảnh, mỗi skill là một arm, 28 đặc trưng (one-hot loại project, giờ/ngày, hoạt động, lịch sử) | Không có query, không có đặc trưng nào mô tả skill. Trọng số khởi đầu bằng 0, nên khi mới chạy thứ tự là ngẫu nhiên. Không lưu bandit sau khi chạy. Flag `--load/--top/--budget/--no-bandit` không được dùng. Tiếp theo: favorites +0.25, hidden bị loại, cooldown 5 phút theo fingerprint, lấy top 5. |
| `ms load --auto` | Khớp với thẻ `context:` của skill theo trọng số 0.40/0.25/0.20/0.10/0.05, trộn với bandit theo hệ số 0.3, ngưỡng 0.3 | `signals` luôn ra 0 (không ai cung cấp nội dung), `historical` là TODO. Load **mọi** skill trên ngưỡng, không có trần. Skill không có `context:` thì không bao giờ được chọn. |
| `ms search` | Tantivy BM25 (name, description, body, tags, aliases; không boost theo field), gộp RRF k=60 với hash embedder | Hash embedder chỉ xếp lại kết quả BM25 (từ `50b6a3ca457e`), không tìm được từ đồng nghĩa. Có vẻ alias không được đưa vào index. |

**Đo chất lượng:** không có. Không có eval set, golden query, NDCG hay MRR. Test của `suggest` chỉ kiểm hình dạng output. Ba lần sửa chất lượng gần nhất đều là lỗi chạy không báo lỗi gì, do người dùng phát hiện:
- `73020b1efa2b`: nửa "lexical" thực ra là `LIKE`.
- `69c1da3ed221`: embedding không bao giờ được ghi, nên semantic search "silently inert".
- `50b6a3ca457e`: RRF kéo kết quả vector không liên quan vào.

**Vì sao họ chọn như vậy:** họ đặt cược vào học từ dữ liệu dùng (bandit, implicit reward) thay vì metadata do người viết. Cái giá là không có thước đo nên không biết nó có hoạt động không. Đây đúng là thứ PRD §56 của host đã cảnh báo.

### 2. Làm giàu thông tin: metadata do người viết, phiên làm việc chỉ sinh bản nháp

- **Metadata để khớp đều do người viết:** `tags, requires, provides, platforms` và `context: {project_types, file_patterns, tools, signals}` (`src/core/skill.rs#L171-L206`, `#L468-L548`). Không có trigger, example hay not_for.
- **Trích tự động:** `ms import` đoán tag bằng so khớp chuỗi con, ví dụ "js" ra javascript, "try" ra error-handling (`src/import/formatting.rs#L508-L549`).
- **Đào phiên (CASS):** chạy quét chống injection (ACIP), rồi chia phiên thành pha và trích pattern lệnh, code, workflow, lỗi (`src/cass/mining.rs#L671-L706`). Đầu ra là **skill nháp không có frontmatter** (`src/cass/brenner.rs#L299-L339`). Nó không bao giờ làm giàu tag hay context cho skill đã có.
- **Điểm chất lượng:** 6 thành phần dạng bậc thang. Ranking không dùng nó, chỉ dùng để lọc và trong prune.
- **Lint vì khả năng được tìm thấy:** `meaningful-description` và `embedding-quality` cảnh báo khi mô tả quá ngắn hoặc thiếu tag. Không rule nào kiểm tra có thẻ `context:` không, dù `load --auto` phụ thuộc vào nó.
- **Lưu trữ:** trạng thái xếp hạng nằm trong file JSON ngoài database (`contextual_bandit.json`, `cooldowns.json`, `user_history.json`). Không tái tạo được từ Git.

### 3. UX curate: CLI phong phú, không có hàng đợi review

Những thứ làm vòng đời nhẹ hơn:
- `ms index` chạy tăng dần theo content hash và tự bù embedding thiếu.
- `ms edit --meta` chỉ sửa metadata.
- `ms fmt --check` dùng làm cổng CI.
- `ms lint --fix --explain RULE --list-rules`.
- `ms quality` hiện điểm theo từng thành phần.
- `ms prune analyze/proposals/review/apply`: đề xuất trước, không làm gì phá huỷ.
- `ms dedup`, `ms alias`, `ms favorite/hide`, `ms doctor --fix`.
- TUI `ms browse`, `ms build --guided` để soạn skill từ phiên.
- 6 định dạng output; có định dạng "toon" mà họ cho là ít token hơn JSON 40–60%.

Không có hàng đợi review skill: `inbox` là hộp thư giữa các agent.

## So sánh & trade-offs

| Chiều | meta_skill | host (mcp-skill-hub) | Bên tốt hơn và vì sao |
|---|---|---|---|
| Agent gửi gì | query, hoặc chỉ cwd | `task.description` (bắt buộc), constraints, operation, active_artifact, facts có provenance, execution signal, activation_context, prior (doc 02 §4) | **host**: mô tả task hiện tại là tín hiệu mạnh nhất, meta_skill không có |
| Server tự suy ra | marker file, file gần đây, tool, git | FactProvider đã thiết kế, chưa xây (doc 03 §12); có trần cho ambient fact 0.15 (§7.2) | **meta_skill có code**, host có thiết kế tốt hơn (trần, provenance, quyền) |
| Tạo ứng viên | BM25 + hash vector (search); toàn bộ catalog (suggest) | FTS5 BM25 nối OR + kênh luật (identity, trigger, requirement, operation) | **host** |
| Xếp hạng | bandit/RRF, không diễn giải được, ngẫu nhiên khi mới chạy | các feature diễn giải được, trọng số trong config có policy revision, tất định | **host**: tái lập được, giải thích được |
| Từ chối/không biết | không có `no_skill`; `load --auto` load không trần | `resolved / needs_context / no_skill / already_covered`, ngưỡng và margin | **host** |
| Học từ việc dùng | implicit reward + bandit (feedback với ngữ cảnh rỗng) | feedback gắn `resolution_id`; không tự sửa metadata; promote thành eval case cần người duyệt | **host** về nguyên tắc; meta_skill cho thấy học "tự động" mà thiếu ngữ cảnh thì không học được gì |
| Đo chất lượng | không có | eval gate: 42 skill fixture, 168 example, 126 counter-example, 34 case no-skill, golden set, CI nightly | **host** |
| Tuỳ chọn của người dùng | favorite +0.25, hide | không có | **meta_skill** (ý tưởng; nó chỉ áp dụng cho `suggest`) |
| Làm giàu metadata | người viết + đoán tag + skill nháp từ phiên | người (và agent curate) viết trigger/example/not_for; lint collision/generic | **host** về chất lượng metadata; **cả hai** đều chưa làm giàu tự động cho skill đã có |
| UX curate | nhiều lệnh nhỏ, `--explain`, `--fix`, `quality` breakdown | preview/confirm, review theo digest, web UI, lint có `fix` text | **ngang nhau**; meta_skill có `lint --explain`/`--fix` và quality breakdown đáng lấy |

## Giải pháp tổng hợp cho host

Giữ nguyên lõi resolver của host. Thêm theo đúng thứ tự ưu tiên:

**Ưu tiên 1: chất lượng gợi ý**
1. **Xây FactProvider tối thiểu theo doc 03 §12**, lấy phần chạy thật của meta_skill: marker file kèm độ tin cố định, chỉ trong thư mục gốc của component đang làm, và bị giới hạn bởi trần ambient 0.15 của host. Bỏ phần "file gần đây/tool trên PATH" cho tới khi eval chứng minh lợi ích, vì đây là chỗ meta_skill thu dữ liệu mà không dùng tới. Lesson `project-signal-detector`.
2. **Ghi cửa lọc vào doc 03 §13 trước khi bật vector:** kết quả có độ giống ≤ 0 bị loại; với embedder rẻ thì chỉ xếp lại, không thêm. Lesson `semantic-admission-gate`.
3. **Không lấy bandit hay implicit reward.** Bằng chứng mới củng cố quyết định PRD §56: ở meta_skill, bandit khởi đầu ngẫu nhiên, không lưu và học không có ngữ cảnh. Thay vào đó, đưa telemetry thật (`resolution_id` + outcome) vào **promotion thành eval case** đã có, để mọi thay đổi trọng số đi qua eval gate. Lesson `eval-before-ranking-changes`.
4. **Tuỳ chọn của người dùng như một fact, không phải boost cộng thêm:** `hidden` là hard exclusion theo người dùng, `pinned` là tie-break; cả hai có giải thích bằng `reason_code`. Lesson `user-skill-preferences`.

**Ưu tiên 2: làm giàu thông tin** (khoảng trống lớn nhất)

5. **Đào telemetry và transcript để đề xuất routing example cho skill đã có.** Ví dụ: task `no_skill` mà sau đó người dùng chọn skill X; một clarification được trả lời bằng skill Y; một task `resolved` rồi bị `rejected`. Mỗi trường hợp thành một *đề xuất* thêm `examples`, `counter_examples` hoặc `not_for`, đi qua preview/confirm như mọi mutation khác. meta_skill đào phiên nhưng chỉ ra skill nháp; host đã có import transcript và promotion nên làm được bước mà họ thiếu. Lesson `session-mining-drafts-only`.
6. **Lint vì khả năng được tìm thấy,** kế thừa ý của `embedding-quality` cho model của host: skill active thiếu example, example quá giống trigger, thiếu `not_for` (một phần đã có). Thêm một rule mới: **skill không có example nào khớp được câu hỏi tiêu biểu**, chạy bằng chính resolver. Lesson `findability-lint`.

**Ưu tiên 3: UX curate**

7. Lấy `lint --explain <code>` và `--fix` (khi fix là cơ học), và một `skillhub skill quality <id>` hiện vì sao một skill khó được chọn (thiếu example, collision, not_for). Lesson `lint-explain-and-fix`.
8. `prune proposals` theo dữ liệu dùng (đã ghi ở `prune-proposals-human-approved`) là cách nhẹ nhàng nhất để catalog không phình ra.

**Bỏ:** bandit và implicit reward (lý do ở trên); `load --auto` load không trần; trạng thái xếp hạng nằm ngoài Git (trái với "Git là database" của host); định dạng output "toon" (chưa có bằng chứng đo).

## Portable ideas

Thành lesson trong `lessons/` (đều là `candidate`):
- **Mới:** `feedback-learns-without-context`, `user-skill-preferences`, `session-mining-drafts-only`, `eval-before-ranking-changes`, `findability-lint`, `lint-explain-and-fix`.
- **Cập nhật** với bằng chứng của lượt này: `project-signal-detector` (chỉ phần loại project và tool là chạy thật), `bandit-adaptive-ranking` và `graded-implicit-rewards` (khởi đầu ngẫu nhiên, không lưu, feedback không có ngữ cảnh), `test-the-claim-not-the-doc` (MCP `suggest` được gọi là "context-aware" nhưng trả skill sửa gần nhất).

## Open questions

- Tuỳ chọn ẩn/ghim của người dùng nên lưu ở đâu cho đúng "Git là database": file trong hub repo (chia sẻ) hay runtime cục bộ (cá nhân)?
- Đề xuất routing example từ telemetry cần ngưỡng bao nhiêu sự kiện thì mới đáng đưa cho người duyệt?
- Có nên đo "skill mới có tìm thấy được không" ngay khi thêm skill, bằng cách chạy resolver trên chính example của nó và example của các skill lân cận?
