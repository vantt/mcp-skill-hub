# Thiết kế lại UX curation: các quyết định

Ghi lại từng quyết định của vòng thảo luận thiết kế, trước khi lập plan. Nguồn đánh giá:
`plans/reports/ux-review-261014-1651-curation-cli-agent.md` và
`plans/reports/ux-review-261014-1651-curation-web.md`.

## D1. Bản release và bản đang sửa (2026-10-14)

**Quyết định:** dùng mô hình checkout ra project. Khi sửa một skill (kể cả skill đã release), cả thư mục skill
được chép vào project; agent sửa mọi file và thử thật ở đó. Hub giữ nguyên bản đang phục vụ cho tới khi lần
checkin được người duyệt, nên các project khác không thấy bản sửa dở. Checkin là một thay đổi có diff đầy đủ,
merge 3 chiều nếu hub đã đổi trong lúc đó. Draft cũng thử được theo cách này.

**Lý do:** sửa trực tiếp hiện nay là live ngay trên máy hub, không tách bản release khỏi bản đang sửa; CLI và
agent không sửa được script/reference; draft không thử được trong agent thật.

**Chưa quyết:** tên lệnh và hình dạng output (D2 trở đi).

## D2. Tên lệnh: `checkout` / `commit` (2026-10-14)

**Quyết định:** `skillhub skill checkout <id>` chép cả skill vào project; `skillhub skill commit <id|folder>` đưa
thay đổi về hub. `commit` là lệnh của skillhub: preview diff, và khi confirm thì chép file vào hub, kiểm tra hợp lệ
rồi tự tạo **một Git commit trong repo của hub**, chỉ gồm file của skill đó, message do skillhub viết, không push.
Người dùng không phải tự `git commit` trong hub cho thay đổi này. `commit` một folder chưa từng checkout là đề xuất
một skill mới. Thư mục checkout trong project được thêm vào `.gitignore` của project.

**Lý do:** quen với người dùng Git, không gợi ý khoá như `checkin`, và khớp nguyên tắc Git là database. Tên chỉ đúng
khi lệnh thật sự tạo commit, vì hiện nay confirm chỉ ghi file và người dùng phải tự commit.

## D3. Mọi thay đổi đã confirm đều tự commit vào hub (2026-10-14)

**Quyết định:** không chỉ `skill commit`, mà mọi thay đổi được confirm (add, create, edit, update từ upstream,
activate/deprecate/archive, nhập từ source) đều tạo một Git commit trong repo của hub, chỉ gồm file của thay đổi đó,
message do skillhub viết, không push. Cờ `--no-commit` giữ cách cũ (chỉ ghi file) cho ai muốn tự gom commit.
Nếu chính các file đó đang bị sửa tay dở thì từ chối và báo rõ. Tôn trọng hook và ký commit của người dùng; nếu ký
cần passphrase mà không có TTY thì file vẫn được ghi và hướng dẫn tự commit. CLI, web và agent hành xử giống nhau.

**Lý do:** hub luôn sạch, lịch sử Git là audit log (revert bằng `git revert`), không mất việc khi chuyển máy qua
Git, web làm trọn một việc mà không cần terminal, và bỏ các câu gợi ý commit đang sai.

## D4. Đẩy skill từ máy khác lên hub (2026-10-14)

**Quyết định:** hỗ trợ `skillhub skill commit` từ máy dùng hub ở xa. Không thêm tool MCP (mỗi tool runtime tốn
token trong `tools/list` của mọi session); thay vào đó là một HTTP endpoint thường trên `skillhub serve`
(cổng 7420), cùng kiểu với `GET /mcp/bundle` hiện có. Máy nào có token của hub là nộp được, không cần công tắc hay
token riêng: ưu tiên dễ dùng trước, bảo mật làm sau. An toàn dựa vào một điều không đổi: bài nộp chỉ vào hàng chờ
trên máy hub, không sửa thẳng skill nào, và chỉ được release khi người trên máy hub duyệt (như nội dung bên thứ
ba). `skillhub status` và web trên máy hub báo bài chờ duyệt. Agent ở máy khác chỉ chạy lệnh CLI.

**Để sau:** quyền nộp theo từng invite (`token invite --can-submit`) nếu hub được chia cho người không tin hết.

## D5. Xác nhận: terminal hỏi y/N, agent dùng `--yes` (2026-10-14)

