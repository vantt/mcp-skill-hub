package web

import (
	"net/url"
	"strings"

	"github.com/vantt/mcp-skill-hub/internal/app"
)

func validateGitHubLocator(raw string) *app.Error {
	trimmed := strings.TrimSpace(raw)
	if len(trimmed) == 0 || len(trimmed) > 2048 {
		return locatorError()
	}

	u, err := url.Parse(trimmed)
	if err != nil {
		return locatorError()
	}

	if u.Scheme != "https" {
		return locatorError()
	}

	if !strings.EqualFold(u.Host, "github.com") {
		return locatorError()
	}

	if u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return locatorError()
	}

	pathSegments := strings.Split(strings.Trim(u.Path, "/"), "/")
	var segments []string
	for _, seg := range pathSegments {
		if seg != "" {
			segments = append(segments, seg)
		}
	}

	if len(segments) < 2 {
		return locatorError()
	}

	if len(segments) >= 3 {
		third := segments[2]
		if third != "tree" && third != "blob" {
			return locatorError()
		}
		if len(segments) < 4 {
			return locatorError()
		}
	}

	return nil
}

func locatorError() *app.Error {
	return app.NewInvalidRequestError(
		"The web UI accepts only public GitHub URLs.",
		"Use https://github.com/<owner>/<repo>[/tree/<ref>/<path>]. Import local folders with the CLI.",
	)
}
