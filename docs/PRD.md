# PRD — Curated Skill Hub over MCP

**Document ID:** PRD-SKILLHUB-001  
**Status:** Draft for architecture review  
**Target:** Open-source / self-hosted  
**Primary use case:** Central curated collection of Agent Skills distributed dynamically to multiple agents/projects through MCP  
**Storage philosophy:** Git-first, local-runtime database  
**Protocol:** MCP + official `io.modelcontextprotocol/skills` extension

---

# 1. Executive Summary

Curated Skill Hub là một hệ thống trung tâm cho phép người dùng:

- sưu tầm Agent Skills từ nhiều GitHub repository;
- quản lý source, version và commit mà từng skill đã được nghiên cứu/distill;
- theo dõi upstream để phát hiện skill nguồn thay đổi;
- distill kiến thức mới từ upstream;
- ghi lại từng insight/functionality đã học được;
- xác định insight nào đã hoặc chưa được incorporate vào bản skill curated;
- chỉnh sửa skill curated độc lập với upstream;
- tìm và recommend skill phù hợp cho agent;
- phân phối skill qua MCP mà không phải copy/install skill vào từng project;
- giữ toàn bộ curated state dưới dạng Git-committable;
- rebuild local database/index hoàn toàn từ repository khi setup trên máy mới;
- cung cấp UI đơn giản cho search, manage, edit, diff và curate.

Hệ thống kết hợp ba hướng triển khai:

1. **MCP `ext-skills`** làm protocol chuẩn cho distribution, identity, manifest và integrity.
2. **tech-leads-club/agent-skills** làm reference cho progressive disclosure và UX `search → read → supporting files`.
3. **gengirish/skills-mcp** làm reference cho source aggregation, catalog indexing và upstream ingestion.

Official MCP Skills extension hiện chuẩn hóa `skills/list`, `skills/get`, `resources/read`, optional `resources/directory/read`, frontmatter và per-file SHA-256 digest; specification cố tình chỉ giải quyết transport/distribution chứ không định nghĩa semantic recommendation.

Tech Leads Club triển khai progressive disclosure ba tầng: discovery bằng `search_skills`, activation bằng `read_skill`, và chỉ lấy references/scripts/assets cần thiết sau đó. Search dùng metadata/fuzzy matching thay vì load toàn catalog vào context.

`gengirish/skills-mcp` cho thấy mô hình aggregation/indexing ở quy mô hàng nghìn skill: source definitions → catalog builder → local index → search không cần network → chỉ fetch upstream khi cần full content/install.

---

# 2. Problem Statement

Agent Skills ngày càng được publish rải rác ở nhiều repository.

Cách thông thường hiện nay thường là:

```text
find skill
    ↓
clone/download
    ↓
copy vào project
    ↓
copy vào ~/.claude / ~/.codex / ...
    ↓
manual update
```

Mô hình này phát sinh:

- duplication;
- version drift;
- khó biết skill lấy từ đâu;
- khó biết upstream đã thay đổi chưa;
- khó ghi nhận tại sao một skill đã được sửa;
- khó merge bài học từ nhiều skill tương tự;
- khó chia sẻ cùng một curated collection cho nhiều project;
- skill catalog lớn gây context explosion nếu agent phải enumerate;
- mỗi agent/client triển khai discovery khác nhau;
- không có governance rõ ràng giữa upstream knowledge và nội dung curated riêng.

Curated Skill Hub phải biến skill collection thành một **managed knowledge asset** thay vì một tập folder được copy thủ công.

---

# 3. Product Principles

## 3.1 One source of truth

Một curated skill chỉ tồn tại ở một nơi.

Projects và agents không sở hữu copy riêng bắt buộc.

```text
Central Curated Repository
             │
             ▼
        MCP Server
             │
     ┌───────┼───────┐
     ▼       ▼       ▼
   Codex   Claude   Gemini
```

## 3.2 Git is the durable database

Mọi state quan trọng phải có representation text/binary nhỏ có thể commit vào Git.

Không được yêu cầu backup riêng cho PostgreSQL/SQLite để tái tạo hệ thống.

Runtime database phải là **derived state**.

```text
Git Repository
     │
     │ init / rebuild
     ▼
SQLite / FTS / vector index
     │
     ▼
MCP runtime
```

Có thể xóa toàn bộ runtime DB và rebuild lại mà không mất curated state.

## 3.3 Runtime database is disposable

SQLite, FTS5, embeddings index, caches và recommendation statistics có thể rebuild từ Git state.

Local database tồn tại để:

- search nhanh;
- query relationship;
- maintain index;
- cache GitHub metadata;
- phục vụ UI;
- recommendation.

Không phải canonical storage.

## 3.4 Curated skill ≠ upstream skill

Upstream là source material.

Curated skill là product riêng.

