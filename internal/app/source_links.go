package app

import (
	"os"
	"path/filepath"
	"strings"

	sourcepkg "github.com/vantt/mcp-skill-hub/internal/source"
	"gopkg.in/yaml.v3"
)

func readSkillSourceLinks(root string) ([]sourcepkg.Link, error) {
	dir := filepath.Join(root, "sources", "skills")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var links []sourcepkg.Link
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, err
		}
		var link sourcepkg.Link
		if err := yaml.Unmarshal(data, &link); err != nil {
			continue
		}
		links = append(links, link)
	}
	return links, nil
}

func learningSourceIDs(links []sourcepkg.Link) map[string]bool {
	set := make(map[string]bool)
	for _, link := range links {
		if link.Role == "learning-source" || link.Role == "inspiration" {
			set[link.SourceID] = true
		}
	}
	return set
}
