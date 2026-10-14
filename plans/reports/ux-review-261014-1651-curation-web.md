# Đánh giá UX curation trên web UI (2026-10-14)

Phạm vi: chỉ web UI (`skillhub serve web`), bốn use case curation mà người dùng nêu: thêm skill có sẵn, tạo skill mới, sửa nội dung (kể cả skill nhiều file) trong lúc thử, và duyệt/phát hành. CLI và agent do một evaluator khác phụ trách; ở đây CLI chỉ dùng để so "web có làm được không".

Cách chạy: build từ worktree này (`make web-build`, `go build ./cmd/skillhub`, commit gốc `3a4453a`), hub dùng một lần tạo bằng `skillhub init` trong scratch với `HOME`, `XDG_*` và `GIT_CONFIG_GLOBAL` cô lập, web chạy ở `127.0.0.1:7493` (PID 3108763, đã dừng), trình duyệt là Playwright Chromium từ `web/node_modules`, ở 1280 px và 390 px. Dữ liệu mồi: `pdf-tools` (SKILL.md, 2 reference, 1 script, 1 JSON, 1 file nhị phân) thêm từ thư mục local; `release-notes` từ thư mục trong một repo Git local; một bare repo local; và `pdf` (11 file) thêm qua web từ `https://github.com/anthropics/skills/tree/main/skills/pdf`. Không đụng `/home/vantt/skill-hub`, HOME thật hay `~/.claude`.

Ảnh chụp nằm trong `/tmp/claude-1000/-home-vantt-projects-mcp-skill-hub/51079e60-96e5-4ef1-9f06-441ce4c05e29/scratchpad/webux/shots/` (kèm file `.txt` cùng tên chứa chữ hiển thị). Trong báo cáo ảnh được gọi bằng tên file.

## Kết luận

Web UI hiện tại tốt cho việc **xem và chỉnh từng file có sẵn** của một skill, nhưng chưa phải nơi để **curate cả một skill**. Câu hỏi "giờ sửa nhiều file đã ổn chưa?" có câu trả lời là **đỡ hơn nhiều nhưng chưa ổn**. So với trước (chỉ sửa được `SKILL.md`, file khác không đọc được), giờ mọi file văn bản đi kèm đều mở, sửa, xem trước diff và lưu được qua cùng một vòng preview → confirm, và vòng đó chạy đúng (4 cú nhấp, ảnh `10`–`13`). Tuy vậy web **không tạo, đổi tên, xoá hay tải lên** được file nào, vì đó là non-goal đã ghi trong plan `archive/plans/261011-1305-skill-resource-file-editor/plan.md` ("Non-goals: creating, renaming or deleting files"). `SKILL.md` và các file còn lại nằm ở hai tab khác nhau, không có cây thư mục, và mỗi lần lưu chỉ được một file. Vì vậy một thay đổi "thêm `references/ocr.md` rồi trỏ tới nó từ `SKILL.md`" không làm được trên web. Ngoài ra web chỉ nhận URL GitHub công khai, nên không thêm được skill từ thư mục local hay repo Git local. Phê duyệt nội dung bên thứ ba vẫn chỉ làm được trong terminal (đây là quyết định có chủ đích). Web cũng không có cách nào để thử bản nháp trong agent thật, và không cho thấy người dùng khác sẽ nhận được gì. Tóm lại, web làm tốt bước "sửa và xem trước", còn các bước "thu thập", "thử" và "phát hành" vẫn phải sang CLI.

## Use case 1: Thêm skill có sẵn (Git repo hoặc thư mục local)

**Đường đi trên web** (ảnh `50-add-form`, `53-add-github-review`, `54-add-github-done`): Skills → **Add from GitHub** → dán URL → **Discover skills** (khoảng 4,9 giây với repo thật) → xem Identity/Origin/Trust/Conflicts/Resources → **Confirm import**. Tổng cộng 3 màn hình và 4 cú nhấp. Phần này chạy tốt: thấy rõ license, các file sẽ được ghi và revision.

