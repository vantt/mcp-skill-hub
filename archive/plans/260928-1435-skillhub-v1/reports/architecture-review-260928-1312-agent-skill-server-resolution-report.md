# Architecture Review — Agent ↔ Skill Server Resolution

**Scope:** PRD-SKILLHUB-001 §25–33, §58–62 (open discussion). Chỉ bàn giao tiếp Agent → Skill Server.
**Date:** 2026-09-28
**Verdict:** Bác bỏ "rich agent-generated SRC" làm contract chính. Đồng ý hướng *evidence-first* đã được đề xuất, nhưng bổ sung 5 điểm sửa và 1 điểm phản bác (xem §1).

---

## 0. TL;DR

- Vấn đề cốt lõi không phải "thiếu structure" mà là **ai làm phép nối giữa ngôn ngữ của task và vocabulary của catalog**. Nếu agent làm, agent phải biết ontology của server, dẫn tới coupling, mỗi model cho kết quả khác nhau, và nhãn sai trở thành hard filter khiến recall về 0.
- Nhãn do agent suy ra **không có hại khi chỉ là soft signal**. Nó **có hại khi bị dùng làm hard filter**. Lỗi của baseline là ranh giới này chưa được xác định.
- Contract đề xuất gồm: `goal` (free text, lời của agent) + `constraints` (tách riêng) + `operation` (closed enum khoảng 11 giá trị, soft) + `artifact`/`state` (facts). Repo facts do **server tự derive** qua MCP `roots` khi có thể.
- Server sở hữu taxonomy, retrieval, ranking, abstention và clarification. Resolver mặc định **deterministic** (BM25 + embedding trên trigger examples + fact rules + calibrated margin).
- **Phản bác baseline §7 và §29:** khi mơ hồ giữa ≤3 skill, **agent chính là reranker tốt nhất** vì agent có đầy đủ context và đang chạy sẵn. Server trả `choose` kèm *discriminator do curator viết*; agent chọn theo discriminator, và lựa chọn đó được log lại. Server-side LLM rerank bị hạ xuống vai trò fallback hiếm dùng.
- Composition chỉ theo quan hệ đã khai báo trong Git (≤2 supporting), không sinh "composed package".

---

## 1. Phản hồi recommendation "evidence-first" đã có

Đồng ý với luận điểm chính: *premature semantic compression* là điểm yếu lớn nhất. Ví dụ Kafka ("do not redesign the service") là đúng.

Bổ sung và phản bác:

1. **Free-text summary vẫn là model-inferred compression.** Không tồn tại ranh giới sạch kiểu "fact vs semantics". Summary vẫn tốt hơn enum label vì retrieval (BM25/embedding) **chịu được paraphrase**, còn hard filter trên enum thì không chịu được lệch nhãn. Đây là lý do thật để chọn free text, và nó nên được ghi rõ vào thiết kế.
2. **Constraints phải tách thành field riêng.** Nếu gộp "do not redesign the service" vào summary, lexical và vector retrieval sẽ *match* với skill "architecture-redesign". Phủ định làm hỏng retrieval, nên constraints được dùng làm **penalty/exclusion**, không dùng làm query.
3. **Giữ đúng một semantic hint: `operation`** (closed enum, soft). Phân biệt review / implement / debug là sai số routing lớn nhất và cũng là nhãn các model đồng thuận cao nhất khi enum nhỏ và nằm ngay trong JSON Schema của tool. `domain` và `topics` thì bỏ: độ đồng thuận thấp, và server tự suy ra được từ goal + facts.
4. **Server LLM reranker gần như dư thừa.** Nó lặp lại reasoning mà agent đã có, lại với ít context hơn. Nên đẩy quyết định mơ hồ *có giới hạn* về agent qua `choose`.
5. **Repo facts không nên do agent liệt kê.** Server là local stdio process (V1) nên có thể đọc workspace qua MCP `roots` và detect stack deterministically. Cách này nhất quán giữa mọi agent. Chỉ fallback sang agent-supplied facts khi client không hỗ trợ roots hoặc server chạy remote.

---

## 2. Trả lời 20 câu hỏi

**Q1. Thông tin nên đi qua ranh giới.**
Là những thứ *chỉ agent biết* và *có giá trị phân biệt skill*:
- goal hiện tại theo lời agent, ≤300 ký tự;
- constraints và exclusions do user hoặc task đặt ra;
- operation đang làm;
- artifact đang active (kind, path tương đối, language);
- tín hiệu thực thi gần nhất (loại lỗi, lệnh fail, dạng tóm tắt ngắn);
- user corrections ảnh hưởng tới phạm vi;
- workspace root URI (để server tự derive).
Khi re-resolve thì gửi thêm: resolution_id trước và lý do từ chối.

