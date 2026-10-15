# Debug: `make remote-bench` trượt budget ở `skill_get` lấy từ cache

Ngày 2026-10-15 (Asia/Saigon). Worktree tại 425f59e (chứa 4e30276).

## Kết luận

Đây không phải regression của đường cache. `skill_get` lấy từ cache vẫn trả lời từ cache của client
mà không phải chờ mạng (2 đến 4 ms, kể cả trên link 100 ms). Một request 384 đến 389 byte là lần
xác nhận load chạy ngầm (`confirmLoad`), một GET có điều kiện tới `/mcp/bundle`. Hub trả 304, không
có nội dung. Từ commit bafc3c5 (2026-10-12, có từ v0.5.1), hub đếm load riêng cho mỗi resolution,
nên request này được gửi một lần cho mỗi resolution mới. Thay đổi đó là chủ ý và có trong
`docs/design/04-telemetry-reproducibility-evaluation.md`. Phần kiểm tra "Changes on the hub" của bench
được viết trước đó (fc2cdb6) và vẫn đòi `requests 0, bytes 0`. Đây là trường hợp (b): budget đã cũ.
Commit 383b684 và các commit bị nghi khác không liên quan.

## Triệu chứng chính xác

Chạy `make remote-bench` (fixture, HOME/XDG do `TestMain` cô lập) ở 425f59e cho thấy hàng
"cached repeat" của từng transport **PASS** với 0 request. Chỉ các hàng trong "Changes on the hub" bị
**FAIL**. Ở mỗi hàng đó, bench gọi một `skill_resolve` mới trước `skill_get`:

```
FAIL  loopback, metadata only, every skill: skill_get test-audit       hit, requests 1, bytes 384
FAIL  loopback, metadata only, every skill: skill_get herdr-cook-plan  hit, requests 1, bytes 389
FAIL  loopback, content of herdr-cook-plan: skill_get test-audit       hit, requests 1, bytes 384
FAIL  slow link, ... (giống trên)
FAIL  slow link, content of herdr-cook-plan: skill_get herdr-cook-plan hit, requests 1, bytes 389  (budget: fetched)
```

## Request bị gửi và đường code

- Request là `GET /mcp/bundle?skill=<id>` với `Accept: <BundleMediaType>`,
  `If-None-Match: "<digest đang cache>"` và `Mcp-Session-Id: <hub session>`. Hub trả 304 cùng `ETag`.
  Hai skill chênh nhau 5 byte, đúng bằng chênh lệch độ dài ID trong query (`test-audit` và
  `herdr-cook-plan`).
- Đường gọi là `Proxy.skillGet` (`internal/delivery/mcpserver/proxy.go:640-646`). Khi có hint từ
  resolution và cache trúng, proxy gọi `proxy.confirmLoad(skillID, hint)` rồi trả `localResult(..., "hit")`
  ngay. `confirmLoad` (`proxy.go`, dưới `markCounted`) chạy `requestBundle(ctx, skillID, []string{hint.digest}, true)`
  trong một goroutine, trừ khi `counted[countedKey(sessionID, hint.resolutionID, skillID, digest)]` đã có.
- Từ bafc3c5, khóa `countedKey` có thêm `resolutionID`. Vì vậy mỗi resolution mới cho cùng phiên bản
  sẽ gửi đúng một request ngầm. Trước bafc3c5, khóa chỉ gồm session, skill và digest, nên lần thứ hai
  trong cùng session không gửi gì.

## Bisect

Bisect chạy thủ công, mỗi bước là một lệnh `TestRemoteBudgetBench` với `SKILLHUB_REMOTE_RUNS=3`. Một
bước là bad khi có hàng "metadata only" hoặc "content of" bị trượt.

| Commit | Kết quả |
|---|---|
| fc2cdb6 (commit đưa vào bench và budget, 2026-10-11) | good (mọi hàng change là 0 request) |
| 42bcab7, 78af2c2 (= bafc3c5^), c601b2f | good |
| 2fbfff4, ec90bae, 3d387ed, 42f9381 | bad |
| **bafc3c5** fix(telemetry): count a remote skill load again under a new resolution | **first bad** |

