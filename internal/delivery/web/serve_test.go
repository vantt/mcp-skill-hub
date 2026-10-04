package web

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"testing"
	"time"
)

func TestServeWildcardAnswersOnLoopback(t *testing.T) {
	root := newWebWorkspace(t)
	ln, err := net.Listen("tcp4", "0.0.0.0:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer ln.Close()

	port := ln.Addr().(*net.TCPAddr).Port
	token := "wildcard-token"

	srv, err := New(Options{
		Workspace:  root,
		Token:      token,
		ListenPort: port,
	})
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	serveErrCh := make(chan error, 1)
	go func() {
		serveErrCh <- srv.Serve(ctx, ln)
	}()

	sessionURL := fmt.Sprintf("http://127.0.0.1:%d/api/v1/session", port)

	// 1. With token -> 200
	req, err := http.NewRequest(http.MethodGet, sessionURL, nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request with token failed: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status with token = %d, want 200", resp.StatusCode)
	}

	// 2. Without token -> 401
	reqNoToken, err := http.NewRequest(http.MethodGet, sessionURL, nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	respNoToken, err := http.DefaultClient.Do(reqNoToken)
	if err != nil {
		t.Fatalf("request without token failed: %v", err)
	}
	respNoToken.Body.Close()
	if respNoToken.StatusCode != http.StatusUnauthorized {
		t.Errorf("status without token = %d, want 401", respNoToken.StatusCode)
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
