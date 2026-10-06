// Package web is the local HTTP delivery adapter for the Skill Hub WebUI.
package web

import (
	"context"
	"errors"
	"io/fs"
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/vantt/mcp-skill-hub/internal/app"
	"github.com/vantt/mcp-skill-hub/internal/workspace"
)

// InterfaceInfo represents one network interface with its status and IP addresses.
type InterfaceInfo struct {
	Name     string
	Up       bool
	Loopback bool
	IPs      []net.IP
}

// Options configure the web server.
type Options struct {
	Workspace  string
	Token      string
	ListenPort int
	AllowHosts []string
	Dev        bool
	Now        func() time.Time
	Interfaces func() ([]InterfaceInfo, error)
	Hostname   func() (string, error)
	Assets     fs.FS
}

// Server delivers the WebUI over local HTTP.
type Server struct {
	workspace    string
	opts         Options
	curation     app.CurationService
	skills       app.SkillService
	skillAdd     app.SkillAddService
	sources      app.SourceService
	distill      app.DistillService
	insights     app.InsightService
	upstream     app.UpstreamService
	sourceImport app.SourceImportService
	throttle     *authThrottle
	hostMu       sync.Mutex
	hostCacheAt  time.Time
	cachedIPs    map[string]bool
	cachedHost   string
}

var defaultAssets = func() fs.FS {
	return nil
}

var routeRegistrars []func(s *Server, mux *http.ServeMux)

func registerRoutes(fn func(s *Server, mux *http.ServeMux)) {
	routeRegistrars = append(routeRegistrars, fn)
}

func defaultInterfaces() ([]InterfaceInfo, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	result := make([]InterfaceInfo, 0, len(ifaces))
	for _, iface := range ifaces {
		info := InterfaceInfo{
			Name:     iface.Name,
			Up:       iface.Flags&net.FlagUp != 0,
			Loopback: iface.Flags&net.FlagLoopback != 0,
		}
		addrs, err := iface.Addrs()
		if err == nil {
			for _, a := range addrs {
				if ipnet, ok := a.(*net.IPNet); ok {
					info.IPs = append(info.IPs, ipnet.IP)
				}
			}
		}
		result = append(result, info)
	}
	return result, nil
}

// DefaultInterfaces returns system network interfaces with their flags and IPs.
func DefaultInterfaces() ([]InterfaceInfo, error) {
	return defaultInterfaces()
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
	if opts.Interfaces == nil {
		opts.Interfaces = defaultInterfaces
	}
	if opts.Hostname == nil {
		opts.Hostname = os.Hostname
	}
	if opts.Assets == nil {
		opts.Assets = defaultAssets()
	}
	srv := &Server{
		workspace: root,
		opts:      opts,
		throttle:  newAuthThrottle(opts.Now),
	}
	_ = srv.skillAdd
	_ = srv.sources
	_ = srv.distill
	_ = srv.insights
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

// Serve starts the HTTP server on ln and gracefully shuts down when ctx is cancelled.
func (s *Server) Serve(ctx context.Context, ln net.Listener) error {
	srv := &http.Server{
		Handler:           s.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Serve(ln)
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			return err
		}
		serveErr := <-errCh
		if errors.Is(serveErr, http.ErrServerClosed) {
			return nil
		}
		return serveErr
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}
