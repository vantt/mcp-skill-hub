<div align="center">

<h1><picture><source media="(prefers-color-scheme: dark)" srcset="docs/images/brand/skill-hub-logo-dark.svg"><img src="docs/images/brand/skill-hub-logo-light.svg" alt="Skill Hub" width="280"></picture></h1>

Skill của bạn ở một nơi. Mọi agent kéo về khi cần.<br>

[![Latest release](https://img.shields.io/github/v/release/vantt/skillhub-mcp)](https://github.com/vantt/skillhub-mcp/releases/latest) [![CI](https://github.com/vantt/skillhub-mcp/actions/workflows/ci.yml/badge.svg)](https://github.com/vantt/skillhub-mcp/actions/workflows/ci.yml) [![License: MIT](https://img.shields.io/badge/license-MIT-green)](LICENSE)

[English](README.md) · [Bắt đầu nhanh](#quickstart) · [Dùng Skill Hub](docs/user-guide.md) · [Quản lý skill](docs/curating-skills.md)

</div>

Skill Hub phục vụ các skill của agent qua MCP từ một kho Git của riêng bạn, nên không có gì phải cài hay sao chép vào từng dự án: agent hỏi hub skill phù hợp khi bắt đầu một việc.
Vì thư viện nằm một chỗ, bạn có thể thu thập skill từ bất kỳ kho nào, chỉnh sửa và tinh chỉnh theo tiêu chí của riêng bạn, kèm phản hồi từ việc dùng thật và một bước duyệt trước khi bất cứ thứ gì đến tay agent.

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="docs/images/web/dark-skills-list.png">
  <img src="docs/images/web/skills-list.png" alt="Danh sách skill với bộ lọc trạng thái và các skill ở trạng thái nháp, đang dùng hoặc không còn dùng." width="800">
</picture>

*Skills: mọi skill kèm trạng thái, và bộ lọc để tìm.*

## Cách hoạt động

1. Bạn giữ các skill trong một hub: một thư mục riêng, nằm trong một kho Git.
2. Bạn kết nối tài khoản người dùng một lần, hoặc một dự án riêng, để agent tìm thấy hub.
3. Agent được dặn hỏi hub skill nào phù hợp với một việc, và hub trả lời.

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

- **Không cài theo từng dự án.** Kết nối tài khoản người dùng một lần bằng `skillhub connect -g` và các thư mục dự án giữ nguyên. Agent hỏi hub skill khi bắt đầu một việc, nên không có gì được sao chép vào từng dự án.
- **Biến skill thành của bạn.** Thu thập skill từ bất kỳ kho nào, chỉnh sửa và tinh chỉnh theo tiêu chí của bạn. Phản hồi từ việc dùng thật cho biết skill nào có ích, và skill mới bắt đầu ở trạng thái nháp để bạn duyệt.
- **Mỗi việc một gợi ý.** Agent được dặn hỏi hub bằng một mô tả ngắn của việc. Nó nhận một gợi ý, hoặc "không skill nào phù hợp", và nội dung đầy đủ của skill chỉ được tải sau khi nó chọn.
- **Bạn duyệt mọi thay đổi của skill.** Skill mới bắt đầu ở trạng thái nháp và agent không được đề xuất chúng. Skill từ kho của người khác chỉ đến được agent sau khi bạn duyệt trong terminal.
- **Một chương trình, không có gì chạy nền.** Skill Hub là một file thực thi duy nhất. Agent khởi động nó khi cần; không có gì chạy ngầm.
- **Git là cơ sở dữ liệu.** Skill của bạn là các file thường trong kho của chính bạn. Xem lại, chia sẻ với đồng đội và hoàn tác bằng Git.
- **Giao diện web chạy cục bộ.** Duyệt, xem xét và sửa skill ngay trong trình duyệt, trên máy của bạn.

<a id="quickstart"></a>

## Bắt đầu nhanh

Bạn cài Skill Hub qua hai bước. Trước hết cài **hub**, là thư mục chứa các skill của bạn. Sau đó cài một **dự án khách**, là dự án mà agent của nó dùng hub. Bạn cần Git và một agent như Claude Code.

### Cài đặt

Cài Skill Hub trên Linux hoặc macOS chỉ với một lệnh.

```bash
curl -fsSL https://github.com/vantt/skillhub-mcp/releases/latest/download/install.sh | sh
```

<details>
<summary>Windows (PowerShell)</summary>

```powershell
powershell -ExecutionPolicy ByPass -c "irm https://github.com/vantt/skillhub-mcp/releases/latest/download/install.ps1 | iex"
```

</details>

Mở một terminal mới, rồi kiểm tra lệnh chạy được:

```bash
skillhub --version
```

[Cài đặt](docs/installation.md) nói về phiên bản, chữ ký, nâng cấp và biên dịch từ mã nguồn.

### Bước 1: Cài hub

Hub là một thư mục riêng, nằm trong một kho Git riêng. Tạo một thư mục trống và biến nó thành hub của bạn:

```bash
mkdir -p ~/skill-hub && cd ~/skill-hub && skillhub init
```

```text
Hub created at <home>/skill-hub.
Agent connection written: claude-code, codex-cli, gemini-cli.
First commit <commit>: "Initialize Skill Hub hub".
Git hooks and commit signing were skipped for this first commit.
This is now the hub of this machine; `skillhub connect` uses it.

Next: connect a project to this hub:
  $ cd <your-project> && skillhub connect
```

`<home>` là thư mục home của bạn. `init` biến thư mục thành kho Git với bố cục của hub và tạo commit đầu tiên. Nó cũng kết nối các agent bạn mở trong thư mục đó, và lưu đường dẫn của hub cho máy này. Nếu Git chưa có tên hay email người dùng, `init` tạo commit duy nhất này dưới tên `Skill Hub <skillhub@localhost>` và cho bạn biết.

<details>
<summary>Windows (PowerShell)</summary>

Windows PowerShell 5.1 không có `&&`. Hãy ngăn cách các lệnh bằng `;`:

```powershell
mkdir -Force ~/skill-hub; cd ~/skill-hub; skillhub init
```

</details>

<details>
<summary>Giữ hub trên GitHub</summary>

Tạo một repo trống trên GitHub, không có README hay file giấy phép. Sau đó đưa URL của nó cho `init` trong hub của bạn:

```bash
cd ~/skill-hub && skillhub init <git-url>
```

Việc `init` làm tùy thuộc vào repo:

- **Repo trống:** `init` đẩy hub lên đó và đặt nó làm `origin`. Đây là lần push duy nhất Skill Hub từng làm giúp bạn.
- **Repo đã chứa hub này:** `init` đặt nó làm `origin` và không đẩy gì.
- **Repo chứa thứ gì khác:** `init` từ chối và không đổi gì.

Sau đó, bạn tự commit và push các thay đổi của skill bằng Git.

</details>

<details>
<summary>Dùng hub trên máy thứ hai</summary>

Trên máy thứ hai, chạy `init` trong một thư mục trống với URL của kho hub:

```bash
mkdir -p ~/skill-hub && cd ~/skill-hub && skillhub init <git-url>
```

`init` tải hub về thư mục, dựng chỉ mục tìm kiếm và đặt nó làm hub của máy này. Muốn dùng một hub qua mạng, xem [Dùng một hub từ nhiều máy](docs/remote-hub-guide.md).

</details>

### Bước 2: Cài dự án khách

Dự án khách là dự án mà agent của nó dùng hub. Vào một dự án bạn đang làm và kết nối nó:

```bash
cd <your-project> && skillhub connect
```

```text
Connected project <home>/<your-project>
to the hub at <home>/skill-hub.
Agent connection written: claude-code, codex-cli, gemini-cli.
  - Claude Code: connected (.mcp.json, .claude/settings.local.json, system-curator skill, CLAUDE.md)
  - Codex: connected (.codex/config.toml, AGENTS.md)
  - Gemini CLI: connected (.gemini/settings.json, GEMINI.md)
  - Agent Skills folder (.agents/skills): connected (system-curator skill)

Next: restart your agent and ask it: "curate my Skill Hub"
```

Kết quả đã được rút gọn. `connect` tìm hub mà `init` đã lưu, và không bao giờ ghi đè nội dung của bạn trong các file đó. Thêm `--dry-run` để xem trước nó sẽ ghi gì. [Những gì được ghi](docs/user-guide.md#what-gets-written) liệt kê mọi file.

Khởi động lại agent trong thư mục đó và nói với nó:

> curate my Skill Hub

Skill curator mà `connect` đã cài sẽ dặn agent hiển thị trạng thái hub và một bước tiếp theo. Lần đầu bạn mở thư mục, Claude Code hiện hộp thoại tin cậy có liệt kê `Bash(skillhub:*)`.

> [!TIP]
> Chạy `skillhub connect -g` để kết nối mọi dự án của tài khoản người dùng cùng lúc.

<details>
<summary>Windows (PowerShell)</summary>

```powershell
cd <your-project>; skillhub connect
```

</details>

Muốn agent ở máy khác dùng hub này? Xem [Dùng một hub từ nhiều máy](#dùng-một-hub-từ-nhiều-máy).

### Kiểm tra

`skillhub status` cho thấy trạng thái của hub và các kết nối của thư mục này. Nó luôn kết thúc bằng một lệnh nên chạy tiếp theo.

```bash
cd <your-project> && skillhub status
```

```text
Hub at <home>/skill-hub
Status:     Skill Hub is up to date.
Health:     Workspace valid; search index current; Git clean.
Inventory:  No skills yet.

This project uses this hub

Check        Status  Detail
Claude Code  ok      project entry in .mcp.json
Codex        ok      project entry in .codex/config.toml
Gemini CLI   ok      project entry in .gemini/settings.json

Next: create your first skill (you will be asked a few questions):
  $ skillhub skill create
```

Khi có gì sai, `status` gọi tên nó. Đây là các trường hợp thường gặp và cách sửa:

- "No hub is set up on this machine": tạo hub bằng `mkdir -p ~/skill-hub && cd ~/skill-hub && skillhub init`.
- "This folder is not a Skill Hub hub or a connected project": kết nối bằng `skillhub connect`.
- "Nothing answers at `<url>`" với hub ở máy khác: trên máy đó chạy `cd ~/skill-hub && skillhub serve`.
- "The hub folder `<path>` is gone": chạy `cd <new-location> && skillhub init` ở nơi hub nằm bây giờ, rồi `skillhub doctor --fix` trong từng dự án.
- File kết nối hoặc quy tắc quyền của Claude Code đã cũ: chạy `skillhub doctor --fix`.

[Xử lý sự cố](docs/troubleshooting.md) giải thích từng thông báo, theo từng bước.

Tiếp theo: [tạo skill đầu tiên](docs/curating-skills.md#create-your-first-skill).

## Dùng một hub từ nhiều máy

Chạy hub trên một máy và cho agent ở các máy khác dùng nó qua mạng. Không có gì bị sao chép: máy chạy agent không giữ skill hay thư mục hub nào.

| Máy | Vai trò | Việc làm ở máy đó |
|---|---|---|
| **Máy hub** | Giữ thư mục hub chứa skill của bạn | Cài Skill Hub, [tạo hub](#bước-1-cài-hub), chạy `skillhub serve` |
| **Máy agent** | Chạy agent của bạn, như Claude Code | Cài Skill Hub, sao chép file token, chạy `skillhub connect` trong dự án |

Hãy cài Skill Hub trên cả hai máy trước, như ở phần [Cài đặt](#cài-đặt). Sau đó:

1. **Trên máy hub**, khởi động server:

   ```bash
   cd ~/skill-hub && skillhub serve
   ```

   Với mỗi địa chỉ mà máy kia dùng được, nó in ra hai dòng để chạy trên máy agent: một dòng `scp` sao chép file token của hub, và dòng `connect`. Cứ để nó chạy.

2. **Trên máy agent**, chạy hai dòng đó. Chạy dòng `connect` trong dự án của bạn:

   ```bash
   scp -p you@<máy-hub>:skill-hub/runtime/config/.hub-token ~/.skillhub-token-<máy-hub>-7420
   cd <dự-án-của-bạn> && skillhub connect --hub http://<máy-hub>:7420 --token-file ~/.skillhub-token-<máy-hub>-7420
   ```

   Hãy chép đúng các dòng server in ra; chúng chứa đúng địa chỉ và tên file. Thêm `-g` vào `connect` để kết nối mọi dự án của tài khoản người dùng cùng lúc.

3. **Khởi động lại agent** và kiểm tra kết nối:

   ```bash
   cd <dự-án-của-bạn> && skillhub doctor
   ```

   Nó kiểm tra hub có trả lời và có nhận token của bạn không, và in một cách sửa cho mỗi mục lỗi. Nếu nó báo "Nothing answers", hãy chạy `skillhub serve` trên máy hub.

`serve` lắng nghe trên mọi giao diện mạng qua HTTP thường, nên chỉ dùng trên mạng của riêng bạn hoặc qua [Tailscale](https://tailscale.com). Trên mạng bạn không tin cậy, hãy để server chỉ nghe trên địa chỉ của chính máy hub và mở một đường hầm SSH từ máy agent:

```bash
cd ~/skill-hub && skillhub serve --addr 127.0.0.1:7420     # trên máy hub
ssh -N -L 7420:127.0.0.1:7420 you@<máy-hub>                 # trên máy agent, để nó chạy
```

Sau đó chạy các dòng `scp` và `connect` cho `127.0.0.1:7420` mà server đã in. Trên máy hub, `skillhub serve --install-service` giữ server chạy sau khi bạn đóng terminal (Linux và macOS). Việc quản lý skill vẫn ở máy hub. [Dùng một hub từ nhiều máy](docs/remote-hub-guide.md) nói về token, Windows, dịch vụ nền, xoay token và quay về một máy.

## Xem thử

Khởi động giao diện web từ hub của bạn, chỉ trên máy này. Nó in ra một liên kết kèm token phiên; hãy mở nó trong trình duyệt. [Hướng dẫn giao diện web](docs/web-ui.md) đi qua từng tác vụ.

```bash
cd ~/skill-hub && skillhub serve web --addr 127.0.0.1:7421
```

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

- **Bắt đầu:** [Bắt đầu nhanh](#quickstart) · [Cài đặt](docs/installation.md)
- **Hướng dẫn:** [Dùng Skill Hub](docs/user-guide.md) · [Quản lý skill](docs/curating-skills.md) · [Nguồn và học](docs/sources-and-learning.md) · [Hướng dẫn giao diện web](docs/web-ui.md) · [Một hub từ nhiều máy](docs/remote-hub-guide.md) · [Xử lý sự cố](docs/troubleshooting.md)
- **Tham chiếu:** [Tham chiếu lệnh](docs/command-reference.md) · [Toàn bộ tài liệu](docs/README.md)
- **Người bảo trì:** [Đóng góp](CONTRIBUTING.md) · [Quy trình phát hành](docs/release-runbook.md)

## Đóng góp và giấy phép

Để biên dịch, kiểm thử hoặc phát hành Skill Hub, xem [CONTRIBUTING.md](CONTRIBUTING.md). Skill Hub được phát hành theo Giấy phép MIT; xem [LICENSE](LICENSE).
