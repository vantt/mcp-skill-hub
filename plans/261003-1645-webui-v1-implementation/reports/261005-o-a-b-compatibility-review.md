# Đối chiếu O, A và B sau thay đổi runtime

Ngày: 2026-10-05. Phạm vi: rà soát kế hoạch và source, không triển khai hoặc sửa A/B.

## Bản được đối chiếu

- O: `plans/261004-1547-skill-execution-measurement-routing/`, trạng thái done, đã có trong `main`.
- A: [plan.md](../plan.md) và các phase; `main` tại `338eff5d84802ef99e8d0a5ee36ff614a36710bd`. Phase 1–3 Complete; phase 4 Superseded; phase 5–6 Pending.
- B: `plans/261004-2234-skill-source-upstream-ux/`, toàn bộ 10 phase trên branch `feat/skill-source-upstream` tại `0d38b78b0803ff3ebbe98c3b90558a98669ede42`. Thư mục này chưa có trong checkout main. Đọc bằng `git show`, không checkout hoặc merge.

## Kết luận

O và B tương thích về kiến trúc chủ đạo. A và B chưa sẵn sàng để giao executor theo nguyên văn: còn xung đột guard, thiếu dependency thực tế, thất lạc phạm vi Handoff/Runs, và giả định sai về frontend contract.

Không cần thiết kế lại cả hai plan. Cần một lượt hòa giải kế hoạch dựa trên source sau O, với phân định rõ phần Sources được thay thế và phần learning/distillation còn phải giao.

## O đã thiết lập những ranh giới nào?

1. Content trust theo toàn bộ file của skill và runtime block; third-party khi origin github/git hoặc có `provenance.source_id`. Approval bằng CLI và gắn với digest; MCP không trả nội dung chưa được duyệt.
2. Snapshot read-only tách khỏi state directory và secret env. MCP/WebUI không chạy setup/check; doctor chạy trong terminal, cache có `basis: terminal`.
3. Resolver ranking độc lập machine state; setup annotation thêm sau ranking. Routing có examples/counter_examples, technologies/topics, lint và eval gate.
4. Telemetry đo resolve/load/blocked load, daily rollups, funnel và transcript import; WebUI có Usage và Runtime.
5. WebUI đọc trust qua `/review`, runtime qua `/runtime`, usage qua `/usage`. Đây là các contract phải giữ khi thêm Sources/Composer.

Nguồn: [O plan](../../261004-1547-skill-execution-measurement-routing/plan.md), `internal/skillruntime/trust.go`, `internal/app/skill_detail.go`, `internal/app/skill_doctor.go`, `internal/app/skill_runtime_status.go`, `internal/delivery/web/routes_runtime.go`, `web/src/api/queries.ts`.

## Các điểm cần chỉnh

### 1. A: guard khóa baseline trước O — chặn thực thi

`guard/guard.sh` so thay đổi từ `37b61880fc9cf33a0fd9428368adfe4a9f1264b4`, cấm sửa baseline test ngoài hai file MCP ở phase 5, và yêu cầu output CLI giống từng byte. Đối chiếu bằng Git cho thấy **16 baseline test files đã thay đổi** trên main; 15 file nằm ngoài hai file được phép. Đây là thay đổi đã giao của O, không phải vi phạm của executor A tương lai.

B còn cố ý thêm tool/schema MCP, thay đổi source watch/triage và CLI output. Các thay đổi được chấp nhận này tiếp tục xung đột với quy tắc “public contracts frozen” của A.

Điều chỉnh cần có: controller thiết lập guard cho phần công việc còn lại trên baseline sau các thay đổi được chấp nhận; giữ baseline cũ làm lịch sử, ghi rõ delta và phạm vi executor. Không bảo executor sửa guard hoặc làm output quay về trước O/B. Chưa chạy hay thay đổi guard trong lượt rà soát này.

### 2. A↔B: supersede toàn bộ phase 4 làm mất Handoff/Runs — chặn đóng toàn scope

A phase 4 sở hữu Sources, Watch, Distill Handoff, Run Return, run endpoints, route safety và fixtures. B phase 8–9 chỉ thay Sources API/UI, Upstream/Learning; B phase 9 Requirement 5 ghi rõ `/sources/watch`, `/sources/distill`, `/sources/runs/:id` vẫn là `LaterPhasePage`.

Vì vậy B không thay thế toàn bộ A4. A6 vẫn yêu cầu watch-check-handoff-run, all routes và all 13 mockup routes. Guard A chạy phase checks tích lũy, nên `check 5/6` vẫn gọi phase4 và đòi file/test không có.

Điều chỉnh đề xuất: supersede riêng standalone Sources/Watch bằng B; giữ hoặc tái phân công rõ Handoff, Run Return, run API và route safety. Cập nhật acceptance/guard theo màn hình Sources mới. Nếu muốn bỏ Handoff/Runs, đó là quyết định giảm phạm vi cần người dùng chấp nhận, không suy ra từ B.

### 3. A5: dependency phase 4 vẫn còn dù ghi decoupled — chặn thực thi

Task 5.3 gọi `seedFinalizedRun` từ A4; Task 5.6 gọi `seedDistillWorkspace` từ A4. Hai helper chưa có trong `internal/delivery/web/fixtures_test.go` hoặc `web/e2e/support/`. B cũng không giao chúng.

