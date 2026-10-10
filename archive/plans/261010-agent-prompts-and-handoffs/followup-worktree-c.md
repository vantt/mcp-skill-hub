# Follow-up for Worktree C: Server-Side O3 Metrics and Error Handling

> **Closed 2026-10-10.** Items 1–2: the O3 counters `unsupported_method_calls` and
> `unlisted_resource_reads` were removed (C round 3); `directoryRead` is deferred entirely.
> Item 3: `snapshot_expired_requests` remains in `internal/app/usage.go`.


**Worktree C is actively rewriting `internal/delivery/mcpserver/server.go`.**
Worktree F (observer fixes) was instructed not to touch `server.go`. This document specifies the exact changes required in `server.go` for Worktree C.

---

## 1. `unsupported_method_calls` Counter Cannot Measure

### Issue
In `server.go:157-160`:
```go
func (adapter *Server) telemetryMiddleware(next mcp.MethodHandler) mcp.MethodHandler {
    return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
        if strings.HasPrefix(method, "skills/") && method != "skills/list" && method != "skills/get" {
            app.RecordServerMetric(ctx, adapter.telemetry, adapter.workspace, app.ServerMetric{Name: "unsupported_method_calls", Value: 1, Client: adapter.clientForReq(req)})
        }
        ...
```
In `go-sdk`, unknown methods are rejected at the protocol dispatch level *before* any middleware is invoked. As a result, `unsupported_method_calls` in middleware is unreachable and always records 0.

### Recommended Change in `server.go`
Either:
1. Intercept unknown method invocations at the JSON-RPC transport / message handler level before protocol dispatch rejects them, or
2. Remove the `unsupported_method_calls` metric if transport-level interception is not supported by the upstream SDK.

---

## 2. `unlisted_resource_reads` Counter False Positives

### Issue
In `server.go:298-313`:
```go
    var unlisted bool
    entries, _, lookupErr := adapter.distribution.LookupSkills(ctx, adapter.workspace, []string{content.SkillID})
    if lookupErr == nil {
        if entry, ok := entries[content.SkillID]; ok {
            unlisted = true
            for _, r := range entry.Resources {
                if r.URI == request.Params.URI {
                    unlisted = false
                    break
                }
            }
        }
    }
    if unlisted {
        app.RecordServerMetric(ctx, adapter.telemetry, adapter.workspace, app.ServerMetric{Name: "unlisted_resource_reads", Value: 1, SkillID: content.SkillID, Client: adapter.callerContext(request.Session).Client})
    }
```
`ReadResource` already validates whether a resource is servable. Looking up `entry.Resources` in the active catalog snapshot only detects discrepancies if the catalog generation changed between `resources/list` and `resources/read`, causing false positives when an active catalog re-indexes.

### Recommended Change in `server.go`
Either:
1. Compare against the client session's pinned manifest snapshot rather than the current live catalog, or
2. Remove `unlisted_resource_reads` if manifest snapshot diffing is not tracked per session.

---

## 3. `snapshot_expired_requests` Must Not Count `ErrNotFound`

### Issue
In `server.go:450-452`:
```go
    case errors.Is(err, skill.ErrSnapshotExpired), errors.Is(err, skill.ErrNotFound):
        app.RecordServerMetric(ctx, adapter.telemetry, adapter.workspace, app.ServerMetric{Name: "snapshot_expired_requests", Value: 1, Client: adapter.callerContext(session).Client})
        return invalidParams("snapshot_expired", "The requested skill snapshot is unavailable; refresh the skill entry.")
```
`snapshot_expired_requests` incorrectly increments when a requested skill is simply not found (`skill.ErrNotFound`). Missing skills are invalid requests or catalog misses, not expired snapshots.

### Recommended Change in `server.go`
Separate `skill.ErrSnapshotExpired` from `skill.ErrNotFound`:
```go
    case errors.Is(err, skill.ErrSnapshotExpired):
        app.RecordServerMetric(ctx, adapter.telemetry, adapter.workspace, app.ServerMetric{Name: "snapshot_expired_requests", Value: 1, Client: adapter.callerContext(session).Client})
        return invalidParams("snapshot_expired", "The requested skill snapshot is unavailable; refresh the skill entry.")
    case errors.Is(err, skill.ErrNotFound):
        return invalidParams("skill_not_found", "The requested skill was not found in the active catalog.")
```
