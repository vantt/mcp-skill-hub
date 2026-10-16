<h1 align="center"><picture><source media="(prefers-color-scheme: dark)" srcset="docs/images/brand/skill-hub-logo-dark.svg"><img src="docs/images/brand/skill-hub-logo-light.svg" alt="Skill Hub" width="280"></picture></h1>

<p align="center">Skill của bạn ở một nơi. Mọi agent kéo về khi cần.</p>

<p align="center"><a href="https://github.com/vantt/skillhub-mcp/releases/latest"><img src="https://img.shields.io/github/v/release/vantt/skillhub-mcp" alt="Latest release"></a> <a href="https://github.com/vantt/skillhub-mcp/actions/workflows/ci.yml"><img src="https://github.com/vantt/skillhub-mcp/actions/workflows/ci.yml/badge.svg" alt="CI"></a> <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-green" alt="License: MIT"></a></p>

<p align="center"><a href="README.md">English</a> · <a href="#bắt-đầu-nhanh">Bắt đầu nhanh</a> · <a href="docs/user-guide.md">Dùng Skill Hub</a> · <a href="docs/curating-skills.md">Quản lý skill</a></p>

Skill Hub phục vụ các skill của agent qua MCP từ một kho Git của riêng bạn, nên không có gì phải cài hay sao chép vào từng dự án: agent hỏi hub skill phù hợp khi bắt đầu một việc.
Vì thư viện nằm một chỗ, bạn có thể thu thập skill từ bất kỳ kho nào, chỉnh sửa và tinh chỉnh theo tiêu chí của riêng bạn, kèm phản hồi từ việc dùng thật và một bước duyệt trước khi bất cứ thứ gì đến tay agent.

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="docs/images/web/dark-skills-list.png">
  <img src="docs/images/web/skills-list.png" alt="Danh sách skill với bộ lọc trạng thái và các skill ở trạng thái nháp, đang dùng hoặc không còn dùng." width="800">
</picture>

*Skills: mọi skill kèm trạng thái, và bộ lọc để tìm.*

## Bắt đầu nhanh

Từ con số không đến một agent nói chuyện được với hub của riêng bạn, trên một máy, trong khoảng hai phút. Bạn cần Git và một agent như Claude Code.

### 1. Cài đặt

Bước này đưa lệnh `skillhub` vào máy của bạn.

```bash
curl -fsSL https://github.com/vantt/skillhub-mcp/releases/latest/download/install.sh | sh
```

<details>
<summary>Windows (PowerShell)</summary>

```powershell
powershell -ExecutionPolicy ByPass -c "irm https://github.com/vantt/skillhub-mcp/releases/latest/download/install.ps1 | iex"
```

PowerShell 5.1 không có `&&`: hãy viết mỗi lệnh dưới đây trên một dòng riêng, hoặc nối bằng `;`.

</details>

Mở một terminal mới để nó tìm thấy lệnh, rồi kiểm tra:

```bash
skillhub --version
```

Dòng đầu tiên cho biết phiên bản, ví dụ 0.7.1.

### 2. Tạo hub

Hub là một thư mục, nằm trong kho Git riêng của nó, chứa các skill của bạn. Git lưu mọi thay đổi kèm tên và email của bạn. Nếu `git config user.email` không in ra gì, hãy chạy:

```bash
git config --global user.name "Your Name"
git config --global user.email "you@example.com"
```

Rồi tạo hub:

```bash
mkdir -p ~/skill-hub && cd ~/skill-hub && skillhub init
```

```text
Hub created at /home/you/skill-hub.
...
This is now the hub of this machine; `skillhub connect` uses it.
```

`init` có thể gợi ý kết nối một dự án; hãy dùng bước 3 thay vào đó, bước này kết nối tất cả dự án.

### 3. Kết nối agent

Bước này cho các agent trên máy biết hub của bạn ở đâu, trong mọi dự án bạn mở.

```bash
skillhub connect -g
```

```text
Connected your user account (all projects) to the hub at
/home/you/skill-hub.
...
Next: restart your agent and ask it: "curate my Skill Hub"
```

Dòng `WARNING` về host activation ở giữa là bình thường. Lần đầu, Claude Code hỏi bạn có tin cậy `Bash(skillhub:*)` không: hãy cho phép, để agent chạy được các lệnh `skillhub`.

### 4. Kiểm tra

