package telemetry

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

var closeQueueBlocked = func() {}

const (
	defaultBufferSize       = 256
	defaultRetention        = 30 * 24 * time.Hour
	defaultRollupRetention  = 180 * 24 * time.Hour
	defaultMaxSize          = int64(100 << 20)
	defaultOperationTimeout = 5 * time.Second
)

// Config controls a local, disposable telemetry recorder.
type Config struct {
	Path               string
	WorkspaceRoot      string
	BufferSize         int
	Retention          time.Duration
	RollupRetention    time.Duration
	MaxSizeBytes       int64
	ContentMode        string
	CaseJournalEnabled bool
	OperationTimeout   time.Duration
	Clock              func() time.Time
	ID                 func() (string, error)
	storeAnchor        *storeAnchor
	storeAnchorErr     error
}

// Health is a retained snapshot of recorder state and counters.
type Health struct {
	State      string `json:"state"`
	Accepted   uint64 `json:"accepted"`
	Written    uint64 `json:"written"`
	Dropped    uint64 `json:"dropped"`
	Rejected   uint64 `json:"rejected"`
	Errors     uint64 `json:"errors"`
	QueueDepth int    `json:"queue_depth"`
	LastError  string `json:"last_error,omitempty"`
}

type operation uint8

const (
	opFlush operation = iota
	opPurge
	opPreview
	opExport
	opPromotionDraft
	opRecordFeedback
	opRecordCurationSession
	opRollups
	opRawEvents
	opHealth
	opClose
)

type request struct {
	event           *storedEnvelope
	op              operation
	ctx             context.Context
	limit           int
	path            string
	from            string
	to              string
	feedback        Feedback
	curationSession CurationSession
	response        chan response
}

type response struct {
	preview               Preview
	export                ExportResult
	promotionDraft        PromotionDraft
	feedbackResult        FeedbackResult
	curationSessionResult CurationSessionResult
	rollups               []RollupRow
	rawEvents             []Event
	err                   error
}

// Recorder owns one bounded asynchronous writer. Record never blocks and never
// returns a storage error; administrative methods report their own failures.
type Recorder struct {
	config    Config
	queue     chan request
	gate      sync.RWMutex
	closeGate chan struct{}
	closed    bool
	done      chan struct{}
	accepted  atomic.Uint64
	written   atomic.Uint64
	dropped   atomic.Uint64
	rejected  atomic.Uint64
	errors    atomic.Uint64
	stateMu   sync.RWMutex
	state     string
	lastError string
	// persisted holds cumulative counters from earlier recorders, loaded by a
	// health check. Only this instance's own counters are written back on Close.
	persisted [len(counterMetaKeys)]uint64
}

// Open validates configuration and workspace confinement before starting the
// writer. SQLite initialization remains asynchronous so database failures
// degrade health instead of changing the caller's primary operation.
func Open(config Config) (*Recorder, error) {
	if config.Path == "" {
		return nil, errors.New("telemetry path is required")
	}
	if config.BufferSize < 0 {
		return nil, errors.New("buffer size cannot be negative")
	}
	if config.BufferSize == 0 {
		config.BufferSize = defaultBufferSize
	}
	if config.Retention < 0 {
		return nil, errors.New("retention cannot be negative")
	}
	if config.Retention == 0 {
		config.Retention = defaultRetention
	}
	if config.RollupRetention < 0 {
		return nil, errors.New("rollup retention cannot be negative")
	}
	if config.RollupRetention == 0 {
		config.RollupRetention = defaultRollupRetention
	}
	if config.MaxSizeBytes < 0 {
		return nil, errors.New("max size cannot be negative")
	}
	if config.MaxSizeBytes == 0 {
		config.MaxSizeBytes = defaultMaxSize
	}
	if config.ContentMode == "" {
		config.ContentMode = ContentModeNone
	}
	if config.ContentMode != ContentModeNone {
		return nil, errors.New("telemetry core supports only content_mode none")
	}
	if config.OperationTimeout <= 0 {
		config.OperationTimeout = defaultOperationTimeout
	}
	if config.Clock == nil {
		config.Clock = time.Now
	}
	if config.ID == nil {
		config.ID = randomEventID
	}
	var err error
	config, err = normalizeStoreConfig(config)
	if err != nil {
		return nil, err
	}
	recorder := &Recorder{config: config, queue: make(chan request, config.BufferSize), closeGate: make(chan struct{}, 1), done: make(chan struct{}), state: "starting"}
	recorder.closeGate <- struct{}{}
	go recorder.run()
	return recorder, nil
}