```text
Upstream Skill A ─┐
                  │
Upstream Skill B ─┼─→ Distillation → Curated Skill
                  │
Internal insight ─┘
```

Do đó không sử dụng mô hình “auto overwrite curated SKILL.md khi upstream đổi”.

## 3.5 Server owns skill discovery

Agent không phải search/rank catalog.

Skill Server chịu trách nhiệm:

```text
retrieve
filter
rank
recommend
resolve
```

Agent chủ yếu cung cấp context và sử dụng kết quả.

## 3.6 Progressive disclosure

Agent không được nhận full skill content trước khi resolution hoàn thành.

```text
resolution
   ↓
skill metadata
   ↓
SKILL.md
   ↓
specific references/scripts only when required
```

## 3.7 LLM recommendation is fallback, not default

Server ưu tiên:

```text
structured metadata
+ deterministic filters
+ text search
+ semantic retrieval
+ scoring
```

Chỉ sử dụng LLM reranker khi ambiguity đủ lớn.

---

# 4. Goals

Hệ thống phải hỗ trợ:

| Goal | Requirement |
|---|---|
| Central storage | Một curated repo phục vụ mọi project |
| MCP distribution | Không phải copy skill vào project |
| Standard compatibility | Support `io.modelcontextprotocol/skills` |
| Source provenance | Biết skill đến từ repo/path/commit nào |
| Change detection | Phát hiện upstream thay đổi |
| Distillation | Học từ source mà không overwrite curated skill |
| Distill history | Lưu lịch sử và rationale |
| Insight tracking | Theo dõi từng bài học đã incorporate chưa |
| User customization | Cho phép sửa curated skill |
| Git portability | Clone repo là đủ rebuild system |
| Recommendation | Server tìm đúng skill |
| Progressive loading | Không load full catalog |
| UI | Search/manage/edit/diff |
| Automation | Weekly upstream checking |

---

# 5. Non-goals — Initial Release

V1 không nhằm:

- trở thành marketplace công cộng;
- tự động execute arbitrary source code từ upstream;
- auto merge mọi upstream change;
- thay Git bằng database server;
- bắt buộc cloud service;
- tự động cài skill vào từng IDE;
- biến skill server thành general-purpose agent orchestrator.

---

# 6. High-Level Architecture

```text
                         GitHub Sources
                              │
                    ┌─────────▼──────────┐
                    │ Source Ingestion    │
                    │                    │
                    │ repos / paths      │
                    │ commit SHA         │
                    │ upstream metadata  │
                    └─────────┬──────────┘
                              │
                              ▼
                    ┌────────────────────┐
                    │ Git-First Registry │
                    │                    │
                    │ skills/            │
                    │ sources/           │
                    │ distill/           │
                    │ history/           │
                    │ collections/       │
                    └─────────┬──────────┘
                              │
                              │ rebuild
                              ▼
                 ┌──────────────────────────┐
                 │ Local Runtime Database   │
                 │                          │
                 │ SQLite                   │
                 │ FTS5                     │
                 │ vector index optional    │
                 │ relationship graph       │
                 │ upstream cache           │
                 └───────────┬──────────────┘
                             │
            ┌────────────────┴─────────────────┐
            │                                  │
            ▼                                  ▼
   ┌───────────────────┐              ┌─────────────────┐
   │ MCP Skill Server  │              │ Simple Web UI   │
   │                   │              │ (serve web)     │
   │ skills/list       │              │ search          │
   │ skills/get        │              │ edit            │
   │ resources/read    │              │ diff            │
   │ skill_resolve     │              │ curate          │
   │ skill_feedback    │              │ source status   │
   └─────────┬─────────┘              └─────────────────┘
             │
             ▼
       Agent / Client
```

*Note:* The Simple Web UI is delivered locally via `skillhub serve web`.
---

# 7. Repository Layout

Recommended initial structure:

```text
skill-hub/
│
├── skills/
│   ├── software/
│   │   └── architecture-review/
│   │       ├── SKILL.md
│   │       ├── references/
│   │       ├── scripts/
│   │       └── skill.meta.yaml
│   │
│   └── research/
│
├── sources/
│   ├── repositories.yaml
│   └── skills/
│       └── architecture-review.sources.yaml
│
├── distill/
│   └── architecture-review/
│       ├── insights.yaml
│       ├── history.md
│       └── runs/
│           ├── 2026-09-28.yaml
│           └── ...
│
├── collections/
│   ├── software-engineering.yaml
│   └── research.yaml
│
├── registry/
│   ├── skills.yaml
│   ├── aliases.yaml
│   └── deprecated.yaml
│
├── config/
│   ├── recommendation.yaml
│   └── sources.yaml
│
├── runtime/
│   └── .gitignore
│
└── .skillhub/
    └── schema-version
```

`runtime/` không commit.

---

# 8. Core Data Model

## 8.1 Skill