`doctor` kiểm tra hub và kết nối, và in một cách sửa cho mỗi chỗ sai.

```bash
skillhub doctor
```

```text
...
Claude Code       ok      user entry in ~/.claude.json; not used here, because this project's own
                          entry wins
...
Healthy.
```

Trong `~/skill-hub`, thư mục hub có kết nối riêng của nó, nên các dòng cho tài khoản người dùng ghi "not used here": điều đó là bình thường. Các dòng ghi `notice` chỉ là thông tin, không phải lỗi. Nếu dòng cuối không phải `Healthy.`, chạy `skillhub doctor --fix`, hoặc tra thông báo trong [Xử lý sự cố](docs/troubleshooting.md) (tiếng Anh).

### 5. Hỏi agent của bạn

Mở agent trong một thư mục dự án bất kỳ (khởi động lại nếu nó đang chạy, để nó nạp kết nối), rồi nói:

> curate my Skill Hub

Agent kiểm tra hub bằng `skillhub status` và gợi ý một bước tiếp theo. Hub mới chưa có skill nào, nên bước đó là skill đầu tiên của bạn: [tạo skill đầu tiên](docs/curating-skills.md#create-your-first-skill) (tiếng Anh). Khi bạn kích hoạt (active) skill, tức là bật nó cho agent, agent sẽ tự hỏi hub để lấy nó khi một việc phù hợp.

### Bước tiếp theo

- [Tạo skill đầu tiên](docs/curating-skills.md#create-your-first-skill), hoặc [thêm skill từ GitHub](docs/curating-skills.md#add-a-skill-from-github) (tiếng Anh).
- [Xem và duyệt skill trên trình duyệt](#xem-thử).
- [Giữ hub trên GitHub](docs/user-guide.md#keep-the-hub-on-github), và [chuyển nó sang máy khác](docs/user-guide.md#move-to-another-machine) (tiếng Anh).
- [Chỉ kết nối một số dự án](docs/user-guide.md#connect-one-project-recommended) thay vì tất cả (tiếng Anh).
- [Tuỳ chọn cài đặt, nâng cấp và gỡ cài đặt](docs/installation.md) (tiếng Anh).
- Mọi lệnh và cờ: `skillhub help`, `skillhub help <command>`, hoặc [tài liệu tham chiếu lệnh](docs/command-reference.md) (tiếng Anh).

### Hub và agent ở hai máy khác nhau

Giữ hub ở một máy và dùng nó từ agent ở máy khác, qua cùng Wi-Fi hoặc LAN, hoặc [Tailscale](https://tailscale.com). Làm bước 1 và 2 trên máy hub, rồi khởi động server ở đó dưới dạng dịch vụ nền và tạo mã dùng một lần:

```bash
cd ~/skill-hub && skillhub serve --install-service
skillhub token invite
```

`token invite` in ra một dòng cho mỗi mạng của máy hub, ví dụ:

```text
Tailscale (100.121.46.35), on the other machine:
  $ skillhub connect --hub http://100.121.46.35:7420 --invite K7QF-9M2X-4TBD
```

Trên máy agent, làm bước 1, rồi chạy dòng ứng với mạng mà máy đó tới được, thêm `-g` để mọi dự án đều dùng hub:

```bash
skillhub connect --hub http://100.121.46.35:7420 --invite K7QF-9M2X-4TBD -g
```

Kiểm tra ở máy đó bằng bước 4, rồi khởi động lại agent: từ giờ nó hỏi hub để lấy skill. Việc quản lý skill vẫn ở máy hub: mở agent trong `~/skill-hub` ở đó và nói "curate my Skill Hub". Mã chỉ dùng được một lần, trong 10 phút. `--install-service` dùng cho Linux và macOS; trên Windows, chạy `skillhub serve` trong một terminal riêng và để nó mở. Kết nối là HTTP thường, ổn ở nhà hoặc văn phòng; với mạng khác, cũng như đường hầm SSH và token, xem [Một hub cho nhiều máy](docs/remote-hub-guide.md) (tiếng Anh).

## Cách hoạt động

1. Skill của bạn nằm trong một hub: một thư mục trong kho Git của riêng bạn.
2. Bạn kết nối agent với hub một lần.
3. Khi bắt đầu một việc, agent hỏi hub skill nào phù hợp, và hub trả lời.

```mermaid
flowchart LR
    agent["AI agent của bạn<br/>Claude Code"]
    hub{{"Skill Hub"}}
    repo[("Skill của bạn<br/>trong kho Git của bạn")]
    you(["Bạn"])
    gh["Các repo GitHub<br/>bạn nhập skill từ đó"]

    agent -- "1 Skill nào phù hợp?" --> hub
    hub -- "2 Skill này" --> agent
    hub <--> repo
    you -- "Trình duyệt hoặc terminal" --> hub
    gh -. "nhập, kiểm tra bản cập nhật" .-> hub

    classDef agentNode fill:#dbeafe,stroke:#1d4ed8,stroke-width:2px,color:#0f172a
    classDef hubNode fill:#fde68a,stroke:#b45309,stroke-width:3px,color:#0f172a
    classDef storeNode fill:#bbf7d0,stroke:#15803d,stroke-width:2px,color:#0f172a
    classDef personNode fill:#fbcfe8,stroke:#be185d,stroke-width:2px,color:#0f172a
    classDef externalNode fill:#e5e7eb,stroke:#4b5563,stroke-width:2px,color:#0f172a
    class agent agentNode
    class hub hubNode
    class repo storeNode
    class you personNode
    class gh externalNode
```


## Vì sao dùng Skill Hub

- **Không phải cài theo từng dự án.** Kết nối một lần; không có thư mục skill nào để sao chép đi khắp nơi.
- **Biến skill thành của bạn.** Thu thập skill từ bất kỳ kho nào, chỉnh sửa, tinh chỉnh theo tiêu chí của bạn. Skill mới bắt đầu ở trạng thái nháp để bạn duyệt.
- **Mỗi việc một gợi ý.** Nội dung đầy đủ của skill chỉ được tải sau khi agent chọn.
- **Git là cơ sở dữ liệu.** Các file thường trong kho của riêng bạn: xem lại, chia sẻ và hoàn tác bằng Git.
- **Giao diện web cục bộ** để duyệt, xem xét và sửa skill.

## Xem thử

Khởi động giao diện web từ hub của bạn. Nó in ra liên kết cho máy này, mạng của bạn và Tailscale, mỗi liên kết kèm token phiên; hãy mở một liên kết trong trình duyệt. [Hướng dẫn giao diện web](docs/web-ui.md) đi qua từng tác vụ.

```bash
cd ~/skill-hub && skillhub serve web
```

Ai có liên kết đều sửa được skill của bạn; xem [giữ an toàn trên mạng của bạn](docs/web-ui.md#keep-it-safe-on-your-network) (tiếng Anh).

<img src="docs/images/web/skill-review.png" alt="Tab Review của một skill đang dùng, hiển thị kiểm tra nội dung và mức sẵn sàng." width="800">

*Review: agent có dùng được skill này không, và nên kiểm tra gì trước.*

## Hoạt động với

| Agent | Trạng thái |
|---|---|
| Claude Code | Đã kiểm chứng ([ma trận tương thích](docs/mcp-compatibility-matrix.json)) |
| Codex | `connect` ghi các file kết nối; chưa kiểm chứng đầu-cuối |
| Gemini CLI | `connect` ghi các file kết nối; chưa kiểm chứng đầu-cuối |

## Tài liệu

Tài liệu chi tiết hiện chỉ có tiếng Anh.

- **Bắt đầu:** [Bắt đầu nhanh](#bắt-đầu-nhanh) · [Cài đặt](docs/installation.md)
- **Hướng dẫn:** [Dùng Skill Hub](docs/user-guide.md) · [Quản lý skill](docs/curating-skills.md) · [Nguồn và học](docs/sources-and-learning.md) · [Hướng dẫn giao diện web](docs/web-ui.md) · [Một hub từ nhiều máy](docs/remote-hub-guide.md) · [Xử lý sự cố](docs/troubleshooting.md)
- **Tham chiếu:** [Tham chiếu lệnh](docs/command-reference.md) · [Toàn bộ tài liệu](docs/README.md)
- **Người bảo trì:** [Đóng góp](CONTRIBUTING.md) · [Quy trình phát hành](docs/release-runbook.md)

## Đóng góp và giấy phép

Để biên dịch, kiểm thử hoặc phát hành Skill Hub, xem [CONTRIBUTING.md](CONTRIBUTING.md). Skill Hub được phát hành theo Giấy phép MIT; xem [LICENSE](LICENSE).
