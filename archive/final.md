# Quyết định kiến trúc cuối cùng — Curated Skill Hub

**Trạng thái:** Lưu trữ (historical). Bản chốt hướng kiến trúc ngày 2026-09-29, trước khi triển khai V1. Nguồn có thẩm quyền hiện tại là [`docs/design/01..07`](../docs/design/01-system-architecture.md); khi khác nhau, tài liệu thiết kế thắng.

## 1. Quyết định tổng thể

Chọn **Git-first Curated Knowledge Hub, triển khai dạng modular monolith local-first, với evidence-first routing do server quản lý và activation do host kiểm soát**.

> **Git sở hữu tri thức và policy bền vững. SQLite cung cấp tốc độ. Hub tìm và recommend skill. MCP phân phối nội dung. Host quyết định activation theo quyền hạn. Agent cung cấp context và thực thi.**

Giữ kiến trúc sản phẩm của PRD, nhưng **thay rich agent-generated SRC bằng task description + constraints + evidence**. Không chọn nguyên xi một báo cáo: dùng nền evidence-first chung, giữ centralized routing, và tiếp nhận ranh giới activation trong Appendix A của báo cáo brainstorm.

Đây là quyết định thiết kế dựa trên ba tài liệu, **không phải kết luận đã được benchmark hoặc xác minh interoperability**.

## 2. Kiến trúc hệ thống

```text
Upstream repositories
        ↓ ingestion / source checks
Git canonical repository
  skills + provenance + insights + history + routing policy
        ↓ validate / rebuild
SQLite + FTS5 + optional derived vector index
        ↓
Shared application services
  ├── CLI
  └── MCP server
        ├── Custom resolver: skill_resolve
        ├── Skills distribution: manifest + resources
        ├── Bundled System Curator Skill
        └── Structured curation tools
                        ↓
                  Agent Host
             activation + permissions
                        ↓
                       Agent
```

**V1:** Go cho CLI, MCP server, scheduler, indexing và resolver; SQLite/FTS5 với driver pure-Go; Git CLI/GitHub API. Bundled System Curator Skill là primary interactive curation surface; CLI là operational/recovery fallback. MCP ưu tiên local stdio. Người dùng cài một binary qua `install.sh`; build/runtime không cần Node/Python. Web UI chưa thuộc V1. Không cần microservices, PostgreSQL hay cloud dependency bắt buộc.

Executable và data tách biệt: `skillhub` được cài vào PATH; `skillhub init <path>` tạo một data workspace là Git repository riêng, không chứa binary/source của hệ thống. Một binary có thể quản lý nhiều workspace. Clone chỉ áp dụng khi đồng bộ/phục hồi data workspace, không phải cách cài ứng dụng.

Tách module ingestion, registry, curation, indexing, routing và transport trong cùng codebase. CLI và MCP/System Skill dùng chung application services; không có hai implementation ghi state khác nhau.

## 3. Git-first và curation: giữ nguyên nguyên tắc PRD

