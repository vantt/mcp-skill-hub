# Plan theo dõi: đo lường, quan sát và việc còn lại

**Ngày:** 2026-10-10
**Trạng thái:** đang mở, plan duy nhất còn sống sau khi 3 plan gốc được archive
**Phạm vi:** mọi việc còn dang dở của `simplify-hub-model`, `observer-and-enrichment` và `curator-via-cli`. Ba plan đó đã xong phần xây dựng và nằm trong `archive/plans/`; phần chưa xong được chuyển hết về đây.
**Nguồn:** kiểm chứng bằng 4 agent chỉ-đọc ngày 2026-10-10 (HEAD `b3d3f7f`), đối chiếu với code và `git log`.

Plan này chủ yếu là **đo lường và chờ dữ liệu**, không phải xây thêm. Phần lớn mục ở §1 không làm được bằng cách viết thêm code: cần hub có đủ skill và đủ lượt dùng thật để sinh số liệu. Các mục §2 là việc sửa nhỏ, làm được ngay.

## 1. Đo lường và quan sát (chờ dữ liệu hoặc chờ người dùng)

| # | Việc | Điều kiện mở khóa | Nguồn gốc | Ai |
|---|---|---|---|---|
| M1 | **Thêm skill thật vào hub.** Hiện chỉ có một skill active nên gần như mọi `skill_resolve` là `catalog_gap`, không sinh case. Có sẵn draft `herdr-cook-plan`, `markdown-to-epub`. | Không cần chờ gì | handoff lead | người dùng |
| M2 | **O7: lấy baseline.** `skillhub telemetry baseline` đã có. Ngày 2026-10-10: 0 resolved và 2 no_skill trên 30 cần mỗi loại, trạng thái `insufficient`. Đếm từ lúc hub lên schema v4/v5. Lưu ý raw giữ 30 ngày. | M1 và vài tuần dùng thật; `--min-chains 30` | observer §1.2 O7, `observer-phase-01b` | quan sát |
| M3 | **Case journal tích lũy.** Đã bật (`runtime/case_journal.json`), 0 case ngày 2026-10-10. Chỉ ghi khi resolver bất đồng. | M1 | observer Phase 2 | quan sát |
| M4 | **Phase 3 enrichment.** Quyết định hướng 1: case đã xác nhận thành eval case trong Git (`evals/routing/<case_id>.json`), lesson trích dẫn `repo@commit:path`; không sửa distill-lab. Hiện chưa có thư mục `evals/routing/`. | M3 có case, ngưỡng 2 case trên 2 ngày khác nhau | observer Phase 3 | agent, sau khi có case |
| M5 | **Phase 4 resolver** (FactProvider, admission gate vector, ghim skill, findability lint, `lint --explain`). Chỉ làm theo số liệu, không dùng bandit. | M2 đủ baseline | observer Phase 4 | agent, sau baseline |
| M6 | **Phase 0 scorecard** (distill-lab, đích `test-audit`). Cần quyết giữ/bỏ từng trường lesson: `notable`, `contrast`, R/E/F. `distill.yaml` hiện vẫn còn `contrast`. Không sửa distill-lab từ phía lead. | người dùng | simplify Phase 0 | người dùng |
| M7 | **Open decision 3: cắt bớt runtime** (một `catalog.db` không generation và pin, phục vụ skill từ checkout, kiểm staleness rẻ hơn, telemetry store đơn giản). Phải giữ D10: không bỏ event type, không đổi `EventVersion`, giữ `catalog_snapshot` ổn định. | M2 baseline và M4 replay | simplify Open decision 3 | quyết sau |
| M8 | **Curator compliance (bước 7).** Bộ kịch bản `claude -p` lặp lại: đọc, ghi có "duyệt trước", MCP bị từ chối. Đo số lần: chạy `skillhub version` trước, dùng preview trước `--yes`, chuyển sang CLI khi MCP bị từ chối, thử lại sau khi bị từ chối. Sửa câu chữ theo số liệu. `-p` không hỏi lại người dùng được nên có thể khác phiên tương tác. | không | curator-via-cli bước 7 | agent, chạy trên bản clone hub có `XDG_*` cô lập |