```yaml
id: architecture-review

name: Architecture Review

status: active

collection:
  - software-engineering

domain:
  - software-architecture

intents:
  - review
  - assess-design

topics:
  - distributed-systems
  - reliability
  - coupling

artifacts:
  - architecture-document
  - source-code

technologies: []

quality:
  curated: true
  reviewed_at: 2026-09-28

content:
  entrypoint: SKILL.md

created_at: 2026-08-01
updated_at: 2026-09-28
```

---

# 9. Source Provenance Model

Mỗi curated skill có thể học từ **nhiều upstream sources**.

Ví dụ:

```yaml
skill_id: architecture-review

sources:

  - source_id: tlc-architecture-review

    repository:
      url: https://github.com/example/repo
      branch: main

    path: skills/architecture-review

    upstream:
      current_commit: "84bd..."
      current_tree_hash: "929d..."

    distilled:
      commit: "139a..."
      tree_hash: "123f..."
      distilled_at: 2026-09-15T11:32:00Z

    monitoring:
      last_checked_at: 2026-09-28T02:00:00Z
      next_check_at: 2026-10-05T02:00:00Z
      status: changed
```

Quan trọng phải phân biệt:

```text
current upstream commit
```

và:

```text
last distilled commit
```

Nhờ đó hệ thống biết:

```text
upstream HEAD      = def456
last distilled SHA = abc123

→ SOURCE_CHANGED
```

---

# 10. Source Change Detection

Mỗi source có:

```text
last_checked_at
last_seen_commit
last_distilled_commit
last_seen_digest
last_distilled_digest
```

Change detector chạy:

```text
manual check
+
scheduled weekly check
```

Flow:

```text
source
 ↓
fetch branch HEAD / tree
 ↓
compare last_seen
 ↓
unchanged
     OR
changed
 ↓
mark distill_pending
```

Status đề xuất:

```text
UP_TO_DATE
SOURCE_CHANGED
DISTILL_PENDING
DISTILLED_WITH_PENDING_INSIGHTS
FULLY_INCORPORATED
SOURCE_UNAVAILABLE
```

Không auto-update curated content.

---

# 11. Weekly Check

Default:

```text
once / week
```

Config:

```yaml
source_check:
  enabled: true
  cadence: weekly
  stale_after_days: 8
```

UI phải hiển thị:

| Skill | Source | Last distilled | Upstream | State |
|---|---|---|---|---|
| architecture-review | repo A | abc123 | abc123 | Current |
| postgres-review | repo B | 10af21 | 932ccd | Changed |
| api-security | repo C | f2221a | 0aa11e | Distill pending |

---

# 12. Distillation Concept

Distillation **không đồng nghĩa merge**.

Distillation trả lời:

> Từ phiên bản upstream mới này, có điều gì đáng học?

Input:

```text
previous distilled source
current source
existing curated skill
existing distill history
```

Output:

```text
new insights
changed insights
removed/obsolete concepts
interesting implementation patterns
better instructions
better examples
better progressive disclosure
useful scripts/references
security implications
```

---

# 13. Distillation Skill

Hệ thống phải ship một system skill riêng:

```text
skill-distiller
```

Responsibility:

```text
read upstream skill source
compare against previous distilled version
read current curated skill
identify useful knowledge
avoid blindly copying
produce structured insight records
update distillation history
```

Distiller **không sửa curated skill**.

Nó chỉ tạo proposal/knowledge.

Example output:

```yaml
run_id: distill-20260928-001

source:
  previous: abc123
  current: def456

insights:

  - id: INS-0042

    title: Add explicit failure-mode review

    type: workflow_improvement

    importance: high

    evidence:
      source_files:
        - SKILL.md

    recommendation:
      Add failure-mode analysis before recommendation section.

    incorporation:
      status: pending
```

---

# 14. Insight Ledger

Mỗi bài học/functionality tìm thấy phải trở thành entity riêng.

Schema:

```yaml
id: INS-0042

skill_id: architecture-review

source:
  source_id: upstream-x
  commit: def456

title: Failure-mode analysis

description: >
  Upstream now explicitly reviews retry,
  timeout and partial failure behavior.

category:
  - methodology

importance: high

status: pending

incorporation:
  state: not_incorporated
  target_files: []
  commit: null

created_at: 2026-09-28
```

Possible incorporation states:

```text
not_incorporated
planned
partially_incorporated
incorporated
rejected
obsolete
```

---

# 15. Distill History

Mỗi curated skill có:

```text
distill/<skill>/history.md
```

Không chỉ machine state mà còn human-readable journal.

Example:

```markdown
## 2026-09-28 — Source abc123 → def456

### Key lessons

- Upstream added explicit failure-mode review.
- Retry analysis is now separated from idempotency.
- Reference loading became more progressive.

### Decisions

INS-0042 — Incorporate.
INS-0043 — Reject; too specific to Kafka.
INS-0044 — Pending discussion.

### Overall assessment

Useful update. No reason to replace current curated workflow.
```

