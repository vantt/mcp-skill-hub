# Ghi chú thiết kế: curator dùng CLI trên host có shell

**Ngày:** 2026-10-10
**Trạng thái:** đã duyệt 2026-10-10 (§9: 1–4 đồng ý). Bước 1–2 giao worktree J; bước 3 bài thử quyền do người dùng chạy.
**Phạm vi:** cách agent curate Skill Hub (skill `system-curator`, `skillhub connect`, profile MCP).
Không đổi resolver, telemetry, hay đường runtime (`skill_resolve`, `skill_get`, `skill_feedback`).
**Liên quan:** [observer plan §5.1](2026-10-08-observer-and-enrichment.md) (tách profile MCP),
[prompt worktree I](prompt-wave4-I-small-fixes.md) (Claude Code ghi 2 entry), `internal/systemskills/curator/SKILL.md`.

## 1. Câu hỏi

Đã có CLI làm được gần hết việc curate, vậy trên host có shell còn cần server MCP
`skillhub-curation` không?

## 2. Đề xuất

| Phần | Host có shell (Claude Code, Codex CLI, Gemini CLI) | Host không có shell (Claude Desktop, web) |
|---|---|---|
| Runtime | MCP `skillhub` profile `runtime` (3 tool), giữ nguyên | MCP như hiện nay |
| Curation | **skill `system-curator` gọi CLI `skillhub … --json`**; không ghi entry `skillhub-curation` | MCP profile `curation` (hoặc `all`) như hiện nay |

Kết quả trên Claude Code: một server `skillhub` với 3 tool, không cần bật/tắt gì trong `/mcp`,
không cần câu nhắc của `connect`.

### Vì sao runtime vẫn phải là MCP

Server MCP sống suốt phiên nên nhớ được session: `VerifyPrior`, session hash, tracker
(`internal/app/resolver.go:60-78`). Case `verified_reformulation` và `repeated_gap` cần điều
đó. `skillhub resolve` qua CLI là một process mới mỗi lần, không nhớ gì giữa các lần gọi, nên
hai loại case này sẽ không bao giờ được ghi, đúng lúc mình cần cases cho Phase 3. Runtime cũng chỉ
tốn ~16,7 KB `tools/list`, và Claude Code còn hoãn tải schema.

### Vì sao curation không cần MCP trên host có shell

