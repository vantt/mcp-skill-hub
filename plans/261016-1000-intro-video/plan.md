---
title: "Video giới thiệu và hướng dẫn sử dụng Skill Hub"
description: "Brief cho AI agent dựng video giới thiệu Skill Hub (skillhub-mcp): công dụng, cài đặt, hướng dẫn từng bước. Dựng bằng code (Remotion, VHS, Playwright) để render lại được sau mỗi release. Dừng xin xác nhận ở ba checkpoint."
status: pending
priority: P3
effort: 3d
branch: main
tags: [promotion, docs, video, demo]
blockedBy: []
blocks: []
created: 2026-10-16
---

# VIDEO BRIEF: Video giới thiệu & hướng dẫn sử dụng Skill Hub

> File này là bản mô tả yêu cầu dành cho AI agent (ví dụ Claude Code) thực hiện.
> Agent đọc toàn bộ file trước khi bắt đầu, làm theo đúng thứ tự các giai đoạn,
> và **dừng lại xin xác nhận** ở mỗi điểm đánh dấu ✋ CHECKPOINT.

Plan này là một phần của [Promote skillhub-mcp](../261014-1700-promote-skillhub-mcp/plan.md): nó sản xuất
"demo asset" của [phase 1](../261014-1700-promote-skillhub-mcp/phase-01-repository-page.md) và video dùng cho
[phase 3 (launch)](../261014-1700-promote-skillhub-mcp/phase-03-launch.md). Mọi ràng buộc của plan đó (không đăng gì
khi chủ project chưa đồng ý, không nói quá, không lộ dữ liệu riêng) áp dụng nguyên vẹn ở đây.

---

## 1. Mục tiêu

Tạo một video giúp người xem:

1. Hiểu Skill Hub **dùng để làm gì** và **giải quyết vấn đề gì**.
2. Biết **cách cài đặt** `skillhub`.
3. Biết **cách sử dụng** các tính năng chính, từng bước một.

**Đối tượng xem** (lấy từ plan quảng bá): người đã dùng agent (Claude Code trước, rồi Codex, Gemini CLI), đang giữ skill
trong `~/.claude/skills` hoặc trong thư mục của từng project, nhất là người dùng nhiều máy hoặc nhiều agent.

**Thông điệp chính** (positioning đã chốt ngày 2026-10-13, giữ đúng thứ tự):

- (b) **Thư viện skill tập trung, của riêng bạn**: gom skill từ bất kỳ repo nào, sửa và tinh chỉnh theo tiêu chí của
  bạn, có phản hồi từ việc dùng thật và bước duyệt trước khi skill tới agent. Tất cả trong một Git repo bạn sở hữu.
- (a) **Phục vụ qua MCP**: không cài, không copy gì vào từng project; agent tự hỏi hub skill phù hợp khi bắt đầu việc.
- Tagline: *"Your skills in one place. Pulled by any agent, on demand."*

Mở đầu bằng (b), dùng (a) để đỡ cho (b).

## 2. Thông tin đầu vào

Các ô đã điền sẵn lấy từ repo; ngôn ngữ, phụ đề, nơi đăng và giọng đọc do chủ project chốt ngày 2026-10-16.
Còn thiếu gì ảnh hưởng nội dung thì agent hỏi trước Giai đoạn 2.