- YAML/JSON/Markdown và skill files là canonical state. SQLite, FTS, embeddings và cache là derived state.
- Mọi thay đổi đáng giữ phải ghi filesystem trước rồi cập nhật index; không dual-write DB và Git như hai nguồn ngang hàng.
- Validate trước khi ghi; serialize mutations, kiểm tra revision để tránh ghi đè concurrent edits. Thay đổi nhiều file cần staging/recovery rõ ràng, không giả định nhiều lần rename là một transaction.
- Nếu refresh index lỗi, báo index stale và rebuild từ canonical files; không mất mutation đã lưu.
- Stable IDs, schema version và migration có kiểm soát là bắt buộc. Git commit vẫn có thể do người dùng thực hiện qua CLI.
- Không dùng global `eventlog.jsonl` làm source of truth. Rebuild đọc current canonical state trực tiếp; immutable operation/run records chỉ phục vụ audit/provenance, không được replay để dựng state.
- Managed multi-file mutation dùng gitignored write-ahead transaction journal, before/after digests và roll-forward recovery. Mỗi semantic mutation có operation receipt trong Git.
- Derived catalog dùng immutable SQLite generation files và atomic `current.json` pointer. Operational và telemetry SQLite là disposable databases riêng.
- Source checks chỉ cập nhật provenance/status. Không overwrite curated content.
- System Curator Skill bắt đầu bằng local Curation Home, ưu tiên failed work/changed sources/high-value insights và đề xuất một next action. Findings/comparisons là evidence layer mở theo nhu cầu, không phải mandatory review flow.
- Distillation tạo findings/insights/proposals và auto-finalize run hợp lệ; active skill chỉ đổi qua pinned preview + explicit approval. Apply đồng thời cập nhật ledger và history.
- Batch intent xử lý mọi source không ambiguous mà không hỏi từng source; errors được isolate và có resume/retry/cancel.
- Routine unchanged source checks chỉ cập nhật runtime operational state, không làm Git dirty. Revision mới, cursor advance và human decisions mới là canonical.
- Giữ riêng upstream revision hiện tại, revision đã distill và revision curated. Snapshot upstream là tùy chọn; không hứa audit offline nếu chỉ lưu URL/SHA mà nguồn có thể biến mất.

## 4. Contract Agent → Hub: evidence-first

### Chốt input

Chỉ **mô tả tác vụ hiện tại** là bắt buộc. Các trường khác bổ sung khi có giá trị phân biệt:

```json
{
  "schema_version": "1",
  "task": {
    "description": "Review retry and idempotency in the order consumer",
    "constraints": ["Do not redesign service boundaries"]
  },
  "operation": "review",
  "context": {
    "active_artifact": {
      "kind": "source-file",
      "path_hint": "consumer/retry.go",
      "language": "go"
    },
    "facts": [
      {
        "key": "dependency",
        "value": "kafka",
        "basis": "tool",
        "scope": "active-component"
      }
    ]
  }
}
```

Đây là **shape định hướng**, chưa phải JSON Schema hoàn chỉnh.

### Quy tắc

- Không bắt agent sinh `domain`, `topics` hoặc taxonomy của catalog.
- `operation` là enum nhỏ, optional và chỉ là soft hint. Không dùng làm hard filter.
- Task ngắn, đủ nghĩa có thể dùng nguyên văn sau redaction; không bắt buộc viết lại bằng lời agent.
- Constraints tách riêng; không đưa phủ định vào positive retrieval query. Free-text constraint chỉ thành hard exclusion khi có rule/diễn giải đủ chắc chắn; nếu không, penalty hoặc clarification.
- Facts phải phân biệt observed, user-stated và inferred; có scope/freshness khi cần. Unknown không đồng nghĩa false.
- Context tác vụ và constraint rõ ràng quan trọng hơn inventory công nghệ của toàn repo.
- Không gửi full conversation, chain-of-thought, source files, secrets, absolute paths hoặc full tool output mặc định.
- Giới hạn payload bằng schema; vượt giới hạn phải báo rõ, không âm thầm cắt mất constraints.

### Workspace facts

Adapter hoặc server local **có thể** derive facts khi được cấp quyền. MCP roots giúp xác định workspace, **không tự cấp quyền đọc filesystem và không bảo đảm mọi client hỗ trợ**.

Vì vậy, server-derived facts là optimization tùy chọn, không phải điều kiện hoạt động. Không có adapter/roots vẫn resolve được từ task description. Quét có giới hạn, ưu tiên active component; không đọc toàn workspace mặc định.

## 5. Resolver: centralized, deterministic-first

```text
Validate + policy checks
 → lexical retrieval + metadata matching
 → optional semantic retrieval
 → applicability / constraints / capability checks
 → deterministic scoring
 → resolve, clarify hoặc abstain
```

