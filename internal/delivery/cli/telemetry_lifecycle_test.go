package cli

import (
	"context"
	"errors"
	"testing"

	"github.com/vantt/mcp-skill-hub/internal/app"
	"github.com/vantt/mcp-skill-hub/internal/telemetry"
)

type lifecycleRecorder struct {
	flushed    int
	closed     int
	panicFlush bool
	panicClose bool
}

func (*lifecycleRecorder) Record(telemetry.Event) {}

func (recorder *lifecycleRecorder) Flush(context.Context) error {
	recorder.flushed++
	if recorder.panicFlush {
		panic("private flush failure")
	}
	return errors.New("private flush failure")
}

func (recorder *lifecycleRecorder) Close(context.Context) error {
	recorder.closed++
	if recorder.panicClose {
		panic("private close failure")
	}
	return errors.New("private close failure")
}

func TestCommandTelemetryLifecycleIsBestEffortAndAlwaysCloses(t *testing.T) {
	original := openCommandTelemetry
	t.Cleanup(func() { openCommandTelemetry = original })

	recorder := &lifecycleRecorder{panicFlush: true, panicClose: true}
	openCommandTelemetry = func(string) (commandTelemetryRecorder, error) { return recorder, nil }
	var injected app.TelemetrySink
	finish := startCommandTelemetry("workspace", func(sink app.TelemetrySink) { injected = sink })
	if injected != recorder {
		t.Fatal("recorder was not injected")
	}
	finish()
	if recorder.flushed != 1 || recorder.closed != 1 {
		t.Fatalf("lifecycle calls: flush=%d close=%d", recorder.flushed, recorder.closed)
	}
}

func TestCommandTelemetryOpensOncePerBatchAfterArgumentsResolve(t *testing.T) {
	original := openCommandTelemetry
	t.Cleanup(func() { openCommandTelemetry = original })

	recorder := &lifecycleRecorder{}
	opens := 0
	openCommandTelemetry = func(string) (commandTelemetryRecorder, error) {
		opens++
		return recorder, nil
	}
	root := newSourceCLIWorkspace(t)
	code, _, _ := runCLIForTest([]string{"source", "capture", "--workspace", root, "--json"})
	if code != 2 || opens != 0 {
		t.Fatalf("invalid invocation: code=%d opens=%d", code, opens)
	}
	code, stdout, stderr := runCLIForTest([]string{"check", "missing-a", "missing-b", "--workspace", root, "--json"})
	if code != 2 || opens != 1 || recorder.flushed != 1 || recorder.closed != 1 {
		t.Fatalf("batch lifecycle: code=%d opens=%d flush=%d close=%d stdout=%s stderr=%s", code, opens, recorder.flushed, recorder.closed, stdout, stderr)
	}
}

func TestCommandTelemetryOpenFailureAndPanicAreIgnored(t *testing.T) {
	original := openCommandTelemetry
	t.Cleanup(func() { openCommandTelemetry = original })

	for _, test := range []struct {
		name string
		open func(string) (commandTelemetryRecorder, error)
	}{
		{name: "error", open: func(string) (commandTelemetryRecorder, error) { return nil, errors.New("private open failure") }},
		{name: "panic", open: func(string) (commandTelemetryRecorder, error) { panic("private open failure") }},
	} {
		t.Run(test.name, func(t *testing.T) {
			openCommandTelemetry = test.open
			injected := false
			finish := startCommandTelemetry("workspace", func(app.TelemetrySink) { injected = true })
			finish()
			if injected {
				t.Fatal("failed recorder was injected")
			}
		})
	}
}
