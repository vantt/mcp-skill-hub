package source

import "testing"

func TestExcludedFromImport(t *testing.T) {
	t.Parallel()
	excluded := []string{
		".env", ".env.local", ".env.production", "config/.env", "key.pem", "certs/server.pem",
		"id_rsa", "id_ed25519", "id_ed25519_sk", "ssh/id_ecdsa", "id_dsa", ".DS_Store", "a/.DS_Store",
		"node_modules/x/i.js", "a/node_modules/x.js", "a/__pycache__/m.pyc", ".git/config", "sub/.git/HEAD",
	}
	for _, rel := range excluded {
		if reason, ok := ExcludedFromImport(rel); !ok || reason == "" {
			t.Errorf("ExcludedFromImport(%q) = (%q, %v), want excluded with a reason", rel, reason, ok)
		}
	}
	kept := []string{
		"id_mapping.py", "environment.md", "scripts/run.sh", "SKILL.md", ".env.example", ".env.sample",
		"config/.env.example", "my.pem.md", "gitignore.txt", ".github/workflows/ci.yml", "id_rsa.pub",
	}
	for _, rel := range kept {
		if reason, ok := ExcludedFromImport(rel); ok {
			t.Errorf("ExcludedFromImport(%q) excluded (%q), want kept", rel, reason)
		}
	}
}