**Những gì làm được:** URL GitHub trỏ tới repo, thư mục hoặc file; có cảnh báo license; có danh sách file sẽ ghi; skill vào ở trạng thái draft; upstream được theo dõi ngay (tab Sources hiện `anthropics/skills`, ảnh `62-pdf-sources`).

**Điểm đau, xếp theo mức độ:**

1. **Cao: không thêm được từ thư mục local hay repo Git local.** Nhập đường dẫn `/tmp/.../pdf-tools` hoặc `file:///.../release-notes.git` đều bị chặn ngay trên trình duyệt với câu "The web UI accepts only public GitHub URLs." (ảnh `51-add-local-path-refused`, `52-add-bare-repo-refused`; `web/src/screens/skill-add/AddSkillScreen.tsx:83-95`). Thông báo không chỉ lệnh thay thế (`skillhub skill add <folder>`), nên người dùng rơi vào ngõ cụt. Repo GitHub riêng tư, GitLab và Git server nội bộ cũng bị chặn theo cùng cách. Với một team, nguồn skill thường chính là các repo nội bộ này.
2. **Trung bình: lộ khái niệm nội bộ.** Ô Summary ghi "Next: review proposal PROP-4a42e13c129178e6eb28 before confirmation." (`internal/app/skill_add.go:803`, ảnh `53`). Phần Technical details hiện `proposal_id`, `proposal_digest`, `base_version` (`AddSkillScreen.tsx:53-55`). Người dùng web không cần những thứ này vì nút Confirm đã giữ sẵn các pin.
3. **Trung bình: sau khi Confirm, web quay về danh sách Skills** (`AddSkillScreen.tsx:152`, ảnh `54`) thay vì mở skill vừa thêm. Danh sách cũng không báo skill này còn cần duyệt nội dung (chỉ ghi "Draft / Not routed"). Bước tiếp theo bắt buộc (Review → approve trong terminal → điền routing → Activate) không được dẫn dắt.
4. **Thấp: nhãn Collection mặc định là `core`.** Danh sách collection được viết cứng trong code (`CreateSkillScreen.tsx:30`, `['core','default','engineering','operations','docs']`), không lấy từ hub. Trong hub thử, skill thêm bằng CLI vào `default`, còn skill thêm bằng web vào `core`, nên chỉ sau hai thao tác hub đã có hai collection.

**So với CLI:** CLI thêm được từ thư mục local (`skillhub skill add <dir> --yes`, chạy được). Tuy nhiên CLI **cũng không** nhận bare repo local hay `file://`: lệnh báo "allowed protocol is https" hoặc đi tìm thư mục con trong bare repo. Ghi chú thêm cho evaluator CLI: sau `skill add` từ local, `scripts/extract.sh` mất bit thực thi (`-rw-r--r--`), và bản xem trước của `release-notes` ghi "1 file, 200 B" trong khi thực tế chép 2 file.

**Luồng lý tưởng:** một nút **Add skill** với ba nguồn: URL Git (bất kỳ host nào, kể cả SSH hoặc private khi máy có credential), thư mục trên máy chạy hub (ô đường dẫn hoặc trình chọn thư mục phía server, vì web chạy local), và tải lên `.zip`/thư mục. Sau Confirm thì mở thẳng skill mới, ở tab "Việc cần làm" liệt kê duyệt nội dung, routing và activate.

## Use case 2: Tạo skill mới

**Đường đi** (ảnh `40-create-form`, `42-create-preview`, `43-create-done`): Skills → **Create skill** → điền ID, Display name, Description (cộng routing nếu muốn) → Instructions (Default scaffold / Upload Markdown / Write) → **Preview draft** → **Create skill**. Mất 1 màn hình, 1 hộp thoại và khoảng 3 cú nhấp ngoài phần gõ. Hộp thoại xem trước nói rõ file nào được tạo.