Mục tiêu:

> Sau một năm vẫn hiểu được skill hiện tại hình thành như thế nào.

---

# 16. Apply Insight Skill

System phải có skill thứ hai:

```text
skill-insight-applier
```

Responsibility:

```text
read selected insight(s)
read current curated skill
propose minimal patch
preserve user-authored content
apply only approved lessons
update insight ledger
update history
```

Flow:

```text
INS-0042
   ↓
apply insight
   ↓
diff proposal
   ↓
user/reviewer approval
   ↓
patch SKILL.md
   ↓
mark incorporated
```

Nếu commit được tạo:

```yaml
incorporation:
  state: incorporated
  commit: 71acde
  files:
    - skills/.../SKILL.md
```

---

# 17. User-Owned Content

Curated skill là user-owned artifact.

User có thể sửa:

```text
SKILL.md
references/*
scripts/*
metadata
```

Hệ thống phải tuyệt đối tránh:

```text
upstream changed
→ overwrite local content
```

Thay vào đó:

```text
upstream changed
→ distill
→ insights
→ explicit apply
```

Điều này làm curated skill gần với:

```text
maintained knowledge fork
```

hơn là:

```text
mirrored package
```

---

# 18. Git-First Database

Canonical state nên dùng:

```text
YAML
JSON
Markdown
SKILL.md
```

Không nên commit SQLite binary.

Runtime initialization:

```text
git clone
 ↓
skillhub init
 ↓
validate schema
 ↓
scan skills
 ↓
scan sources
 ↓
scan distill records
 ↓
build SQLite
 ↓
build FTS
 ↓
build optional embeddings
 ↓
ready
```

Command:

```text
skillhub init
```

hoặc:

```text
skillhub rebuild
```

phải tái tạo toàn bộ runtime state.

---

# 19. Write-through State Model

Mọi state mutation đáng giữ phải:

```text
runtime change
       +
Git representation update
```

Ví dụ source check phát hiện commit mới:

```text
update DB
+
update sources/...yaml
```

Do đó:

```text
git status
```

ngay lập tức cho thấy thay đổi nào chưa commit.

Đây là requirement quan trọng.

Runtime database không được chứa hidden durable state mà Git không biết.

Ngoại lệ:

```text
cache
temporary recommendation statistics
embeddings
session data
```

---

# 20. Runtime Database

Recommended V1:

```text
SQLite
+
FTS5
```

Optional:

```text
sqlite-vec
```

hoặc embedded vector store.

Không cần PostgreSQL ở V1.

Suggested tables:

```text
skills
sources
source_versions
distill_runs
insights
skill_insight_links
collections
aliases
relations
files
search_documents
usage_events
resolution_events
```

---

# 21. Search Index

Search document không nên chứa full SKILL.md mặc định.

Index high-signal metadata:

```text
name
description
intent
domain
topics
triggers
artifacts
technology
desired outcome
quality
relationships
```

Có thể thêm selected summaries của SKILL.md.

Không index references/assets toàn bộ trừ khi cần.

---

# 22. Skill Relationships

System nên support:

```text
requires
complements
alternative_to
supersedes
conflicts_with
usually_followed_by
specializes
generalizes
```

Example:

```yaml
relationships:

  - type: complements
    skill: api-security-review

  - type: usually_followed_by
    skill: test-plan
```

Relationship graph sẽ cải thiện recommendation mà không cần LLM.

---

# 23. Collections

Collection là namespace/search scope, không phải context bundle.

```yaml
id: software-engineering

includes:
  - architecture/*
  - coding/*
  - testing/*
  - security/*
```

Agent không được load cả collection.

Collection chỉ giúp:

```text
filter
authorization
search scope
management
```

---

# 24. MCP Standard Interface

Hệ thống phải implement official:

```text
skills/list
skills/get
resources/read
resources/directory/read
```

khi applicable.

Official extension yêu cầu `skills/list` và `skills/get`; skill entry chứa frontmatter và file manifest/digest. `skills/get` đặc biệt hữu ích vì specification cho phép catalog listing partial hoặc thậm chí không enumerate toàn bộ catalog.

`skills/list` không phải normal recommendation API.

---

# 25. Custom MCP Recommendation Interface

Ngoài official protocol, server expose custom MCP tool:

```text
skill_resolve
```

V1 preferred contract:

```json
{
  "routing_context": {
    "intent": "review",

    "domain": "software-architecture",

    "artifact_types": [
      "source-code",
      "architecture-document"
    ],

    "technologies": [
      "go",
      "kafka"
    ],

    "topics": [
      "event-processing",
      "retry",
      "idempotency"
    ],

    "execution_phase": "review",

    "desired_outcomes": [
      "find-risks"
    ]
  },

  "task_summary": "Review reliability of an event-driven service"
}
```