// Record validates and enqueues an event without waiting. It deliberately
// returns no result: rejection, exhaustion, and storage failures are observable
// through Health and can never alter the caller's primary operation.
func (r *Recorder) Record(event Event) {
	id := event.ID
	if id == "" {
		generated, err := r.config.ID()
		if err != nil {
			r.recordFailure("generate event ID: " + err.Error())
			r.dropped.Add(1)
			return
		}
		id = generated
	}
	envelope, err := validateAndBuild(event, r.config.ContentMode, r.config.Clock().UTC(), id)
	if err != nil {
		r.rejected.Add(1)
		r.setError("reject event: "+err.Error(), false)
		return
	}
	r.gate.RLock()
	defer r.gate.RUnlock()
	if r.closed {
		r.dropped.Add(1)
		return
	}
	select {
	case r.queue <- request{event: &envelope}:
		r.accepted.Add(1)
	default:
		r.dropped.Add(1)
	}
}

// Flush waits until all events accepted before the flush request have been handled.
func (r *Recorder) Flush(ctx context.Context) error { return r.admin(ctx, request{op: opFlush}).err }

// Purge immediately removes all events and recreates an empty WAL database.
func (r *Recorder) Purge(ctx context.Context) error { return r.admin(ctx, request{op: opPurge}).err }

// Preview returns sanitized deterministic JSONL without writing a file.
func (r *Recorder) Preview(ctx context.Context, limit int) (Preview, error) {
	result := r.admin(ctx, request{op: opPreview, limit: limit})
	return result.preview, result.err
}

// Export writes the same sanitized JSONL returned by Preview using atomic replacement.
func (r *Recorder) Export(ctx context.Context, path string) (ExportResult, error) {
	result := r.admin(ctx, request{op: opExport, path: path})
	return result.export, result.err
}

// Rollups returns daily aggregates for the inclusive UTC day range [from, to]
// (YYYY-MM-DD; an empty bound is open). Rollups are aggregates, not events, so
// they never appear in Preview or Export.
func (r *Recorder) Rollups(ctx context.Context, from, to string) ([]RollupRow, error) {
	// Validate before queueing so a bad caller range never degrades health.
	from, to, err := normalizeRollupRange(from, to)
	if err != nil {
		return nil, err
	}
	result := r.admin(ctx, request{op: opRollups, from: from, to: to})
	return result.rollups, result.err
}

// RawEvents synchronously queries retained raw events within the bounded window.
func (r *Recorder) RawEvents(ctx context.Context, from, to string) ([]Event, error) {
	if r == nil {
		return []Event{}, nil
	}
	result := r.admin(ctx, request{op: opRawEvents, from: from, to: to})
	return result.rawEvents, result.err
}

// PromotionDraft locates one exact resolution and returns a sanitized,
// deliberately incomplete evaluation-case draft for human review.
func (r *Recorder) PromotionDraft(ctx context.Context, resolutionID string) (PromotionDraft, error) {
	result := r.admin(ctx, request{op: opPromotionDraft, path: resolutionID})
	return result.promotionDraft, result.err
}

// RecordFeedback synchronously validates and stores bounded feedback for one
// retained exact resolution. It is ordered after all previously accepted
// asynchronous events, so a caller may report feedback immediately after a
// resolution without an explicit flush.
func (r *Recorder) RecordFeedback(ctx context.Context, feedback Feedback) (FeedbackResult, error) {
	result := r.admin(ctx, request{op: opRecordFeedback, feedback: feedback})
	return result.feedbackResult, result.err
}