**Điểm đau:**

1. **Cao: skill tạo trên web chỉ có `SKILL.md`, và sau đó cũng không thêm file nào được.** Form chỉ có một ô Instructions. Tab Resources của skill mới là trạng thái rỗng với câu "Scripts, reference documents or other files you put in the skill folder are listed here." Nghĩa là người dùng phải tự chép file vào thư mục hub ngoài web, rồi chạy `skillhub maintain rebuild` (xem use case 3). Một skill có `references/` hay `scripts/` không thể tạo hoàn toàn trên web.
2. **Trung bình: Skill ID không được gợi ý từ tên** (ảnh `42`: điền tên trước thì báo "Skill ID is required"), trong khi CLI `skill create` có gợi ý ID.
3. **Trung bình: thiếu ô Examples / Counter-examples** ở cả Create lẫn Editor (`EditorTab.tsx` chỉ có name, description, content, operations, triggers, notFor, minScope). Trong khi đó `docs/curating-skills.md` khuyên ít nhất 3 example, và `skillhub eval routing` đo dựa trên example. Trên web không có cách nào để làm skill được chọn tốt hơn bằng ví dụ.
4. **Thấp:** nhãn "Not for / Rationale" ở form và "Operations & Rationale" trong Activation readiness (ảnh `04-pdf-review`) gọi cùng một yêu cầu bằng hai tên khác nhau. "Files: 2 files" (ảnh `43`) đếm cả `.meta/skill.yaml`, trong khi người dùng chỉ thấy một file.

**So với CLI:** ngang nhau về trường dữ liệu, trừ `--example`/`--counter-example` (chỉ CLI có) và `--content-file` (web có Upload Markdown, tương đương).

**Luồng lý tưởng:** tạo skill ra một "thư mục nháp" có sẵn cây file (`SKILL.md`, nút "+ File", "+ Folder", kéo-thả tải lên), ô Examples, ID gợi ý từ tên, và collection lấy từ hub.

## Use case 3: Sửa nội dung khi đang thử (skill nhiều file)

**Đường đi sửa một file đi kèm** (ảnh `04-pdf-resources`, `10-resource-editor-open`, `11-resource-editor-dirty`, `12-resource-preview-dialog`, `13-resource-saved`): tab **Resources** → **Edit** trên dòng file → sửa (Markdown có preview song song) → **Preview changes** → **Save changes**. Mất 4 cú nhấp. Nháp được giữ trong trình duyệt. Có xử lý xung đột (kế thừa từ editor `SKILL.md`). File nhị phân chỉ có View (ảnh `14-resource-binary-view`). Skill bên thứ ba có cảnh báo "agents get no content until you approve it again" ngay trong hộp thoại (ảnh `63-pdf-thirdparty-edit-preview`). Phần này **chạy tốt và an toàn**.

**Đường đi sửa `SKILL.md` + routing** (ảnh `20`–`24`): Review → **Go to field** → con trỏ vào đúng ô → sửa → **Preview changes**. Hộp thoại mô tả bằng lời từng thay đổi (ảnh `22-editor-preview`: "Triggers: added …", "Min scope: empty becomes single_step") → **Save changes**. Đây là màn hình tốt nhất của web.

**Bảng "thấy / thêm / đổi tên / xoá / sửa"** cho skill nhiều file:

