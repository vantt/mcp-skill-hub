// Package web is the local HTTP delivery adapter for the Skill Hub WebUI.
package web

import (
	"context"
	"errors"
	"io/fs"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/vantt/mcp-skill-hub/internal/app"
	"github.com/vantt/mcp-skill-hub/internal/delivery/hostpolicy"
	"github.com/vantt/mcp-skill-hub/internal/workspace"
)

// Options configure the web server.
type Options struct {
	Workspace string
	Token     string
	// ListenAddrs are the addresses the listeners are bound to, as each
	// listener reports them; the Host check accepts their ports only.
	ListenAddrs []net.Addr
	AllowHosts  []string
	Dev         bool
	Now         func() time.Time
	Hostname    func() (string, error)
	Assets      fs.FS
}

// Server delivers the WebUI over local HTTP.
type Server struct {
	workspace    string
	opts         Options
	curation     app.CurationService
	skills       app.SkillService
	skillAdd     app.SkillAddService
	sources      app.SourceService
	upstream     app.UpstreamService
	sourceImport app.SourceImportService
	throttle     *authThrottle
	hosts        *hostpolicy.Policy
}

var defaultAssets = func() fs.FS {
	return nil
}

var routeRegistrars []func(s *Server, mux *http.ServeMux)

func registerRoutes(fn func(s *Server, mux *http.ServeMux)) {
	routeRegistrars = append(routeRegistrars, fn)
}

// New constructs a Server for the configured workspace and options.
func New(opts Options) (*Server, error) {
	root, err := workspace.Discover(opts.Workspace)
	if err != nil {
		return nil, err
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.Hostname == nil {
		opts.Hostname = os.Hostname
	}
	hostname, _ := opts.Hostname()
	if opts.Assets == nil {
		opts.Assets = defaultAssets()
	}
	srv := &Server{
		workspace: root,
		opts:      opts,
		throttle:  newAuthThrottle(opts.Now),
		hosts: hostpolicy.New(hostpolicy.Options{
			Listeners:   opts.ListenAddrs,
			AllowHosts:  opts.AllowHosts,
			Hostname:    hostname,
			RequirePort: true,
		}),
	}
	_ = srv.skillAdd
	_ = srv.sources
	return srv, nil
}

// Handler builds a http.ServeMux and wraps it with the security middleware chain.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	for _, reg := range routeRegistrars {
		reg(s, mux)
	}
	return s.withMiddleware(mux)
}

// Serve serves the web UI on every listener with one HTTP server and shuts
// down gracefully when ctx is cancelled. If one listener fails, the server
// stops on all of them.
func (s *Server) Serve(ctx context.Context, listeners []net.Listener) error {
	if len(listeners) == 0 {
		return errors.New("no listener")
	}
	srv := &http.Server{
		Handler:           s.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	errCh := make(chan error, len(listeners))
	for _, ln := range listeners {
		go func() { errCh <- srv.Serve(ln) }()
	}
	remaining := len(listeners)
	firstError := func(errs []error) error {
		for _, err := range errs {
			if err != nil && !errors.Is(err, http.ErrServerClosed) {
				return err
			}
		}
		return nil
	}

	var errs []error
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		shutdownErr := srv.Shutdown(shutdownCtx)
		for ; remaining > 0; remaining-- {
			errs = append(errs, <-errCh)
		}
		if shutdownErr != nil {
			return shutdownErr
		}
	case err := <-errCh:
		remaining--
		_ = srv.Close()
		errs = append(errs, err)
		for ; remaining > 0; remaining-- {
			errs = append(errs, <-errCh)
		}
	}
	return firstError(errs)
}