**Quyết định:** mọi lệnh thay đổi hub in preview (danh sách file, số dòng, routing lint). Khi chạy trong terminal
có người, lệnh hỏi ngay `[y/N]` bên dưới preview, không phải gõ lại lệnh. Khi không có terminal (agent, script),
giữ kiểu hai bước: không `--yes` là xem trước, có `--yes` là áp dụng, để luật hỏi quyền của Claude Code cho `--yes`
vẫn bắt người duyệt. Preview có một cờ dễ nhớ để xem diff đầy đủ (ví dụ `--diff`) và in sẵn dòng gợi ý đó; bỏ
cờ ẩn `--full-diff`. Áp dụng cho mọi lệnh thay đổi hub, không chỉ `commit`.

## D6. Web sửa nhanh, gom thành một commit (2026-10-14)

**Quyết định:** màn hình sửa skill có cây file bên trái (sửa, thêm, đổi tên, xoá, tải lên) và một thanh "N changes
not committed · agents still get the current version" ở dưới cùng với [Discard] [Review →]. Review hiện từng file
(A/M/D, số dòng, diff mở/đóng được), routing lint và commit message sửa được; [Commit to the hub] tạo một commit
(D3). Sau commit web báo ai nhận bản mới khi nào ("this machine now; others after you push"). Thay đổi cần thử
trong agent dùng checkout/commit; web có nút "Checkout to a project" kèm lệnh để chép. Bỏ hai tab tách rời và kiểu
"mỗi file lưu một lần, lưu là live".

## D7. Thêm skill có sẵn (2026-10-14)

**Quyết định:** thêm từ folder là `skill commit <folder>` (D2); lần sau commit lại cùng folder là cập nhật có diff,
không còn báo conflict. `skill add` chỉ nhận URL GitHub (skill có upstream được theo dõi). Preview liệt kê mọi
file, nêu file đã bị loại (bí mật/rác như `.env*`, `*.pem`, `id_*`, `node_modules/`), license hiện ở cả chế độ
text, và hub gợi ý routing (trigger, not_for, ví dụ) từ `description` để chấp nhận hoặc sửa tại chỗ
(`--edit-routing`). Trong terminal, các câu hỏi y/N lần lượt: thêm làm draft, duyệt nội dung (chỉ khi là bên thứ
ba), activate; tất cả thành một commit (D3). Không có terminal (agent): dừng ở draft, agent không bao giờ duyệt nội
dung bên thứ ba.

## D8. Tạo skill mới trong project (2026-10-14)

**Quyết định:** giữ tên `skill create <id>`, nhưng nó tạo folder trong project (`.claude/skills/<id>/SKILL.md`)
thay vì trong hub. Form hỏi 3-4 câu: làm gì, khi nào dùng, khi nào không, một hai ví dụ. Người viết hướng dẫn và
thử trong agent, rồi `skill commit <id>` đi luồng D7. Agent không cần form: tự viết folder rồi `skill commit`.
Sửa thẳng file trong hub không còn là đường chính thức; docs, form và curator skill nói cùng một điều (sửa bằng
checkout, sửa nhanh bằng web). `status` không báo `FAILED` cho index cũ sau khi có người sửa tay.

## D9. Duyệt và release (2026-10-14)

**Quyết định:** web có trang "Waiting for review" liệt kê bài nộp từ máy khác (D4), draft mới và skill đổi nội dung.
Mỗi mục mở màn hình đọc toàn bộ skill: mọi file, diff so với bản đang phục vụ, license, nguồn gốc. Duyệt nội dung
bên thứ ba làm được ngay trên web; nút Approve chỉ bật sau khi người dùng đã mở hết các file thay đổi. Terminal vẫn
duyệt bằng y/N (D5, D7); agent không bao giờ duyệt. Nguy cơ agent điều khiển trình duyệt bấm hộ được chấp nhận lúc
này và xử lý sau cùng phần security. Sửa skill bên thứ ba qua checkout không còn làm skill bị chặn: hub phục vụ bản
cũ tới khi commit, và nội dung mới được duyệt ngay trong bước commit. Mọi commit báo ai nhận bản mới khi nào.

## Còn lại sau vòng thiết kế

- Port Git host (GitHub-only hiện nay, tách `HostProvider`): đã đồng ý hướng, làm như một việc nhỏ riêng.
- Các lỗi P0 nhỏ trong hai báo cáo không phụ thuộc thiết kế mới (lọc `.env`, mất bit thực thi, thông điệp sai, nút
  copy `workspace_diff`, placeholder `<ws>`, toast `OP-…`) có thể sửa trước.
- Plan đang hoãn: findability-lint giữ (phase 1 sửa lỗi review không bao giờ chạy lint), cảnh báo hiện trong preview
  của `commit`; session-mining tiếp tục hoãn.
