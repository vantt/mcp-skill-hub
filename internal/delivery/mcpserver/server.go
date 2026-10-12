// Package mcpserver exposes Skill Hub application services over MCP, on stdio
// for a local agent host and over Streamable HTTP for agents on other machines.
package mcpserver

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"runtime/debug"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/vantt/mcp-skill-hub/internal/app"
	"github.com/vantt/mcp-skill-hub/internal/delivery/paging"
	"github.com/vantt/mcp-skill-hub/internal/telemetry"
	"github.com/vantt/mcp-skill-hub/internal/version"
	"github.com/vantt/mcp-skill-hub/internal/workspace"
)

const (
	MaxFrameBytes = 4 << 20
	cacheTTLMS    = 300_000
)

type Profile string

const (
	ProfileAll      Profile = "all"
	ProfileRuntime  Profile = "runtime"
	ProfileCuration Profile = "curation"
)

type Server struct {
	workspace    string
	profile      Profile
	distribution app.DistributionService
	resolver     app.ResolverService
	feedback     app.FeedbackService
	source       app.SourceService
	curationUX   app.CurationTelemetryService
	snapshots    app.SnapshotService
	telemetry    app.TelemetrySink
	tracker      *activationTracker
	logger       *slog.Logger
	// remote marks a server whose sessions arrive through the network
	// listener: callers run on other machines, so their telemetry client names
	// carry the remote label and no response may carry a path on this machine's
	// disk. It is set once before the listener accepts requests and never
	// changes afterwards.
	remote bool

	// tools lists the tools of the profile on the server.
	tools *toolRegistry

	// toolsListSize caches the encoded size of the tools/list result for the
	// tool set identified by toolsListKey.
	toolsListMu   sync.Mutex
	toolsListKey  string
	toolsListSize int
}

// New constructs one stateless server surface for a configured workspace.
func New(workspacePath string, diagnostics io.Writer, profiles ...Profile) (*Server, *mcp.Server, error) {
	root, err := workspace.Discover(workspacePath)
	if err != nil {
		return nil, nil, err
	}
	if diagnostics == nil {
		diagnostics = io.Discard
	}
	logger := slog.New(slog.NewTextHandler(diagnostics, &slog.HandlerOptions{Level: slog.LevelWarn}))
	activeDiagnostics.Store(logger)
	profile := ProfileAll
	if len(profiles) > 0 && profiles[0] != "" {
		profile = profiles[0]
	}
	adapter := &Server{
		workspace: root,
		profile:   profile,
		snapshots: app.NewSnapshotService(),
		tracker:   newActivationTracker(nil, root),
		logger:    logger,
	}
	capabilities := &mcp.ServerCapabilities{
		Resources: &mcp.ResourceCapabilities{},
		Tools:     &mcp.ToolCapabilities{},
	}
	servesSkills := profile != ProfileCuration
	if servesSkills {
		capabilities.AddExtension("io.modelcontextprotocol/skills", map[string]any{"directoryRead": false})
	}
	server := mcp.NewServer(&mcp.Implementation{Name: "skillhub", Version: version.Current().Version}, &mcp.ServerOptions{
		Capabilities: capabilities,
		PageSize:     paging.MaximumLimit,
		Logger:       logger,
		SetCacheable: func(_ context.Context, _ mcp.Request, cache *mcp.Cacheable) {
			cache.TTLMs = cacheTTLMS
			cache.CacheScope = "private"
		},
	})
	server.AddReceivingMiddleware(adapter.telemetryMiddleware)
	if servesSkills {
		if err := adapter.registerSkills(server); err != nil {
			return nil, nil, err
		}
	}
	adapter.tools = newToolRegistry(server)
	adapter.registerTools(adapter.tools)
	return adapter, server, nil
}

// Serve performs startup validation/rebuild before accepting stdio frames.
func Serve(ctx context.Context, workspacePath string, diagnostics io.Writer, profiles ...Profile) error {
	_, server, stop, err := startServing(ctx, workspacePath, diagnostics, profiles...)
	if err != nil {
		return err
	}
	defer stop()
	err = server.Run(ctx, &mcp.StdioTransport{MaxLineLength: MaxFrameBytes})
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, io.EOF) {
		return nil
	}
	return err
}

// startServing prepares one long-lived server for any transport: it ensures
// the catalog, registers the tools of the profile, starts the idle memory
// releaser, collects old snapshots, and opens the telemetry recorder. Every
// session of the returned server shares the same tools, activation tracker,
// and recorder. stop flushes and closes the recorder, then stops the releaser;
// call it once, after the transport has stopped.
func startServing(ctx context.Context, workspacePath string, diagnostics io.Writer, profiles ...Profile) (*Server, *mcp.Server, func(), error) {
	if _, err := (app.CatalogService{}).EnsureCatalog(ctx, workspacePath); err != nil {
		if diagnostics == nil {
			diagnostics = os.Stderr
		}
		fmt.Fprintf(diagnostics, "warning: catalog ensure/rebuild failed on startup: %v\n", err)
	}
	adapter, server, err := New(workspacePath, diagnostics, profiles...)
	if err != nil {
		return nil, nil, nil, err
	}
	releaser := &idleMemoryReleaser{release: debug.FreeOSMemory}
	releaser.noteActivity(time.Now())
	server.AddReceivingMiddleware(releaser.middleware)
	releaseContext, stopRelease := context.WithCancel(ctx)
	releaseDone := make(chan struct{})
	go func() {
		defer close(releaseDone)
		releaser.run(releaseContext, memoryReleaseCheckInterval, memoryReleaseIdleAfter)
	}()
	if gcErr := adapter.snapshots.CollectGarbage(ctx, adapter.workspace, app.SnapshotGCMinAge); gcErr != nil {
		adapter.logger.Warn("local skill snapshot garbage collection failed", "error", gcErr)
	}
	recorder, telemetryErr := (app.TelemetryService{}).Open(adapter.workspace)
	if telemetryErr != nil {
		adapter.logger.Warn("MCP telemetry recorder could not be opened")
		recorder = nil
	} else {
		adapter.useRecorder(recorder)
	}
	stop := func() {
		if recorder != nil {
			closeServeTelemetry(recorder, adapter.logger)
		}
		stopRelease()
		<-releaseDone
	}
	return adapter, server, stop, nil
}

func (adapter *Server) useRecorder(recorder *telemetry.Recorder) {
	adapter.telemetry = recorder
	adapter.resolver.Telemetry = recorder
	adapter.feedback.Recorder = recorder
	adapter.source.Telemetry = recorder
	adapter.curationUX.Recorder = recorder
}

func closeServeTelemetry(recorder *telemetry.Recorder, logger *slog.Logger) {
	flushContext, cancelFlush := context.WithTimeout(context.Background(), 10*time.Second)
	if err := recorder.Flush(flushContext); err != nil {
		logger.Warn("MCP telemetry recorder could not be flushed")
	}
	cancelFlush()

	closeContext, cancelClose := context.WithTimeout(context.Background(), 10*time.Second)
	if err := recorder.Close(closeContext); err != nil {
		logger.Warn("MCP telemetry recorder could not be closed")
	}
	cancelClose()

	if health := recorder.Health(); health.Errors > 0 {
		logger.Warn("MCP telemetry recorder reported storage failures", "error_count", health.Errors)
	}
}