| Thao tác | Web | Bằng chứng |
|---|---|---|
| Thấy mọi file | Có, nhưng ở hai nơi (`SKILL.md` trong Editor, phần còn lại trong Resources); dạng bảng phẳng với đường dẫn đầy đủ `skills/default/pdf-tools/...`, không có cây thư mục | ảnh `04-pdf-resources`, `ResourcesTab.tsx:94-96` |
| Sửa file văn bản | Có, từng file một, tối đa 1 MiB | ảnh `10`–`13` |
| Thêm file / thư mục | **Không** | không có route; plan resource editor ghi non-goal |
| Đổi tên / di chuyển | **Không** | như trên |
| Xoá | **Không** | như trên |
| Tải lên / thay file nhị phân | **Không**; ảnh PNG cũng không có preview | ảnh `14` |
| Sửa runtime (`requires`) | **Không**; chỉ hiện lệnh `skillhub skill edit pdf-tools --runtime-file <yaml>` | ảnh `04-pdf-runtime` |
| Một thay đổi đụng nhiều file cùng lúc | **Không**; mỗi proposal chỉ một file | `POST /api/v1/skills/{id}/resources/preview` nhận một `path` |

**Điểm đau:**

1. **Cao: không thêm, đổi tên hay xoá file.** Mọi thay đổi cấu trúc phải làm ngoài web. Tôi đã thử thêm `references/ocr.md` và đổi `forms.md` → `form-filling.md` trực tiếp trên đĩa (ảnh `30-resources-after-disk-change`). Web có hiện file mới, nhưng kèm một banner đỏ "Search index needs rebuild. Catalog is stale. Run skillhub maintain rebuild, then reload." và dòng "Rebuild catalog with `skillhub maintain rebuild` to publish 1 modified canonical file(s)" (`internal/app/skill_review.go:613`). Web **không có nút Rebuild**, nên người dùng buộc phải sang terminal. Câu chữ cũng sai số lượng (một file thêm và một file đổi tên bị gọi là "1 modified") và dùng thuật ngữ "canonical".
2. **Cao: sửa một skill đang active thì có hiệu lực ngay với agent.** Không có "bản nháp của skill đang chạy" (plan resource editor: "an active skill serves the new file at once"). Vì vậy "sửa trong lúc đang thử" trên một skill active đồng nghĩa với sửa thẳng lên bản đang chạy. Lựa chọn còn lại là Deprecate rồi tạo skill mới, nhưng như vậy là sai mô hình.
3. **Cao: không có cách thử bản nháp trong agent thật.** Tab Runtime ghi "Agents receive this skill only while it is active." (ảnh `04-pdf-runtime`). Resolver không bao giờ đề xuất draft. Đọc code thì `skill_get` vẫn trả nội dung của một draft đáng tin nếu agent gọi đúng ID (`internal/delivery/mcpserver/skill_tools.go:177-193`), nhưng web không nói điều này, không có nút "Copy prompt để thử", và cũng không có "chỉ kích hoạt cho project này".
4. **Trung bình: cột State của mọi file luôn là "Active"**, kể cả khi skill là Draft (`ResourcesTab.tsx:20,191`; ảnh `04-pdf-resources`). Đây là chữ viết cứng nên dễ khiến người dùng tưởng file đã được phát hành. Cột Digest (`sha256:…`) cũng không giúp ích gì cho người dùng.
5. **Trung bình: diff bị gập mặc định.** Hộp thoại sửa file chỉ ghi "Edited skills/default/pdf-tools/references/tables.md"; muốn thấy dòng nào đổi phải mở "Exact file changes" (ảnh `12`). Với file đi kèm, khác với `SKILL.md` không có tóm tắt theo trường, nên diff là thông tin duy nhất có ý nghĩa và cần được mở sẵn.
6. **Trung bình: chuyển giữa các file tốn nhiều bước.** Mỗi file phải qua "Back to resources" rồi chọn file khác, vì không có danh sách file bên cạnh editor. Riêng `SKILL.md` lại phải sang tab khác.
7. **Thấp: banner vàng "Changed resource: …" lúc nào cũng hiện**, kể cả khi không có gì đổi (`ResourcesTab.tsx:18,225`; ảnh `04-pdf-resources`), nên người dùng dần quen bỏ qua cảnh báo.