**Q2. Không được đi qua.**
- Raw conversation.
- Nội dung source code và file.
- Secrets, env, token.
- Absolute path chứa tên user (nên dùng path tương đối với root).
- Danh sách skill mà agent "nghĩ là phù hợp", vì làm vậy thì catalog bị rò sang agent.
- Nhãn domain/topic theo ontology của server.
- Chain-of-thought của agent.
- Full tool output (chỉ gửi dạng tóm tắt đã phân loại).

**Q3. Gửi gì.**
Kết hợp: **goal summary + constraints + active artifact facts + recent execution signal + 1 closed-enum hint**. Không gửi raw task (tốn kém, rò riêng tư, lặp lại việc diễn giải). Không gửi rich interpreted SRC (coupling, lệch giữa model).

**Q4. Factual vs inferred.**
| Field | Loại | Server dùng thế nào |
|---|---|---|
| `artifact.paths`, `languages`, `kind` | factual | fact rules, boost |
| `workspace.facts` (Dockerfile, go.mod, openapi…) | factual, **server tự derive** | hard precondition + boost |
| `state.signal` (test_failure, build_error…) | factual (kind) + tóm tắt ngắn | boost cho skill debug |
| `constraints` | lời user, gần factual | penalty / exclusion |
| `goal` | inferred (free text) | query cho BM25 + embedding |
| `operation` | inferred, closed enum | soft boost, **không** hard filter |
| intent / domain / topics | **bỏ** | server tự suy ra |

**Q5. Agent dựng structured context có reliable không?**
Reliable với *factual* fields và closed enum nhỏ nằm trong JSON Schema. **Không** reliable với taxonomy mở (domain, topics): mỗi model chọn độ chi tiết khác nhau (ví dụ `kafka` vs `event-streaming` vs `messaging`). Đây là giả thuyết, cần đo bằng thí nghiệm ở §8.

**Q6. Nhãn agent-generated gây inconsistency và mất thông tin?**
Có, theo ba cách:
- (a) lệch nhãn giữa các model dẫn tới routing phụ thuộc client;
- (b) nén mất constraint quyết định (ví dụ Kafka);
- (c) agent buộc phải biết vocabulary của server, nên mỗi lần đổi taxonomy là phải cập nhật instructions ở mọi agent, vi phạm success criterion §67.

**Q7. Server nên tự infer intent?**
Có. Server là nơi duy nhất vừa thấy catalog vừa sửa được tập trung.

**Q8. Infer mà không cần LLM mỗi request.**
- Mỗi skill có `routing.triggers` (5–15 câu goal mẫu) và `not_for` (câu phản ví dụ).
- Embed trigger bằng model embedding nhỏ chạy local (ONNX, cỡ bge-small), precompute lúc `rebuild`.
- Mỗi request chỉ tốn một lần embed goal (<20ms CPU), sau đó chạy kNN trên trigger vectors + BM25 trên name/description/triggers.
- Kết hợp bằng Reciprocal Rank Fusion + fact rules + operation boost − penalty (`not_for` similarity, constraint conflict).
- Thực chất đây là intent classifier dạng nearest-neighbor, không cần LLM.
- ⚠ Hệ quả: embeddings **không nên để Phase 2** như PRD §47 đang ghi. Với contract free-text, embeddings là phần lõi. Prototype so sánh "chỉ BM25 + triggers" và "có embeddings" trước khi quyết định.

**Q9. Kiểu resolution.**
**Hybrid, ưu tiên deterministic:**
- fact preconditions (rule-based, chỉ trên facts);
- retrieval lexical + semantic;
- score fusion;
- calibrated confidence.
- Hierarchy chỉ dùng để nhóm câu hỏi `needs_context` và cho UI, **không** làm stage lọc cứng, vì lỗi dồn tầng và thất bại với task cross-cutting.
- LLM chỉ là fallback (§4.6).

**Q10. Trả về gì.**
Tùy status:
- `resolved`: 1 primary + ≤2 supporting, chỉ khi có quan hệ khai báo;
- `choose`: ≤3 option kèm discriminator;
- `needs_context`: 1 câu hỏi;
- `no_skill`.
Không trả shortlist dài. Không trả composed package, vì nội dung sinh ra thì không curated và phá vỡ digest integrity.

**Q11. Agent có được chọn lại từ candidates?**
**Chỉ trong status `choose`**: tối đa 3 option, mỗi option có `choose_if` do curator viết (lấy từ `distinguish_from` trong metadata). Agent không rerank tự do mà trả lời một câu hỏi phân biệt bằng context mình có. Lựa chọn được gửi lại qua `skill_feedback`.
Lý do phản bác baseline: bắt agent gửi `context_patch` rồi chờ server re-resolve chỉ để truyền *đúng thông tin đó* là tốn thêm một round-trip mà không làm tăng tính nhất quán.

**Q12. Khi phát hiện mismatch sau `resolved`.**
Agent gửi re-resolve kèm `prior.outcome=rejected` + `reason_code` + facts mới. Server loại skill vừa bị từ chối và resolve lại. Không cho agent tự chọn skill khác ngoài danh sách server trả. Giới hạn 2 lần re-resolve cho mỗi goal, sau đó trả `no_skill`.