Điều chỉnh cần có: chuyển các fixture này sang task do A5 sở hữu, hoặc đặt một task foundation trước A5. Seed sau B phải gắn learning source với `consumer-review` để phù hợp invariant mới. Phân biệt dependency fixture với dependency màn hình: Inbox có thể không chờ Handoff UI, nhưng phải có seed hợp lệ.

### 4. O→B: frontend provenance type sai shape — cần sửa contract trước B9

Go `app.SkillProvenance` trả object phẳng: `created_by`, `created_at`, `source_id`, `source_locator`, `source_revision`, `upstream_path` (`internal/app/skill_review.go:49`, `extractSkillProvenance`). TypeScript hiện khai báo `source_url`, `source_path`, nested `origin` (`web/src/api/types.ts:265`).

B9 Requirement 1 nói tái sử dụng type do O thêm và chỉ thêm nếu absent. Type đã tồn tại nhưng không đúng, nên kiểm tra sự tồn tại không đủ.

Điều chỉnh cần có: B9 đối chiếu và sửa type theo JSON Go thực tế; `ProvenanceCard` đọc các field phẳng. Giữ Go public contract hiện có; nếu cần ref bổ sung thì lấy từ endpoint upstream. Khi triển khai, fixture cho provenance remote phải pin shape này; fixture local không phát hiện được sai lệch.

### 5. O→B/A5: mutation phải làm mới Runtime/trust và giữ metadata — cần bổ sung integration acceptance

B9 liệt kê invalidation sau confirm: skill, skill-review, skill-sources, skills, home; thiếu `['skill-runtime', id]`. O đã tạo query Runtime riêng (`web/src/api/queries.ts:66`), và doctor cache không được đọc cho skill chưa trusted (`internal/app/skill_doctor.go:217`).

Điều chỉnh cần có: upstream confirm và Composer apply làm mới các read model chịu ảnh hưởng, bao gồm Runtime; source mutations làm mới Sources/Home/list tương ứng. Giữ Usage/Runtime/ContentTrustCard khi thêm tab. B4/B5 và A5 phải giữ `runtime`, routing examples/counter_examples, các quality field và approval digest khi sửa meta/content. Sau content update, approval cũ còn nguyên nhưng trust phải chuyển stale; không chạy doctor/setup hay xóa secret env.

B4 đang dùng `ReviewRequiredAfterApply = ThirdParty && any skill file changes`. Nên suy ra verdict sau write set từ digest hiện tại/kết quả: metadata-only re-pin không làm approval đang hợp lệ mất hiệu lực, nhưng cũng không khiến skill vốn chưa được duyệt trở thành approved. Đây là refinement về tính đúng của thông báo, không thay policy approval.

### 6. A6 và B10: tài liệu/acceptance phải dùng cùng scope mới

A6 yêu cầu mọi flow cũ của spec 04, trong đó có watch-check-handoff-run; B10 đổi spec Sources và ghi dedicated Watch page chưa shipped. Cả hai sửa README, user guide, spec 04 và tài liệu liên quan.

Điều chỉnh cần có: B10 cập nhật source/upstream contracts; A6 kiểm tra toàn bộ shipped WebUI sau B và phần distillation còn giữ lại, bao gồm Usage/Runtime của O. Viết endpoint reference từ code sau tích hợp, không tái áp các line-based replacement cũ làm mất nội dung O/B.

## Điểm tương thích đã xác minh — không cần đảo quyết định

- `origin.files_digest` của B cố ý là files-only; content trust của O là files + runtime. Hai digest có hai nhiệm vụ; không thay thế lẫn nhau. Test B1 chỉ cho chúng bằng nhau khi không có runtime block.
- B không sửa `quality.content_reviewed_digest` khi update; đúng với approval gắn digest của O.
- Learning link không đặt `provenance.source_id`; đúng với trust classification của O và giữ local authored skill là local.
- B dùng operational.db cho upstream state, giữ catalog schema version sau O; resolver/telemetry không cần thiết kế lại.
- B giữ upstream apply ở CLI/WebUI, MCP metadata-only và generic-confirm kind guard; không mở thêm agent approval/apply path.

## Reconcile branch và thứ tự đề xuất

Branch B có hai commit plan riêng và thiếu các commit O cuối cùng đang ở main. Trước triển khai, cần tích hợp hai commit plan B trên nền main sau O, xử lý metadata branch/dependency và xác minh lại file ownership. Không dùng executor note cũ để checkout branch đã tồn tại hoặc `git pull` theo giả định.

Thứ tự đơn giản sau khi sửa các điểm trên:

1. Đưa B plan lên nền source sau O; hòa giải scope A4/B8–9, acceptance và guard.
2. Thực hiện B1–10 theo thứ tự của B.
3. Thực hiện phần Handoff/Runs còn giữ từ A4 và fixture foundation; A5 triển khai Inbox/Insight/Composer.
4. A6 làm hardening/release cuối cùng cho toàn bộ màn hình và contract đã tích hợp.

A5 có thể được làm trước B nếu fixture được tách và guard có baseline riêng cho từng bước; đó không phải thứ tự cần thiết để đạt mục tiêu và sẽ cần một lần hòa giải thêm sau B.

## Giới hạn xác minh

Đã đọc O index/runtime WebUI contract, A index/decisions/phases 4–6/guard, B index và toàn bộ 10 phase; kiểm tra source hiện tại, Git ancestry và danh sách baseline tests thay đổi. Không chạy full guard hoặc test suite: lượt này không đổi source, và blocker của guard được xác minh trực tiếp từ predicate và Git diff. Không sửa A/B, không merge/checkout branch, không thay quyết định người dùng.