Mọi commit bị nghi (7404bb7, 383b684, 2516de0, fb95038, 5a94f99, 60ea147) đều có ngày 2026-10-14,
nghĩa là chúng nằm sau commit bad đầu tiên. Vì vậy chúng không gây ra lỗi này.

Lưu ý khi tái lập: lần đầu tôi chạy bằng `git bisect run`. Lệnh `skillhub init` của bench lúc đó ghi
vào chính worktree (`.gitignore`, `AGENTS.md` và index). Rất có thể biến môi trường git mà
`bisect run` truyền cho script đã làm init chọn nhầm repository. Tôi đã khôi phục worktree bằng
`git reset --hard` và chạy lại thủ công, kiểm tra `git status` sạch sau mỗi bước. Nguyên nhân chính
xác của việc ghi nhầm này chưa được chứng minh.

## Thí nghiệm chứng minh (đã hoàn tác)

1. Tôi đo thời gian từng `skill_get` trong phần change, rồi đếm lại traffic sau 3 giây.
   - Trên link 100 ms, các lần hit mất 2.3 đến 3.6 ms, ít hơn nhiều so với một round trip. Request vì
     thế không nằm trên đường agent phải chờ.
   - Sau khi traffic ổn định, mỗi lần hit có 1 request và 549 byte: 384 hoặc 389 byte request, cộng
     khoảng 165 byte cho câu trả lời 304 không có body.
   - Khi có 3 giây chờ, hàng "slow link, content of herdr-cook-plan" trở lại `fetched, 1 request, 7533 B`.
     Trong lần chạy gốc, hàng này là `hit` vì một race. Request ngầm của pha metadata vẫn đang đi
     trên link chậm khi nội dung đổi. Hub trả 200 với digest mới, và proxy cài bản đó vào cache ở nền.
     Lần `skill_get` sau trúng cache. Việc kiểm tra digest trong bench vẫn đạt, nên agent nhận đúng
     nội dung mới.
2. Tôi tạm tắt `proxy.confirmLoad(skillID, hint)`. Khi đó mọi hàng change trở về 0 request và 0 byte,
   hàng content change là `fetched 1`, và `TestRemoteBudgetBench` PASS.

## Câu hỏi về cổng servable

Commit 383b684 chỉ đổi `skill_get` ở phía hub. Nó không chạm vào cache của proxy. Cache chỉ được dùng
khi có hint từ một `skill_resolve` trong vòng `proxyHintTTL`, tức 5 phút (`cacheTTLMS`), và resolution
chỉ đề xuất skill servable. Nếu không có hint, proxy gửi request đồng bộ. Hub trả 404
`skill_not_found` cho skill draft hoặc unservable (`bundleHTTPError`), proxy chuyển tiếp `skill_get`, và
hub trả `not_found` hoặc `not_servable`. Như vậy client biết một skill đã thành unservable qua
resolution kế tiếp. Còn một khe hở tồn tại từ trước, không do bafc3c5 hay 383b684 gây ra: trong vòng
TTL 5 phút của một hint, proxy vẫn phục vụ bản cache dù skill vừa thành unservable. Nếu request xác
nhận ngầm nhận 404, kết quả bị bỏ qua và hint không bị xóa.

## Đề xuất (chưa thay đổi budget)

Budget "later use of the same version: at most 20 ms, zero bytes of skill content" trong
`docs/design/remote-hub.md` vẫn đạt. Điều đã cũ là cách bench kiểm tra các hàng change. Đổi ngưỡng là
quyết định của owner, nên tôi chỉ đề xuất:

1. Ở mỗi hàng change là hit, bench nên kiểm tra hai thứ tách biệt. Đường agent chờ phải có
   0 request và 0 byte. Ngoài ra chỉ cho phép tối đa một request xác nhận ngầm được trả lời 304,
   không có body nội dung. Hàng skill có nội dung đổi vẫn phải là `fetched, requests 1`.