**Không gửi raw conversation mặc định.**

---

# 26. Skill Resolution Context — SRC

Current preferred design:

Agent đã hiểu task nên cung cấp cho server một representation có cấu trúc.

SRC gồm bốn nhóm signal:

### Semantic

```text
intent
goal
desired outcome
```

### Artifact

```text
artifact type
language
framework
technology
```

### Execution

```text
phase
scope
operation
available capabilities
```

### Observed signals

```text
Dockerfile exists
Kubernetes manifests exist
OpenAPI detected
Kafka client detected
SQL migrations detected
```

Facts/artifact signals nên được scoring cao hơn free-text wording.

---

# 27. Recommendation Engine

Pipeline:

```text
SRC
 ↓
hard filters
 ↓
metadata matching
 ↓
FTS/BM25-style retrieval
 ↓
optional semantic/vector retrieval
 ↓
rule scoring
 ↓
candidate confidence
 ↓
optional LLM reranker
 ↓
resolution
```

LLM chỉ chạy nếu:

```text
candidate scores close
OR
intent ambiguous
OR
cross-domain composition required
```

---

# 28. Recommendation Result

Normal successful result:

```json
{
  "status": "resolved",

  "resolution_id": "res_123",

  "primary": {
    "skill": "event-driven-architecture-review",
    "confidence": 0.94
  },

  "supporting": [
    "kafka-reliability-review"
  ],

  "reason_codes": [
    "artifact_match",
    "intent_match",
    "technology_match"
  ]
}
```

Không cần expose reasoning dài.

---

# 29. Agent Should Not Re-rank

Current preferred behavior:

```text
Server = recommendation authority
Agent  = execution-context authority
```

Agent không nhận 10 results rồi tự rank lại.

Normal path:

```text
server resolves A
 ↓
agent accepts A
 ↓
load
```

Agent chỉ được veto nếu recommendation conflict rõ với context hiện tại mà server không biết.

---

# 30. Re-resolution Instead of Agent Override

Ví dụ server recommend skill quá rộng.

Agent gửi:

```json
{
  "resolution_id": "res_123",

  "feedback": "scope_mismatch",

  "context_patch": {
    "artifact_type": "pull-request",
    "scope": "retry-logic-only"
  }
}
```

Server resolve lại.

Agent không tự chọn candidate B.

Lợi ích:

```text
all routing intelligence stays server-side
all decisions measurable
all feedback learnable
behavior consistent across agents
```

---

# 31. Ambiguity Handling

Nếu confidence thấp:

```json
{
  "status": "needs_context",

  "resolution_id": "res_456",

  "missing_fields": [
    "review_focus"
  ],

  "question": "Review focus is design, security, or both?"
}
```

Agent có thể:

```text
use context already known
```

hoặc relay user question nếu cần.

Không nên gọi LLM server-side chỉ để rediscover thông tin agent đã có.

---

# 32. No-skill Result

Server phải có quyền trả:

```json
{
  "status": "no_skill"
}
```

Không force skill cho mọi task.

Đây là protection chống over-routing.

---

# 33. Progressive Skill Loading

Sau resolution:

```text
skill_resolve
 ↓
skills/get
 ↓
resources/read(SKILL.md)
 ↓
skill executes
 ↓
read referenced files only if needed
```

Pattern này giữ tinh thần progressive disclosure từ tech-leads-club, nơi discovery payload rất nhỏ, full `SKILL.md` chỉ load sau selection, và supporting files chỉ fetch khi instruction cần chúng.

---

# 34. Catalog Aggregation

Source definition:

```yaml
repositories:

  - id: anthropic-skills
    url: ...
    branch: main

  - id: tech-leads
    url: ...
    branch: main
```

Indexer:

```text
repository
 ↓
Git tree
 ↓
detect SKILL.md
 ↓
parse frontmatter
 ↓
calculate source identity
 ↓
update catalog
```

Có thể học pattern của `gengirish/skills-mcp`: source registry, Git tree scanning, cached SHA và build local catalog để search không phụ thuộc network runtime.

---

# 35. Ingestion States

Một discovered skill có lifecycle:

```text
DISCOVERED
 ↓
WATCHING
 ↓
REVIEWED
 ↓
DISTILLED
 ↓
CURATED_SOURCE
```

Không nhất thiết mọi source skill trở thành curated skill.

Một skill nguồn có thể chỉ đóng vai trò:

```text
learning source
```

cho một curated skill khác.

---

# 36. Duplicate Detection

Catalog aggregation cần detect gần-duplicate:

```text
same upstream fork
same skill copied between repos
similar name
similar description
similar content digest
```

Potential states:

```text
exact duplicate
probable fork
conceptual duplicate
independent
```