- Server sở hữu taxonomy, retrieval, ranking, supporting-skill selection và abstention trong catalog của Hub.
- Hard filters dành cho authorization, disabled skills và incompatibility/precondition đã xác nhận. Thiếu required evidence có thể dẫn tới clarification hoặc không activation; không được suy diễn là tương thích.
- Metadata routing được curate trong Git: activation examples/triggers, exclusions/not_for, capabilities, scope và quan hệ liên quan.
- `distinguish_from` hữu ích để tạo câu hỏi phân biệt, nhưng không bắt buộc cho mọi cặp skill.
- Hierarchy chỉ là soft optimization có global fallback, không chặn retrieval bằng một nhãn suy luận ban đầu.
- Không tự học weights ngầm trong SQLite. Thresholds, calibration và policy ảnh hưởng hành vi phải version trong Git.

**Embeddings:** V1 bắt đầu với FTS5/BM25 + metadata + rules. Thiết kế retrieval interface cho phép thêm embeddings; chỉ bật mặc định khi evaluation cho thấy lợi ích đáng kể. Không coi ước lượng latency/model quality trong báo cáo là số đo thực tế.

## 6. Kết quả và xử lý mơ hồ

Chốt custom resolver có bốn kết quả nghiệp vụ:

| Status | Ý nghĩa |
|---|---|
| `resolved` | Một primary skill phù hợp trong phạm vi được giao |
| `needs_context` | Thiếu một thông tin có thể thay đổi quyết định |
| `no_skill` | Không có skill đủ phù hợp/hữu ích, hoặc chưa đủ chắc chắn |
| `already_covered` | Host khai báo procedure đang active đã đủ bao phủ tác vụ |

`already_covered` phải dựa trên scope/evidence được khai báo, không tuyên bố đã kiểm tra nội dung local mà Hub chưa thấy. Local state không được cung cấp nghĩa là **unknown**, không phải không tồn tại.

Lỗi infrastructure, timeout backend hoặc index hỏng là **error**, không giả thành `no_skill`.

### Không chọn `choose` trong V1

Không đưa bounded shortlist cho agent làm đường xử lý mơ hồ mặc định. Dù chỉ 2–3 skill, lựa chọn này vẫn phân tán quyết định routing và tăng khác biệt giữa model.

Thay vào đó:

1. Nếu thiếu evidence: hỏi một discriminator cụ thể bằng `needs_context`.
2. Agent trả lời từ context sẵn có; chỉ hỏi user khi thật sự cần. Cho phép `unknown`.
3. Server chọn lại hoặc abstain.
4. Nếu hai skill tương đương, dùng canonical preference ổn định thay vì hỏi vô ích.

Giữ `choose` là **phương án benchmark đối chứng**, không là contract production V1. Explicit user selection vẫn được host tôn trọng trong giới hạn policy, không phải bị cấm bởi centralized routing.

### Veto và vòng lặp

Agent có thể từ chối vì scope/capability/safety mismatch; gửi corrected snapshot kèm resolution ID và context revision. V1 ưu tiên snapshot nhỏ thay vì một ngôn ngữ patch phức tạp dễ giữ context cũ.

Giới hạn mặc định: một vòng clarification và tối đa hai lần re-resolution cho một scope ổn định; hết budget thì abstain. Reuse resolution khi scope còn hiệu lực; không gọi lại mỗi turn. Scope, constraint, operation hoặc capability thay đổi đáng kể mới cần resolve lại.

Confidence trả dạng band, không giả raw retrieval score là probability. Response cần applicability ngắn, reason codes, catalog snapshot và policy revision để debug/tái lập.

## 7. LLM fallback

**Tắt mặc định trong MVP.** Có extension point để bổ sung khi eval chứng minh hiệu quả.

Chỉ dùng khi candidates có vẻ phù hợp, evidence đã đủ nhưng ranker rẻ hơn chưa phân biệt được, đồng thời privacy/latency policy cho phép. Input chỉ gồm request đã minimize và một số curated cards; output structured, có abstention, timeout và token budget.

Không dùng LLM để đoán facts bị thiếu, chữa catalog không có coverage hoặc tự hợp nhất skills. Timeout không được chuyển thành shortlist tùy ý; trả kết quả deterministic đủ điều kiện, abstain vì ambiguity, hoặc error phù hợp.

