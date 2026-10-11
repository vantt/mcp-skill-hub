package web

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"testing"
	"time"
)

// TestServeSeveralListenersWithOneToken serves one web server on two
// loopback listeners: the session token works on both ports and the Host
// check accepts each bound port.
func TestServeSeveralListenersWithOneToken(t *testing.T) {
	root := newWebWorkspace(t)
	var listeners []net.Listener
	var addresses []net.Addr
	for range 2 {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("failed to listen: %v", err)
		}
		defer ln.Close()
		listeners = append(listeners, ln)
		addresses = append(addresses, ln.Addr())
	}

	token := "multi-listener-token"
	srv, err := New(Options{
		Workspace:   root,
		Token:       token,
		ListenAddrs: addresses,
	})
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	serveErrCh := make(chan error, 1)
	go func() {
		serveErrCh <- srv.Serve(ctx, listeners)
	}()

	status := func(port int, authorization string) int {
		t.Helper()
		req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://127.0.0.1:%d/api/v1/session", port), nil)
		if err != nil {
			t.Fatalf("failed to create request: %v", err)
		}
		if authorization != "" {
			req.Header.Set("Authorization", authorization)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	for _, address := range addresses {
		port := address.(*net.TCPAddr).Port
		if got := status(port, "Bearer "+token); got != http.StatusOK {
			t.Errorf("port %d with token = %d, want 200", port, got)
		}
		if got := status(port, ""); got != http.StatusUnauthorized {
			t.Errorf("port %d without token = %d, want 401", port, got)
		}
	}

	cancel()
	select {
	case err := <-serveErrCh:
		if err != nil {
			t.Errorf("server shutdown error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for server shutdown")
	}
}
