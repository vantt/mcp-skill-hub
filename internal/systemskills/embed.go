// Package systemskills provides bundled, non-operational system skill assets.
package systemskills

import _ "embed"

// CuratorSkill is the bundled System Curator asset. Domain mutation remains in
// application services; this asset is guidance only.
//
//go:embed curator/SKILL.md
var CuratorSkill string
