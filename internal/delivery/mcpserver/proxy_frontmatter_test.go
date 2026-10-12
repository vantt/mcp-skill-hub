package mcpserver

import (
	"path/filepath"
	"testing"
)

// A skill's frontmatter reaches the agent only as the text of its SKILL.md,
// which the proxy reads from the cached bundle, never through a decoded JSON
// value. Integers beyond float64 precision, hex values, dates, and ordinary
// scalars, lists and maps therefore arrive exactly as written.
func TestProxyReturnsFrontmatterScalarsExactlyAsWritten(t *testing.T) {
	root := newBundleHubFixture(t)
	skillMD := "---\n" +
		"name: test-audit\n" +
		"description: Audit the fixture tests.\n" +
		"big: 9007199254740993\n" +
		"hex: 0x1FFFFFFFFFFFFF1\n" +
		"released: 2026-10-12\n" +
		"small: 42\n" +
		"ratio: 0.5\n" +
		"flag: true\n" +
		"label: \"quoted 123\"\n" +
		"tags: [a, 9007199254740993]\n" +
		"nested:\n  id: 9007199254740993\n  when: 2026-10-12\n" +
		"---\n\n# Audit\n\nBody.\n"
	writeFixtureFile(t, filepath.Join(root, "skills", "default", "test-audit", "SKILL.md"), []byte(skillMD))
	rebuildFixtureCatalog(t, root)
	hub := startHTTPTestHub(t, root)
	proxied := startTestProxy(t, hub.baseURL, hub.token, t.TempDir(), nil)

	proxyResolve(t, proxied.session, root, "test-audit")
	got, result, encoded := proxySkillGet(t, proxied.session, "test-audit")
	if result.IsError || got.Content != skillMD {
		t.Fatalf("skill_get through the proxy changed the frontmatter:\nwant %q\ngot  %q\nraw  %s", skillMD, got.Content, encoded)
	}
}