Mục tiêu tránh distill cùng một knowledge nhiều lần.

---

# 37. Source Snapshot Strategy

Không nhất thiết commit toàn bộ upstream code.

Canonical record tối thiểu:

```text
repository URL
path
branch/tag
commit SHA
tree/file digest
distilled commit
```

Optional:

```text
snapshot cache
```

nếu người dùng muốn audit offline.

Default recommendation:

> Do not vendor full upstream source unless required.

---

# 38. UI — V1

Simple local web UI.

Primary screens:

### Search

```text
search box
filters
results
quality
source
status
```

### Skill Detail

```text
SKILL.md preview
metadata
sources
history
relationships
insights
```

### Source Status

```text
last checked
last distilled
upstream commit
change detected
```

### Distill

```text
Run Distill
diff source versions
new insights
```

### Insight Inbox

```text
Pending
Incorporated
Rejected
Obsolete
```

### Diff/Edit

```text
current curated
proposed patch
side-by-side diff
edit
approve
```

---

# 39. UI Search Result

Recommended fields:

```text
Skill
Description
Domain
Source count
Quality
Last updated
Pending insights
Upstream changed?
```

---

# 40. Git UX

UI mutations must write files immediately.

Example:

```text
Edit SKILL.md
 ↓
save
 ↓
filesystem updated
 ↓
runtime DB refresh
 ↓
git diff available
```

Optional UI panel:

```text
Changed files
Uncommitted
Last commit
```

But Git commit itself can remain CLI-controlled in V1.

---

# 41. CLI

Minimum commands:

```text
skillhub init
skillhub rebuild

skillhub source add
skillhub source check
skillhub source check --all

skillhub distill <source>
skillhub apply <insight>

skillhub search "<query>"
skillhub inspect <skill>

skillhub validate
skillhub serve
skillhub ui
```

---

# 42. Automatic Maintenance

Background scheduler may handle:

```text
weekly source checks
catalog rebuild
stale source detection
index refresh
```

Scheduler results must write durable state to Git-compatible files.

No invisible daemon-only state.

---

# 43. Security

Remote skills are untrusted input.

Required safeguards:

```text
never auto-execute upstream scripts during ingestion
sanitize paths
restrict repository protocols
limit file size
limit recursive traversal
detect symlinks/path escape
digest every served file
```

Curated scripts may only execute according to host policy.

Official MCP Skills manifests expose per-file digests specifically to support integrity verification.

---

# 44. Quality / Trust Metadata

Each source and curated skill may carry:

```yaml
trust:
  source: community
  reviewed: true

quality:
  usefulness: 4
  clarity: 5
  originality: 3
  maintainability: 4
```

Do not initially let LLM invent these scores automatically.

Human review should remain authoritative.

---

# 45. Usage Feedback

Optional V2:

Agent/server records:

```text
skill recommended
skill accepted
agent vetoed
re-resolved
skill executed
skill abandoned
```

This data can improve future ranking.

However usage history must not silently modify curated Git content.

Derived model/index state can remain runtime-only.

---

# 46. Resolution Metrics

Track:

```text
resolution latency
candidate count
LLM rerank rate
no-skill rate
veto rate
re-resolution rate
top recommendation acceptance
average tokens returned
```

Critical metric:

```text
% resolutions completed without LLM
```

Goal should increase over time.

---

# 47. MVP Scope

MVP should include:

```text
Git-first repo structure
SQLite rebuild
source definitions
GitHub source ingestion
source commit tracking
weekly/manual source checks
curated skills
distill history
insight ledger
skill-distiller
skill-insight-applier
FTS search
skill_resolve
official MCP skills/list + skills/get
resources/read
basic UI
```

Vector retrieval and feedback learning may wait.

---

# 48. Phase 2

Potential Phase 2:

```text
embeddings
hybrid retrieval
skill relationship graph
duplicate/fork detection
LLM conditional reranking
multi-source composition
automatic change summarization
recommendation evaluation suite
```

---

# 49. Phase 3

Potential Phase 3:

```text
organization permissions
multiple curated repositories
remote MCP deployment
signed curated releases
public/private collections
cross-registry federation
skill quality benchmarking
```

---

# 50. Acceptance Criteria — Repository

A new user must be able to:

```text
git clone repo
skillhub init
skillhub serve
```

and obtain a fully working system without importing a database dump.

Deleting:

```text
runtime/*
```

then running:

```text
skillhub rebuild
```

must reproduce equivalent durable state.

---

# 51. Acceptance Criteria — Source Tracking

For every curated source, system must answer:

```text
Where did this come from?
Which repo?
Which path?
Which branch?
Which upstream commit exists now?
Which commit was last distilled?
When was it last checked?
Has upstream changed?
```

---

# 52. Acceptance Criteria — Distillation

After source changes:

```text
check
→ changed
→ distill
→ insights created
```

