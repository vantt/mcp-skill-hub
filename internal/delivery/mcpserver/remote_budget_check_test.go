package mcpserver

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestHubChangeVerdict holds the bench's hub-change budget: a cache hit waits
// for nothing and may cost one background load confirmation answered 304 with
// no body; a changed skill is one request the agent waits for.
func TestHubChangeVerdict(t *testing.T) {
	confirmation := benchExchange{method: http.MethodGet, path: HTTPBundlePath, conditional: true, answered: true, status: http.StatusNotModified, done: true}
	withBody := confirmation
	withBody.bodyBytes = 120
	fullBundle := confirmation
	fullBundle.status, fullBundle.bodyBytes = http.StatusOK, 7500
	unconditional := confirmation
	unconditional.conditional = false
	unanswered := confirmation
	unanswered.done, unanswered.ended = false, true
	tests := []struct {
		name string
		row  changeRow
		want bool
	}{
		{"hit with nothing on the link", changeRow{answer: remoteCacheHit}, true},
		{"hit with one background confirmation", changeRow{answer: remoteCacheHit, background: []benchExchange{confirmation}}, true},
		{"hit that waited for the confirmation", changeRow{answer: remoteCacheHit, waitedRequests: 1, waitedBytes: 165}, false},
		{"hit that waited for bytes only", changeRow{answer: remoteCacheHit, waitedBytes: 10}, false},
		{"hit whose confirmation grew a body", changeRow{answer: remoteCacheHit, background: []benchExchange{withBody}}, false},
		{"hit that downloads in the background", changeRow{answer: remoteCacheHit, background: []benchExchange{fullBundle}}, false},
		{"hit with an unconditional background request", changeRow{answer: remoteCacheHit, background: []benchExchange{unconditional}}, false},
		{"hit whose confirmation was never answered", changeRow{answer: remoteCacheHit, background: []benchExchange{unanswered}}, false},
		{"hit with two background confirmations", changeRow{answer: remoteCacheHit, background: []benchExchange{confirmation, confirmation}}, false},
		{"fetch where a hit is due", changeRow{answer: remoteCacheFetched, waitedRequests: 1}, false},
		{"changed skill fetched", changeRow{answer: remoteCacheFetched, waitedRequests: 1, wantFetch: true}, true},
		{"changed skill served from the cache", changeRow{answer: remoteCacheHit, background: []benchExchange{confirmation}, wantFetch: true}, false},
		{"changed skill with a background request", changeRow{answer: remoteCacheFetched, waitedRequests: 1, background: []benchExchange{confirmation}, wantFetch: true}, false},
		{"changed skill in two requests", changeRow{answer: remoteCacheFetched, waitedRequests: 2, wantFetch: true}, false},
	}
	for _, test := range tests {
		measured, limit, ok := test.row.verdict()
		if ok != test.want {
			t.Errorf("%s: verdict %v, want %v (measured %q, limit %q)", test.name, ok, test.want, measured, limit)
		}
	}
}

// TestBenchLinkRecordsHeldExchanges checks what the bench builds the budget
// on: a held answer does not reach the client until release, and each
// exchange is recorded with its status and body size as delivered.
func TestBenchLinkRecordsHeldExchanges(t *testing.T) {
	hub := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("ETag", `"abc"`)
		if request.Header.Get("If-None-Match") != "" {
			writer.WriteHeader(http.StatusNotModified)
			return
		}
		_, _ = writer.Write([]byte("bundle"))
	}))
	defer hub.Close()
	link := startLink(t, strings.TrimPrefix(hub.URL, "http://"), 0)
	client := &http.Client{Transport: &http.Transport{}}
	defer client.CloseIdleConnections()
	send := func(conditional bool) <-chan int {
		answered := make(chan int, 1)
		go func() {
			request, err := http.NewRequest(http.MethodGet, link.url+HTTPBundlePath+"?skill=a", nil)
			if err != nil {
				answered <- -1
				return
			}
			if conditional {
				request.Header.Set("If-None-Match", `"abc"`)
			}
			response, err := client.Do(request)
			if err != nil {
				answered <- -1
				return
			}
			_ = response.Body.Close()
			answered <- response.StatusCode
		}()
		return answered
	}

	link.hold()
	answered := send(true)
	if !link.waitFor(0, 10*time.Second, func(exchanges []*benchExchange) bool { return len(exchanges) == 1 }) {
		t.Fatal("the request was not recorded")
	}
	select {
	case status := <-answered:
		t.Fatalf("a held answer reached the client: %d", status)
	default:
	}
	if got := link.exchangesFrom(0)[0]; got.answered || link.delivered.Load() != 0 {
		t.Fatalf("a held answer was recorded as delivered: %+v, %d bytes", got, link.delivered.Load())
	}
	link.release()
	if status := <-answered; status != http.StatusNotModified {
		t.Fatalf("status %d, want 304", status)
	}
	if !link.waitFor(0, 10*time.Second, linkQuiet) {
		t.Fatal("the answer was not recorded as complete")
	}
	if got := link.exchangesFrom(0)[0]; !got.isLoadConfirmation() {
		t.Fatalf("a 304 to a conditional bundle request is not a load confirmation: %+v", got)
	}

	if status := <-send(false); status != http.StatusOK {
		t.Fatalf("status %d, want 200", status)
	}
	if !link.waitFor(0, 10*time.Second, func(exchanges []*benchExchange) bool { return len(exchanges) == 2 && linkQuiet(exchanges) }) {
		t.Fatal("the second exchange was not recorded as complete")
	}
	if got := link.exchangesFrom(1)[0]; got.status != http.StatusOK || got.bodyBytes != int64(len("bundle")) || got.conditional || got.isLoadConfirmation() {
		t.Fatalf("second exchange = %+v", got)
	}
}
