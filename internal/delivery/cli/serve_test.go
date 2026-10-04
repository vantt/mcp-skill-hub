package cli

import (
	"bytes"
	"context"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (n int, err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func TestServeWebCommand(t *testing.T) {
	tableTests := []struct {
		name       string
		args       []string
		wantCode   int
		wantStdout string
		wantStderr string
	}{
		{
			name:       "serve requires web subcommand",
			args:       []string{"serve"},
			wantCode:   2,
			wantStderr: "skillhub serve web",
		},
		{
			name:     "serve web bad addr",
			args:     []string{"serve", "web", "--addr", "bad"},
			wantCode: 2,
		},
		{
			name:     "serve web bogus flag",
			args:     []string{"serve", "web", "--bogus"},
			wantCode: 2,
		},
		{
			name:       "help serve contains loopback-only",
			args:       []string{"help", "serve"},
			wantCode:   0,
			wantStdout: "--loopback-only",
		},
		{
			name:       "web --help contains alias",
			args:       []string{"web", "--help"},
			wantCode:   0,
			wantStdout: "alias",
		},
	}

	for _, tc := range tableTests {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := RunContext(context.Background(), tc.args, &stdout, &stderr)
			if code != tc.wantCode {
				t.Errorf("exit code = %d, want %d (stdout: %s, stderr: %s)", code, tc.wantCode, stdout.String(), stderr.String())
			}
			if tc.wantStdout != "" && !strings.Contains(stdout.String(), tc.wantStdout) {
				t.Errorf("stdout %q does not contain %q", stdout.String(), tc.wantStdout)
			}
			if tc.wantStderr != "" && !strings.Contains(stderr.String(), tc.wantStderr) {
				t.Errorf("stderr %q does not contain %q", stderr.String(), tc.wantStderr)
			}
		})
	}

	t.Run("live server lifecycle", func(t *testing.T) {
		ws := initTestWorkspace(t)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		var stdout, stderr syncBuffer
		exitCh := make(chan int, 1)

		go func() {
			exitCh <- RunContext(ctx, []string{"serve", "web", "--addr", "127.0.0.1:0", "--no-open", "--workspace", ws}, &stdout, &stderr)
		}()

		deadline := time.Now().Add(10 * time.Second)
		var rawURL string
		for time.Now().Before(deadline) {
			out := stdout.String()
			if idx := strings.Index(out, "http://127.0.0.1:"); idx != -1 {
				line := out[idx:]
				if end := strings.IndexAny(line, " \r\n"); end != -1 {
					rawURL = line[:end]
				} else {
					rawURL = line
				}
				break
			}
			time.Sleep(10 * time.Millisecond)
		}

		if rawURL == "" {
			t.Fatalf("timed out waiting for web server startup URL; stdout: %s; stderr: %s", stdout.String(), stderr.String())
		}

		parsed, err := url.Parse(rawURL)
		if err != nil {
			t.Fatalf("failed to parse startup URL %q: %v", rawURL, err)
		}

		token := strings.TrimPrefix(parsed.Fragment, "token=")
		sessionURL := "http://" + parsed.Host + "/api/v1/session"

		// 1. GET with token and Host -> 200
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, sessionURL, nil)
		if err != nil {
			t.Fatalf("failed to create request: %v", err)
		}
		req.Host = parsed.Host
		req.Header.Set("Authorization", "Bearer "+token)

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("session request with token failed: %v", err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("status with token = %d, want 200", resp.StatusCode)
		}

		// 2. GET without token -> 401
		reqNoToken, err := http.NewRequestWithContext(context.Background(), http.MethodGet, sessionURL, nil)
		if err != nil {
			t.Fatalf("failed to create request: %v", err)
		}
		reqNoToken.Host = parsed.Host

		respNoToken, err := http.DefaultClient.Do(reqNoToken)
		if err != nil {
			t.Fatalf("session request without token failed: %v", err)
		}
		respNoToken.Body.Close()
		if respNoToken.StatusCode != http.StatusUnauthorized {
			t.Errorf("status without token = %d, want 401", respNoToken.StatusCode)
		}

		// 3. cancel context -> clean shutdown
		cancel()

		select {
		case status := <-exitCh:
			if status != 0 {
				t.Errorf("exit code = %d, want 0", status)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for server to shut down cleanly")
		}
	})
}