| Mục | Giá trị |
|---|---|
| Repo | `https://github.com/vantt/skillhub-mcp` (local: thư mục gốc của repo này) |
| Tên hiển thị | **Skill Hub** (`skillhub-mcp`). Luôn kèm tên repo vì "skillhub" trùng tên với nhiều project khác. |
| Loại project | CLI viết bằng Go (`skillhub`) + MCP server qua stdio + Web UI local (`skillhub serve web`) + server cho máy khác (`skillhub serve`) |
| Agent đã kiểm chứng | Chỉ **Claude Code** được kiểm chứng end to end. Codex và Gemini CLI: `connect` có ghi file nhưng chưa kiểm chứng. Video phải nói đúng như vậy. |
| Ngôn ngữ lời thoại | **English** cho bản chính (khán giả GitHub/YouTube quốc tế); bản Tiếng Việt là tùy chọn, render từ cùng storyboard. Chốt 2026-10-16. |
| Ngôn ngữ phụ đề | **Cả hai** (`subtitles.en.srt`, `subtitles.vi.srt`), vì README đã có bản `README.vi.md`. Chốt 2026-10-16. |
| Màu thương hiệu | Lấy từ logo trong `docs/images/brand/`. Nền sáng: xanh lá `#3f7a35`, `#4f8545`, `#5f8f54`, `#6f9f62`, chữ `#1c1a15`. Nền tối: `#6f9f62`, `#84b577`, `#9bc78f`, `#b3d6a9`, chữ `#f0ece2`. |
| Logo | `docs/images/brand/skill-hub-logo-{light,dark}.svg` (logo chữ), `docs/images/brand/skill-hub-mark-{light,dark}.svg` (biểu tượng) |
| Ảnh Web UI có sẵn | `docs/images/web/*.png` (chụp từ `make web-shots`); dùng làm tham chiếu hoặc ảnh tĩnh nếu chưa quay kịp |
| Nơi đăng | **Cả hai**: `full.mp4` lên YouTube, `preview.gif` + link YouTube ở đầu README. Chốt 2026-10-16. Không đăng gì trước khi chủ project nói "đăng". |
| Người đọc lời thoại | **Giọng TTS** (edge-tts, xem mục 5). Chốt 2026-10-16. |
| Tính năng bắt buộc | Xem danh sách bên dưới. |

**Tính năng bắt buộc có trong video** (mọi lệnh đã đối chiếu với `docs/command-reference.md` và `README.md`; Giai đoạn 1
vẫn phải chạy lại từng lệnh trên bản release mới nhất):

1. Cài đặt: `curl -fsSL https://github.com/vantt/skillhub-mcp/releases/latest/download/install.sh | sh`, kiểm tra bằng
   `skillhub --version` (nhắc ngắn có lệnh PowerShell cho Windows).
2. Tạo hub trên một máy: `mkdir -p ~/skill-hub && cd ~/skill-hub && skillhub init`.
3. Kết nối mọi project: `skillhub connect -g`, khởi động lại agent.
4. Nói với agent **"curate my Skill Hub"** và thấy agent lấy skill từ hub (cảnh trong Claude Code).
5. Thêm một skill từ GitHub làm draft, duyệt rồi kích hoạt: `skillhub skill add <github-url>`,
   `skillhub skill review <skill-id>`, `skillhub skill activate <skill-id>`; mỗi thay đổi thành một Git commit trong hub.
6. Web UI: `skillhub serve web`, trang danh sách skill với bộ lọc trạng thái (draft / active / deprecated).
7. (Phần nâng cao) Hub trên máy này, agent ở máy khác: `skillhub serve --install-service`, `skillhub token invite`, dán
   dòng `skillhub connect --hub <url> --invite <code>` ở máy kia.
8. (Mẹo) `skillhub status` và `skillhub doctor` khi có gì không chạy.

Không đưa vào video: lệnh dev (`resolve`, `eval`, `telemetry`), distill/learning (để video sau), và bất kỳ con số đo
hiệu năng nào chưa có phương pháp đo được công bố.

## 3. Sản phẩm cần bàn giao

| # | File | Mô tả | Lưu ở đâu khi xong |
|---|---|---|---|
| 1 | `video/output/full.mp4` | Video đầy đủ, 2–5 phút, 1920×1080, 30fps, có lời thoại, phụ đề, nhạc nền nhỏ | Không commit vào Git. Đăng YouTube và/hoặc đính kèm GitHub release. |
| 2 | `video/output/short.mp4` | Bản ~30 giây, chỉ phần công dụng | `docs/images/demo/short.mp4` nếu ≤ 10MB, ngược lại chỉ đăng lên release |
| 3 | `video/output/preview.gif` | GIF 10–15 giây, cảnh demo hấp dẫn nhất. Mục tiêu ≤ 5MB (README tải nhanh), tối đa 10MB | `docs/images/demo/preview.gif`, gắn ở đầu `README.md` và `README.vi.md` |
| 4 | `video/output/subtitles.<lang>.srt` | Phụ đề theo từng ngôn ngữ đã chốt ở mục 2 | Commit trong `video/` (file nhỏ, dùng lại khi upload) |
| 5 | `video/output/youtube.md` | Tiêu đề, mô tả, chapter (timestamp thật), tags, nguồn và giấy phép của nhạc/font | Commit trong `video/` |
| 6 | Toàn bộ mã nguồn dựng video trong `video/`, kèm `video/README.md` | Render lại được khi project cập nhật | Commit |

Chỉ `preview.gif` (và `short.mp4` nếu đủ nhỏ) được copy vào `docs/images/demo/`; mọi file video lớn khác ở ngoài Git.

