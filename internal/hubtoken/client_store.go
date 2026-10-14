package hubtoken

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// ClientStorePath returns where this machine keeps its copy of the token of
// the hub at hubURL when the token came from an invite:
// <configDir>/hub-tokens/<host>-<port>. The host keeps letters, digits, dots
// and dashes, lower-cased; a colon becomes an underscore and IPv6 brackets are
// dropped, so the addresses ::1 and 1 never share a file. A URL without a port
// uses its scheme's default.
func ClientStorePath(configDir, hubURL string) string {
	host, port := "hub", ""
	if parsed, err := url.Parse(strings.TrimSpace(hubURL)); err == nil {
		if name := parsed.Hostname(); name != "" {
			host = name
		}
		port = parsed.Port()
		if port == "" {
			port = map[string]string{"http": "80", "https": "443"}[parsed.Scheme]
		}
	}
	var sanitized strings.Builder
	for _, character := range strings.ToLower(host) {
		switch {
		case character >= 'a' && character <= 'z', character >= '0' && character <= '9', character == '.', character == '-':
			sanitized.WriteRune(character)
		case character == ':':
			sanitized.WriteByte('_')
		}
	}
	name := sanitized.String()
	if name == "" {
		name = "hub"
	}
	if port != "" {
		name += "-" + port
	}
	return filepath.Join(configDir, "hub-tokens", name)
}

// CheckWritable creates the folder of path as Save would and proves a file
// can be created in it, so a caller can find out before an action that cannot
// be repeated.
func CheckWritable(path string) error {
	if info, err := os.Stat(path); err == nil && info.IsDir() {
		return fmt.Errorf("%s is a folder", path)
	}
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("create token directory: %w", err)
	}
	probe, err := os.CreateTemp(directory, FileName+".probe-*")
	if err != nil {
		return fmt.Errorf("write in the token directory: %w", err)
	}
	probe.Close()
	return os.Remove(probe.Name())
}
