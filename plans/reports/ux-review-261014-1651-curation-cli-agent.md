# Đánh giá UX curation qua CLI và agent (2026-10-14)

**Phạm vi:** thêm, tạo, sửa, thử và công bố skill qua `skillhub` CLI và qua agent (curator skill gọi CLI, hoặc MCP profile `curation`). Web UI do người đánh giá khác phụ trách; ở đây chỉ đọc code web để so sánh.
**Cách làm:** binary build từ `main` tại `f337753` (có `9cb61e4`), chạy trên một hub tạm (`skillhub init`), HOME và `XDG_*` tách riêng, một project tạm đã `connect`, một thư mục local và GitHub thật (`anthropics/skills`, `--path skills/pdf`) làm nguồn. Đường runtime được thử bằng cách nói chuyện thẳng với `skillhub mcp serve --profile runtime` qua stdio (JSON-RPC), giữ một phiên mở trong khi sửa skill. Hub thật `/home/vantt/skill-hub`, HOME thật và project thật không bị đụng tới.

**Bổ sung lần 2 (cùng ngày):** sau ba quyết định và một yêu cầu mới của người dùng, báo cáo có thêm bốn phần. (1) Git host: tạm thời chỉ hỗ trợ GitHub, nhưng phải dựng dưới dạng một interface (port) để sau này thêm GitLab; xem phần [Port cho Git host](#port-cho-git-host). (2) Cập nhật từ upstream: giữ mô hình hiện tại (không tự áp dụng; luôn preview/confirm), chỉ làm cho UX thật tốt; xem phần [Cập nhật từ upstream](#cập-nhật-từ-upstream-outdated--upstream--update). Mọi gợi ý kiểu tự cập nhật đã bị loại khỏi báo cáo. (3) Use case mới UC5: một agent ở project có một skill chưa có trên hub và muốn đẩy nó lên để duyệt và publish, với cả hub cùng máy lẫn hub ở máy khác; xem phần [UC5](#uc5-đưa-một-skill-mới-từ-project-lên-hub). (4) Người dùng muốn thiết kế và bàn UX cùng hình dạng lệnh *trước* khi làm. Vì vậy mọi tên lệnh, cờ và output trong báo cáo này là **phương án để thảo luận**, không phải thiết kế đã chốt. Phần [Hình dạng lệnh để thảo luận](#hình-dạng-lệnh-để-thảo-luận) đưa ra hai phương án cho mỗi luồng chính, kèm transcript mẫu và đánh đổi. Báo cáo cố ý không có kế hoạch triển khai. Phần bổ sung được kiểm tra thực tế trên một hub tạm khác (HOME/XDG riêng), có `skillhub serve` chạy ở `127.0.0.1:47931` cho trường hợp hub ở xa; server đã được dừng sau khi thử.

## Kết luận chung

Phần lõi đã chắc chắn: preview/confirm bám pin, quy tắc trust cho nội dung bên thứ ba, và đường phục vụ runtime nhận bản sửa ngay ở lần `skill_get` kế tiếp (10–50 ms, tự rebuild, không cần khởi động lại agent; đã kiểm chứng với cả bản sửa có quản lý lẫn sửa file trực tiếp). Cảm giác "chưa đúng" không đến từ một lệnh hỏng nào. Nó đến từ ba khoảng trống trong mô hình. (1) **Không có cách thử một skill trong agent thật trước khi activate**: draft không có `local.path` nên script không chạy được, và đường duy nhất còn lại (gọi `skill_get` draft bằng ID) sẽ bị gỡ khi plan `261014-1650-runtime-skill-get-gate` được làm. (2) **Mỗi lần sửa chỉ đụng được `SKILL.md` và metadata.** Script và file reference chỉ sửa được trên web UI hoặc bằng cách sửa file trong hub, mà curator skill lại cấm agent làm vậy. (3) **Preview không cho thấy thay đổi**: `--content-file` và MCP `skill_update_preview` chỉ liệt kê tên file, trong khi curator yêu cầu agent "cho người dùng xem diff". Thêm vào đó, mỗi lần chủ hub tự sửa một skill bên thứ ba đã duyệt, skill đó im lặng trở thành không phục vụ được ở *mọi* project, mà resolver vẫn tiếp tục gợi ý nó. Tôi đề xuất sửa nhóm P0 nhỏ trước (diff, các câu "Next" sai, cảnh báo trust), rồi thêm **một** cơ chế mới là `skillhub skill checkout` / `checkin`. Cơ chế này gộp được ba khoảng trống còn lại (thử trong agent thật, sửa nhiều file, cập nhật từ thư mục) mà không thêm trạng thái mới vào Git.

---

## UC1: Thêm skill có sẵn

### 1a. Từ thư mục local: thực tế đã chạy

```text
skillhub skill add <dir>                 # preview, 0.04 s
skillhub skill add <dir> --yes           # draft trong skills/default/
skillhub skill review review-checklist   # thiếu trigger, not_for, min_scope
skillhub skill activate review-checklist --yes   # bị từ chối, exit 2, FIX đúng
skillhub skill edit review-checklist --trigger … --not-for … --min-scope single_step --example … ×3 --yes
skillhub skill activate review-checklist --yes
git -C <hub> add -- history skills && git -C <hub> commit …
```

Tối thiểu 4 lệnh `skillhub` cộng 2 lệnh Git; 6 lệnh nếu có review. Người dùng cần biết các khái niệm draft, trigger, not_for, min_scope và collection.

**Những gì chạy tốt:** preview không ghi gì. Lỗi khi activate thiếu trường có đủ ERROR/WHY/FIX và FIX in sẵn lệnh. `review` cho biết interpreter (`sh`) và gợi ý runtime block.

**Điểm đau, xếp theo mức độ:**

1. **Cao: không có cách cập nhật skill từ chính thư mục nguồn.** Sau khi chủ hub sửa thư mục gốc (ví dụ trong repo riêng của họ), lệnh `skill add <dir>` lần hai báo `Skill conflict detected … edit the existing skill with skillhub skill edit`. Nhưng `skill edit` chỉ nhận `SKILL.md` (`--content-file`), không nhận script hay reference. Vì vậy thư mục gốc và bản trong hub sẽ trôi xa nhau, và cách duy nhất để đồng bộ resource là sửa tay file trong hub.
2. **Cao: thêm lại một thư mục không đổi thì báo sai.** `skill add <dir> --yes` trả "Draft review-checklist added. Agent use: off." (exit 0), trong khi skill đang **active** và bản trong hub đã được sửa nhiều lần. JSON thì nói thật hơn (`"already added (replayed)"`). Bản preview cùng lúc in một lệnh confirm với pin rỗng: `skillhub skill confirm --proposal '' --proposal-digest '' --base-version ''`. Người dùng tưởng mình vừa ghi đè, hoặc vừa đưa skill về draft.
3. **Trung bình: gợi ý commit sau activate là sai.** Sau `activate`/`edit` trên một skill mới, dòng "Commit with `git -C <hub> commit`" thiếu bước `add`. File mới đang untracked nên commit đó không ghi gì. `skillhub status` thì in đúng (`add -- history skills && commit`). Hai chỗ nói hai cách khác nhau.
4. **Thấp: đếm file không khớp nhau.** Preview báo "2 files, 369 B" (chỉ đếm resource), còn review báo "4 files, 512 B" (tính cả `SKILL.md` và `.meta`).
5. **Thấp: collection mặc định không thống nhất.** `add` đặt skill vào `default`, còn `create` (form hỏi đáp và ví dụ trong docs) dùng `core`. Sau một phiên làm việc, hub có hai collection mà người dùng không hề chọn.

**Luồng lý tưởng:** `skillhub skill add <dir>` gợi ý luôn trigger, not_for, min_scope và ví dụ từ `description` trong frontmatter, rồi cho sửa ngay trong preview. Từ lần thứ hai, `skill add <dir>` (hoặc `checkin <dir>`, xem UC4) trở thành một proposal *cập nhật* có diff đầy đủ, thay vì báo xung đột.

### 1b. Từ Git: thực tế đã chạy

Repo bare local (`/path/up.git`, `file://…`) bị từ chối: `invalid source locator: allowed protocol is https`. Chỉ test mới được dùng `file://` (`internal/app/skill_add.go:359-364`, `AllowFileProtocol`). GitLab cũng bị từ chối: `unsupported host "gitlab.com"; only public github.com is supported` (`internal/source/locator.go:213-216`). Vì vậy tôi đi tiếp bằng GitHub thật:

```text
skillhub skill add https://github.com/anthropics/skills          # lỗi 8 MiB
skillhub skill add https://github.com/anthropics/skills --path skills/pdf   # preview 1.8 s
skillhub skill confirm PROP-…            # áp dụng ngay, không cần --yes
skillhub skill review pdf                # khối Content trust + digest 64 ký tự hex
(đọc 13 file trong <hub>/skills/default/pdf/)
skillhub skill edit pdf --approve-content sha256:<64 hex>        # lại là preview
skillhub skill edit pdf --approve-content sha256:<64 hex> --yes
skillhub skill edit pdf --trigger … --not-for … --min-scope … --example … --yes
skillhub skill activate pdf --yes
git add / commit
```

Khoảng 8–9 lệnh. Người dùng phải biết thêm các khái niệm proposal, digest, third-party và approval.

**Những gì chạy tốt:** upstream được ghi lại (`anthropics-skills`). Sửa routing không làm mất approval, và điều này đúng, vì routing không phải nội dung. Review có lệnh duyệt in sẵn. Confirm lại lần hai trả kết quả cũ (replay), nên lặp lại không gây hại.

**Điểm đau:**

1. **Đã có quyết định: chỉ hỗ trợ github.com công khai.** Repo trên GitLab, Gitea hay server tự host, và cả một bản clone local, đều không vào được với upstream được theo dõi. Cách vòng hiện có là `git clone` rồi `skill add <dir>`, nhưng khi đó origin thành "local authoring" và mất `skill update` (merge 3 bên). Người dùng đã chọn giữ GitHub-only lúc này và tách phần phụ thuộc GitHub ra sau một port; xem phần [Port cho Git host](#port-cho-git-host).
   - Thêm một lỗi đi kèm: `--ref` không nhận commit SHA, chỉ nhận branch hoặc tag. Bản preview báo đúng lỗi (`reference not found for "3d59511/skills/docx"`). Nhưng cùng lệnh đó với `--yes` lại báo `ERROR: The proposal is stale … FIX: Regenerate the add proposal.` (exit 2), tức là che mất nguyên nhân thật và dẫn người dùng đi thử lại vô ích. Tôi gặp lỗi này hai lần liên tiếp trên một hub mới khởi tạo.
2. **Trung bình: lỗi repo quá lớn chỉ sai hướng.** `source exceeded bytes limit (8560432 > 8388608)` đi kèm `FIX: Correct the skill fields or workspace state and retry.` FIX đúng phải là "dùng `--path <thư mục skill>` hoặc `--skill <tên>`".
3. **Trung bình: cảnh báo license bị ẩn ở chế độ người đọc.** Preview dạng text không in cảnh báo "Proprietary"; cảnh báo này chỉ có trong `--json` (`warnings[].code = license_warning`). Người dùng gõ tay không bao giờ thấy nó.
4. **Trung bình: duyệt nội dung là thao tác hai bước dù người đã xem.** `--approve-content <digest>` tạo thêm một preview ("Skill edit is ready for review … No files changed"), phải chạy lại với `--yes`. Sau khi duyệt xong, dòng "Next" lại bảo "edit the instructions with `--editor`", trong khi bước kế tiếp thực ra là điền routing hoặc activate.
5. **Thấp:** ngay sau khi add, `status` đã báo "1 source is due for an update check", tức là thêm việc ngay ở lần đầu.

**Luồng lý tưởng:** `skill add <url>` → một màn hình gồm file, license, routing gợi ý và lệnh duyệt → người dùng gõ một lệnh confirm, trong terminal có thể kèm `--approve-content`, vì chính họ vừa đọc diff đó. Agent vẫn không bao giờ được duyệt.

---

## UC2: Tạo skill mới

### Đường hỏi đáp (`skillhub skill create` trong terminal, chạy qua `script` để có TTY)

Có 6 câu hỏi: tên, id, mô tả, khi nào dùng (lặp), khi nào không dùng (lặp). Draft được lưu ngay sau câu trả lời cuối, kèm `File: <hub>/skills/core/deploy-notes/SKILL.md` và "replace the starter text in that file … then `skillhub skill activate deploy-notes --yes`". Tôi sửa thẳng file đó rồi activate. Toàn bộ là 2 lệnh, 1 lần sửa file, cộng commit. Đây là luồng nhanh nhất trong cả bài đánh giá.

**Điểm đau:**

1. **Cao: sản phẩm tự mâu thuẫn về việc sửa file trực tiếp.** Form tạo skill bảo người dùng sửa file canonical trong hub. Sau đó `skillhub status` lập tức báo `Search index FAILED … stale` và `FIX: skillhub doctor --fix`; trong Claude Code, `--fix` là lệnh phải hỏi quyền. Trong khi đó, lệnh đọc kế tiếp hay `activate` đều tự rebuild (đã thấy: activate chạy ngay 0.09 s). Thêm nữa, curator skill (`internal/systemskills/curator/SKILL.md`, đoạn "Never edit canonical Hub files directly") cấm agent làm đúng việc mà form vừa dặn. Còn `docs/design/07-storage-and-mutation-model.md` §17.1 nói "Direct edits are allowed because files are canonical". Ba nguồn nói ba điều khác nhau.
2. **Trung bình: form không hỏi ví dụ (`--example`) hay `--operation`.** Skill vẫn activate được, nhưng `skillhub eval routing` và findability-lint sắp tới không có gì để kiểm. Docs cũng nói "At least 3 examples are recommended".
3. **Trung bình: FIX khi activate với starter text bị cụt.** `FIX: Preview the fix with `skillhub skill edit kafka-review `` thiếu cờ (`--editor` hoặc `--content-file`).
4. **Thấp: lỗi khi không có TTY khó hiểu.** `create requires --id, --collection, --name, and --description` liệt kê cả những cờ đã truyền, và nhắc `--id` dù id là đối số vị trí.

### Đường cờ (script/agent)

Viết file Markdown → `skill create <id> --collection … --name … --description … --content-file … --trigger … --not-for … --min-scope … --example … --yes` → `activate --yes` → commit. Cách này chạy được, nhưng là một dòng hơn 10 cờ. Với agent đi qua curator, đó là preview, xin chấp thuận trong chat, prompt quyền của host cho lệnh `cli`, rồi lặp lại tất cả cho activate.

**Luồng lý tưởng:** form hỏi thêm 1–3 câu ví dụ, rồi hỏi "Mở editor để viết hướng dẫn ngay? [Y/n]" (dùng lại `--editor`, đã có diff và file khôi phục). `status` coi "index cũ sau khi sửa trực tiếp" là việc tự lo được, không phải `FAILED`.

---

## UC3: Sửa skill trong lúc đang thử (vòng lặp bên trong)

### Đo được gì

| Tình huống | Bước mỗi vòng | Thời gian máy | Agent thấy bản mới? |
|---|---|---|---|
| Skill **active**, tự viết, sửa `SKILL.md` bằng `skill edit --editor --yes` hoặc `--content-file … --yes` | 1 lệnh (người), 2 lệnh + 2 lần chấp thuận (agent) | 0.06 s + 10 ms | Có, ở lần `skill_get` kế tiếp trong cùng phiên MCP; `local.path` trỏ tới bản cache mới |
| Skill active, sửa **script/reference** | Không có lệnh CLI hay MCP nào. Chỉ có web UI (`internal/delivery/web/routes_skill_resources.go`) hoặc sửa thẳng file trong hub | ~50 ms (tự rebuild ở lần đọc kế) | Có, nếu sửa file trực tiếp |
| Skill **draft** | Không thử được qua resolve. `skill_get <id>` trả `content`, nhưng `local.status: unavailable`, `reason_codes: [not_servable]` (lý do dễ hiểu sai) và không có `local.path`, nên script và reference không dùng được | n/a | Chỉ phần text, và chỉ khi agent biết ID |
| Skill **bên thứ ba đã duyệt**, chủ hub tự sửa một dòng | Sửa + `review` + duyệt (2 lệnh, chỉ người làm được) | 0.06 s | **Không**: `content: null`, `review_required`, `content_review_stale`. Trong lúc đó `skill_resolve` vẫn trả skill này làm `primary` |

Bằng chứng: script probe giữ một phiên `mcp serve --profile runtime` qua 4 lần sửa (v1→v4). Mỗi phiên bản để lại một thư mục cache bất biến `runtime/cache/skills/review-checklist@<digest>` (5 thư mục sau một vòng thử ngắn, chưa thấy có dọn dẹp).

**Điểm đau:**

1. **Nghiêm trọng: không có cách thử draft trong agent thật.** Muốn chạy thử thật (có script, qua resolve) thì phải activate, nghĩa là công bố cho mọi project đã connect ngay lập tức. Đây đúng là bài học `owner-trial-before-publish` (`docs/distillery/lessons/curation.yaml:317`). `docs/use-cases/02-core-curation-use-cases.md:42` định nghĩa draft là "đang … kiểm thử cục bộ" nhưng ngay sau đó lại nói "Agent không nhìn thấy". Plan `261014-1650-runtime-skill-get-gate` sẽ đóng nốt đường `skill_get <draft>`, và plan đó tự ghi rủi ro này ("An agent that relied on reading drafts from a project loses that path. Intended"). Sau khi plan đó làm xong, vòng thử draft chỉ còn cách activate.
2. **Nghiêm trọng: chủ hub sửa skill bên thứ ba thì mọi project bị chặn, và không có cảnh báo trước.** Preview của lệnh sửa không nói "lần sửa này làm agent không nhận được nội dung cho tới khi bạn duyệt lại". Resolver vẫn gợi ý skill, nên agent ở project khác đi vào ngõ cụt `review_required` giữa chừng công việc. Thông báo "Approved before, but the approved content could not be found in Git history: review the full skill" còn có nghĩa là, nếu chưa commit trước khi sửa, người dùng mất luôn diff so với bản đã duyệt và phải đọc lại toàn bộ 13 file.
3. **Cao: preview không cho xem diff.** `skill edit --content-file` (text) và `--json` chỉ có `diff: {modified: [path]}`. MCP `skill_update_preview` cũng vậy. Unified diff chỉ hiện khi dùng `--editor`, hoặc khi truyền cờ ẩn `--full-diff` (`internal/delivery/cli/skill.go:398`, `:254`). Cờ này không có trong help, docs hay curator. `docs/use-cases/02-core-curation-use-cases.md` UC-04 bước 4 yêu cầu "Sinh bản so sánh trực quan (Unified Diff)". Curator yêu cầu agent "show the proposal diff", nhưng hub không đưa diff cho agent. Dòng "No files changed. Confirm with:" trong preview còn dễ bị đọc thành "lệnh không làm gì".
4. **Cao: sửa nhiều file không có đường có quản lý cho agent hay CLI.** `ResourceUpdateInput` (`internal/skill/resource_edit.go:64`) chỉ được web gọi, mỗi lần một file đã tồn tại. Không có cách thêm hay xóa file. Một thay đổi thường gặp ("đổi script và cập nhật `SKILL.md` cho khớp") thành 2 proposal riêng, hoặc thành sửa file trực tiếp.
5. **Trung bình: lịch sử thao tác sinh nhiễu Git.** Mỗi mutation ghi một file `history/operations/…/OP-*.yaml`. Sau phiên ngắn này có 15 file receipt cần commit, mỗi vòng sửa thêm một file. Điều này đi ngược sở thích "file tối thiểu" của người dùng.
6. **Thấp:** `skill edit --help` in help chung của `skill`; `--editor` với editor không tồn tại thì báo FIX chung chung "Correct the skill fields".

**Luồng lý tưởng:** agent sửa thoải mái ngay trong project bằng công cụ file bình thường (nhiều file, có script), gọi trực tiếp skill để thử, lặp lại tùy ý, rồi đẩy **một** proposal có diff đầy đủ về hub để người duyệt một lần. Đó chính là UC4.

---

## Đường agent: kẹt ở đâu

Thiết lập hiện tại khi `connect` cho Claude Code: MCP chỉ có profile `runtime` (3 tool). Curator skill được cài vào `.claude/skills/`. Quyền: `allow Bash(skillhub:*)`, `ask` với `--yes`, `confirm`, `init`, `connect`, `--fix`, `serve`…, `deny --approve-content` (`.claude/settings.local.json` do `connect` ghi).

| Use case | Agent làm được | Chỗ kẹt |
|---|---|---|
| Thêm từ thư mục | `skillhub skill add <dir> --json` → chạy `cli` sau khi người đồng ý | Hai lớp chấp thuận (chat rồi prompt của host); điều này đã được thiết kế như vậy. MCP không nhận đường dẫn local, nên chỉ host có shell mới làm được |
| Thêm từ GitHub | Có | Không duyệt được nội dung (đúng thiết kế); phải dừng lại nhờ người. Không xem được diff nội dung khi preview |
| Tạo | Có, phải viết file tạm cho `--content-file` (trong project hoặc `/tmp`; ngoài project thì host hỏi quyền) | Không có trường ví dụ trong form; có hai lớp chấp thuận cho cả create lẫn activate |
| Sửa `SKILL.md` | Có | Preview không có diff (trừ khi agent tự biết dùng `--full-diff`) |
| Sửa script/reference | **Không** có đường hợp lệ | Curator cấm sửa file trong hub; hub nằm ngoài project, nên host hỏi quyền cho mỗi lần ghi; `additionalDirectories` chỉ mở `runtime/cache` (file chỉ đọc, `-r--r--r--`), `envs`, `config` |
| Thử draft | Hôm nay: `skill_get <id>` lấy text. Sau gate plan: không | Không có `local.path` cho draft, nên script không chạy được |
| Commit | `status` in sẵn lệnh `git -C <hub> …` | Host hỏi quyền vì đây không phải lệnh `skillhub` |

Kết luận: profile split (runtime cho project, CLI cho curation) không phải chỗ kẹt chính; CLI với `--json` làm tốt việc đó. Chỗ kẹt là ba khoảng trống trong mô hình đã nêu ở phần kết luận: không thử được trước khi publish, không sửa được nhiều file, và preview không có diff. Một agent ở project có thể curate metadata khá trơn, nhưng không thể "lặp" trên nội dung skill.

---

## UC4: "Checkout → sửa → thử → checkin": các phương án

Các ràng buộc cần tôn trọng: Git là database; ít file; không over-engineer; người duyệt nội dung bên thứ ba; mọi mutation có preview/confirm.

Ba phương án dưới đây là các *mô hình*. Tên lệnh dùng ở đây (`checkout`, `checkin`, `trial`) chỉ để gọi tên cho dễ bàn; hai cách đặt tên và cách trình bày cho mỗi luồng nằm ở phần [Hình dạng lệnh để thảo luận](#hình-dạng-lệnh-để-thảo-luận).

### Phương án A: lấy skill ra project, sửa, rồi đẩy về (`checkout` / `checkin`; hướng đề xuất)

```text
skillhub skill checkout review-checklist            # mặc định: <project>/.claude/skills/review-checklist/
   (agent sửa SKILL.md, scripts/, references/ bằng công cụ file bình thường;
    thử bằng cách gọi thẳng skill: /review-checklist trong Claude Code, hoặc .agents/skills cho Codex)
skillhub skill checkin review-checklist             # preview: unified diff mọi file (thêm/sửa/xóa) + routing lint
skillhub skill checkin review-checklist --yes       # người chấp thuận; thư mục checkout có thể được xóa
```

- **Hub cần cung cấp:** (1) export cả thư mục skill ra một chỗ ghi được, kèm một file đánh dấu nhỏ (`.skillhub-checkout.json`: id, `manifest_version` gốc, workspace). Phần lớn code đã có sẵn ở đường phân phối cache, và `connect` đã có tiền lệ cài một skill (system-curator) vào `.claude/skills` và `.agents/skills`. (2) Nhập cả thư mục thành **một** proposal cập nhật, gồm thêm, sửa và xóa resource, và đi qua `ConfirmSkillMutation` như mọi mutation khác. Đây cũng chính là "cập nhật từ thư mục local" còn thiếu ở UC1a. (3) Thêm `.gitignore`/cảnh báo để thư mục checkout không bị commit nhầm vào repo của project.
- **Xung đột:** nếu hub chưa đổi kể từ khi checkout (`manifest_version` khớp) thì cập nhật thẳng. Nếu hub đã đổi thì merge 3 bên từng file, dùng lại engine của `skill update` (`--accept`, `--manual`, `--write-conflicts`). Bản gốc cho merge lấy từ thư mục cache bất biến `runtime/cache/skills/<id>@<digest>`, vốn đã tồn tại sẵn, nên không cần thêm file. Nếu cache đã bị `cache clear`, checkin từ chối và ghi xung đột ra thư mục để người dùng tự giải quyết.
- **Trust:** checkin một skill bên thứ ba làm approval thành stale, giống mọi lần sửa khác. Khác biệt là preview của checkin cảnh báo trước. Khi người đang ở TTY, cho phép `checkin --yes --approve-content` trong cùng bước, vì họ vừa xem đúng diff đó; agent vẫn bị deny. Với skill đang active, bản trong hub không đổi cho tới khi checkin được confirm, nên các project khác không bị chặn trong lúc đang thử. Đây là cải tiến lớn so với hiện tại.
- **Ưu điểm:** thử trong agent thật, có script, với cả draft lẫn active. Không cần trạng thái mới trong Git. Agent dùng công cụ file mà nó giỏi nhất. Sửa nhiều file thành một lần duyệt. Không đụng tới gate plan.
- **Nhược điểm:** có hai bản sao (có thể quên checkin; `status` nên liệt kê các checkout đang mở, đọc từ file đánh dấu trong project đã connect). Thử bằng cách gọi trực tiếp, nên **không** thử được routing; routing được kiểm bằng lint/eval ngay trong preview của checkin. Mức hỗ trợ skill native khác nhau giữa các host: Claude Code và Codex có, Gemini cần kiểm tra.
- **Cỡ việc (ước lượng thô, không phải kế hoạch):** khoảng 3 ngày.

### Phương án B: sửa thẳng trong Git worktree của hub

- **Các bước:** agent hoặc người sửa `<hub>/skills/<c>/<id>/…` → lần `skill_get` kế tiếp đã phục vụ bản mới (đã đo ~50 ms) → `skillhub maintain validate` → xem `git diff` → commit.
- **Hub cần cung cấp:** công nhận việc sửa trực tiếp là đường chính thức (bỏ lệnh cấm trong curator đối với skill tự viết; `review` hiện diff so với HEAD; `status` không còn báo `FAILED` khi index cũ). Gần như không cần code mới. Công sức khoảng 0.5–1 ngày.
- **Xung đột:** chính là Git (người tự merge).
- **Trust:** digest đổi thì approval stale, giống hiện tại.
- **Vì sao không chọn:** bỏ qua preview/confirm. Một skill active bị sửa dở **ngay lập tức** được phục vụ cho mọi project. Agent ở project phải ghi ra ngoài project (mỗi lần ghi bị hỏi quyền; mở `additionalDirectories` cho `skills/` thì lại phá ranh giới an toàn). Draft vẫn không thử được. B hợp với chủ hub làm việc ngay trong thư mục hub, nên nên ghi nó vào docs như đường "power user", nhưng không làm đường chính.

### Phương án C: "trial scope", phục vụ draft cho một project được chỉ định

- **Các bước:** `skillhub skill trial kafka-review --project <path> [--for 24h]` ghi một marker runtime (gitignore, `runtime/trials.json`). Server runtime của project đó cho draft này tham gia `skill_resolve` (gắn cờ `trial: true`) và cho phép `skill_get` có `local.path`. Sửa bằng `skill edit`/B, thử, rồi `activate`.
- **Hub cần cung cấp:** danh tính project trong server runtime (hiện server chỉ biết `--workspace`; `connect` phải truyền thêm `--project` hoặc dựa vào cwd). Một ngoại lệ trong gate plan. Gắn nhãn trial cho telemetry. Công sức khoảng 2–3 ngày.
- **Trust:** draft bên thứ ba vẫn phải được duyệt trước khi trial.
- **Ưu điểm:** thử cả routing trong phiên thật.
- **Nhược điểm:** thêm một trạng thái và một khái niệm mới; vẫn chưa giải được bài toán sửa nhiều file cho agent; làm phức tạp gate plan vừa được quyết định.

### Đề xuất

Hướng đề xuất là mô hình **A**; tên lệnh và output vẫn để bàn. Nó là phương án duy nhất giải được cùng lúc cả ba khoảng trống (thử trước khi publish, sửa nhiều file, cập nhật từ thư mục). Hub chỉ làm đúng vai "cung cấp chức năng" như người dùng hình dung: export, import thành proposal, merge, duyệt. Git vẫn là database, và mỗi checkout chỉ thêm đúng một file đánh dấu nằm ngoài hub. Ghi B vào docs như đường cho người làm trực tiếp trong hub, kèm sửa thông điệp `status`. Để C cho sau, chỉ khi dùng A một thời gian mà vẫn thấy cần thử *routing* trong phiên thật; lint/eval trong preview của checkin có thể đã đủ.

---

## Ảnh hưởng tới các plan đang hoãn

- **`261014-1545-findability-lint`: giữ, nhưng đổi thứ tự và nơi hiển thị.** Phase 1 sửa một lỗi thật: `ReviewSkill` chưa bao giờ báo routing lint, vì `DecodeSkillDocument` chỉ đọc JSON. Phase này nên làm ngay, độc lập với phần còn lại. Giá trị lớn nhất của lint là hiện ngay trong preview của edit, create và checkin (câu hỏi mở 1 của plan: nối vào `RoutingImpactHook`). Đề xuất đưa việc đó vào phạm vi, vì hiện tại "Routing impact" trong preview chỉ là câu vô nghĩa ("Direct routing fields … were evaluated against sha256:…"). Nếu form create không hỏi ví dụ thì lint không có gì để kiểm, nên P1 #11 bên dưới là điều kiện để lint có tác dụng.
- **`261014-1540-session-mining-routing-proposals`: tiếp tục hoãn (P2).** Plan này cải thiện metadata của skill *đã active* dựa trên telemetry; nó không chạm tới nỗi đau hiện tại (thêm, tạo, sửa, thử, publish). Plan cần case journal (opt-in), và đầu ra `--from-suggestion` của nó là một edit preview, mà hiện edit preview chưa có diff. Nên làm sau P0 "preview có diff". Không cần đổi thiết kế.
- **`261014-1650-runtime-skill-get-gate` (không hoãn nhưng có liên quan):** quy tắc này đúng cho runtime sạch, nhưng nó gỡ đường thử draft duy nhất hiện có. Nên làm cùng lúc hoặc sau `checkout` (câu hỏi mở 2), và đổi lý do `not_servable` cho draft thành một mã dễ hiểu hơn (ví dụ `draft_not_served`). Với UC5 ở hub xa, plan này không đổi gì: proxy vốn chỉ có 3 tool runtime.

---

## UC5: Đưa một skill mới từ project lên hub

Tình huống: agent đang làm việc trong một project và có một skill **chưa có trên hub**. Skill đó có thể nằm trong `.claude/skills/<id>/` của project hoặc ở một thư mục bất kỳ, và có thể do chính agent viết trong phiên làm việc. Agent muốn đẩy skill lên hub để người duyệt rồi publish. Tôi thử với một skill `team-release` gồm `SKILL.md`, `scripts/bump.sh` và một file `.env` chứa `API_TOKEN=do-not-ship`. File `.env` mô phỏng một thứ thường bị bỏ quên trong thư mục làm việc.

### 5a. Hub cùng máy (project được `skillhub connect` tới hub local): đã chạy

```text
cd <project> && skillhub skill add .claude/skills/team-release        # preview
cd <project> && skillhub skill add .claude/skills/team-release --yes  # draft trong skills/default/
skillhub skill review team-release
skillhub skill edit team-release --trigger … --not-for … --min-scope … --yes
skillhub skill activate team-release --yes
git -C <hub> add -- history skills && git -C <hub> commit …
```

Cũng giống UC1a: 5–6 lệnh. Agent làm được qua curator (CLI với `--json`), với hai lớp chấp thuận ở mỗi bước ghi.

**Điểm đau:**

1. **Nghiêm trọng: bí mật trong thư mục bị copy vào hub một cách im lặng, rồi sẵn sàng bị commit.** Preview dạng text chỉ báo "2 files, 227 B". Nó không liệt kê file nào, và không cảnh báo gì về `.env`. Chỉ trong `--json` mới thấy `resources: ['.env', 'scripts/bump.sh']`; cảnh báo duy nhất có ở đó là `license_warning`. Sau `--yes`, `.env` nằm trong `<hub>/skills/default/team-release/`. `skillhub maintain validate` báo "Workspace validation passed". `.gitignore` của hub không loại `.env` (`git check-ignore` không khớp), nên chính lệnh `git add -- history skills` mà `status` gợi ý sẽ stage file bí mật này. `docs/curating-skills.md` có dặn "keep secrets out of it", nhưng sản phẩm không giúp người dùng làm việc đó. Rủi ro tăng khi agent là người gửi skill lên: agent hay để lại file tạm, cache, `.env` hay log trong thư mục làm việc.
2. **Cao: một skill do agent viết được coi là first-party và không cần duyệt nội dung.** Origin là `local` ("local authoring"), nên không có bước `--approve-content`, và skill có thể activate ngay. Với cùng một người trên cùng một máy thì điều này hợp lý. Nhưng agent có thể đã chép script từ Internet vào thư mục đó, nên "do project viết" chưa chắc là "do chủ hub viết". Xem câu hỏi mở về trust.
3. **Trung bình: sau khi đẩy lên hub, bản gốc vẫn nằm trong `.claude/skills/` của project.** Claude Code sẽ nạp bản native đó, còn hub lại phục vụ bản trong hub. Hai bản sẽ trôi xa nhau, và lần `skill add` thứ hai báo `Skill conflict detected` (UC1a). Không có lời nhắc nào về việc dọn bản trong project hay biến nó thành một bản checkout có đánh dấu.
4. **Thấp:** id lấy từ tên thư mục và collection là `default`. Không có cách gán collection hay routing ngay trong preview.

### 5b. Hub ở máy khác (`connect --hub …`, chạy qua `skillhub mcp proxy`): đã chạy

`skillhub serve --addr 127.0.0.1:47931` chạy trên hub. Project được kết nối bằng `skillhub connect --hub http://127.0.0.1:47931 --token-file …`.

- Khi `connect`, lệnh tự nói rõ: "Curation stays on the hub machine: no curation server and no system-curator skill are installed here."
- `skillhub mcp proxy` chỉ có 3 tool: `skill_feedback`, `skill_get`, `skill_resolve`. Gọi `skill_create_preview` qua proxy bị trả lỗi `unknown tool "skill_create_preview"`.
- Trong project đó, `skillhub skill add .claude/skills/team-release`, `skill create …`, và cả `skill list` đều trả cùng một lỗi rõ ràng: "This project uses the hub at http://127.0.0.1:47931. Skills are curated on the hub machine. FIX: Run this command on the hub machine." Các lệnh này trả exit 2, đúng như mong đợi.
- `docs/remote-hub-guide.md:491-493` coi điều này là chủ ý: "Curation tools … any write to your skills … never travel", và một token bị đánh cắp "does not allow any curation".
- MCP `skill_create_preview` của profile curation, ngay cả trên hub cùng máy, chỉ nhận `content` (một chuỗi `SKILL.md`; `internal/delivery/mcpserver/types.go:117-125`). Không có trường cho file kèm theo. Vì thế, kể cả khi mở tool này qua mạng, nó cũng không mang được `scripts/` lên.

**Kết luận 5b:** hôm nay **không có đường nào** để đưa một skill từ máy A lên hub ở máy B, trừ những cách thủ công ngoài sản phẩm: chép thư mục sang B rồi chạy `skill add` ở B, hoặc commit vào một repo GitHub rồi `skill add <url>` ở B. Cách thứ hai biến skill của chính mình thành third-party, và repo phải công khai.

### UC5: luồng lý tưởng và hub cần cung cấp gì

UC5 chính là trường hợp "check-in một thư mục chưa có dấu checkout" của mô hình A. Nên dùng chung một cơ chế với UC4, không đặt thêm lệnh riêng:

1. **Cùng máy:** agent chạy lệnh đẩy lên trên thư mục. Không có file đánh dấu nghĩa là đây là **skill mới**, nên lệnh tạo proposal *tạo draft*. Có file đánh dấu nghĩa là đây là bản đang sửa, nên lệnh tạo proposal *cập nhật* (UC4).
   - Preview phải liệt kê **mọi file** cùng kích thước.
   - Preview phải loại mặc định các file giống bí mật hoặc rác (`.env*`, `*.pem`, `id_*`, `.DS_Store`, `node_modules/`, `__pycache__/`) và nói rõ đã loại file nào. Muốn giữ một file thì người dùng phải chủ động thêm.
   - Preview phải cho gán collection và routing ngay.
   - Sau khi confirm, bản trong project được chuyển thành một bản checkout có đánh dấu, hoặc được gợi ý xóa, để hết chuyện hai bản trôi nhau.
2. **Hub ở xa:** máy A không đọc được hệ thống file của B và ngược lại, nên file phải đi qua mạng. Có hai lựa chọn khác nhau về bản chất (xem luồng "Gửi skill lên hub ở xa" trong phần hình dạng lệnh):
   - **(R1) Nộp qua MCP:** proxy có thêm một tool để nộp skill. Tool này nhận các file dưới dạng base64, trong giới hạn hiện có (2 MiB/file, 8 MiB tổng, 2048 file). Ở B, skill vào hàng chờ như một **draft cách ly**, mang origin "submitted from <machine>". Nó luôn cần người ở B duyệt nội dung bằng `--approve-content`, giống third-party. Server chỉ nhận bài nộp khi chủ hub bật rõ ràng, ví dụ khi `serve` được bật cho nhận skill nộp lên, vì cho phép nộp làm rộng những gì một token bị đánh cắp có thể làm.
   - **(R2) Đi qua Git:** máy A đóng gói skill (một file bundle, hoặc đẩy lên một nhánh `submissions/<id>` của remote Git của hub). Ở B, chủ hub chạy `skill add` từ bundle hoặc từ nhánh đó. Không cần mở thêm gì trên server MCP, và hợp với "Git là database". Đổi lại, máy A phải có quyền push vào remote Git, và luồng này không còn gói gọn trong một câu lệnh agent.
3. **Trust (đề xuất để bàn):** skill tạo từ project cùng máy coi là first-party như hiện nay, vì cùng người và cùng máy; bộ lọc bí mật cùng danh sách file đầy đủ là điều kiện tối thiểu. Skill nộp từ máy khác thì coi là **chưa duyệt**, giống third-party, cho tới khi người ở hub duyệt. Lý do: token runtime không chứng minh được ai đã viết nội dung.
4. **Cỡ việc (thô):** UC5 cùng máy gần như được miễn phí nếu đã có `checkin` của mô hình A (cộng khoảng 0.5 ngày cho bộ lọc bí mật và danh sách file; phần này nên làm ngay ở P0 cho `skill add <dir>`). R1 khoảng 2–3 ngày (tool mới trên proxy và HTTP listener, giới hạn kích thước, hàng chờ, cấu hình bật/tắt, tài liệu bảo mật). R2 khoảng 1–2 ngày.

---

## Cập nhật từ upstream (`outdated` → `upstream` → `update`)

Người dùng giữ mô hình hiện tại: không tự áp dụng, luôn preview/confirm. Phần này chỉ đánh giá UX của mô hình đó.

**Cách dựng tình huống:** trên hub tạm, tôi add `docx` từ nhánh cũ `klazuka/export-20260203` (`d5119f8`) của `anthropics/skills`, rồi chuyển ref theo dõi sang `main` (`dbd4588`). Các digest đều do hub tự tính, nên trạng thái này tương đương một skill được add từ lâu. Upstream đổi 17 file, trong đó `SKILL.md` đổi gần như toàn bộ (482 → 92 dòng).

### Đã chạy và đếm bước

```text
skillhub skill outdated              # "not checked", chỉ có cache
skillhub skill outdated --check      # 2.4 s: "1 of 1 … need attention … 17 files … update available"
skillhub skill upstream docx         # liệt kê 17 file đổi upstream (không bắt buộc)
skillhub skill update docx           # preview: bảng file + toàn bộ diff (2,864 dòng)
skillhub skill confirm --proposal … --proposal-digest … --base-version …   (hoặc --yes)
skillhub skill review docx           # approval đã stale, có lệnh git diff so với bản đã duyệt
skillhub skill edit docx --approve-content sha256:… --yes
git add / commit
```

Không có xung đột: khoảng 7 lệnh. Có xung đột (sau khi tôi sửa cục bộ `SKILL.md` và `scripts/office/validators/docx.py`), thêm `update --write-conflicts <dir>`, sửa file, `update --manual SKILL.md=<file>` (preview), rồi chạy lại với `--yes`: khoảng 10–11 lệnh.

### Những gì đã tốt, và nên giữ

- Bảng tóm tắt trước diff rất rõ: mỗi file có một cột `CHANGE` (changed upstream / added / removed / changed here and upstream) và một cột `ACTION` (take upstream / merged / needs decision). Ngoài ra có dòng "48 file(s) unchanged".
- Merge 3 bên tự động chạy được: `validators/docx.py` có sửa ở cả hai phía nhưng không chồng dòng, nên được đánh dấu `merged`.
- Khi có xung đột, output in đủ ba lựa chọn cho từng file (`--accept SKILL.md=upstream|local`, `--manual SKILL.md=<file>`) cùng lệnh `--write-conflicts`. File xung đột có marker `<<<<<<< local` / `>>>>>>> upstream` chuẩn, và chỉ bao đúng đoạn chồng nhau (dòng 10–26).
- Preview nói trước: "After applying, agents cannot use docx until you approve the new content". `review` sau đó in sẵn `git diff fd317b3f3024 -- skills/default/docx`, tức là diff so với bản đã duyệt. Đây đúng là thứ còn thiếu ở luồng edit (UC3, điểm đau 2).
- Sau `--yes`, cả bản sửa cục bộ (`TEAM NOTE`, `# team tweak`) lẫn phần thay đổi của upstream đều được giữ.

### Điểm đau

1. **Cao: lỗi bị giấu ở chế độ text, và exit code là 0.** Khi skill đã cập nhật, hoặc không có upstream, `skill update` in "Update docx from - to - (…, source )." cùng một lệnh confirm có pin rỗng, và trả **exit 0**. JSON thì đúng: `status: error`, "Upstream is unchanged." Đây là cùng một lỗi hiển thị với `skill add` thêm lại thư mục (UC1a): bộ in text bỏ qua `status: error`.
2. **Cao: `skill upstream` gợi ý một bước không làm gì.** Khi trạng thái là "up to date (local copy is clean)", lệnh vẫn in "Next: skillhub skill update docx", và lệnh đó chính là no-op lỗi ở điểm 1.
3. **Trung bình: preview đổ toàn bộ diff (2,864 dòng) ra terminal.** Bảng tóm tắt bị cuộn mất. Cảnh báo về approval và dòng "Next" nằm ở cuối cùng. Luồng `edit` thì ngược lại: không có diff (UC3, điểm đau 3). Hai luồng cần một quy ước chung, xem luồng "Cách hiện preview" trong phần hình dạng lệnh.
4. **Trung bình: sau khi đã giải quyết xung đột, preview vẫn báo "needs decision".** Khi đã truyền `--manual SKILL.md=<file>`, bảng vẫn ghi `needs decision (1 conflict)` cho `SKILL.md`, dù lần apply ngay sau đó thành công. Người dùng không biết mình đã giải quyết xong hay chưa.
5. **Trung bình: phần giải quyết xung đột thiếu một nước đi tự nhiên.** Muốn sửa thì phải `--write-conflicts` ra một thư mục, sửa, rồi truyền lại `--manual <path>=<file>` cho từng file. Không có cách "mở xung đột trong editor ngay tại chỗ" khi đang ở TTY, cũng không có lựa chọn `--accept-all=upstream`. Với 1 file thì chấp nhận được; với 5 file thì nặng nề.
6. **Thấp: hai bước chỉ để kiểm tra.** `outdated` (cache) báo "not checked" và bảo chạy `--check`, rồi `--check` mới cho kết quả. Lệnh `upstream` lặp lại phần lớn thông tin của `update` preview. Ba động từ cho một việc làm tăng số bước.
7. **Thấp:** approval sau cập nhật vẫn là bước riêng (`--approve-content` dạng preview, rồi `--yes`), dù người dùng vừa xem đúng diff đó ngay trong preview của update.

**Luồng lý tưởng (giữ preview/confirm):** một lệnh kiểm tra cho ra danh sách skill có cập nhật. Một lệnh cập nhật cho ra bảng tóm tắt, mặc định chỉ thống kê theo file, và xem diff đầy đủ qua pager hoặc theo yêu cầu. Xung đột được hỏi từng file khi ở TTY (upstream / giữ của mình / mở editor). Có một lần confirm, và người ở TTY có thể duyệt nội dung luôn trong lần đó. Hai phương án hình dạng lệnh nằm ở luồng "Cập nhật từ upstream" bên dưới.

---

## Port cho Git host

Quyết định của người dùng: tạm thời chỉ hỗ trợ GitHub, nhưng phải dựng sao cho sau này thêm GitLab mà không đụng nhiều chỗ. Phần này chỉ đánh giá chỗ nối hiện có và đề xuất port nhỏ nhất; không có kế hoạch triển khai.

### Hiện trạng

- **Đã trung lập về host:** `Adapter` (`internal/source/types.go`: `Identify`, `CurrentRevision`, `Diff`, `Read`, `List`) và `GitRepositoryAdapter` (`internal/source/git_repository.go`) làm việc với bất kỳ URL HTTPS Git nào qua go-git: mirror, liệt kê ref, đọc commit, giới hạn kích thước. Dữ liệu lưu trong Git cũng đã trung lập: `NormalizedLocator.Kind` nhận `"github"` hoặc `"git"` (`locator.go:102`, `:114`), và schema canonical đã chấp nhận `kind: git` (`internal/canonical/skill.go:205`). `ResolveGitHubRoute` (`locator.go:287`) thực ra cũng trung lập; chỉ có tên là GitHub.
- **Phần phụ thuộc GitHub:**
  - `ParseGitHubLocatorWithOptions` chặn host ở `locator.go:213-214` (`host != "github.com"`).
  - Dựng URL canonical ở `locator.go:238`.
  - Hiểu dạng đường dẫn `/tree/<ref>/…` và `/blob/<ref>/…/SKILL.md` của GitHub, và dùng `validGitHubName` ở `:510`.
- **Nơi gọi:**
  - `internal/app/skill_add.go:364` và `:378`;
  - `internal/app/source_watch.go:95` và `:104`;
  - `internal/app/source.go:486`, `:491` và `:1224`.
- **Origin `"github"` được ghi cứng:**
  - `internal/app/skill_add.go:452`;
  - `internal/app/source_import.go:630` và `:700`;
  - `internal/app/source_backfill.go:405`.
  - Ngoài ra nhiều chỗ so sánh `== "github" || == "git"`, trong đó có `internal/skillruntime/trust.go:39`. Chỗ này quyết định một skill có phải third-party hay không, nên host mới **bắt buộc** phải map vào một trong hai kind này.
- **Câu chữ GitHub-only:**
  - `internal/app/skill_add.go:367`;
  - `internal/app/source_watch.go:17`, `:31`, `:73`, `:80` và `:98`;
  - `internal/delivery/web/locator.go:60`;
  - mô tả schema MCP ở `internal/delivery/mcpserver/types.go:34` và `:44`, cùng `skill_add_tools.go:23`;
  - web UI: `web/src/screens/skill-add/AddSkillScreen.tsx` và `web/src/screens/skill-detail/AddSourceForm.tsx`.

### Port nhỏ nhất, giữ nguyên hành vi hiện nay

Một interface "nhà cung cấp host" trong `internal/source`. Interface này chỉ lo phần *hiểu URL*; phần Git vẫn do `GitRepositoryAdapter` làm:

```go
// HostProvider turns a web URL of a hosting service into a Git repository route.
type HostProvider interface {
    Kind() string                                   // "github"; stored as origin kind
    Matches(u *url.URL) bool                        // host check (today: github.com)
    ParseRoute(u *url.URL, ref, path string) (*ParsedRoute, error) // tree/blob/root shapes, canonical repo URL
}
```

- Có một registry chỉ chứa `GitHubProvider`. Hàm `ParseLocatorWithOptions` duyệt qua registry; host không khớp provider nào trả lại đúng lỗi hiện tại (`unsupported host …; only public github.com is supported`), nên hành vi và test hiện có giữ nguyên.
- `ParsedGitHubRoute` và `ResolveGitHubRoute` đổi tên thành `ParsedRoute` và `ResolveRoute`, kèm type alias tạm để giảm diff. Nhánh `file://` dành cho test vẫn giữ.
- Bốn chỗ ghi cứng `"github"` lấy `Kind()` từ provider. Các phép so sánh `"github" || "git"` giữ nguyên, vì một provider GitLab sau này có thể ghi `kind: git`, hoặc thêm kind của nó vào cùng chỗ.
- Câu chữ cho người dùng lấy từ registry ("supported hosts: github.com"), để khi thêm host thì không phải sửa câu ở năm chỗ.
- **File đụng tới:**
  - `internal/source/locator.go` (tách phần GitHub);
  - một file mới `internal/source/host_github.go`;
  - `internal/app/skill_add.go`, `source_watch.go`, `source.go`, `source_import.go`, `source_backfill.go`;
  - câu chữ ở `internal/delivery/web/locator.go` và `internal/delivery/mcpserver/types.go`.
  - Test hiện có trong `internal/source/locator_test.go` trở thành test của provider GitHub và phải pass mà không đổi kỳ vọng.
  - Digest danh sách tool MCP sẽ đổi nếu mô tả schema đổi; có thể giữ nguyên câu chữ để tránh việc này.
- **Cỡ việc:** khoảng 0.5–1 ngày, tùy có kéo theo web UI hay không.
- **Những gì port này cố ý *chưa* giải quyết, và GitLab tự host sau này sẽ cần:**
  - Chính sách địa chỉ chặn IP nội bộ và riêng tư (`internal/source/policy.go`, `deniedAddressRanges`), nên một GitLab trong mạng LAN công ty vẫn bị chặn dù đã có provider.
  - Chỉ hỗ trợ HTTPS không có thông tin đăng nhập (URL có user info bị từ chối), nên repo riêng tư cần thêm một cơ chế credential.
  - Nên nêu hai điểm này ngay trong ghi chú thiết kế của port, để không ai tưởng "có provider là đủ".

---

## Hình dạng lệnh để thảo luận

Mỗi luồng dưới đây có hai phương án, kèm transcript mẫu (bịa ra để minh họa, không phải output thật) và đánh đổi. Không phương án nào đã được chọn. Tôi chỉ nêu nghiêng của mình khi có lý do rõ.

### Luồng 1: Lấy skill ra project để sửa và thử (UC3/UC4)

**Phương án 1A: một cặp động từ riêng, `checkout` / `checkin`.**

```text
$ skillhub skill checkout review-checklist
Copied review-checklist (active, 4 files) to .claude/skills/review-checklist/
Hub copy unchanged; agents in other projects keep using it.
Try it: ask your agent to use /review-checklist.
Next: skillhub skill checkin review-checklist

$ skillhub skill checkin review-checklist
review-checklist: 2 files changed since checkout (hub copy unchanged)
  M SKILL.md                 +3 -1
  M scripts/lint-diff.sh     +1 -1
Routing: 3/3 examples still resolve to review-checklist.
Show the diff: skillhub skill checkin review-checklist --diff
Apply: skillhub skill checkin review-checklist --yes
```

**Phương án 1B: không thêm động từ; `edit` có một chế độ làm việc trong project.**

```text
$ skillhub skill edit review-checklist --here
Working copy of review-checklist in .claude/skills/review-checklist/ (4 files).
Edit and try it there; when you are done:
  skillhub skill edit review-checklist --from .claude/skills/review-checklist

$ skillhub skill edit review-checklist --from .claude/skills/review-checklist
Skill edit is ready for review: 2 files changed (+4 -2).
  M SKILL.md, M scripts/lint-diff.sh
Next: skillhub skill edit review-checklist --from … --yes
```

**Đánh đổi:**

- **1A:** dễ nhớ và có tên cho một trạng thái thật ("đang có người sửa bên ngoài"), nên `status` có thể liệt kê các checkout đang mở. Đổi lại, có thêm hai động từ, và cặp từ này gợi cảm giác khóa (lock) mà thực tế không có.
- **1B:** giữ bộ lệnh nhỏ, đúng với sở thích tối giản, và `--from <dir>` dùng lại được cho UC1a (cập nhật từ thư mục nguồn). Đổi lại, "bản làm việc đang mở" khó hiện trong `status`, và người dùng phải nhớ cặp cờ `--here` / `--from`.
- **Nghiêng của tôi:** 1B nếu ưu tiên ít động từ, 1A nếu muốn `status` nhắc các bản đang sửa dở.

### Luồng 2: Đẩy một skill mới từ project lên hub cùng máy (UC5a)

**Phương án 2A: dùng lại `skill add <dir>`, và làm preview cho đủ.**

```text
$ skillhub skill add .claude/skills/team-release
Add team-release as a draft (collection: core) from this project.
Files (2, 227 B):
  SKILL.md            181 B
  scripts/bump.sh      46 B
Left out (looks like a secret or build output):
  .env                 include with --include .env
Routing: none yet. Suggested from the description:
  trigger "cut a release"; add more with --trigger/--example.
After adding, .claude/skills/team-release/ stays in this project; it will
drift from the hub copy. Remove it, or keep it as a working copy (--keep-as-working-copy).
Next: skillhub skill add .claude/skills/team-release --yes
```

**Phương án 2B: một động từ "đẩy lên" duy nhất cho cả skill mới lẫn skill đã có.**

```text
$ skillhub skill publish .claude/skills/team-release
team-release is not on the hub: this creates a new draft.
Files: SKILL.md, scripts/bump.sh   (left out: .env)
Next: skillhub skill publish .claude/skills/team-release --yes

$ skillhub skill publish .claude/skills/review-checklist
review-checklist is on the hub (active); this updates it: 2 files changed.
Next: … --diff | … --yes
```

**Đánh đổi:**

- **2A:** không thêm lệnh, và người dùng đã biết `add`. Đổi lại, `add` vẫn còn khác với "cập nhật từ thư mục", nên cần thêm luồng 1 cho phần cập nhật.
- **2B:** một động từ, và agent không cần biết skill đã tồn tại hay chưa. Đổi lại, có rủi ro ghi đè nhầm khi trùng id (bắt buộc phải có một câu hỏi rõ ràng "this updates an existing skill"). Ngoài ra, chữ "publish" dễ bị hiểu thành activate, trong khi thực ra skill chỉ vào hub dưới dạng draft.
- **Dù chọn phương án nào**, danh sách file đầy đủ cùng bộ lọc bí mật nên làm ngay (P0).

### Luồng 3: Gửi skill lên hub ở máy khác (UC5b)

**Phương án 3A: nộp qua MCP (proxy có thêm một tool nộp).**

```text
(machine A) $ skillhub skill submit .claude/skills/team-release
Hub http://hub-b:7420 accepts submissions.
Sent team-release (2 files, 227 B; left out: .env) as a submission.
On the hub machine it waits as a draft that needs content approval:
  skillhub skill review team-release      (run on hub-b)

(machine B) $ skillhub status
Waiting
  - 1 submitted skill needs review: team-release (from machine-a, 2026-10-14)
```

**Phương án 3B: đi qua Git, không mở thêm gì trên server.**

```text
(machine A) $ skillhub skill pack .claude/skills/team-release --to submissions/team-release
Pushed branch submissions/team-release to the hub's Git remote (2 files; left out: .env).
Ask the hub owner to run: skillhub skill add --branch submissions/team-release

(machine B) $ skillhub skill add --branch submissions/team-release
Add team-release as a draft from branch submissions/team-release (pushed by t@t).
```

**Đánh đổi:**

- **3A:** agent tự làm trọn được, và trải nghiệm giống hệt cùng máy. Đổi lại, nó làm rộng những gì token runtime cho phép (hôm nay một token bị đánh cắp chỉ đọc được). Nó cần một chỗ bật/tắt, giới hạn kích thước, hàng chờ, và tài liệu bảo mật mới.
- **3B:** không đổi gì ở bề mặt mạng, đúng tinh thần "Git là database", và có sẵn danh tính người push trong Git. Đổi lại, máy A phải có quyền push vào remote của hub (nhiều người dùng một hub qua SSH tunnel thì không có), và cần thêm một bước người làm trên B.
- **Dù chọn phương án nào**, skill đến từ máy khác nên được coi là chưa duyệt cho tới khi người ở B duyệt.

### Luồng 4: Cách hiện preview (edit, add, update)

**Phương án 4A: tóm tắt trước, diff theo yêu cầu.**

```text
$ skillhub skill update docx
Update docx d5119f8 → dbd4588 (upstream commit of 2026-10-09)
  17 files: 9 changed, 5 added, 3 removed; 1 needs a decision (SKILL.md)
  SKILL.md   changed here and upstream   needs decision
  …
After applying, agents cannot use docx until you approve the new content.
See the diff:   skillhub skill update docx --diff        (or --diff SKILL.md)
Decide SKILL.md: --accept SKILL.md=upstream|local, or --manual SKILL.md=<file>
```

**Phương án 4B: diff đầy đủ luôn có, nhưng qua pager khi ở TTY.**

```text
$ skillhub skill update docx
(opens $PAGER: summary table on the first screen, then the full diff)
(after the pager closes)
1 file needs a decision. Next: … --accept SKILL.md=upstream|local
```

**Đánh đổi:**

- **4A:** màn hình đầu luôn ngắn. Agent đọc JSON và tự lấy `full_diff`. Người dùng phải gõ thêm một lệnh nếu muốn xem. Nếu chọn 4A thì `edit` cũng phải có `--diff`, và curator phải dùng nó.
- **4B:** không bỏ sót gì và đúng tinh thần "chỉ duyệt thứ mình đã thấy". Đổi lại, pager khó dùng trong một số terminal hay CI (khi không ở TTY thì phải tự tắt), và kết quả hai lần chạy khác nhau tùy môi trường.
- **Nghiêng của tôi:** 4A, với điều kiện JSON và MCP *luôn* trả `full_diff`, để agent luôn có diff thật để cho người xem.

### Luồng 5: Cập nhật từ upstream

**Phương án 5A: giữ ba động từ, sửa output.**

`outdated` (cache), `outdated --check`, `upstream <id>` và `update <id>` giữ nguyên tên. Các thay đổi:
- `outdated` tự kiểm tra khi cache đã cũ hơn N ngày (đây vẫn chỉ là kiểm tra, không phải áp dụng);
- `upstream` không gợi ý `update` khi không có gì để cập nhật;
- `update` dùng luồng 4;
- xung đột được hỏi từng file khi ở TTY:

```text
$ skillhub skill update docx
… 1 file needs a decision.
SKILL.md changed here and upstream.
  [u] take upstream  [l] keep mine  [e] edit merged file in $EDITOR  [d] show diff  [q] quit
> e
(editor opens with conflict markers; save without markers)
Resolved SKILL.md. Apply update? Agents cannot use docx until you approve it. [y/N/a=apply and approve]
```

**Phương án 5B: một động từ.**

`skillhub skill update` không kèm id trở thành danh sách những skill có cập nhật (thay cho `outdated`). Thông tin của `upstream <id>` được gộp vào phần đầu của `update <id>`.

```text
$ skillhub skill update
2 skills have upstream changes (checked 3 min ago):
  docx   d5119f8 → dbd4588   17 files   diverged (you changed 2 files)
  pdf    dbd4588 → 1ed29a0    3 files   update available
Next: skillhub skill update docx
```

**Đánh đổi:**

- **5A:** không đổi thói quen hay docs, và chỉ sửa đúng chỗ đau. Đổi lại, vẫn có ba động từ cho một việc.
- **5B:** ít bước hơn và dễ đoán. Đổi lại, phải giữ `outdated` và `upstream` làm alias một thời gian, và `update` không kèm id dễ bị nhầm với `skillhub update` (lệnh cập nhật chính binary). Help hiện đã phải có một dòng "Note" để phân biệt hai lệnh này.
- **Cả hai** đều giữ nguyên preview/confirm, đúng quyết định của người dùng.

### Luồng 6: Duyệt nội dung bên thứ ba (chỉ người, chỉ ở terminal)

**Phương án 6A: gắn vào lần confirm.**

```text
$ skillhub skill update docx --yes --approve
Applied the upstream update and approved the content you just reviewed
(sha256:290fa83…). Agents can use docx again.
```

`--approve` chỉ chạy được ở TTY. Agent bị chặn bằng luật `deny` hiện có, mở rộng thêm cho cờ mới.

**Phương án 6B: một động từ riêng, không bắt người dùng nhìn digest.**

```text
$ skillhub skill approve docx
docx changed since you last approved it (fd317b3):
  A scripts/merge_runs.py  M SKILL.md  … (17 files; attention: scripts changed)
Show the diff? [Y/n] y   (pager)
Approve exactly this content? [y/N] y
Approved. Agents can use docx.
```

**Đánh đổi:**

- **6A:** ít bước nhất, đúng lúc người dùng vừa xem diff. Đổi lại, nó gộp hai quyết định (áp dụng và tin cậy) vào một cờ, và luật `deny` của host phải phủ thêm cờ mới.
- **6B:** rõ ràng, tách biệt, và digest được ghim ở bên trong chứ không bắt người dùng chép 64 ký tự hex. Đổi lại, vẫn là một bước riêng. Nó cũng cần TTY thật, nên không chạy được trong script; điều này đúng với ý đồ.

---

## Danh sách cải tiến theo ưu tiên

Đây là danh sách *việc nên làm*. Hình dạng lệnh của các mục P1 vẫn chờ người dùng chọn ở phần trên. Cỡ việc là ước lượng thô.

### P0: nhỏ, sửa ngay (tổng khoảng 2–2.5 ngày)

1. **Thêm skill từ thư mục: liệt kê đủ file và chặn bí mật.**
   - Preview dạng text liệt kê mọi file cùng kích thước.
   - Mặc định loại các file giống bí mật hoặc rác (`.env*`, khóa, `node_modules/`, `__pycache__/`, …), báo rõ đã loại file nào, và cho người dùng giữ lại một cách chủ động.
   - `maintain validate` cảnh báo khi trong `skills/` có file giống bí mật.
   - Cỡ việc khoảng 0.5 ngày. **Mức cao nhất trong danh sách:** hiện `.env` đi thẳng vào hub và sẵn sàng bị commit.
2. **Preview luôn có unified diff:** text (theo luồng 4), `--json` (`full_diff`) và MCP. Ghi `--full-diff` vào help, hoặc bỏ cờ này. Đổi câu "No files changed" thành "Nothing written yet". Cỡ việc 2–3 giờ.
3. **Bộ in text phải tôn trọng `status: error` và exit code.**
   - `skill update` khi không có gì để cập nhật, hoặc khi skill không có upstream: hiện in "from - to -" với pin rỗng và trả exit 0.
   - `skill add <dir>` thêm lại một thư mục không đổi: báo "Draft added" sai sự thật.
   - `--ref <sha>` cùng `--yes` báo "proposal is stale" thay vì "reference not found".
   - `skill upstream` không được gợi ý `update` khi đã cập nhật.
   - Cỡ việc khoảng 0.5 ngày.
4. **Cảnh báo trust khi sửa skill bên thứ ba:** preview của edit nói rõ "agents get no content until you re-approve", giống như preview của update đã làm. Resolver không trả skill đang `review_required` làm primary, hoặc trả kèm chú thích. Cỡ việc khoảng 0.5–1 ngày.
5. **Gợi ý commit sau activate/edit:** dùng cùng lệnh `add -- … && commit` như `status`. Cỡ việc 0.5 giờ.
6. **FIX khi activate với starter text:** in `--editor` hoặc `--content-file <file>`. Cỡ việc 0.5 giờ.
7. **FIX khi repo vượt 8 MiB:** "dùng `--path` hoặc `--skill`". Cỡ việc 0.5 giờ.
8. **In cảnh báo license ở chế độ text.** Cỡ việc 0.5 giờ.
9. **"Next" sau `--approve-content`:** chỉ tới routing hoặc activate, không phải `--editor`. Cỡ việc 0.5 giờ.
10. **`status` sau khi sửa file trực tiếp:** thông báo nhẹ (hoặc tự rebuild), không phải `FAILED` + `doctor --fix`. Thống nhất thông điệp giữa form create, curator và doc 07. Cỡ việc 2 giờ.
11. **Update preview với `--manual`:** không ghi "needs decision" cho file đã được giải quyết. Cỡ việc 1 giờ.

### P1: thay đổi mô hình, sau khi người dùng chọn hình dạng lệnh (khoảng 5–6 ngày)

12. **Lấy ra, sửa, đẩy về (mô hình A; luồng 1 và 2).** Một cơ chế dùng chung cho UC3, UC4 và UC5 cùng máy: sửa nhiều file, cập nhật từ thư mục, tạo skill mới từ thư mục, merge 3 bên khi hub đã đổi. Cỡ việc khoảng 3 ngày.
13. **UX cập nhật upstream (luồng 4, 5 và 6):** tóm tắt trước diff, hỏi xung đột từng file khi ở TTY, `upstream` không gợi ý no-op, duyệt nội dung ngay sau khi xem diff. Cỡ việc khoảng 1–1.5 ngày.
14. **Port Git host** (phần [Port cho Git host](#port-cho-git-host)): hành vi giữ nguyên, chỉ GitHub. Cỡ việc khoảng 0.5–1 ngày.
15. **Form create hỏi 1–3 ví dụ và mở editor ở cuối;** thống nhất collection mặc định giữa `add` và `create`. Cỡ việc khoảng 0.5 ngày.
16. **findability-lint phase 1** (lỗi decode trong review) và nối lint vào preview (edit, create, đẩy về). Phase 1 mất vài giờ; phần nối vào preview theo plan.

### P2

17. **UC5 hub ở xa** (luồng 3: nộp qua MCP hoặc đi qua Git). Đây là mức P2 vì nó mở rộng bề mặt bảo mật và cần người dùng chọn hướng trước. Cỡ việc khoảng 1–3 ngày, tùy phương án.
18. Trial scope (phương án C), chỉ khi vẫn cần sau khi đã có mô hình A.
19. Giảm nhiễu `history/operations` (gộp receipt theo phiên, hoặc chỉ giữ cho thay đổi bên thứ ba). Cần người dùng quyết.
20. Dọn thư mục cache cũ theo phiên bản trong `runtime/cache/skills`.
21. Không đánh dấu source "due" ngay sau khi add; thống nhất cách đếm file giữa preview và review. `--ref` nhận commit SHA (hiện chỉ nhận branch hoặc tag).
22. Khi thêm GitLab tự host: chính sách địa chỉ cho mạng nội bộ và credential cho repo riêng tư (nằm ngoài port ở P1 #14).

---

## Câu hỏi mở cho người dùng

1. **Luồng 1:** chọn cặp động từ riêng `checkout`/`checkin` (1A), hay một chế độ của `edit` với `--here`/`--from` (1B)? Thư mục làm việc đặt mặc định ở `.claude/skills/<id>` (Claude Code nạp native) hay ở một chỗ trung tính?
2. **Luồng 2:** đẩy skill mới lên bằng `add <dir>` có preview đầy đủ (2A), hay bằng một động từ chung cho cả mới và cập nhật (2B)? Nếu là 2B, có muốn dùng chữ khác "publish" để khỏi bị hiểu thành activate không?
3. **UC5 hub ở xa (luồng 3):** có thật sự cần agent ở máy A nộp skill lên không? Nếu cần, chọn nộp qua MCP, chấp nhận mở rộng token runtime (3A), hay đi qua Git, cần quyền push vào remote của hub (3B)?
4. **Trust của skill viết trong project:** skill tạo từ project cùng máy coi là first-party như hiện nay (không cần `--approve-content`), còn skill nộp từ máy khác coi là chưa duyệt. Anh/chị đồng ý với cách chia này không?
5. **Luồng 4 và 6:** preview mặc định là "tóm tắt + `--diff`" (4A) hay "diff đầy đủ qua pager" (4B)? Duyệt nội dung gắn vào confirm (6A) hay là một động từ `approve` riêng có hỏi y/N (6B)?
6. **Luồng 5:** giữ `outdated`/`upstream`/`update` (5A) hay gộp thành một `skill update` (5B)?
7. Có chấp nhận để gate plan `261014-1650` đợi mô hình A không? Nếu gate làm trước thì trong một thời gian, cách duy nhất để thử draft là activate.
8. Receipt `history/operations/*.yaml` có còn cần commit cho mỗi lần sửa không, khi Git diff đã là bằng chứng?
9. Có muốn ghi việc sửa trực tiếp trong hub (phương án B) vào docs như một đường chính thức cho người làm ngay trong hub, và nới lệnh cấm trong curator cho skill tự viết không?