**Q13. Phát hiện ambiguity.**
Dựa trên calibrated score:
- `top1 < floor` → no_skill;
- `top1 ≥ high` và `margin(top1, top2) ≥ m` → resolved;
- margin nhỏ và các candidate có quan hệ `distinguish_from` với nhau → `choose`;
- margin nhỏ nhưng một fact *chưa biết* (unknown, không phải false) sẽ tách được các candidate → `needs_context`;
- còn lại → resolved với confidence `medium`.
Ngưỡng được học từ golden eval set (§8) và lưu trong `config/recommendation.yaml`.

**Q14. Protocol `needs_context` tốt nhất.**
- Server hỏi **một fact cụ thể**, dùng tên field trong schema và kèm `options` đóng nếu có. Ví dụ: `artifact.kind ∈ {pull-request, service, design-doc}`.
- Kèm `answer_from: agent|user`.
- Agent tự trả lời nếu đã có context. Chỉ hỏi user khi `answer_from=user` hoặc agent thật sự không biết.
- Tối đa 1 vòng. Sau đó server phải resolve best-effort hoặc trả no_skill.
- Không hỏi câu mở kiểu "Review focus là gì?", vì câu mở khiến các model trả lời khác nhau.

**Q15. Khi nào `no_skill`.**
- Điểm dưới floor.
- Skill tốt nhất vi phạm precondition dạng fact.
- Constraint loại trừ mục đích chính của mọi candidate.
- Goal khớp `not_for` mạnh hơn khớp trigger.
- Task dưới `min_scope` của skill (ví dụ "rename 1 biến").
Ngoài ra: no_skill được cache theo (session, goal hash) để agent không gọi lặp.

**Q16. Chống over-routing.**
- **Phía agent:** instruction chỉ gọi resolve *khi bắt đầu task không tầm thường hoặc khi đổi operation*, không gọi mỗi turn.
- **Phía skill:** mỗi skill khai báo `min_scope` và `not_for`.
- **Phía server:**
  - abstain có calibration;
  - áp penalty cho "hub skills", tức skill quá generic thắng ở >X% query (đo và cảnh báo);
  - resolution kèm `valid_for: operation`, nên agent không re-resolve khi operation chưa đổi.
- **Metric theo dõi:** skill loaded rồi bị abandoned.

**Q17. Những gì derive tự động được.**
Qua MCP `roots`, server local tự làm, có cache theo mtime:
- manifest files: package.json, go.mod, pyproject, Cargo;
- Dockerfile, k8s manifests, openapi, SQL migrations;
- CI config, test framework;
- frameworks suy ra từ dependency list.
Ngoài ra:
- client identity lấy từ MCP `initialize.clientInfo`;
- session grouping lấy từ connection;
- git branch/diff stat nếu được phép.
Agent chỉ cần gửi những thứ server *không thể thấy*: active file, operation, goal, constraints, signal gần nhất.

**Q18. Persist cho feedback mà không có hidden state.**
- **Runtime (disposable, gitignored):** `resolution_events`, `feedback_events`, gồm goal đã redact, request hash, candidates, scores, outcome.
- **Git (durable, được review):** routing chỉ thay đổi qua file trong Git:
  - `evals/routing/*.yaml` là golden cases, được *promote* từ event log bằng lệnh `skillhub routing promote <resolution_id>`, có bước redact bởi người;
  - `skills/*/skill.meta.yaml#routing` thêm trigger hoặc not_for do UI đề xuất và người duyệt;
  - `config/recommendation.yaml` chứa threshold và weights. Nếu có tune tự động thì phải xuất ra file này.
- Quy tắc: **không có weight nào tự học ngầm trong SQLite**. Rebuild từ Git phải cho ra đúng hành vi routing đó (khớp acceptance §50).

**Q19. Metrics báo kiến trúc tệ.**
- Top-1 accuracy trên golden set < 85% hoặc abstention precision thấp.
- **Cross-client agreement**: cùng một task chạy qua Claude/Codex/Gemini mà resolve ra skill khác nhau > 10%.
- Tỉ lệ veto/re-resolve > 10%.
- Tỉ lệ `choose` > 15% (dấu hiệu catalog chồng lấn).
- Tỉ lệ `needs_context` > 10%.
- Tỉ lệ loaded-then-abandoned cao.
- Calibration error (ECE) cao.
- LLM fallback rate tăng dần.
- Top 5 skill chiếm > 40% resolved (hub dominance).
- Skill không bao giờ được chọn (dead catalog).
- **Under-invocation**: session không trivial nhưng không gọi resolve.
- p95 latency > 300ms (khi không dùng LLM).

