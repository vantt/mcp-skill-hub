# Đánh giá UX curation qua CLI và agent (2026-10-14)

**Phạm vi:** thêm, tạo, sửa, thử và công bố skill qua `skillhub` CLI và qua agent (curator skill gọi CLI, hoặc MCP profile `curation`). Web UI do người đánh giá khác phụ trách; ở đây chỉ đọc code web để so sánh.
**Cách làm:** binary build từ `main` tại `f337753` (có `9cb61e4`), chạy trên một hub tạm (`skillhub init`), HOME và `XDG_*` tách riêng, một project tạm đã `connect`, một thư mục local và GitHub thật (`anthropics/skills`, `--path skills/pdf`) làm nguồn. Đường runtime được thử bằng cách nói chuyện thẳng với `skillhub mcp serve --profile runtime` qua stdio (JSON-RPC), giữ một phiên mở trong khi sửa skill. Hub thật `/home/vantt/skill-hub`, HOME thật và project thật không bị đụng tới.

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

1. **Cao: chỉ hỗ trợ github.com công khai.** Repo nội bộ trên GitLab, Gitea hay server tự host, và cả một bản clone local, đều không vào được với upstream được theo dõi. Cách vòng là `git clone` rồi `skill add <dir>`, nhưng khi đó origin thành "local authoring" và mất `skill update` (merge 3 bên). Với người dùng ở công ty, đây có thể là chặn cứng. Xem câu hỏi mở 1.
2. **Trung bình: lỗi repo quá lớn chỉ sai hướng.** `source exceeded bytes limit (8560432 > 8388608)` đi kèm `FIX: Correct the skill fields or workspace state and retry.` FIX đúng phải là "dùng `--path <thư mục skill>` hoặc `--skill <tên>`".
3. **Trung bình: cảnh báo license bị ẩn ở chế độ người đọc.** Preview dạng text không in cảnh báo "Proprietary"; cảnh báo này chỉ có trong `--json` (`warnings[].code = license_warning`). Người dùng gõ tay không bao giờ thấy nó.
4. **Trung bình: duyệt nội dung là thao tác hai bước dù người đã xem.** `--approve-content <digest>` tạo thêm một preview ("Skill edit is ready for review … No files changed"), phải chạy lại với `--yes`. Sau khi duyệt xong, dòng "Next" lại bảo "edit the instructions with `--editor`", trong khi bước kế tiếp thực ra là điền routing hoặc activate.
5. **Thấp:** ngay sau khi add, `status` đã báo "1 source is due for an update check", tức là thêm việc ngay ở lần đầu.

**Luồng lý tưởng:** `skill add <url>` → một màn hình gồm file, license, routing gợi ý và lệnh duyệt → người dùng gõ một lệnh confirm, trong terminal có thể kèm `--approve-content`, vì chính họ vừa đọc diff đó. Agent vẫn không bao giờ được duyệt. Hỗ trợ mọi URL HTTPS Git, không chỉ GitHub.

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

### Phương án A: `skillhub skill checkout` / `checkin` (đề xuất)

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
- **Công sức:** khoảng 3 ngày (checkout 0.5 ngày, checkin + merge + resource add/delete 1.5–2 ngày, curator/docs 0.5 ngày).

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

Chọn **A**. Nó là phương án duy nhất giải được cùng lúc cả ba khoảng trống (thử trước khi publish, sửa nhiều file, cập nhật từ thư mục). Hub chỉ làm đúng vai "cung cấp chức năng" như người dùng hình dung: export, import thành proposal, merge, duyệt. Git vẫn là database, và mỗi checkout chỉ thêm đúng một file đánh dấu nằm ngoài hub. Ghi B vào docs như đường cho người làm trực tiếp trong hub, kèm sửa thông điệp `status`. Để C cho sau, chỉ khi dùng A một thời gian mà vẫn thấy cần thử *routing* trong phiên thật; lint/eval trong preview của checkin có thể đã đủ.

---

## Ảnh hưởng tới các plan đang hoãn

- **`261014-1545-findability-lint`: giữ, nhưng đổi thứ tự và nơi hiển thị.** Phase 1 sửa một lỗi thật: `ReviewSkill` chưa bao giờ báo routing lint, vì `DecodeSkillDocument` chỉ đọc JSON. Phase này nên làm ngay, độc lập với phần còn lại. Giá trị lớn nhất của lint là hiện ngay trong preview của edit, create và checkin (câu hỏi mở 1 của plan: nối vào `RoutingImpactHook`). Đề xuất đưa việc đó vào phạm vi, vì hiện tại "Routing impact" trong preview chỉ là câu vô nghĩa ("Direct routing fields … were evaluated against sha256:…"). Nếu form create không hỏi ví dụ thì lint không có gì để kiểm, nên P1 #11 bên dưới là điều kiện để lint có tác dụng.
- **`261014-1540-session-mining-routing-proposals`: tiếp tục hoãn (P2).** Plan này cải thiện metadata của skill *đã active* dựa trên telemetry; nó không chạm tới nỗi đau hiện tại (thêm, tạo, sửa, thử, publish). Plan cần case journal (opt-in), và đầu ra `--from-suggestion` của nó là một edit preview, mà hiện edit preview chưa có diff. Nên làm sau P0 #1. Không cần đổi thiết kế.
- **`261014-1650-runtime-skill-get-gate` (không hoãn nhưng có liên quan):** quy tắc này đúng cho runtime sạch, nhưng nó gỡ đường thử draft duy nhất hiện có. Nên làm cùng lúc hoặc sau `checkout` (câu hỏi mở 2), và đổi lý do `not_servable` cho draft thành một mã dễ hiểu hơn (ví dụ `draft_not_served`).