## 8. MCP distribution và progressive disclosure

Giữ official Skills extension làm lớp distribution theo PRD; recommendation nằm ở custom tool riêng. Triển khai/kiểm thử theo phiên bản specification và SDK thực tế được pin, không giả định mọi client hỗ trợ đồng đều.

Normal path:

```text
skill_resolve
 → host approves activation
 → skills/get
 → resources/read(SKILL.md)
 → references/scripts/assets khi cần
```

`skills/list` không phải recommendation API. `resources/directory/read` chỉ khi applicable/supported.

**Không tự embed SKILL.md trong resolve response ở V1.** Giữ selection tách loading, tránh nạp nội dung trước activation approval và giảm phụ thuộc cách client xử lý embedded resources. Có thể tối ưu round-trip sau khi kiểm chứng tương thích.

Pin selection và loading vào cùng content version/manifest; digest từng file. Catalog snapshot phải phản ánh cả nội dung Git-visible chưa commit, không dùng riêng HEAD SHA nếu working tree dirty. Nếu snapshot không còn phục vụ được, yêu cầu resolve lại thay vì lặng lẽ trả bản khác.

## 9. Local skill và Hub skill: host sở hữu activation

**Không chọn precedence tuyệt đối `project-local > hub > user-global`.** Vị trí lưu trữ không chứng minh authority, chất lượng hay độ mới phù hợp.

- Host áp dụng instruction hierarchy, permissions, approved project constraints và explicit user choice.
- Hub chỉ recommend trong delegated scope; không tự thay procedure đã active hoặc host-locked.
- Mỗi operation có tối đa một primary procedure. Loaded không đồng nghĩa selected-as-active.
- Gửi optional `activation_context` nhỏ: active procedure ID, scope, role và replacement policy do host xác nhận. Không gửi toàn local inventory.
- Nếu active procedure đủ dùng: `already_covered`. Nếu coverage không rõ: clarification hoặc không đề xuất thêm procedure chồng lấn.
- V1 không tự replace local primary, không tự merge hai workflow. Adapter-less clients chỉ có best-effort coordination qua instructions; phải công khai giới hạn này.
- Duplicate detection dựa trên identity, provenance và digest; khác digest có thể là fork, không mặc định là bản cũ cần xóa.

`doctor --scan-local` và trao đổi local candidates là cải tiến sau MVP, opt-in, không tự xóa/migrate local skills.

## 10. Composition, security và observability

### Composition

Một primary mặc định; tối đa hai supporting theo quan hệ curated, có scope riêng và applicability rõ ràng. Conditional supporting chỉ load on demand. Quan hệ `complements` đơn thuần không đủ để tự kích hoạt. Không sinh composed package; host duyệt mọi activation.

### Security

Upstream và skill content là dữ liệu không đáng tin mặc định. Chặn path escape/symlink escape, giới hạn protocol/file size/traversal; không execute scripts trong ingestion. Digest kiểm tra integrity, không chứng minh nội dung an toàn. Scripts chỉ chạy theo host policy.

### Telemetry và reproducibility

Resolution/feedback/session events là runtime, có minimization và retention; không lưu raw task text mặc định nếu không cần. Promote sanitized cases sang Git phải qua review.

Phân biệt recommended, loaded, activated, used, abandoned và useful outcome. Acceptance không thay thế usefulness. Rebuild phải khôi phục canonical state và cấu hình hành vi; không yêu cầu tái tạo telemetry/cache đã xóa.

## 11. Kế hoạch triển khai và điều kiện kiểm chứng

### MVP