**Mô hình "check out về project, sửa và thử ở đó, check in lại":** web hiện **không hỗ trợ** mô hình này. Không có nút "Check out" (chép skill sang `.claude/skills/<id>` của một project để agent dùng như skill local), cũng không có "Check in" (so thư mục đó với bản trong hub, rồi đưa thay đổi về bằng một proposal nhiều file gồm thêm, sửa, xoá, đổi tên). Phần nền đã có sẵn: proposal `mutation.Change{Path, BeforeDigest, Contents}` nhiều file, khả năng nhận diện xung đột theo digest, và bộ hiển thị diff cùng `ConflictDrawer`. Phần còn thiếu là (a) một route nhận cả cây file và (b) cách để web biết các project đang kết nối (`skillhub connect` đã ghi đường dẫn hub). Nhìn từ góc UX, mô hình này giải quyết cùng lúc điểm đau 1, 2 và 3: người dùng thử trong agent thật với bản local, còn hub chỉ thay đổi khi check in, và check in có preview cho cả cây.

**Luồng lý tưởng:** tab **Files** thay cho Editor + Resources, gồm cây thư mục bên trái (có `SKILL.md` ở đầu, `.meta` ẩn), editor bên phải, nút + / đổi tên / xoá / tải lên, và một **thay đổi gom nhiều file** được xem trước một lần ("3 file sửa, 1 thêm, 1 đổi tên") rồi lưu một lần. Kèm theo đó là nút **Try in a project** (check out) với hướng dẫn quay lại (check in).

## Use case 4: Duyệt (trust) và phát hành (draft → active)

**Đường đi activate** (ảnh `25-review-after-routing`, `26-activate-confirm`, `27-activated`): sau khi đủ routing, nút **Activate skill** sáng lên → hộp thoại "Activate pdf-tools? draft → active … Agents will be able to find and use this skill" → **Activate skill**. Chỉ cần 2 cú nhấp, rõ ràng.

**Đường đi duyệt nội dung bên thứ ba** (ảnh `60-pdf-review-trust`): thẻ **Content trust** hiện "Review required", digest và lệnh `skillhub skill edit pdf --approve-content sha256:…` có nút Copy. Web cố ý không có nút Approve (`docs/web-ui.md`: "content approval … happens only in your terminal").

**Điểm đau:**

1. **Cao: không có màn hình "đọc toàn bộ skill để duyệt".** Thẻ trust ghi "Never approved: review the full skill." nhưng không có nút dẫn tới đâu. Với skill `pdf` có 11 file (ảnh `61-pdf-resources`), người dùng phải mở từng file một, không có chế độ đọc liền mạch hay "đã đọc file này". Khi duyệt lại sau một lần cập nhật cũng không có diff "so với bản đã duyệt" trên web, vì CLI chỉ in ra một lệnh `git diff`. Đây là chỗ web *có thể* làm tốt hơn terminal mà vẫn giữ việc approve ở CLI.
2. **Cao: không thấy "người khác sẽ nhận gì".** Sau khi activate, các agent nối với hub này nhận bản mới **ngay** (catalog được rebuild khi confirm), dù chưa commit. Với hub dùng chung qua Git, đồng nghiệp chỉ nhận khi đã commit, push rồi họ pull. Web không phân biệt hai mức này. Thẻ "In Git vs what agents see" (ảnh `25`) chỉ so trạng thái lifecycle (`draft`/`active`), không so nội dung. Thẻ Git chỉ có `git status`.
3. **Trung bình: câu hướng dẫn commit có placeholder chưa thay.** Sau khi activate, header ghi "Stage the skill folder with `git -C <ws> add <skill folder>`, then commit it with `git -C <ws> commit`." (`internal/app/skill_review.go:619`; ảnh `27-activated`, `71-m-resources`). Người dùng không chép dùng được câu này, trong khi `skillhub status` đã in sẵn lệnh Git đúng với đường dẫn thật.
4. **Trung bình: Home "Copy command" chép `workspace_diff`.** Đó là tên tool MCP, không phải lệnh shell (`internal/app/curation_home.go:522,545`; CLI có ánh xạ ở `internal/delivery/cli/next_commands.go:131` nhưng web thì không; ảnh `01-home`). Lỗi này lọt qua guard "no internal id (snake_case)" của đợt đánh giá trước, vì guard không kiểm tra nội dung khối lệnh.
5. **Trung bình: toast sau activate là "Transitioned · OP-8603b4…"** (`web/src/screens/skill-detail/SkillDetailScreen.tsx:180`; ảnh `27`). Đây là đúng loại lỗi D18 mà đợt trước đã sửa cho editor, nhưng vẫn còn ở luồng chuyển trạng thái.
6. **Thấp: "Provenance: Created in this workspace"** cho một skill thêm từ thư mục local (`ProvenanceCard.tsx:6`, ảnh `03-pdf-overview`). Thông tin này sai nguồn gốc.
7. **Thấp: lệch tài liệu.** `docs/web-ui.md` nói archive chỉ làm được trong terminal, nhưng web đã có nút **Archive** cho skill deprecated (`SkillDetailScreen.tsx:308-314`).

