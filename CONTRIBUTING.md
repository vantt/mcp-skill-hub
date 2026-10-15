# Contributing to Skill Hub

This page is for developers who build, test or release Skill Hub.
You need Go 1.26.9 (from `go.mod`) and, for the web UI, Node.js 24 (from `web/.nvmrc`).

## Build and test

Run the full gate before you commit. It runs:

- the gofmt check and `go vet`, for this platform and for Windows;
- the lint check for new issues since `origin/main`;
- the full test suite.

```bash
make check
```

Run the full suite with the race detector.

```bash
make test-race
```

List every lint issue, including existing debt.

```bash
make lint-all
```

Run the command from source to try a change.

```bash
go run ./cmd/skillhub help
```

The web UI has its own targets:

- `make web-check`: frontend typecheck, lint, unit tests and build.
- `make web-e2e`: build the UI and binary, then run Playwright against the real server.
- `make web-shots`: capture the README and guide screenshots from an invented demo hub into `docs/images/web/`.
  Set `SHOTS_OUT_DIR` to write them somewhere else.

### Wait for CI on every platform before merging

Local gates run on one platform. Code that compiles everywhere can still fail at run time on another one: file
modes do not exist on Windows, paths there have drive letters and backslashes, and commands printed for PowerShell
are joined with `; `. CI runs the test suite on Linux, macOS and Windows for every pull request. Push a branch or
open a pull request and wait until all three `Verify` jobs pass before merging a change into `main`, above all for
work made of several phases or merged locally. In a test, guard checks that are meaningless on Windows (file modes,
POSIX paths) by platform and keep every other assertion running everywhere.

## Keep tests isolated

Tests run in parallel by default. A test stays serial (no `t.Parallel()`) when it sets environment variables, changes the working directory or touches process-global state.

Some packages reach agent configuration: `app`, `cli`, `mcpserver`, `web` and `hostintegration`. They isolate `HOME`, `USERPROFILE` and the XDG directories in `TestMain`. This happens before parallel tests start, and subprocesses inherit it.

Running the suite never reads or writes your own agent configuration.

Before that isolation, `TestMain` fills in any missing `GOCACHE`, `GOMODCACHE` and `GOPATH` from `go env`. Subprocess builds then reuse your existing Go caches instead of filling the temporary home with read-only modules. Values you set yourself are kept.

## Release

Follow the [release runbook](docs/release-runbook.md) to build, sign and verify a release.

## Design records

Architecture and design decisions are listed in the maintainer section of the [docs index](docs/README.md); most records are written in Vietnamese.