## 4. Cấu trúc nội dung video

| # | Phần | Thời lượng | Hình thức | Nội dung cụ thể cho Skill Hub |
|---|---|---|---|---|
| 1 | Mở đầu: một câu nêu vấn đề + cảnh kết quả cuối cùng | 5–10s | Motion graphics + trích đoạn demo | Vấn đề: skill rải rác trong từng project, từng máy, copy tay rồi lệch nhau. Kết quả: agent trong một project mới tự lấy đúng skill từ hub. |
| 2 | Công dụng: vấn đề → giải pháp → 2–3 tính năng chính | 20–40s | Motion graphics | Sơ đồ: một Git repo (hub) → MCP → nhiều agent / nhiều project / nhiều máy. Ba ý: thư viện của bạn trong Git; duyệt trước khi tới agent; không cài gì vào từng project. |
| 3 | Cài đặt: yêu cầu hệ thống + lệnh cài | ~30s | Quay terminal (VHS), lệnh hiển thị chữ to | Cần Git và một agent (Claude Code). Lệnh `install.sh`, rồi `skillhub --version`. Một dòng nhắc Windows có lệnh PowerShell trong README. |
| 4 | Hướng dẫn sử dụng, mỗi bước có tiêu đề trên màn hình | phần chính | Quay demo + chú thích, highlight | Bước 1: `skillhub init`. Bước 2: `skillhub connect -g`. Bước 3: "curate my Skill Hub" trong Claude Code. Bước 4: thêm, duyệt, kích hoạt một skill. Bước 5: xem skill trong Web UI. |
| 5 | Nâng cao / mẹo (tùy chọn) | 20–40s | Quay demo | Dùng hub từ máy khác bằng `token invite` (mã dùng một lần, hết hạn sau 10 phút). Mẹo: `skillhub status`, `skillhub doctor`. |
| 6 | Kết: link repo, tài liệu, mời star / góp ý | 5–10s | Motion graphics | `github.com/vantt/skillhub-mcp`, `docs/user-guide.md`, mời mở issue / Discussions. Ghi rõ: "Verified with Claude Code". |

Nguyên tắc nội dung:

- Mọi lệnh, tham số, đường dẫn trong video **phải khớp với code thực tế** của repo ở phiên bản đang quay. Không bịa
  tính năng. Nếu `docs/command-reference.md` và `skillhub help <command>` khác nhau thì `skillhub help` thắng.
- Mỗi bước hướng dẫn chỉ làm một việc. Thao tác chậm, rõ ràng.
- Lời thoại ngắn gọn, câu đơn, dễ hiểu khi nghe một lần.
- Gọi đúng tên: "Skill Hub", "hub", "skill", "agent". Không nói Skill Hub hỗ trợ đầy đủ Codex hay Gemini CLI.

## 5. Công cụ (ưu tiên miễn phí / mã nguồn mở)

| Hạng mục | Công cụ chính | Ghi chú |
|---|---|---|
| Dựng video bằng code | **Remotion** (React) | Miễn phí cho cá nhân / công ty ≤ 3 người; agent kiểm tra lại điều khoản license hiện hành trước khi dùng và ghi vào `video/README.md`. Nếu có Remotion Agent Skills chính thức thì cài và dùng. Project Node riêng trong `video/remotion/`, không dùng chung `package.json` với `web/`. |
| Demo terminal (CLI) | **VHS** (charmbracelet/vhs) | Viết file `.tape`, xuất MP4 (cho Remotion) và GIF. |
| Demo Web UI | **Playwright** + quay video tích hợp | Repo đã có Playwright trong `web/` (`make web-e2e`, `make web-shots`); tham khảo cách khởi động binary thật ở đó, nhưng script quay đặt trong `video/demos/`. |
| Demo trong Claude Code / thao tác thủ công | **OBS Studio** | Cảnh agent "curate my Skill Hub" và cảnh hai máy: agent viết kịch bản thao tác chi tiết, chủ project tự quay. |
| Giọng đọc | **edge-tts** | English: ví dụ `en-US-AndrewNeural`, `en-US-AvaNeural`. Tiếng Việt: `vi-VN-HoaiMyNeural` / `vi-VN-NamMinhNeural`. Dự phòng offline: Kokoro hoặc Piper. edge-tts gọi dịch vụ của Microsoft qua mạng: chỉ gửi văn bản kịch bản, không gửi gì khác. |
| Phụ đề | Tạo trực tiếp từ kịch bản + thời lượng audio từng cảnh; **Whisper** (local) để kiểm tra lại | Kiểm tra khớp thời gian. |
| Nhạc nền | YouTube Audio Library / Pixabay Music | Chủ project tự tải; ghi rõ nguồn và giấy phép trong `youtube.md`. |
| Font | Font mã nguồn mở (ví dụ Inter cho chữ, JetBrains Mono cho terminal), license OFL | Ghi nguồn trong `video/README.md`. |
| Xử lý media | **ffmpeg** | Ghép, nén, xuất GIF (palettegen/paletteuse). |