Distillation must not directly modify curated skill.

Each insight must independently record:

```text
pending
incorporated
rejected
obsolete
```

---

# 53. Acceptance Criteria — Application

Applying an insight must:

```text
produce diff
update curated file
update insight state
write history
remain Git-visible
```

---

# 54. Acceptance Criteria — Recommendation

Given SRC:

```text
intent
artifact
technology
topics
phase
```

server must return one of:

```text
resolved
needs_context
no_skill
```

Normal recommendation must not require the agent to rank a candidate list.

---

# 55. Acceptance Criteria — MCP

Server must expose official Skill extension semantics compatible with:

```text
skills/list
skills/get
resources/read
```

and expose recommendation as separate custom MCP tool rather than modifying the official specification.

---

# 56. Reference Architecture Mapping

## ext-skills

Use for:

```text
standard discovery primitives
skill identity
resource transport
manifest
digest/integrity
```

Do not extend its semantics unnecessarily.

## tech-leads-club

Borrow concepts:

```text
small discovery payload
intent-oriented lookup
progressive disclosure
SKILL.md loaded after discovery
supporting files loaded only on demand
```

Do not necessarily copy Fuse.js scoring unchanged.

## gengirish/skills-mcp

Borrow concepts:

```text
multi-source repository registry
catalog generation
Git tree ingestion
SHA caching
local index
separation between search index and upstream fetch
```

Extend it substantially for curated-state management.

---

# 57. Key Architectural Decision

The product is **not** primarily:

```text
an MCP search tool
```

It is:

```text
a Git-backed curated procedural-knowledge system
             +
an MCP distribution/recommendation layer
```

This distinction matters.

Search is replaceable infrastructure.

Curated knowledge, provenance, distillation history and recommendation policy are the durable value.

---

# 58. OPEN DISCUSSION — Agent ↔ Skill Server Resolution

**This section is intentionally NOT finalized.**

Frontier models reviewing this PRD should challenge the assumptions below rather than merely accepting the proposed architecture.

## Current Preferred Solution

Current proposal:

```text
Agent understands task
        ↓
Agent constructs compact
Skill Resolution Context
        ↓
skill_resolve(SRC)
        ↓
Server performs:
  filtering
  retrieval
  ranking
  optional LLM rerank
        ↓
Server selects primary skill
        ↓
Agent sanity-checks
        ↓
execute
```

Agent does **not** send full raw conversation/task by default.

Agent does **not** independently rank returned skills.

Agent may veto only if it knows execution context unavailable to server.

On veto:

```text
context_patch
   ↓
server re-resolution
```

rather than:

```text
agent chooses another candidate
```

---

# 59. Why This Is Currently Preferred

Agent already has substantial high-quality context:

```text
user goal
files currently inspected
artifact types
project stack
workflow phase
tool results
scope
environment
```

Sending the raw prompt alone loses some of this information.

Sending the full context creates:

```text
noise
privacy exposure
token cost
duplicate interpretation
server-side LLM cost
```

Therefore current design assumes the agent should generate a compact structured SRC.

---

# 60. Important Concern

This design introduces a new dependency:

> Can different agents reliably produce equivalent Skill Resolution Contexts?

Potential problem:

```text
Claude produces:
domain=architecture

Codex produces:
intent=code-review

Gemini produces:
topic=kafka
```

for the same underlying task.

If SRC generation varies too much, server recommendation becomes client/model-dependent.

This risk must be evaluated carefully.

---

# 61. Frontier Model Discussion Questions

Reviewing models should explicitly challenge at least the following.

### Question A — Should agent create SRC?

Is using the agent as a semantic compressor the right architecture?

Or should server receive a less interpreted representation?

### Question B — What is the minimum sufficient context?

Could a better contract be:

```text
task summary
artifact facts
environment facts
current operation
```

without requiring agent-derived domain/intents?

### Question C — Who should infer intent?

Options:

```text
agent
server
shared ontology/rules
hybrid
```

Compare correctness, cost and consistency.

### Question D — Should raw task be included?

Possibilities:

```text
never
optional
always
only on ambiguous resolution
```

Evaluate information value versus noise/cost/privacy.

### Question E — Should agent see candidates?

Current design says:

```text
normally no
```

But would returning:

```text
primary
+
2 hidden/visible alternatives
```

improve robustness?

### Question F — Should agent be allowed to choose an alternative?

Current preference:

```text
no
```

Agent instead sends context correction and server re-resolves.

Challenge this assumption.

### Question G — Should server run an LLM?

Can structured metadata + lexical + vector retrieval reach adequate quality?

When exactly is LLM reranking justified?

### Question H — Can recommendation be completely deterministic?

For a curated catalog, perhaps strong skill metadata and trigger definitions are enough.

Determine likely breakpoints.

### Question I — Could skill metadata itself become executable routing rules?