**So với CLI:** approve thì chỉ CLI làm được (cố ý). Activate, deprecate và archive thì cả hai đều làm được. Commit và push thì cả hai đều không làm (cố ý; CLI in ra lệnh Git đúng, web in ra lệnh có placeholder).

**Luồng lý tưởng:** nút **Review content** mở màn hình đọc toàn bộ file liền nhau (hoặc diff so với bản đã duyệt), cuối trang là lệnh approve có nút copy. Sau activate có một thẻ "Ai nhận bản này": agent trên máy này nhận ngay; đồng nghiệp nhận sau commit + push, kèm lệnh Git thật để chép.

## Màn hình hẹp (390 px)

Không có cuộn ngang (scrollWidth = 390) và bảng Resources xếp thành thẻ (ảnh `70-m-skills`, `71-m-resources`, `72-m-resource-edit`, `73-m-editor`, `74-m-trust`). Đợt sửa trước đã giữ được phần này. Riêng đường dẫn file đầy đủ bị ngắt dòng giữa chừng (`skills/default/pdf-` / `tools/...`), nên trên điện thoại tên file khó đọc.

## Trạng thái các phát hiện của đợt đánh giá trước

Nguồn: `docs/design/web-ux-scorecard.md` và `archive/plans/261010-1454-web-curation-ux-evaluation/plan.md`.

| Id | Nội dung | Trạng thái hôm nay | Bằng chứng |
|---|---|---|---|
| D1, D4, D5, D28–D30, D32 | Inbox, runs, `curation_run_start`, tên category cũ | Đã sửa, không thấy tái phát | ảnh `01-home` |
| D2 | Lệnh bị cắt | Đã sửa: lệnh approve xuống dòng trọn vẹn | ảnh `60`, `74-m-trust` |
| D3, D8, D11 | Home lặp số, filter không nhãn, next action vô nghĩa | Đã sửa phần bố cục. **D11 tái phát dạng khác**: next action hiện `workspace_diff` | ảnh `01-home`, `curation_home.go:522` |
| D6, D7, D9, D10, D22, D31, D34, D36 | Lỗi bố cục mobile và CSS | Đã sửa | ảnh `70`–`74` |
| D12 | Nói chuyện CLI cạnh nút Activate | Đã sửa cho draft. **Một phần tái phát**: skill active hiện câu Git có placeholder `<ws>` | ảnh `27`, `skill_review.go:619` |
| D13, D16 | Hộp thoại chuyển trạng thái | Đã sửa | ảnh `26` |
| D14 | Thuật ngữ nội bộ ở Review | Phần lớn đã sửa; còn "canonical" trong thông báo rebuild | ảnh `30` |
| D17, D19, D20, D21, D23, D24, D27 | Preview, conflict, gợi ý trường, Go to field | Đã sửa, xác nhận bằng tay | ảnh `20`, `22` |
| D18 | Toast có operation id thô | **Chưa sửa cho chuyển trạng thái** ("Transitioned · OP-…") | ảnh `27`, `SkillDetailScreen.tsx:180` |
| D25 | Add from GitHub | Đã sửa phần Discover; bước Review vẫn lộ `PROP-…` trong Summary | ảnh `53` |
| D26, D33, D35 | Empty state, token màu, Distill | Đã sửa (không kiểm lại sâu) | |
| Walk-through 2026-10-11 "Only SKILL.md could be edited" | Resources chỉ đọc | **Đã sửa phần sửa file có sẵn; chưa có thêm, đổi tên, xoá** | ảnh `10`–`14` |