Ràng buộc:

- Không dùng dịch vụ trả phí nếu chưa được chủ project đồng ý.
- Không dùng tài nguyên (font, nhạc, hình) có bản quyền không rõ ràng.
- Không đưa API key, token, dữ liệu thật vào cảnh quay. Dùng dữ liệu mẫu.

### Môi trường quay an toàn (bắt buộc)

Skill Hub ghi vào thư mục cấu hình của agent và in token, nên mọi cảnh quay phải chạy trong môi trường cô lập:

- `HOME` tạm (ví dụ `HOME=$(mktemp -d)`), hub mẫu mới tạo bằng `skillhub init`, skill mẫu tự đặt tên, repo GitHub mẫu
  công khai cho bước `skill add` (không dùng hub thật của chủ project).
- Prompt terminal gọn, không có tên user, hostname, đường dẫn cá nhân. Ví dụ `PS1='$ '`.
- Địa chỉ IP trong cảnh hai máy: dùng địa chỉ của máy quay thử, sau đó kiểm tra từng frame; mã invite chỉ dùng một lần và
  hết hạn sau 10 phút, nhưng vẫn không để lộ token thật (`token show` không xuất hiện trong video).
- Không quay `~/.claude`, `~/.config` thật, không mở email, không có thông báo hệ thống trên màn hình.

## 6. Cấu trúc thư mục

Thư mục `video/` nằm ở gốc repo, ngang hàng với `web/`:

```
video/
├── README.md            # cách cài phụ thuộc, render lại, quay lại demo
├── script.md            # kịch bản lời thoại + storyboard
├── demos/               # file .tape (VHS), script Playwright, hướng dẫn quay OBS, script dựng HOME tạm
├── recordings/          # video demo đã quay (không commit)
├── audio/               # giọng đọc từng cảnh, nhạc nền (không commit)
├── remotion/            # project Remotion
└── output/              # sản phẩm cuối (chỉ commit .srt và youtube.md)
```

Thêm vào `.gitignore` của repo khi tạo thư mục:

```gitignore
# Video build artifacts
video/recordings/
video/audio/
video/remotion/node_modules/
video/output/*
!video/output/*.srt
!video/output/youtube.md
```

`video/README.md` phải đủ để tạo lại `recordings/` và `audio/` từ đầu (trừ nhạc nền và cảnh OBS, ghi rõ nguồn và cách quay).

## 7. Các giai đoạn thực hiện

### Giai đoạn 1: Phân tích project

- Đọc `README.md`, `README.vi.md`, `docs/README.md`, `docs/user-guide.md`, `docs/command-reference.md`,
  `docs/curating-skills.md`, `docs/web-ui.md`, `docs/remote-hub-guide.md`, và plan quảng bá.
- Xác định: loại project, vấn đề giải quyết, tính năng chính, cách cài đặt, luồng sử dụng cơ bản. Đối chiếu với mục 2;
  báo lại chỗ nào khác.
- Cài bản release mới nhất trong `HOME` tạm và chạy thử **từng lệnh** sẽ xuất hiện trong video. Ghi phiên bản
  (`skillhub --version`) vào `video/script.md`.
- Hỏi chủ project nếu có gì ở mục 2 không còn đúng với repo.

### Giai đoạn 2: Kịch bản & storyboard → `video/script.md`

Với mỗi cảnh ghi rõ: số thứ tự, thời lượng ước tính, lời thoại, hình ảnh trên màn hình, lệnh/thao tác cần quay,
chữ/chú thích hiển thị, và công cụ quay (VHS / Playwright / OBS / Remotion).

✋ **CHECKPOINT 1**: gửi `script.md` cho chủ project duyệt. Chưa duyệt thì không quay, không dựng.

### Giai đoạn 3: Quay demo