1. Freeze UX acceptance transcripts cho Curation Home, batch maintenance, insight review/apply và recovery.
2. Từ UX đó freeze CLI/MCP tool contracts và application-command boundaries; không thiết kế command theo table/entity CRUD.
3. Implement Git schema, validation/migrations, registry và SQLite/FTS rebuild phía sau các application commands.
4. Implement source intake/check, provenance và runtime-vs-canonical operational state; unchanged checks không làm Git dirty.
5. Implement batch distill → findings/coverage → insight ledger → pinned preview/approved apply.
6. Implement evidence-first resolver, abstention, bounded clarification và MCP distribution với version/digest consistency.
7. Chạy golden eval, curation UX fixtures và client compatibility tests trước khi freeze schema.

### Evaluation bắt buộc

Khởi đầu khoảng 150 cases/30–50 skills, gồm no-skill, negation, misleading repo facts, unknown capabilities, local/Hub conflicts và corrections. Đây là quy mô prototype đề xuất, không phải bằng chứng đủ để generalize.

So sánh rich SRC, evidence-first, raw-task interpretation và bounded agent selection trên cùng backend; thử request generation qua nhiều model. Sau đó đo riêng tác động embeddings và LLM fallback trên held-out cases.

Theo dõi routing accuracy, false-positive/false-negative abstention, cross-model agreement, invocation rate, duplicate activation, clarification/veto loops, p95 latency, tokens và **task quality so với không dùng skill**. Benchmark phải cho phép nhiều skill tương đương hợp lệ.

### Chưa đóng băng

Enum, payload budget, score thresholds, embedding model và target latency sẽ chốt từ evaluation. Khả năng roots/Skills extension/activation hooks phải kiểm thử trên từng client. Những điểm này không thay đổi ranh giới trách nhiệm đã chọn.

## 12. Tóm tắt phân xử giữa ba tài liệu

| Điểm tranh luận | Quyết định cuối |
|---|---|
| Rich SRC của PRD | Thay bằng evidence-first, không shared ontology bắt buộc |
| Server recommendation authority | Giữ trong Hub catalog; host giữ activation authority |
| Bounded `choose` của resolution report | Không vào V1; dùng discriminator để hỏi context, giữ làm benchmark |
| Server tự đọc repo qua roots | Optional, authorized, scoped; không là prerequisite |
| Embeddings là core ngay V1 | Không bắt buộc; bật theo kết quả eval |
| Embedded SKILL.md khi confidence cao | Không mặc định; tách resolve/approve/load |
| Local-first precedence | Bác bỏ; dùng host policy, provenance và activation state |
| Git-first + curated ≠ upstream | Giữ; làm rõ filesystem-first mutation và rebuild |
| LLM reranking | Optional, tắt mặc định, chỉ khi đủ evidence và có lợi ích đo được |

**Kết luận:** Chọn **evidence-first centralized recommendation + host-governed activation**, trên nền **Git-first modular monolith**. Đây là phương án giữ được giá trị curation của PRD, giảm coupling giữa model và catalog, không trao quyền activation vượt khả năng quan sát của Hub, đồng thời cho phép nâng cấp retrieval mà không buộc mọi agent đổi cách tích hợp.

## Tài liệu thiết kế

- [Kiến trúc hệ thống tổng thể](../docs/design/01-system-architecture.md)
- [Giao tiếp Agent ↔ Skill Hub](../docs/design/02-agent-hub-protocol.md)
- [Thiết kế Resolver](../docs/design/03-resolver-design.md)
- [Telemetry, reproducibility và evaluation](../docs/design/04-telemetry-reproducibility-evaluation.md)
- [Curation lifecycle](../docs/design/05-curation-lifecycle.md)
- [Source learning và distillation](../docs/design/06-source-learning-and-distillation.md)
- [Git-first storage, mutation và database model](../docs/design/07-storage-and-mutation-model.md)

## Kế hoạch triển khai

- [Skill Hub V1 — phased implementation plan](plans/skillhub-v1-implementation-plan.md)

## Tài liệu nguồn

- `docs/PRD.md`
- `archive/plans/260928-1435-skillhub-v1/reports/architecture-review-260928-1312-agent-skill-server-resolution-report.md`
- `docs/brainstorm/2026-09-28-g6a--01a0e68b-routing-report.md`