Ghi chú về guard: `web/e2e/ux/guards.spec.ts` khẳng định không có id snake_case trên mọi flow, nhưng fixture của nó không có trạng thái "Git có thay đổi chưa commit". Vì vậy lỗi `workspace_diff` không bị bắt.

## Danh sách cải tiến theo ưu tiên

| Ưu tiên | Việc | Lý do | Công sức |
|---|---|---|---|
| P0 | Ánh xạ `workspace_diff` sang lệnh thật trên Home (dùng lại bảng của `next_commands.go` ở tầng app) và thêm fixture "git dirty" vào guard | Nút chính của Home đang chép một chuỗi vô nghĩa | 0,5 ngày |
| P0 | Thay placeholder `<ws>`/`<skill folder>` bằng đường dẫn thật (như `skillhub status` đã làm) | Câu hướng dẫn phát hành không dùng được | 0,25 ngày |
| P0 | Toast chuyển trạng thái viết bằng lời ("pdf-tools is now active.") | Sót lại từ D18 | 0,1 ngày |
| P1 | Thêm, đổi tên, xoá, tải lên file trong skill (route preview nhiều file dùng `mutation.Change`; cùng luật path/symlink/1 MiB như resource editor) | Điều kiện để "sửa skill nhiều file" thực sự ổn | 2–3 ngày |
| P1 | Gộp Editor + Resources thành tab **Files** có cây thư mục, editor cạnh danh sách, và một proposal gom nhiều file | Bỏ việc nhảy qua lại giữa các tab; cho phép một thay đổi logic đụng nhiều file | 2–3 ngày |
| P1 | Nút **Rebuild index** trên banner "Index stale" (một proposal không đổi file) | Hiện là ngõ cụt buộc mở terminal | 0,5 ngày |
| P1 | Add skill từ thư mục local và từ URL Git không phải GitHub (ít nhất là đường dẫn trên máy chạy hub) | Hai nguồn phổ biến nhất của team hiện là ngõ cụt; tối thiểu phải chỉ lệnh CLI trong thông báo lỗi | 1–2 ngày (hoặc 0,1 ngày nếu chỉ sửa thông báo) |
| P1 | Màn hình **Review content** đọc toàn bộ skill, có diff so với bản đã duyệt; approve vẫn ở CLI | Duyệt bên thứ ba hiện phải mở từng file | 1,5 ngày |
| P1 | Hướng dẫn **thử bản nháp trong agent** (prompt "dùng skill_get <id>" có nút copy) | Không có đường thử nào được nêu | 0,25 ngày; làm thật ("chỉ bật cho project X") cần thiết kế |
| P2 | Check out / check in với project đã kết nối | Mô hình người dùng đang cân nhắc; giải quyết cùng lúc chuyện thử, sửa nhiều file và tách bản đang chạy | 4–6 ngày, cần quyết định thiết kế trước |
| P2 | Thẻ "Ai nhận bản này" (máy này ngay; đồng nghiệp sau commit + push) | Làm rõ "phát hành" với hub dùng chung | 0,5–1 ngày |
| P2 | Ô Examples / Counter-examples ở Create và Editor | Chất lượng routing chỉ chỉnh được trong CLI | 0,5 ngày |
| P2 | Dọn bảng Resources: bỏ cột State "Active" viết cứng và cột Digest, đường dẫn tính từ thư mục skill, banner chỉ hiện khi có vấn đề, diff mở sẵn khi sửa file đi kèm | Giảm nhiễu và tránh hiểu sai | 0,5 ngày |
| P2 | Sau Add: mở skill mới; ẩn `PROP-…` khỏi Summary; collection lấy từ hub; ID gợi ý từ tên; Provenance đúng nguồn local; sửa `docs/web-ui.md` về Archive | Các lỗi nhỏ cộng dồn | 1 ngày |