- CLI và MCP gọi chung tầng app service, nên kết quả và kiểm tra giống nhau.
- CLI mặc định là **preview**, chỉ ghi khi có `--yes` hoặc `skillhub skill confirm <PROP-id>`
  (đã thử 2026-10-10 trên hub tạm: `skill add`, `skill create`, `skill edit`,
  `skill activate`, `source watch` đều trả `action_required` + proposal, "No canonical files
  changed during preview").
- Hầu hết lệnh có `--json` với cùng phong bì `schema_version/status/summary/items/…` như MCP.
- Tiết kiệm context ở host tải hết schema (curation ~100 KB `tools/list`). Trên Claude Code
  lợi ích token nhỏ, lợi ích chính là bớt một server phải quản lý.
- Hợp với hướng tối giản: một đường curate (CLI), một chỗ để test.

## 3. Ánh xạ tool → CLI (đã thử trên hub tạm, binary `d65b0e7`)

| Tool MCP | Lệnh CLI | `--json` | Ghi chú |
|---|---|---|---|
| `hub_status` | `skillhub status` | có | |
| `skill_list` | `skillhub skill list [--state s]` | có | |
| `skill_review` | `skillhub skill review <id>` | có | `skill show <id>` cũng có |
| `skill_add_preview/confirm` | `skillhub skill add <locator>` → `skill confirm <PROP> --yes` | có | CLI nhận cả **thư mục local**, MCP thì không |
| `skill_create_preview/confirm` | `skillhub skill create <id> …` → confirm | có | |
| `skill_update_preview/confirm` | `skillhub skill edit <id> …` → confirm | có | |
| `skill_transition_preview/confirm` | `skillhub skill activate/deprecate/archive <id>` → confirm | có | |
| `skill_upstream_status` | `skillhub skill upstream <id>`, `skill outdated` | có | |
| `source_list` | `skillhub source list` | có | |
| `source_check` | `skillhub check` | có | **gọi mạng và chạy ngay**, không có preview |
| `source_import_preview/confirm` | `skillhub source capture` + `source triage --decision import` | cần kiểm | luồng khác MCP, 2 bước |
| `source_watch_preview/confirm` | `skillhub source watch <locator> --skill-id <id>` | có | |
| `workspace_validate/rebuild/diff` | `skillhub validate/rebuild/diff` | có | `rebuild` ghi ngay (derived, không phải canonical) |
| `routing_evaluate` | `skillhub eval routing` | **JSON trần**, không có phong bì | |
| `curation_session_record` | **không có** | | chỉ ghi telemetry; đề xuất bỏ ở đường CLI |

## 4. Lỗ hổng cần sửa trước khi chuyển

1. **`suggested_actions[].command` trả tên kiểu MCP** (`skill_active`, `skill_edit`,
   `workspace_diff`), không phải lệnh CLI. Agent đi đường CLI phải tự đoán. Sửa: thêm field
   `cli` (ví dụ `skillhub skill confirm PROP-… --yes`) vào mọi suggested action và vào kết quả
   preview; giữ `command` cho MCP.
2. **`eval routing --json`** in JSON không có phong bì chung. Đưa về cùng phong bì.
3. **`source import`** qua CLI là capture + triage, khác hẳn `source_import_preview/confirm`.
   Cần kiểm có preview không ghi gì và JSON có đủ danh sách skill/xung đột không; nếu không, thêm
   `skillhub source import <locator>` (preview mặc định, `--yes` để áp dụng) bọc đúng service MCP đang dùng.
4. **Confirm theo ID không ràng buộc digest.** MCP `*_confirm` cần proposal digest và base
   version; CLI `skill confirm <PROP>` chỉ cần ID. Đề xuất: curator luôn dùng dạng
   `--proposal <id> --proposal-digest <d> --base-version <v>` lấy từ JSON preview, để thứ người dùng
   duyệt đúng là thứ được ghi. Cân nhắc cho JSON preview trả sẵn lệnh confirm đầy đủ.
5. `curation_session_record`: bỏ ở đường CLI (chỉ là đo lường, không ai dùng tới hiện nay).

## 5. An toàn: ranh giới "người duyệt"

Qua MCP, các tool `*_confirm` bắt preview trước, và **không có tool duyệt nội dung**
(`--approve-content` chỉ có ở CLI, curator dặn "never attempt it", `SKILL.md:143-144`).

Qua CLI, agent về kỹ thuật có thể chạy thẳng `--yes` hoặc `skill edit --approve-content`. **Nhưng đây
không phải rủi ro mới:** agent có shell đã chạy được các lệnh này từ trước, dù có server
`skillhub-curation` hay không. Ranh giới hiện nay vốn chỉ dựa vào lời dặn trong skill và quyền
của host. Đề xuất làm nó rõ ràng hơn:

1. Curator dặn: luôn chạy bản preview, cho người xem diff, chỉ confirm khi người dùng đồng ý rõ ràng;
   không bao giờ dùng `--approve-content`, `--force`, `purge`, `migrate --yes`.
2. `connect` ghi quyền cho Claude Code vào `.claude/settings.local.json` (đã ghi file này cho
   `additionalDirectories`):
   - `allow`: `Bash(skillhub status:*)`, `Bash(skillhub skill list:*)`, … (các lệnh chỉ đọc);
   - `ask`: mọi lệnh có `--yes` hoặc `confirm`;
   - `deny`: `--approve-content`.

   **Phải kiểm chứng** cú pháp và thứ tự ưu tiên allow/ask/deny trên Claude Code 2.1.296 bằng
   một bài thử như lần profile. Ở chế độ `--dangerously-skip-permissions` thì quyền không có tác
   dụng, chỉ còn lời dặn trong skill; vẫn như hiện nay.
   **Đã kiểm chứng 2026-10-10** (người dùng, Claude Code 2.1.296, chế độ quyền mặc định, project
   `.claude/settings.local.json` với allow `Bash(skillhub:*)`, ask `Bash(skillhub * --yes*)` và
   `Bash(skillhub * confirm *)`, deny `Bash(skillhub * --approve-content*)`):
   - `status`, `skill list`, `skill add` (preview), `cd hub && skillhub skill list`: chạy thẳng.
   - `skill add … --yes --json`, `skill confirm PROP-…`, `skillhub --workspace hub skill add … --yes`:
     **hỏi**, thông báo nêu đúng luật ask. Luật ask thắng allow; vị trí cờ không lách được.
   - `skill edit … --approve-content …`: **bị chặn**, không chạy.
   - Bản tóm tắt của agent ghi "no prompt" cho các lệnh bị hỏi: agent không thấy hộp thoại quyền,
     nên chỉ tin điều người dùng thấy.
   - Lần mở đầu, hộp thoại trust báo "This folder pre-approves 1 tool permission … Bash(skillhub:*)".
     `connect` nên báo trước điều này.
   - Phụ: CLI không nhận `--workspace` đứng trước lệnh con (`skillhub --workspace hub skill …` lỗi);
     curator phải đặt cờ sau lệnh con. Chưa thử lệnh ghép có `--yes` (`cd x && skillhub … --yes`).
3. Codex/Gemini: chỉ có lời dặn trong skill cho tới khi kiểm chứng được cơ chế quyền tương ứng.

## 6. Curator: một file hay hai

**Đề xuất: một `SKILL.md`.** Bảng "User intent → Behavior" thêm cột CLI cạnh cột MCP, và một đoạn
ngắn ở đầu: "nếu có shell và lệnh `skillhub` chạy được thì dùng CLI với `--json`; nếu không thì
dùng tool MCP". Hai file riêng sẽ lệch nhau theo thời gian. Bản native (Claude Code/Codex/Gemini) và
bản MCP (host khác) vẫn cùng nội dung như hiện nay (5(c)).

Viết theo cách người dùng muốn cho skill: mỗi mục nói vì sao, hỏi gì, xem gì, ví dụ lệnh, không chỉ
là bảng ngắn.

## 7. Các bước (sau khi duyệt)

| Bước | Việc | Ai |
|---|---|---|
| 0 | Worktree I chạy xong như prompt hiện tại; merge. Matrix giữ bằng chứng toggle | agent I, lead |
| 1 | Sửa lỗ hổng CLI §4 (field `cli`, phong bì `eval routing`, `source import`, lệnh confirm đầy đủ trong JSON preview); test | agent |
| 2 | Curator §6: cột CLI, đoạn chọn giao diện, quy tắc an toàn §5.1; test server-boundary giữ nguyên cho bản MCP | agent |
| 3 | Bài thử quyền Claude Code §5.2 (người dùng chạy, lead chuẩn bị); ghi matrix | người dùng, lead |
| 4 | `connect`: host có shell chỉ ghi `skillhub --profile runtime` + quyền §5.2; bỏ entry `skillhub-curation` mà `connect` đã ghi trước đó (chỉ khi nội dung khớp đúng thứ mình ghi); `doctor` gợi ý chạy lại `connect`. Host không có shell giữ nguyên | agent K — xong cho Claude Code; Codex/Gemini giữ một entry full trong wave này |
| 5 | Smoke: temp HOME, Claude Code curate thật bằng CLI (như bằng chứng cũ trong matrix: `claude -p … "Curate my Skill Hub"`, chỉ đọc) | lead — **xong 2026-10-10**: project do `connect` (main `7767a4b`) ghi, `claude -p --mcp-config .mcp.json --strict-mcp-config --permission-mode default`: chỉ server `skillhub`; curator dùng `skillhub status`, `skill show`; hub không đổi. Lần thử ghi bị chặn (hub không đổi), nhưng chặn ở `command -v skillhub && …` chứ không phải luật ask. Khi user scope còn `skillhub-curation` (wave 4), agent dùng MCP trước và bỏ cuộc khi bị từ chối thay vì chuyển sang CLI |
| 6 | Curator 1.6.1: thăm dò bằng đúng `skillhub version` (một lệnh, không nối `&&`), ưu tiên CLI kể cả khi có tool MCP curation, và chuyển sang CLI khi tool MCP bị từ chối | lead — **1.6.1 xong**: mô tả skill nêu CLI trước, "First step" bắt buộc, "duyệt trước" không thay preview, chỉ gợi ý bật `skillhub-curation` khi không có shell. `claude -p` lần cuối: agent dùng CLI (trước đó có lần dừng hẳn), nhưng vẫn không chạy `skillhub version`, vẫn chạy thẳng `--yes` bỏ preview, và thử lại sau khi bị từ chối. Luật ask chặn được; hub không đổi |
| 7 | Mức tuân thủ curator: đo bằng một bộ kịch bản `claude -p` lặp lại (đọc, ghi có "duyệt trước", MCP bị từ chối), sửa câu chữ theo số liệu thay vì từng lần; lưu ý `-p` không hỏi lại người dùng được nên có thể khác phiên tương tác | mở |

Bước 1–2 có thể chung một worktree. Bước 4 phụ thuộc bước 3.

## 8. Ảnh hưởng tới worktree I

Mục 3 của I (Claude Code ghi 2 entry + câu nhắc `/mcp`) trở thành bước tạm: bằng chứng toggle trong
matrix vẫn đúng và vẫn dùng cho host khác, nhưng bước 4 ở trên sẽ bỏ entry `skillhub-curation` trên
Claude Code. Không cần dừng I; phần bị thay là câu nhắc và lựa chọn 2 entry cho Claude Code.

## 9. Câu hỏi cho người dùng (đã trả lời 2026-10-10: cả 4 đồng ý; §5.2 chỉ ghi vào `connect` sau khi bài thử đạt)

1. Duyệt hướng §2 (runtime MCP, curation CLI trên host có shell)?
2. Bỏ `curation_session_record` ở đường CLI (§4.5)?
3. Một `SKILL.md` chung cho cả hai đường (§6)?
4. `connect` có được ghi quyền allow/ask/deny vào `.claude/settings.local.json` (§5.2) không, sau khi
   kiểm chứng?