**Q20. Failure modes ở scale (xếp theo khả năng xảy ra).**
1. **Under-routing**: agent không gọi tool hoặc gọi không đều. Đây là failure lớn nhất trong thực tế và protocol không tự sửa được.
2. **Metadata rot**: skill được sửa nhưng triggers/not_for không cập nhật, khiến retrieval trượt dần.
3. **Near-duplicate skills** chia điểm cho nhau, margin luôn nhỏ, dẫn tới `choose`/ambiguity thường trực.
4. **Hub skills** (generic) thắng mọi thứ.
5. **Resolution lỗi thời** khi task đổi phase mà agent vẫn dùng skill cũ.
6. **Vòng lặp** re-resolve/needs_context.
7. **Prompt injection** từ metadata upstream đi vào LLM reranker hoặc vào agent qua `choose_if`, vì text upstream là untrusted (§43).
8. **Goal text trong log** gây rò riêng tư khi chuyển sang remote (Phase 3).
9. **Eval set overfit**: threshold tune trên tập nhỏ.

---

## 3. So sánh kiến trúc

Thang điểm: ●●● tốt · ●● trung bình · ● kém (với "coupling", ●●● = coupling thấp).

| Tiêu chí | A. Rich SRC | B. Facts + summary | C. Raw task, server LLM | D. Hierarchical deterministic | E. Server shortlist, agent chọn | **R. Đề xuất (B+)** |
|---|---|---|---|---|---|---|
| Routing accuracy | ●● (có vực: nhãn sai thì recall = 0) | ●●● | ●● (thiếu execution state) | ●● (lỗi dồn tầng, cross-cutting fail) | ●●● | ●●● |
| Consistency giữa model | ● | ●● | ●●● | ●●● | ● (rerank tự do) | ●●–●●● |
| Latency | ●●● | ●●● | ● (LLM mỗi call) | ●●● | ●●● | ●●● (rare LLM) |
| Token cost | ●●● | ●●● | ● | ●●● | ●● (K skill cards mỗi call) | ●●● |
| Privacy | ●●● | ●● | ● | ●●● | ●●● | ●● (goal text, không code) |
| Implementation complexity | ●● (shared taxonomy có version) | ●● | ●●● đơn giản | ● (duy trì cây taxonomy) | ●●● | ●● |
| Debuggability | ●● (khó phân biệt nhãn sai hay rank sai) | ●●● | ● (LLM mờ) | ●●● | ● (vì sao agent chọn?) | ●●● (reason codes + eval) |
| Cải thiện tập trung | ● (đổi taxonomy phải đổi agent) | ●●● | ●●● | ●● | ● (logic nằm ở agent) | ●●● |
| Coupling agent–server | ● | ●●● | ●●● | ●● | ●● | ●●● |

Nhận xét:
- **A** tốt nhất về cost nhưng lại đặt phần khó nhất (map task sang ontology) vào nơi *kém nhất quán nhất*.
- **C** nhất quán vì input thô, nhưng input thô *không phải* là thứ quyết định routing, vì execution state nằm ngoài raw prompt. Ngoài ra C còn tốn kém và lặp lại reasoning.
- **D** hấp dẫn về debug nhưng taxonomy tree là gánh nặng curation và thất bại ở task nằm giữa nhiều nhánh.
- **E** chính xác nhất ở từng request nhưng đưa routing intelligence về phía agent, khiến metric và chất lượng phụ thuộc model.
- **R** lấy B làm nền, cộng thêm: một enum nhỏ từ A, server-derived facts, và phần tốt nhất của E ở dạng *bounded và server-discriminated*.

---

## 4. Kiến trúc đề xuất (R)

### 4.1 Agent responsibility
- Gọi `skill_resolve` khi bắt đầu một task không tầm thường, hoặc khi `operation` đổi. Không gọi mỗi turn.
- Viết `goal` bằng lời của mình: động từ + đối tượng + phạm vi, ≤300 ký tự, không code, không secret.
- Tách constraints của user thành list riêng.
- Gửi active artifact và signal thực thi gần nhất.
- Làm theo status:
  - `resolved`: load và dùng;
  - `choose`: chọn theo `choose_if`;
  - `needs_context`: trả lời fact được hỏi;
  - `no_skill`: làm tiếp không cần skill.
- Báo outcome qua `skill_feedback` (used / abandoned / rejected + reason).
- **Không** biết catalog, **không** gửi domain/topics, **không** chọn skill ngoài option server trả.

### 4.2 Server responsibility
- Derive workspace facts qua roots và cache.
- Retrieval, rank, calibrate, abstain.
- Viết câu hỏi `needs_context` (dạng field-level).
- Chọn supporting skills từ quan hệ đã khai báo.
- Log events (runtime), đề xuất promote eval và metadata vào Git.
- Enforce giới hạn vòng lặp.
- Trả SKILL.md dạng embedded resource khi resolved với confidence high, để tiết kiệm 2 round-trip.

