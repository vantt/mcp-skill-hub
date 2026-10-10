# Wave 5 · Worktree K round 3 prompt

Paste into the same agent/worktree (`/home/vantt/projects/mcp-skill-hub-connect-cli`).

```text
Worktree K, round 3 (one small fix). Round 2 is accepted: disconnect and
receipts are gone, the servable fix works (a fresh clone of the live hub now
rebuilds with no warnings), the real HOME stayed unchanged.

One defect in fec78b8: with the user's normal shell environment (GOMODCACHE,
GOCACHE and GOPATH NOT set as env vars, so Go derives them from HOME), `make
check` fails:

  unlinkat /tmp/skillhub-mcpserver-tests-.../HOME/go/pkg/mod/modernc.org/mathutil@v1.7.1/test_deps.go: permission denied
  FAIL github.com/vantt/mcp-skill-hub/internal/delivery/mcpserver

internal/delivery/mcpserver/subprocess_test.go runs `go` with the temp HOME,
so Go downloads the whole module cache into the temp HOME (read-only files,
slow), and TestMain's os.RemoveAll then fails and sets the exit code to 1.
Your runs passed only because you exported the Go cache paths.

Fix: in every TestMain you added (app, cli, mcpserver, web, hostintegration),
before overriding HOME/XDG, pin GOCACHE, GOMODCACHE and GOPATH to their
current resolved values (e.g. from `go env`, or os.UserCacheDir/build defaults)
when they are not already set, so subprocess `go` keeps using the user's caches.
Keep cleanup strict, but a cleanup failure must not hide test results silently:
make it fail with a clear message only for files the tests created. Add a
short comment saying why.

Done means `make check` passes in a shell where `env -u GOMODCACHE -u GOCACHE
-u GOPATH make check` (real HOME, nothing else exported), and the real HOME
host files are unchanged (same comparison as round 2). One commit. Report the
commit, the command output tail, and the HOME hashes.
```