// RecordCurationSession synchronously validates and stores one content-free
// completed-session observation. Reusing an event ID is idempotent only when
// every supplied measurement is identical.
func (r *Recorder) RecordCurationSession(ctx context.Context, report CurationSession) (CurationSessionResult, error) {
	result := r.admin(ctx, request{op: opRecordCurationSession, curationSession: report})
	return result.curationSessionResult, result.err
}

// Close gracefully flushes accepted events and stops the writer. A canceled
// attempt that cannot queue shutdown leaves the recorder open for a retry.
func (r *Recorder) Close(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case <-r.closeGate:
	case <-ctx.Done():
		return ctx.Err()
	}
	item := request{op: opClose, ctx: ctx, response: make(chan response, 1)}
	for {
		r.gate.Lock()
		if r.closed {
			r.gate.Unlock()
			r.closeGate <- struct{}{}
			select {
			case <-r.done:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		select {
		case r.queue <- item:
			// Mark closed only after shutdown is irrevocably queued. This keeps a
			// canceled/full-queue attempt retryable while excluding later records.
			r.closed = true
			r.gate.Unlock()
			r.closeGate <- struct{}{}
			goto queued
		default:
			r.gate.Unlock()
			closeQueueBlocked()
		}
		select {
		case <-ctx.Done():
			r.closeGate <- struct{}{}
			return ctx.Err()
		case <-time.After(time.Millisecond):
		}
	}

queued:
	select {
	case result := <-item.response:
		if result.err == nil {
			select {
			case <-r.done:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		return result.err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Health applies retention before returning the current recorder snapshot.
// Maintenance failures degrade health but remain isolated from callers.
func (r *Recorder) Health() Health {
	r.gate.RLock()
	closed := r.closed
	r.gate.RUnlock()
	if !closed {
		ctx, cancel := context.WithTimeout(context.Background(), r.config.OperationTimeout)
		_ = r.admin(ctx, request{op: opHealth}).err
		cancel()
	}
	return r.healthSnapshot()
}

func (r *Recorder) healthSnapshot() Health {
	r.stateMu.RLock()
	state, last, base := r.state, r.lastError, r.persisted
	r.stateMu.RUnlock()
	return Health{State: state, Accepted: base[0] + r.accepted.Load(), Written: base[1] + r.written.Load(), Dropped: base[2] + r.dropped.Load(), Rejected: base[3] + r.rejected.Load(), Errors: base[4] + r.errors.Load(), QueueDepth: len(r.queue), LastError: last}
}

func (r *Recorder) admin(ctx context.Context, item request) response {
	r.gate.RLock()
	defer r.gate.RUnlock()
	if r.closed {
		return response{err: errors.New("telemetry recorder is closed")}
	}
	return r.sendLocked(ctx, item)
}

func (r *Recorder) sendLocked(ctx context.Context, item request) response {
	item.ctx = ctx
	item.response = make(chan response, 1)
	select {
	case r.queue <- item:
	case <-ctx.Done():
		return response{err: ctx.Err()}
	}
	select {
	case result := <-item.response:
		return result
	case <-ctx.Done():
		return response{err: ctx.Err()}
	}
}

func (r *Recorder) run() {
	defer close(r.done)
	if err := initialize(r.config); err != nil {
		r.recordFailure(err.Error())
	} else {
		r.setState("healthy", "")
	}
	for {
		item := <-r.queue
		if item.event != nil {
			batch := []storedEnvelope{*item.event}
			for len(batch) < 64 {
				select {
				case next := <-r.queue:
					if next.event == nil {
						r.writeBatch(batch)
						if r.handleOperation(next) {
							return
						}
						batch = nil
						break
					}
					batch = append(batch, *next.event)
				default:
					break
				}
				if batch == nil || len(r.queue) == 0 {
					break
				}
			}
			if len(batch) != 0 {
				r.writeBatch(batch)
			}
			continue
		}
		if r.handleOperation(item) {
			return
		}
	}
}

func (r *Recorder) writeBatch(events []storedEnvelope) {
	ctx, cancel := context.WithTimeout(context.Background(), r.config.OperationTimeout)
	defer cancel()
	if err := writeEvents(ctx, r.config, events); err != nil {
		r.dropped.Add(uint64(len(events)))
		r.recordFailure("write telemetry: " + err.Error())
		return
	}
	r.written.Add(uint64(len(events)))
	r.setState("healthy", "")
}

func (r *Recorder) handleOperation(item request) bool {
	result := response{}
	switch item.op {
	case opFlush:
		result.err = maintainStore(item.ctx, r.config)
	case opPurge:
		result.err = purgeStore(item.ctx, r.config)
		if result.err == nil {
			r.resetCounters()
		}
	case opPreview:
		result.preview, result.err = previewStore(item.ctx, r.config, item.limit)
	case opExport:
		result.export, result.err = exportStore(item.ctx, r.config, item.path)
	case opPromotionDraft:
		result.promotionDraft, result.err = promotionDraftStore(item.ctx, r.config, item.path)
	case opRecordFeedback:
		result.feedbackResult, result.err = recordFeedbackStore(item.ctx, r.config, item.feedback)
	case opRecordCurationSession:
		result.curationSessionResult, result.err = recordCurationSessionStore(item.ctx, r.config, item.curationSession)
	case opRollups:
		result.rollups, result.err = rollupsStore(item.ctx, r.config, item.from, item.to)
	case opRawEvents:
		result.rawEvents, result.err = rawEventsStore(item.ctx, r.config, item.from, item.to)
	case opHealth:
		result.err = maintainStore(item.ctx, r.config)
		if result.err == nil {
			if totals, err := loadCounters(item.ctx, r.config); err == nil {
				r.stateMu.Lock()
				r.persisted = totals
				r.stateMu.Unlock()
			}
		}
	case opClose:
		result.err = maintainStore(item.ctx, r.config)
		if result.err == nil {
			// Best effort: counter persistence must never fail shutdown.
			_ = persistCounters(item.ctx, r.config, [len(counterMetaKeys)]uint64{r.accepted.Load(), r.written.Load(), r.dropped.Load(), r.rejected.Load(), r.errors.Load()})
		}
		if closeErr := r.config.storeAnchor.close(); result.err == nil {
			result.err = closeErr
		}
	}
	if result.err != nil {
		r.recordFailure(result.err.Error())
	} else if item.op != opClose {
		r.setState("healthy", "")
	}
	if item.op == opClose {
		r.setState("closed", r.healthSnapshot().LastError)
		item.response <- result
		return true
	}
	item.response <- result
	return false
}

// resetCounters forgets everything this recorder counted so far. After a purge
// the store is brand new, so a failure that only existed in the discarded store
// (for example the corrupt file that purge replaced) must not be persisted into
// the fresh store's cumulative counters.
func (r *Recorder) resetCounters() {
	r.accepted.Store(0)
	r.written.Store(0)
	r.dropped.Store(0)
	r.rejected.Store(0)
	r.errors.Store(0)
	r.stateMu.Lock()
	r.persisted = [len(counterMetaKeys)]uint64{}
	r.stateMu.Unlock()
}

func (r *Recorder) recordFailure(message string) { r.errors.Add(1); r.setError(message, true) }
func (r *Recorder) setError(message string, degraded bool) {
	r.stateMu.Lock()
	if degraded {
		r.state = "degraded"
	}
	r.lastError = message
	r.stateMu.Unlock()
}
func (r *Recorder) setState(state, message string) {
	r.stateMu.Lock()
	r.state, r.lastError = state, message
	r.stateMu.Unlock()
}

func randomEventID() (string, error) {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", fmt.Errorf("randomness unavailable: %w", err)
	}
	return "evt_" + hex.EncodeToString(bytes[:]), nil
}

// NewEventID generates a random event identifier with prefix evt_.
func NewEventID() string {
	id, err := randomEventID()
	if err != nil {
		return "evt_fallback"
	}
	return id
}