---

## Danh sách cải tiến theo ưu tiên

### P0: nhỏ, sửa ngay (tổng khoảng 1.5 ngày)

1. **Preview luôn có unified diff:** text, `--json` (`full_diff`) và MCP; ghi `--full-diff` vào help hoặc bỏ cờ này. Đổi câu "No files changed" thành "Nothing written yet". Công sức 2–3 giờ.
2. **Cảnh báo trust khi sửa skill bên thứ ba:** preview nói rõ "agents get no content until you re-approve" và in sẵn lệnh duyệt. Resolver không trả skill đang `review_required` làm primary, hoặc trả kèm chú thích. Công sức khoảng 0.5–1 ngày.
3. **Thêm lại thư mục local không đổi:** text phải nói "already in the hub (active, edited since); nothing changed", và không in lệnh confirm với pin rỗng. Tương tự cho `skill update` trên skill không có upstream (`from - to - (source )`, exit 0). Công sức khoảng 2 giờ.
4. **Gợi ý commit sau activate/edit:** dùng cùng lệnh `add -- … && commit` như `status`. Công sức 0.5 giờ.
5. **FIX khi activate với starter text:** in `--editor` hoặc `--content-file <file>`. Công sức 0.5 giờ.
6. **FIX khi repo vượt 8 MiB:** "dùng `--path` hoặc `--skill`". Công sức 0.5 giờ.
7. **In cảnh báo license ở chế độ text.** Công sức 0.5 giờ.
8. **"Next" sau `--approve-content`:** chỉ tới routing hoặc activate, không phải `--editor`. Công sức 0.5 giờ.
9. **`status` sau khi sửa file trực tiếp:** thông báo nhẹ (hoặc tự rebuild), không phải `FAILED` + `doctor --fix`. Thống nhất thông điệp giữa form create, curator và doc 07 về việc sửa trực tiếp. Công sức 2 giờ.

### P1: thay đổi mô hình (khoảng 5 ngày)

10. **`skill checkout` / `checkin`** (phương án A). Cơ chế này gộp việc sửa resource cho CLI/agent và cập nhật từ thư mục. Công sức khoảng 3 ngày.
11. **Form create hỏi 1–3 ví dụ và mở editor ở cuối;** thống nhất collection mặc định giữa `add` và `create`. Công sức khoảng 0.5 ngày.
12. **Duyệt nội dung một bước khi người ở TTY:** `--approve-content` không tạo thêm preview; cho phép kèm vào confirm của edit/checkin. Công sức khoảng 2 giờ.
13. **findability-lint phase 1** (lỗi decode trong review) và nối lint vào preview. Phase 1 mất vài giờ; phần nối vào preview theo plan.
14. **Locator Git tổng quát:** mọi host HTTPS, hoặc ít nhất ghi lại upstream cho một bản clone local. Công sức 1–2 ngày, tùy câu trả lời ở câu hỏi mở 1.

### P2

15. Trial scope (phương án C), chỉ khi vẫn cần sau khi đã có A.
16. Giảm nhiễu `history/operations` (ví dụ gộp receipt theo phiên, hoặc chỉ giữ cho mutation bên thứ ba). Cần người dùng quyết.
17. Dọn thư mục cache cũ theo phiên bản trong `runtime/cache/skills`.
18. Không đánh dấu source "due" ngay sau khi add; thống nhất cách đếm file giữa preview và review.

---

## Câu hỏi mở cho người dùng

1. Skill của anh/chị có nằm ở GitLab, Gitea hay server nội bộ không? Nếu có, P1 #14 lên P0, vì hiện chỉ có `github.com` công khai.
2. Có chấp nhận để gate plan `261014-1650` đợi `checkout` không? Nếu gate làm trước thì trong một thời gian, cách duy nhất để thử draft là activate.
3. Khi chính anh/chị (trong terminal) sửa một skill bên thứ ba đã duyệt, có muốn duyệt luôn trong cùng lần confirm không, hay vẫn giữ hai bước riêng?
4. Thư mục checkout nên đặt ở đâu mặc định: `.claude/skills/<id>` (Claude Code nạp native) hay một thư mục trung tính như `.skillhub/checkout/<id>`, rồi chỉ agent cách nạp?
5. Receipt `history/operations/*.yaml` có còn cần commit cho mỗi lần sửa không, khi Git diff đã là bằng chứng?
6. Có muốn ghi sửa trực tiếp trong hub (phương án B) vào docs như một đường chính thức cho người dùng, và nới lệnh cấm trong curator cho skill tự viết, không?
