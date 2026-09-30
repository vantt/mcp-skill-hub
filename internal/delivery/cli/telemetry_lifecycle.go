package cli

import (
	"context"
	"time"

	"github.com/vantt/mcp-skill-hub/internal/app"
)

const commandTelemetryTimeout = 10 * time.Second

type commandTelemetryRecorder interface {
	app.TelemetrySink
	Flush(context.Context) error
	Close(context.Context) error
}

var openCommandTelemetry = func(workspacePath string) (commandTelemetryRecorder, error) {
	return (app.TelemetryService{}).Open(workspacePath)
}

// startCommandTelemetry opens one disposable recorder for a command invocation.
// Every telemetry lifecycle operation is deliberately silent and panic-safe so
// it cannot alter command output, exit status, or the primary domain error.
func startCommandTelemetry(workspacePath string, inject func(app.TelemetrySink)) func() {
	recorder := safelyOpenCommandTelemetry(workspacePath)
	if recorder == nil {
		return func() {}
	}
	inject(recorder)
	return func() {
		flushCtx, cancelFlush := context.WithTimeout(context.Background(), commandTelemetryTimeout)
		safelyRunTelemetry(func() { _ = recorder.Flush(flushCtx) })
		cancelFlush()

		closeCtx, cancelClose := context.WithTimeout(context.Background(), commandTelemetryTimeout)
		safelyRunTelemetry(func() { _ = recorder.Close(closeCtx) })
		cancelClose()
	}
}

func safelyOpenCommandTelemetry(workspacePath string) (recorder commandTelemetryRecorder) {
	defer func() {
		if recover() != nil {
			recorder = nil
		}
	}()
	opened, err := openCommandTelemetry(workspacePath)
	if err != nil {
		return nil
	}
	return opened
}

func safelyRunTelemetry(operation func()) {
	defer func() { _ = recover() }()
	operation()
}