- Viết kịch bản demo có thể chạy lại (`.tape`, Playwright) và script dựng `HOME` tạm + hub mẫu dùng chung.
- Terminal: font ≥ 20px, theme tương phản cao, cửa sổ gọn, prompt `$ `.
- Web UI: cửa sổ 1920×1080, zoom trình duyệt đủ để chữ đọc được trên điện thoại; quay cả theme sáng hoặc tối theo
  storyboard.
- Thao tác nào agent không tự quay được (Claude Code, hai máy thật) thì viết hướng dẫn quay chi tiết cho chủ project
  trong `video/demos/`.
- Xem lại từng recording, kiểm tra theo mục "Môi trường quay an toàn".

### Giai đoạn 4: Giọng đọc & phụ đề

- Tạo audio **theo từng cảnh** (mỗi cảnh một file) để dễ khớp hình.
- Đo thời lượng audio thật (ffprobe) và điều chỉnh thời lượng cảnh theo audio.
- Tạo `subtitles.<lang>.srt` cho từng ngôn ngữ đã chốt.

### Giai đoạn 5: Dựng bằng Remotion

- Mỗi cảnh là một component riêng; thời lượng lấy từ độ dài audio.
- Motion graphics: tiêu đề, icon, sơ đồ hub → MCP → agent, chuyển cảnh nhẹ nhàng; màu và logo theo mục 2.
- Phần demo: chèn video quay, thêm tiêu đề bước, khung highlight, mũi tên, zoom vào vùng quan trọng khi cần.
- Phụ đề hiển thị ở dưới, chữ dễ đọc trên mọi nền.
- Nhạc nền âm lượng thấp, giảm thêm khi có lời thoại.

✋ **CHECKPOINT 2**: render bản nháp độ phân giải thấp, gửi chủ project xem và góp ý.

### Giai đoạn 6: Hoàn thiện & xuất

- Sửa theo góp ý, render bản cuối `full.mp4`.
- Tạo `short.mp4`, `preview.gif` (nén bằng ffmpeg theo giới hạn ở mục 3).
- Viết `youtube.md` (tiêu đề, mô tả, chapter theo timestamp thật, tags, nguồn nhạc/font, câu "Verified with Claude Code").
- Viết `video/README.md`: lệnh cài phụ thuộc, lệnh render từng bản, cách quay lại demo khi project đổi.
- Chuẩn bị (chưa commit nếu chủ project chưa đồng ý) thay đổi README: chèn `preview.gif` và link video ở đầu
  `README.md` và `README.vi.md`.

✋ **CHECKPOINT 3**: bàn giao, liệt kê các file và điểm cần chủ project tự làm (quay OBS, tải nhạc, upload YouTube,
gắn vào release). Không upload, không đăng khi chủ project chưa nói "đăng".

## 8. Tiêu chí nghiệm thu

- [ ] Mọi lệnh/thao tác trong video chạy đúng với phiên bản `skillhub` ghi trong `video/script.md`.
- [ ] Người chưa biết project xem xong có thể tự cài và dùng tính năng cơ bản (init, connect, curate).
- [ ] Lời thoại, hình, phụ đề khớp thời gian; không có khoảng lặng hoặc cắt giữa câu.
- [ ] Chữ trên màn hình đọc được rõ khi xem trên điện thoại.
- [ ] Không lộ thông tin nhạy cảm (key, token, đường dẫn cá nhân, hostname, email, hub thật).
- [ ] Video nói đúng phạm vi kiểm chứng: chỉ Claude Code đã kiểm chứng end to end.
- [ ] Toàn bộ tài nguyên có nguồn và giấy phép rõ ràng.
- [ ] Render lại được từ đầu chỉ bằng các lệnh trong `video/README.md`.
- [ ] Không có file video lớn nào bị commit vào Git ngoài những file được phép ở mục 3.

## 9. Khi gặp vướng mắc

- Thiếu thông tin ở mục 2 → hỏi chủ project, không tự đoán những thứ ảnh hưởng nội dung.
- Một lệnh trong README không chạy được → báo lại, đề xuất cách sửa (sửa docs hoặc code là việc riêng, theo quy trình
  của repo), không giấu lỗi trong video.
- Công cụ nào không cài được → đề xuất phương án thay thế miễn phí trước khi đổi.
- Release mới đổi lệnh sau khi đã quay → cập nhật `.tape`/script Playwright, quay lại cảnh đó theo `video/README.md`,
  không sửa video bằng tay.