### 4.3 Skill metadata bổ sung (Git, trong `skill.meta.yaml`)
```yaml
routing:
  operations: [review]                 # soft
  triggers:                            # 5–15 goal mẫu, được embed lúc rebuild
    - "review retry and idempotency handling in a message consumer"
    - "check whether consumer offsets are committed after side effects"
  not_for:
    - "design a new event-driven service topology"
    - "tune kafka broker configuration"
  requires_facts:                      # hard, CHỈ trên facts; unknown ≠ fail
    any: [{tech: kafka}, {tech: rabbitmq}, {tech: sqs}, {tech: nats}]
  boosts:
    artifact_kind: [source-file, pull-request]
  min_scope: multi_step                # chống over-routing
  distinguish_from:                    # nguồn của discriminators cho `choose`
    - skill: event-driven-architecture-review
      choose_this_if: "the work is limited to consumer code, not service boundaries"
  supporting:
    - skill: idempotency-test-design
      role: "when retry paths lack tests"
      load: on_demand
```
`skillhub validate` bắt buộc có `triggers` và `not_for` với mọi skill `status: active`. `skill-distiller` có thể đề xuất trigger mới nhưng người phải duyệt.

### 4.4 Request schema (`skill_resolve`, tool input)
```jsonc
{
  "goal": "string ≤300, agent's own words: verb + object + scope",       // required
  "operation": "explore|design|implement|review|debug|test|refactor|migrate|document|operate|research|other", // optional, soft
  "constraints": ["string ≤120"],                                        // optional, max 5
  "artifact": {                                                          // optional
    "kind": "source-file|pull-request|diff|design-doc|spec|config|dataset|ticket|other",
    "paths": ["relative/path"],                                          // max 5
    "languages": ["go"]
  },
  "state": {                                                             // optional
    "signal": { "kind": "test_failure|build_error|runtime_error|lint|review_comment|none",
                "summary": "string ≤200" },
    "corrections": ["string ≤120"]                                       // user corrections affecting scope
  },
  "workspace": {
    "root": "file:///… (only if server has no roots)",
    "facts": ["tech:kafka", "file:Dockerfile"]                           // fallback only
  },
  "prior": {                                                             // re-resolution / choose-less retry
    "resolution_id": "res_…",
    "outcome": "rejected",
    "reason": "scope_too_broad|scope_too_narrow|wrong_operation|wrong_stack|already_known|other",
    "note": "string ≤120"
  }
}
```
Schema version nằm trong tên tool/`_meta` (`skillhub.resolve/1`), không nằm trong payload.

### 4.5 Response schema
```jsonc
{
  "resolution_id": "res_…",
  "status": "resolved|choose|needs_context|no_skill",
  "catalog": "git:<sha>",                         // tái lập được
  "skill": { "id": "…", "uri": "skill://…", "digest": "sha256:…",
             "confidence": "high|medium", "score": 0.0 },   // resolved
  "entrypoint": { "embedded": true },             // SKILL.md đính kèm khi confidence=high
  "supporting": [{ "id": "…", "role": "…", "load": "on_demand" }],   // ≤2
  "options": [{ "id": "…", "choose_if": "…" }],   // choose, 2–3
  "ask": { "field": "artifact.kind", "options": ["pull-request","service","design-doc"],
           "question": "…", "answer_from": "agent|user" },          // needs_context
  "no_skill": { "reason": "below_floor|precondition_failed|excluded_by_constraint|below_min_scope|not_for_match" },
  "reasons": ["trigger~0.82", "fact:tech=kafka", "op:review", "penalty:constraint(redesign)"],
  "valid_for": { "operation": "review" }          // re-resolve khi operation đổi
}
```
Agent chỉ thấy **band** confidence. `score` và `reasons` phục vụ debug và UI.

### 4.6 Sequences

**Normal path (1 call):**
```text
agent: skill_resolve{goal, operation, constraints, artifact}
server: facts(roots, cached) → candidates(BM25 ∪ kNN ∪ fact-rules)
        → preconditions → fuse + penalties → calibrate → high & margin ok
server → resolved + SKILL.md embedded
agent: execute; resources/read(references) on demand
agent: skill_feedback{resolution_id, outcome: used}
```

**Ambiguous path:**
```text
margin nhỏ & candidates có distinguish_from  → status=choose, 2–3 options + choose_if
   agent chọn bằng context sẵn có → skills/get(id) → execute
   skill_feedback{resolution_id, chosen: id}
margin nhỏ & 1 fact unknown tách được         → status=needs_context{field, options}
   agent trả lời (hoặc hỏi user nếu answer_from=user)
   skill_resolve{... + field, prior.resolution_id}   // tối đa 1 vòng
   → resolved | choose | no_skill (không hỏi lại)
```

**Re-resolution (veto sau resolved):**
```text
agent: skill_resolve{goal', …, prior{resolution_id, outcome: rejected, reason, note}}
server: exclude rejected id(s) cho goal hash này; áp dụng facts mới
        → resolved | no_skill      // ≤2 lần / goal, sau đó ép no_skill
```