Chuỗi phụ thuộc: M1 → M3 → M4; M1 → M2 → M5 và M7. M6 và M8 độc lập.

## 2. Việc sửa nhỏ (không cần chờ dữ liệu)

| # | Việc | Bằng chứng | Nguồn gốc |
|---|---|---|---|
| F1 | Panic `$defs` của `committedResponse` khi schema không có `$defs`, chưa sửa và chưa có chủ | `archive/plans/261008-observer-and-enrichment/observer-handoff-G.md` | observer handoff G |
| F2 | Dòng gợi ý dạng text `Next: skillhub skill confirm PROP-id` không kèm digest | `skill_add.go:125`, `skill_editor.go:132`, `skill_upstream.go:456`; đường JSON đã ràng buộc đủ | curator-via-cli §4 |
| F3 | Hai bộ đếm O3 `unsupported_method_calls` và `unlisted_resource_reads` báo "unknown", không đo được. Hoặc bỏ hẳn khỏi báo cáo, hoặc đo thật. | `telemetry_baseline.go:200` | observer Phase 5, followup C |
| F4 | Lệnh ghép có `--yes` (`cd x && skillhub … --yes`) chưa thử với luật ask của Claude Code | curator-via-cli §5.2 | curator-via-cli |
| F5 | Skill `test-audit` thật có 0 ví dụ (validate cảnh báo) | handoff lead | handoff |
| F6 | Không tìm thấy `TestTrustVerdictAndUpstreamStatusUnchangedAfterMigration` trong code dù phase 4 nhắc tới. Kiểm tên test hoặc bổ sung. Các test migration chưa được chạy lại trong đợt kiểm này. | `plans` phase 4 (đã archive) | simplify Phase 4 |
| F7 | Web tab của O6 chưa kiểm | | observer O6 |
| F8 | Hai tag local `pre-schema-v4` và `pre-schema-v5` chưa push | `git tag` | handoff |

## 3. Kiểm chứng host (chưa có bằng chứng)

| # | Việc | Ghi chú |
|---|---|---|
| H1 | **Codex và Gemini chưa có cơ chế quyền.** `connect` vẫn ghi entry full gồm cả `skillhub-curation`; mới chỉ có lời dặn trong skill. Cần tìm cơ chế allow/ask/deny tương ứng, kiểm chứng như đã làm với Claude Code, rồi chuyển hai host này sang runtime-only. | `internal/hostintegration/preview.go`, `matrix.go` |
| H2 | Cursor, Claude Desktop, Codex: toggle `skillhub-curation` chưa kiểm. Gemini không có toggle theo server. Với Claude Code đã verified và không còn cần. | `docs/mcp-compatibility-matrix.json` |
| H3 | `skills_extension` chưa verified ở client stock nào; mọi host native giữ bản native và server ẩn bản MCP. | observer Phase 5 (c) |
| H4 | `directoryRead: false` (quyết định hoãn hẳn). Chỉ mở lại khi có client thuần MCP thật sự thiếu file. | observer Phase 5 |

## 4. Quy tắc cho mọi việc ở đây

- Không bỏ event type và không đổi `EventVersion` (D10 cũ).
- Không ghi vào hub thật `/home/vantt/skill-hub`, không push, không sửa distill-lab nếu người dùng chưa yêu cầu.
- Thử migration hoặc ghi trên bản clone có `XDG_*` cô lập.
- Mỗi việc xong thì ghi commit vào bảng này rồi xóa dòng, khi bảng rỗng thì archive plan.

## 5. Tài liệu đã archive (tham chiếu)

- `archive/plans/261008-1433-simplify-hub-model/`: plan và 5 phase, mô hình Git-native.
- `archive/plans/261008-observer-and-enrichment/`: plan observer, `observer-phase-01b-baseline.md`, hai handoff còn lại.
- `archive/plans/261010-curator-via-cli/`: plan curator qua CLI.
- `archive/plans/261010-agent-prompts-and-handoffs/`: prompt các worktree wave 2–5 và handoff đã dùng hết.