Example:

```yaml
activation:

  when:
    artifact: pull-request
    intent: review

  boost:
    technology:
      - typescript
```

Would this outperform generic semantic ranking?

### Question J — Should routing be hierarchical?

Example:

```text
task
 ↓
capability/domain
 ↓
task family
 ↓
specific skill
```

versus global retrieval.

### Question K — What should agent transmit that server cannot obtain elsewhere?

Specifically identify high-value context held only by agent.

Potential examples:

```text
current user intention
active artifact
current workflow phase
recent tool result
task scope
current failure state
```

### Question L — How do we prevent over-use of skills?

Skill server must be capable of returning:

```text
NO_SKILL
```

What confidence/model should govern this?

### Question M — Recommendation versus composition

Should server only identify one skill?

Or should it construct:

```text
primary + supporting skills
```

or even return a composed procedural package?

### Question N — Where should composition happen?

Potential:

```text
server
agent
skill itself
orchestrator
```

This deserves separate architectural analysis.

---

# 62. Required Output From Frontier Architecture Review

Any frontier model reviewing this section should return:

```text
1. Critique of current SRC approach.

2. What information should cross the
   Agent → Server boundary.

3. What information should NOT cross it.

4. Whether agent-derived semantic fields
   are useful or harmful.

5. Proposed request schema.

6. Proposed recommendation response schema.

7. Agent responsibility.

8. Server responsibility.

9. When LLM is required server-side.

10. Failure/ambiguity handling.

11. Whether agent should ever select
    among server candidates.

12. Alternative architecture if superior.
```

Models should be explicitly encouraged to propose a materially different design if appropriate.

---

# 63. Separate Open Discussion — Git-first Data Architecture

Frontier review should also evaluate:

```text
YAML/Markdown canonical state
       +
generated SQLite runtime
```

Specifically:

- schema evolution;
- merge conflicts;
- IDs;
- ordering;
- Git diffs;
- concurrent edits;
- runtime reconciliation;
- rebuild reproducibility;
- large insight histories;
- generated state accidentally committed;
- whether SQLite export/import would eventually be preferable.

Current preference remains Git-first.

---

# 64. Separate Open Discussion — Distillation Semantics

Questions:

```text
Should distillation compare only
last-distilled → latest?

Or also:
latest source → current curated skill?

Should deleted upstream knowledge generate insight?

Should similar insights across sources be merged?

Can insight identity survive wording changes?

When is insight "incorporated"?

How is partial incorporation measured?

Should rejected insights reappear after later source changes?
```

These require careful design before automation becomes aggressive.

---

# 65. Recommended Initial Technology Stack

Suggested lightweight implementation:

```text
TypeScript
Node.js
MCP TypeScript SDK

SQLite
FTS5

Git CLI / libgit2 wrapper

GitHub API

React / lightweight web frontend
```

Potential optional later component:

```text
sqlite-vec
```

This keeps deployment:

```text
npm install
skillhub init
skillhub serve
```

without infrastructure dependencies.

---

# 66. Final Product Mental Model

The system should ultimately behave like:

```text
                   CURATED KNOWLEDGE
                          │
                ┌─────────▼─────────┐
                │ Git Skill Library │
                └─────────┬─────────┘
                          │
              ┌───────────▼───────────┐
              │ Knowledge Management  │
              │                       │
              │ provenance            │
              │ upstream tracking     │
              │ distillation          │
              │ insights              │
              │ user editing          │
              │ history               │
              └───────────┬───────────┘
                          │
                  derived indexes
                          │
              ┌───────────▼───────────┐
              │ Skill Intelligence    │
              │                       │
              │ retrieval             │
              │ recommendation        │
              │ resolution            │
              │ relations             │
              └───────────┬───────────┘
                          │
                        MCP
                          │
          ┌───────────────┼────────────────┐
          ▼               ▼                ▼
        Claude          Codex            Gemini
```

The central architectural idea is:

> **Git owns knowledge.  
> The local database owns speed.  
> The Skill Server owns discovery and recommendation.  
> MCP owns distribution.  
> The agent owns task execution context, but not the skill catalog.**

---

# 67. Success Definition

The product is successful when a user can maintain hundreds or thousands of curated skills in one Git repository while an arbitrary MCP-compatible agent needs only a small stable instruction such as:

```text
For non-trivial work, consult the Skill Hub
using the task's current routing context.

Use the server's resolved skill by default.

Load only the resolved skill and resources
required by its instructions.

If the recommendation conflicts with execution
context unavailable to the server, return that
context and request re-resolution.
```

Adding the 1,001st skill should not materially increase the agent's base context.

Changing recommendation algorithms should not require updating every agent.

Updating an upstream skill should not silently alter curated knowledge.

And cloning the Git repository onto a new machine should be sufficient to reconstruct the entire operational Skill Hub.