### 4.7 LLM fallback policy
Thứ tự ưu tiên khi mơ hồ: **choose (agent) > needs_context > server LLM > no_skill**.

Server LLM chỉ chạy khi *tất cả* điều kiện sau đúng:
- (a) margin nhỏ;
- (b) các candidate **không** có `distinguish_from` để dựng `choose`;
- (c) không có fact unknown nào để hỏi;
- (d) `config.llm_rerank.enabled`.

Input cho LLM:
- goal + constraints + operation;
- ≤8 skill cards (id, description, when / not_for). Không đưa SKILL.md vào.

Output là JSON bị ràng buộc: `{pick|none|ask}`. Các ràng buộc khác:
- timeout 2s, hết giờ thì trả `choose` với top-3 kèm description;
- cache theo request hash + catalog sha;
- metadata upstream coi là untrusted, nên chỉ curated card được đưa vào prompt.

Mỗi lần LLM fallback còn được log thành *tín hiệu catalog*: hoặc cần thêm `distinguish_from`, hoặc có hai skill cần gộp. Mục tiêu dài hạn là tỉ lệ fallback tiến về 0 (khớp metric §46).

### 4.8 Confidence handling
- Score thô → xác suất qua isotonic/Platt calibration fit trên golden set, lưu ở `config/recommendation.yaml` cùng `high`, `floor`, `margin`.
- Chưa có dữ liệu thì dùng threshold bảo thủ (ưu tiên `choose`/`no_skill` hơn là resolved sai), rồi re-fit mỗi khi golden set tăng.
- Band `medium` thì agent vẫn dùng skill, nhưng được khuyến khích veto nếu mâu thuẫn.

### 4.9 Composition policy
- Server chỉ trả supporting skill khi có quan hệ **khai báo trong Git** (`requires`, `supporting`, `usually_followed_by`), tối đa 2, dạng `load: on_demand`.
- Composition theo thời gian (review → implement → test) = **re-resolve khi operation đổi**, không lập kế hoạch nhiều skill từ đầu.
- Không sinh composed package. Nếu một tổ hợp lặp lại nhiều, curator viết skill mới (tri thức mới phải curated và có digest).

---

## 5. Ví dụ

### 5.1 Code review có ràng buộc phạm vi (resolved)
```json
{
  "goal": "Inspect retry and idempotency behavior in the order-service Kafka consumer",
  "operation": "review",
  "constraints": ["do not redesign service boundaries", "only consumer package"],
  "artifact": { "kind": "source-file", "paths": ["services/order/consumer/retry.go"], "languages": ["go"] }
}
```
Server derive được `tech:kafka` (sarama trong go.mod). `event-driven-architecture-review` bị penalty vì constraint khớp mục đích chính của nó.
```json
{
  "resolution_id": "res_7Q2",
  "status": "resolved",
  "catalog": "git:3fa91c0",
  "skill": { "id": "consumer-reliability-review", "uri": "skill://software/consumer-reliability-review",
             "digest": "sha256:…", "confidence": "high", "score": 0.91 },
  "entrypoint": { "embedded": true },
  "supporting": [{ "id": "idempotency-test-design", "role": "when retry paths lack tests", "load": "on_demand" }],
  "reasons": ["trigger~0.84", "fact:tech=kafka", "op:review", "penalty:constraint→event-driven-architecture-review"],
  "valid_for": { "operation": "review" }
}
```

### 5.2 Debug theo tín hiệu thực thi (resolved, signal quyết định)
```json
{
  "goal": "Find why the checkout E2E test fails only in CI",
  "operation": "debug",
  "artifact": { "kind": "source-file", "paths": ["e2e/checkout.spec.ts"], "languages": ["typescript"] },
  "state": { "signal": { "kind": "test_failure",
             "summary": "TimeoutError: locator('#pay') 30000ms exceeded; passes locally" } }
}
```
```json
{
  "resolution_id": "res_8KD",
  "status": "resolved",
  "skill": { "id": "flaky-e2e-diagnosis", "confidence": "high", "score": 0.88 },
  "entrypoint": { "embedded": true },
  "reasons": ["trigger~0.79", "signal:test_failure", "fact:dep=@playwright/test", "op:debug"],
  "valid_for": { "operation": "debug" }
}
```
Nếu chỉ có rich SRC (`intent=debug, domain=testing, topics=[e2e]`), thông tin "passes locally / only in CI" là thứ phân biệt flaky-env với logic bug sẽ bị mất.