2. Trước khi đổi hub và trước khi đọc bộ đếm, bench phải chờ request ngầm xong. Có thể chờ đến khi
   bộ đếm của link ổn định, hoặc đóng proxy và đếm lại. Như vậy race trên link chậm biến mất.
3. Cập nhật đoạn "Changes on the hub" trong `docs/design/remote-hub.md`: mỗi `skill_get` sau một
   resolution mới gửi một GET 304 ngầm để đếm load.
4. Phương án khác nếu owner muốn giữ "0 request": gộp lần xác nhận load vào request MCP kế tiếp, hoặc
   vào `skill_feedback`. Đổi lại, telemetry sẽ trễ hơn và có thể mất load khi session kết thúc trước
   request đó. Đây là thay đổi thiết kế telemetry chứ không phải bản sửa lỗi.

## Kết quả kiểm thử

- `make remote-bench` ở 425f59e: FAIL, 7 hàng change, như mô tả ở trên.
- `TestRemoteBudgetBench` ở fc2cdb6, 42bcab7, 78af2c2 và c601b2f: PASS. Ở bafc3c5 và các commit sau: FAIL.
- Khi tạm tắt `confirmLoad`: PASS. Thay đổi tạm đã được hoàn tác, worktree không còn thay đổi code.

## Đã triển khai (người dùng chọn đề xuất 1 đến 3)

Hành vi sản phẩm không đổi: `confirmLoad` vẫn giữ nguyên. Mọi thay đổi nằm trong code test và tài liệu.

- **Relay của bench** (`internal/delivery/mcpserver/proxy_test.go`, `benchLink`). Relay giờ đọc từng
  exchange HTTP trên mỗi kết nối, gồm method, path, có `If-None-Match` hay không, status, số byte body,
  và thời điểm câu trả lời tới proxy. Relay cũng có `hold` và `release` để giữ mọi thứ hub gửi về cho
  tới khi được thả.
- **Các hàng "Changes on the hub"** (`measureChangeGet` và `changeRow.verdict` trong
  `remote_budget_bench_test.go`) đo hai phần riêng.
  - *Waited* là các request có câu trả lời tới proxy trước khi `skill_get` trả về. Khi chờ một hit, bench
    giữ câu trả lời của hub trong suốt lần gọi. Một hit cần hub sẽ không trả về được, và sau 5 s bench
    thả ra rồi tính request đó là waited.
  - *Background* là các request còn lại.
  - Budget cho hit: waited phải là 0 request và 0 byte. Background được tối đa 1 request, và request đó
    phải là `GET /mcp/bundle` có điều kiện, được trả 304 với body 0 byte.
  - Budget cho skill có nội dung đổi: 1 request waited và không có gì ở background.
  - Các hàng cached repeat vẫn đòi tổng cộng 0 request, không nới.
- **Chờ request ngầm.** Trước mỗi thay đổi trên hub và trước mỗi lần đo, bench chờ tới khi relay đã
  giao hết mọi câu trả lời. Proxy là một process riêng và không báo lúc request ngầm xong, nên không có
  hook nào chỉ thêm trong code test được. Vì vậy bench đọc trạng thái này từ relay. Sau một hit, bench
  chờ tới khi relay thấy request của proxy và đã giao câu trả lời. Giới hạn 2 s chỉ có tác dụng khi hit
  không gửi request ngầm nào, trường hợp mà budget cho phép. Race trên link chậm vì thế đã hết.
- **Tài liệu.** `docs/design/remote-hub.md` mô tả request ngầm, cách đo waited và background, và budget
  mới.
- **Test** chạy trong `go test` thường (`remote_budget_check_test.go`).
  - `TestHubChangeVerdict` thất bại khi request ngầm chuyển sang đường agent chờ, khi nó có body hay là
    200, khi có 2 request, hoặc khi request không được trả lời.
  - `TestBenchLinkRecordsHeldExchanges` kiểm tra rằng câu trả lời bị giữ không tới client, và status
    cùng kích thước body được ghi đúng.

