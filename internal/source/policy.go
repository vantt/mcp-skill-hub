package source

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"path"
	"strings"
	"time"
)

func Digest(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// URLValidationOptions configures remote URL validation rules.
type URLValidationOptions struct {
	AllowHTTP bool
	AllowFile bool
}

// ValidateRemoteURL permits credential-free HTTPS only. HTTP can be enabled for
// controlled tests, but address policy is never bypassed.
func ValidateRemoteURL(raw string, allowHTTP bool) (*url.URL, error) {
	return ValidateRemoteURLWithOptions(raw, URLValidationOptions{AllowHTTP: allowHTTP})
}

// ValidateRemoteURLWithOptions permits credential-free HTTPS, and optionally HTTP or local file URLs for tests.
func ValidateRemoteURLWithOptions(raw string, opts URLValidationOptions) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil || u.User != nil || u.Fragment != "" {
		return nil, ErrInvalidLocator
	}
	if opts.AllowFile && u.Scheme == "file" {
		if u.Path == "" {
			return nil, ErrInvalidLocator
		}
		return u, nil
	}
	if u.Hostname() == "" {
		return nil, ErrInvalidLocator
	}
	if u.Scheme != "https" && !(opts.AllowHTTP && u.Scheme == "http") {
		return nil, fmt.Errorf("%w: allowed protocol is https", ErrInvalidLocator)
	}
	if u.RawQuery != "" {
		return nil, fmt.Errorf("%w: URL query parameters are not allowed", ErrInvalidLocator)
	}
	host := strings.TrimSuffix(u.Hostname(), ".")
	if address, parseErr := netip.ParseAddr(host); parseErr == nil {
		if !publicAddress(address) {
			return nil, ErrUnsafeAddress
		}
	} else if numericAddressLike(host) {
		// Reject legacy inet_aton forms such as 127.1, 0177.0.0.1,
		// 0x7f000001, and 2130706433 instead of leaving their interpretation to
		// a platform resolver.
		return nil, ErrUnsafeAddress
	}
	return u, nil
}
func numericAddressLike(host string) bool {
	if host == "" {
		return false
	}
	for _, r := range host {
		if (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F') || r == 'x' || r == 'X' || r == '.' {
			continue
		}
		return false
	}
	return true
}

var deniedAddressRanges = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"), netip.MustParsePrefix("10.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"), netip.MustParsePrefix("127.0.0.0/8"),
	netip.MustParsePrefix("169.254.0.0/16"), netip.MustParsePrefix("172.16.0.0/12"),
	netip.MustParsePrefix("192.0.0.0/24"), netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("192.88.99.0/24"), netip.MustParsePrefix("192.168.0.0/16"),
	netip.MustParsePrefix("192.175.48.0/24"), netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"), netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("224.0.0.0/4"), netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("::/128"), netip.MustParsePrefix("::1/128"),
	netip.MustParsePrefix("64:ff9b:1::/48"), netip.MustParsePrefix("100::/64"),
	netip.MustParsePrefix("2001::/32"), netip.MustParsePrefix("2001:2::/48"),
	netip.MustParsePrefix("2001:10::/28"), netip.MustParsePrefix("2001:20::/28"),
	netip.MustParsePrefix("2001:db8::/32"), netip.MustParsePrefix("2002::/16"),
	netip.MustParsePrefix("3fff::/20"), netip.MustParsePrefix("fc00::/7"), netip.MustParsePrefix("fe80::/10"),
	netip.MustParsePrefix("ff00::/8"),
}

func publicAddress(address netip.Addr) bool {
	if !address.IsValid() {
		return false
	}
	address = address.Unmap()
	if !address.IsGlobalUnicast() {
		return false
	}
	for _, blocked := range deniedAddressRanges {
		if blocked.Contains(address) {
			return false
		}
	}
	return true
}

func publicIP(ip net.IP) bool {
	address, ok := netip.AddrFromSlice(ip)
	return ok && publicAddress(address)
}

func safeResourcePath(value string) bool {
	if value == "" || value == "." || strings.Contains(value, `\`) || strings.ContainsRune(value, '\x00') || strings.HasPrefix(value, "/") {
		return false
	}
	clean := path.Clean(value)
	return clean == value && clean != ".." && !strings.HasPrefix(clean, "../")
}

func boundedRead(reader io.Reader, maximum int64) ([]byte, error) {
	if maximum < 0 {
		return nil, ErrLimitExceeded
	}
	limited := io.LimitReader(reader, maximum+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maximum {
		return nil, &LimitExceededError{Limit: "bytes", Actual: int64(len(data)), Max: maximum}
	}
	return data, nil
}

// NewSafeHTTPClient returns a DNS-rebinding-aware client. Redirects are
// revalidated for document sources. Git uses newPinnedHTTPClient directly with
// redirects disabled.
type IPResolver interface {
	LookupNetIP(context.Context, string, string) ([]netip.Addr, error)
}

func NewSafeHTTPClient(timeout time.Duration, allowHTTP bool, resolver IPResolver) *http.Client {
	client := newPinnedHTTPClient(timeout, resolver, 0, false)
	client.CheckRedirect = func(request *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return fmt.Errorf("too many source redirects")
		}
		_, err := ValidateRemoteURL(request.URL.String(), allowHTTP)
		return err
	}
	return client
}

func newPinnedHTTPClient(timeout time.Duration, resolver IPResolver, maximum int64, refuseRedirects bool) *http.Client {
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	dialer := &net.Dialer{Timeout: timeout / 2}
	transport := &http.Transport{
		Proxy:              nil,
		DisableCompression: false,
		ForceAttemptHTTP2:  true,
		TLSClientConfig:    &tls.Config{MinVersion: tls.VersionTLS12},
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(address)
			if err != nil {
				return nil, ErrInvalidLocator
			}
			ips, err := resolvePublicAddresses(ctx, resolver, host)
			if err != nil {
				return nil, err
			}
			// Dial the validated address directly. net/http still derives TLS SNI
			// and certificate verification from the request URL's original host.
			return dialer.DialContext(ctx, network, net.JoinHostPort(ips[0].String(), port))
		},
	}
	var roundTripper http.RoundTripper = transport
	if maximum > 0 {
		budget := &budgetRoundTripper{base: transport, remaining: maximum}
		roundTripper = budget
	}
	client := &http.Client{Transport: roundTripper, Timeout: timeout}
	if refuseRedirects {
		client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	}
	return client
}

func resolvePublicAddresses(ctx context.Context, resolver IPResolver, host string) ([]netip.Addr, error) {
	ips, err := resolver.LookupNetIP(ctx, "ip", host)
	if err != nil {
		return nil, fmt.Errorf("resolve source host: %w", err)
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("resolve source host: no addresses")
	}
	for _, ip := range ips {
		if !publicAddress(ip) {
			return nil, ErrUnsafeAddress
		}
	}
	return ips, nil
}