### 5.3 Task mơ hồ, không phải code (choose)
```json
{
  "goal": "Get the billing service ready for public launch next week",
  "operation": "operate",
  "artifact": { "kind": "other" }
}
```
```json
{
  "resolution_id": "res_9ZB",
  "status": "choose",
  "options": [
    { "id": "production-readiness-review", "choose_if": "the question is whether the service is safe to run: SLOs, alerts, rollback, capacity" },
    { "id": "launch-checklist-security", "choose_if": "the focus is exposure to the public internet: auth, rate limits, secrets, pen-test" },
    { "id": "release-communication-plan", "choose_if": "the work is announcing/coordinating the launch, not changing the system" }
  ],
  "reasons": ["margin:0.04", "distinguish_from:declared"]
}
```
Agent biết user trước đó nói "lo nhất là alerting", nên chọn `production-readiness-review` và gửi `skill_feedback{chosen}`. Nếu agent không có context này thì hỏi user bằng chính 3 dòng `choose_if`.

### 5.4 Task tầm thường (no_skill)
```json
{ "goal": "Rename variable cnt to retryCount in retry.go", "operation": "refactor",
  "artifact": { "kind": "source-file", "paths": ["services/order/consumer/retry.go"] } }
```
```json
{ "resolution_id": "res_A01", "status": "no_skill", "no_skill": { "reason": "below_min_scope" } }
```
Lý tưởng thì agent không gọi resolve cho task như vậy. Server vẫn phải an toàn nếu bị gọi.

---

## 6. Recommendation

**Chọn R (B+):**
- evidence-first request (goal + constraints tách riêng + facts + 1 closed-enum hint);
- server-derived workspace facts;
- deterministic hybrid resolver có calibration;
- bounded `choose` với curator discriminators thay cho server LLM rerank làm đường mơ hồ chính;
- composition chỉ theo quan hệ khai báo;
- mọi thứ ảnh hưởng routing nằm trong Git.

## 7. Top 3 rủi ro

1. **Under-invocation.** Kiến trúc tốt tới đâu cũng vô ích nếu agent không gọi `skill_resolve` đúng lúc. Việc gọi phụ thuộc tool description và instruction, và thay đổi theo client/model. *Mitigation:* đo tỉ lệ gọi trên transcript thật; tool description ngắn và rõ trigger; cân nhắc MCP prompt/instructions của server.
2. **Chất lượng routing metadata là nút cổ chai.** Triggers, not_for và distinguish_from cho hàng trăm skill tốn công curation và dễ mục. *Mitigation:* validate bắt buộc; distiller đề xuất trigger; UI promote từ event; eval chạy trong CI của repo.
3. **`choose` đưa lại phần phụ thuộc model.** Nếu tỉ lệ choose cao, tính nhất quán giảm và việc học phân tán. *Mitigation:* ngưỡng cảnh báo 15%; mỗi cặp hay rơi vào choose là ticket curation (gộp skill hoặc làm rõ biên).

## 8. Prototype trước khi freeze protocol

1. **Golden eval set** (~150 task, 30–50 skill thật; ~20% đáp án `no_skill`; ~15% cần constraint để đúng). Nguồn: transcript thật, có context thực thi.
2. **Thí nghiệm cross-model request generation**: cho Claude, Codex và Gemini sinh request theo schema A (rich SRC) và R trên cùng transcript. Đo:
   - (a) mức đồng thuận từng field;
   - (b) mức đồng thuận của *kết quả routing*.
   Đây là câu trả lời trực tiếp cho PRD §60 và làm được rẻ, trong vài ngày.
3. **Resolver deterministic** (BM25 + triggers, có và không có embeddings) chạy trên golden set. Đo top-1, abstention precision/recall, phân phối margin, rồi fit threshold.
4. **Hai upper bound** để biết còn bao nhiêu headroom: E (agent chọn trong top-5) và C (server LLM với raw task).
5. Prototype tool description, rồi đo **invocation rate** trên 20–30 session thật mỗi client.

## 9. Bằng chứng làm đổi recommendation

| Nếu thấy | Thì chuyển hướng |
|---|---|
| Schema A có mức đồng thuận field ≥90% giữa các model **và** routing accuracy ≥ R | Dùng A (rẻ hơn, filter mạnh hơn) |
| Deterministic top-1 < 80% dù metadata đầy đủ, và server LLM rerank thu hẹp khoảng cách với chi phí chấp nhận được | Nâng vai trò LLM (rerank ở gray zone thường xuyên hơn) |
| E vượt R >10 điểm accuracy mà cross-model agreement vẫn ≥90% | Dịch về E (shortlist ngắn + agent chọn, log đầy đủ) |
| Tỉ lệ `choose` > 20% kéo dài sau khi curation | Catalog chồng lấn mang tính cấu trúc; xem lại granularity skill hoặc dùng hierarchy D cho vùng đó |
| Goal text trong log/request bị coi là blocker riêng tư (remote, org) | Chuyển sang facts + operation + hashed goal embedding tính phía client; chấp nhận giảm accuracy |
| "Chỉ BM25 + triggers" ≈ có embeddings (chênh <3 điểm) | Giữ embeddings ở Phase 2 như PRD |

---

## 10. Local skill vs Hub skill — conflict & precedence