## Câu hỏi mở cho người dùng

1. **Sửa skill đang active:** muốn giữ hành vi "lưu là agent nhận ngay", hay muốn có bản nháp riêng cho skill active (sửa xong mới "publish revision")? Câu trả lời quyết định tab Files nên lưu thẳng hay gom thành một bản sửa chờ phát hành.
2. **Check out / check in:** check out chép skill vào `.claude/skills/<id>` của project (agent coi là skill local), hay giữ trong hub nhưng "chỉ bật cho project X"? Cách đầu đơn giản và hợp mô hình Git-native, nhưng bỏ qua resolver. Cách sau thử được cả routing.
3. **Phát hành cho đồng nghiệp:** web vẫn giữ nguyên tắc "không bao giờ commit/push", hay chấp nhận một nút "Commit các thay đổi của skill này" có preview, giới hạn ở thư mục skill đó?
4. **Nguồn thêm skill trên web:** chỉ cần thư mục local trên máy chạy hub, hay cần cả Git nội bộ (SSH, GitLab, repo GitHub riêng tư)? Với hub chạy từ xa (`docs/remote-hub-guide.md`), "thư mục local" là thư mục của máy hub chứ không phải máy người dùng.
5. **Duyệt nội dung:** giữ approve chỉ trong CLI và chỉ làm màn hình đọc trên web, hay chấp nhận approve trên web khi web chạy ở `127.0.0.1`?

## Ảnh chụp (không commit)

Thư mục: `/tmp/claude-1000/-home-vantt-projects-mcp-skill-hub/51079e60-96e5-4ef1-9f06-441ce4c05e29/scratchpad/webux/shots/`

`01-home.png`, `02-skills-list.png`, `03-pdf-overview.png`, `04-pdf-review.png`, `04-pdf-editor.png`, `04-pdf-resources.png`, `04-pdf-runtime.png`, `04-pdf-sources.png`, `10-resource-editor-open.png`, `11-resource-editor-dirty.png`, `12-resource-preview-dialog.png`, `13-resource-saved.png`, `14-resource-binary-view.png`, `20-editor-goto-field.png`, `21-editor-filled.png`, `22-editor-preview.png`, `23-editor-preview-diff.png`, `24-editor-saved.png`, `25-review-after-routing.png`, `26-activate-confirm.png`, `27-activated.png`, `30-resources-after-disk-change.png`, `32-review-after-disk-change.png`, `33-home-after-disk-change.png`, `40-create-form.png`, `41-create-after-name.png`, `42-create-preview.png`, `43-create-done.png`, `50-add-form.png`, `51-add-local-path-refused.png`, `52-add-bare-repo-refused.png`, `53-add-github-review.png`, `54-add-github-done.png`, `60-pdf-review-trust.png`, `61-pdf-resources.png`, `62-pdf-sources.png`, `63-pdf-thirdparty-edit-preview.png`, `70-m-skills.png`, `71-m-resources.png`, `72-m-resource-edit.png`, `73-m-editor.png`, `74-m-trust.png`.

Kịch bản Playwright dùng lại được nằm cùng cấp, trong `.../scratchpad/webux/` (`drive.mjs`, `s1-tour.mjs` … `s8-mobile.mjs`).