## `skillhub init` ghi vào worktree khi chạy dưới `git bisect run`

Nguyên nhân là `git bisect run` export `GIT_DIR=/home/vantt/projects/mcp-skill-hub/.git/worktrees/agent-af0558e1f4795fefc`
cho script. Tôi đã kiểm chứng bằng `git bisect run env`. Bench truyền nguyên môi trường đó
(`os.Environ()`) cho `skillhub init` chạy trong thư mục tạm. Git ưu tiên `GIT_DIR` hơn thư mục làm
việc, nên init stage file vào index của worktree. Git hook cũng export `GIT_DIR`.

Cách sửa là `TestMain` của `mcpserver` gỡ `GIT_DIR`, `GIT_WORK_TREE`, `GIT_INDEX_FILE`,
`GIT_COMMON_DIR`, `GIT_PREFIX`, `GIT_OBJECT_DIRECTORY`, `GIT_ALTERNATE_OBJECT_DIRECTORIES` và
`GIT_NAMESPACE`. `TestTestsRunOutsideTheCallersRepository` bảo vệ điều này. Khi chạy với
`GIT_DIR=/nonexistent/git`, test này thất bại nếu không có phần gỡ, và đạt khi có. Các package khác
có `TestMain` riêng chưa được sửa.

## Kết quả kiểm thử sau thay đổi

- `make remote-bench` (fixture, loopback và link 100 ms) đạt cả 26/26 budget. Mỗi hit trong phần
  change có waited `0 / 0` và một background `304 (body 0 B)`, tổng 549 đến 554 B trên link. Hàng
  `herdr-cook-plan` có nội dung đổi là `fetched`, waited 1 request, không có background, trên cả hai link.
- Tạm cho `skill_get` gửi request xác nhận ngay trên đường agent chờ (đã hoàn tác): các hàng change
  bị FAIL với `hit, waited: requests 1, bytes 165; background: none`, và các hàng cached repeat cũng FAIL.
  Vậy budget phát hiện được khi request ngầm chuyển sang đường agent chờ.
- `go test ./internal/delivery/mcpserver/` đạt, `make check` đạt (golangci-lint báo 0 issue).
  Ba test mới cũng đạt khi chạy với `-race`.
- Sau mọi lần chạy, `git status` chỉ còn các thay đổi của tôi. `skillhub init` không ghi vào worktree.

Có một lỗi chập chờn có từ trước, không liên quan tới thay đổi này. Khoảng 4 trên 55 lần chạy
`TestRemoteBudgetBench` thất bại với `hub_unreachable` trong phase đo loopback. Exchange log của relay
mới cho thấy nguyên nhân: hub trả `429 too many requests; slow down` cho `POST /mcp` ở session thứ 16
của loopback. Câu trả lời đó đến từ bộ giới hạn toàn cục của listener (`httpRequestsPerSecond = 100`,
`httpRequestBurst = 200` trong `internal/delivery/mcpserver/http.go`, có từ d3d92ff). Phase loopback gửi
khoảng 18 request cho mỗi session trên 20 session, liên tiếp nhau. Khi máy chạy nhanh, tốc độ vượt
100 request/giây và hết burst. Relay không thể tự sinh ra 429. Proxy coi 429 là `hub_unreachable`.
Tôi không sửa phần này vì nằm ngoài phạm vi.

## Câu hỏi chưa giải quyết

- Khi request xác nhận ngầm nhận 404, có nên xóa hint để đóng khe hở 5 phút không?
- Với 429 nói trên, nên giãn nhịp phase loopback của bench, hay để proxy coi 429 là "hub bận, thử lại"
  thay vì `hub_unreachable`?
- Có nên gỡ biến `GIT_*` trong `TestMain` của các package khác có test tạo repository Git
  (`internal/app`, `internal/delivery/cli`, `internal/delivery/web`, `internal/hostintegration`) không?