PRD chưa đề cập. Điểm mấu chốt: **hub không thấy skill local; chỉ agent/host thấy cả hai** → nếu không có luật rõ, việc phân xử tùy từng model.

### 10.1 Các kiểu conflict
1. **Visibility bias**: host (vd Claude Code) inject sẵn name + description của skill local vào context. Muốn dùng skill hub thì phải chủ động gọi `skill_resolve`, nên local thắng mặc định.
2. **Stale copy**: user đã copy skill về `~/.claude/skills` trước khi có hub. Bản đó lệch dần so với bản curated nhưng vẫn thắng theo (1).
3. **Double activation**: agent nạp cả local lẫn hub cho cùng một việc, dẫn tới chỉ dẫn chồng hoặc mâu thuẫn và tốn token.
4. **Trùng tên khác nội dung**, hoặc local là fork của một version hub.
5. **Convention conflict**: hub nói phương pháp chung, project-local nói "repo này làm khác".

### 10.2 Phân vai theo scope (không phân xử theo "cái nào tốt hơn")
| Loại | Vai trò | Xử lý |
|---|---|---|
| Project-local (`<repo>/.claude/skills`) | quy ước, context riêng của repo | giữ local; **thắng khi mâu thuẫn** (cụ thể hơn) |
| Hub | phương pháp dùng lại được | nguồn chính cho skill generic |
| User-global local (`~/.claude/skills`) có tính generic | thường là bản copy/tiền thân của skill hub | **không arbitrate lúc chạy, mà dọn**: import vào hub, xóa local |
| Plugin/vendor skill do host cài | của bên ngoài | để nguyên, hoặc ingest làm upstream source |

Precedence theo nguyên tắc giống CLAUDE.md, cái cụ thể hơn thắng: **project-local > hub > user-global generic**. Với trường hợp cuối, hub không chỉ thắng mà còn nên thay thế hẳn bản local.

### 10.3 Dùng cả hai = layering, không phải chọn một
Hub skill là *method*, project-local skill là *overlay ràng buộc*. Instruction cho agent chỉ thêm 1 dòng:
> Nếu chỉ dẫn của skill project-local mâu thuẫn với skill từ hub, theo project-local.

### 10.4 Mở rộng protocol tối thiểu
Request `skill_resolve` thêm field optional:
```jsonc
"local_candidates": [                 // 0–2 skill local mà host thấy liên quan; KHÔNG gửi toàn bộ inventory
  { "name": "code-review", "scope": "project|user|plugin", "digest": "sha256:…" }
]
```
Response thêm:
```jsonc
"local": {
  "decision": "defer_to_local|supersedes_local|compose",
  "target": "code-review",
  "reason": "project_scoped|unknown_to_hub|digest_matches_hub_v3_stale|overlay"
}
```
- `defer_to_local`: skill là project-scoped, hoặc hub không nhận diện được nó. Hub không phán xét nội dung mà nó không biết.
- `supersedes_local`: digest cho thấy bản local là bản copy cũ của một version hub từng phát hành. Để làm được việc này, hub lưu digest của mọi version đã phát hành và của mọi upstream source, là dữ liệu Git có sẵn.
- `compose`: dùng method từ hub cộng overlay local theo §10.3.

### 10.5 Fix gốc = dọn dẹp, không phải protocol
Còn skill trùng ở local thì (1) vẫn khiến model không gọi hub. Do đó cần thêm lệnh:
```text
skillhub doctor --scan-local
```
- Quét `~/.claude/skills`, `~/.codex/…`, `<repo>/.claude/skills`.
- Phân loại: exact duplicate / fork của hub version / cùng tên khác nội dung / conceptual duplicate (embedding).
- Đề xuất (không tự xóa):
  - **remove local**;
  - **import vào hub** làm source có type `local-path`, đi qua luồng distill → insight → apply như upstream khác;
  - **mark project overlay**.

### 10.6 Rủi ro còn lại
- Agent đã tự kích hoạt skill local thì có gọi hub nữa hay không là do hành vi của host quyết định; protocol không ép được.
- `local_candidates` để lộ tên skill local cho server. V1 chạy local nên ổn; Phase 3 remote cần cân nhắc lại.
- Metric bổ sung cho §2 Q19: tỉ lệ session dùng skill local trùng với một skill hub. Tỉ lệ này cao nghĩa là việc dọn dẹp chưa xong.

---

## Unresolved questions
- Mức hỗ trợ MCP `roots` thực tế ở Claude Code / Codex / Gemini CLI hiện tại. Cần kiểm tra trước khi coi server-derived facts là đường chính.
- Tool result chứa embedded resource (SKILL.md) có được mọi client hiển thị cho model hay không. Nếu không thì fallback về `resources/read`.
- Chính sách lưu `goal` text trong runtime log (retention, redact) cho V1 local so với Phase 3 remote.
- Có cần thêm `operation` nào (ví dụ `write` cho tài liệu phi-code) hay không. Quyết định bằng eval set, không bằng cảm tính.
