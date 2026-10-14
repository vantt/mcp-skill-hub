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

## D4. Đẩy skill từ máy khác lên hub (2026-10-14, hướng đã chọn, còn một chi tiết mở)

**Quyết định:** hỗ trợ `skillhub skill commit` từ máy dùng hub ở xa. Không thêm tool MCP (mỗi tool runtime tốn
token trong `tools/list` của mọi session); thay vào đó là một HTTP endpoint thường trên `skillhub serve`
(cổng 7420), cùng kiểu với `GET /mcp/bundle` hiện có. Endpoint tắt mặc định kể cả khi `serve` đang chạy. Bài nộp
vào hàng chờ trên máy hub, luôn phải được duyệt như nội dung bên thứ ba, không sửa thẳng skill nào; `skillhub
status` trên máy hub báo bài chờ duyệt. Agent ở máy khác chỉ chạy lệnh CLI, không cần tool mới.

**Còn mở:** bật nhận bài nộp bằng một công tắc trên hub, hay bằng token riêng có quyền nộp.

## D5. Xác nhận: terminal hỏi y/N, agent dùng `--yes` (2026-10-14)

**Quyết định:** mọi lệnh thay đổi hub in preview (danh sách file, số dòng, routing lint). Khi chạy trong terminal
có người, lệnh hỏi ngay `[y/N]` bên dưới preview, không phải gõ lại lệnh. Khi không có terminal (agent, script),
giữ kiểu hai bước: không `--yes` là xem trước, có `--yes` là áp dụng, để luật hỏi quyền của Claude Code cho `--yes`
vẫn bắt người duyệt. Preview có một cờ dễ nhớ để xem diff đầy đủ (ví dụ `--diff`) và in sẵn dòng gợi ý đó; bỏ
cờ ẩn `--full-diff`. Áp dụng cho mọi lệnh thay đổi hub, không chỉ `commit`.

## Đề xuất chờ xác nhận

- **Web sửa nhanh:** web sửa được từng file, thêm, đổi tên, xoá, tải lên; các thay đổi gom thành "thay đổi đang
  chờ", một nút xem diff đầy đủ và confirm thành một commit (D3). Thay đổi cần thử trong agent thì dùng
  checkout/commit; web có nút "Checkout to a project" kèm lệnh để chép.
