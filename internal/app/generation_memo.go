package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/vantt/skillhub-mcp/internal/catalog"
	resolverpkg "github.com/vantt/skillhub-mcp/internal/resolver"
	"github.com/vantt/skillhub-mcp/internal/skill"
)

// The values below are derived from one immutable catalog generation and are
// kept in that generation's memo (catalog.GenerationMemo), so they are
// recomputed only when the published generation changes. Callers treat them as
// read-only.

const (
	memoPolicyKey         = "policy"
	memoResolverSkillsKey = "resolver-skills"
	memoActiveIDsKey      = "active-ids"
	memoRowPrefix         = "row:"
	memoRecordPrefix      = "record:"
)

// skillRow is what the generation stores about one skill, in any lifecycle
// state: its columns, its manifest document and its resource rows.
type skillRow struct {
	Name        string
	Description string
	Status      string
	// Manifest is the skill's manifest document (content_json); it is nil
	// when the generation holds no document for the skill.
	Manifest  []byte
	Resources []skill.Resource

	decode    sync.Once
	routing   skill.RoutingInput
	rationale string
	decodeErr error
}

// routingAndRationale decodes the routing block and rationale of the manifest
// document once per row.
func (row *skillRow) routingAndRationale() (skill.RoutingInput, string, error) {
	row.decode.Do(func() {
		var document map[string]any
		if row.decodeErr = json.Unmarshal(row.Manifest, &document); row.decodeErr != nil {
			return
		}
		row.routing = skill.RoutingFromDocument(document)
		row.rationale = skill.RationaleFromDocument(document)
	})
	return row.routing, row.rationale, row.decodeErr
}

// loadSkillRow returns the row of a skill, or skill.ErrNotFound.
func loadSkillRow(ctx context.Context, view *catalog.View, id string) (*skillRow, error) {
	key := memoRowPrefix + id
	if row, ok := catalog.MemoGet[*skillRow](view, key); ok {
		return row, nil
	}
	database, err := view.DB(ctx)
	if err != nil {
		return nil, err
	}
	row := &skillRow{Resources: []skill.Resource{}}
	var manifest sql.NullString
	err = database.QueryRowContext(ctx, `SELECT skills.name,skills.description,skills.status,canonical_entities.content_json FROM skills LEFT JOIN canonical_entities ON canonical_entities.id=skills.id WHERE skills.id=?`, id).
		Scan(&row.Name, &row.Description, &row.Status, &manifest)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, skill.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if manifest.Valid {
		row.Manifest = []byte(manifest.String)
	}
	rows, err := database.QueryContext(ctx, `SELECT path,kind,digest,size_bytes FROM resources WHERE skill_id=? ORDER BY path`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var resource skill.Resource
		if err := rows.Scan(&resource.Path, &resource.Kind, &resource.Digest, &resource.SizeBytes); err != nil {
			return nil, err
		}
		row.Resources = append(row.Resources, resource)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	catalog.MemoPut(view, key, row)
	return row, nil
}

// activeSkillIDs lists the ids of the active skills in order.
func activeSkillIDs(ctx context.Context, view *catalog.View) ([]string, error) {
	return catalog.Memoized(ctx, view, memoActiveIDsKey, func(database *sql.DB) ([]string, error) {
		rows, err := database.QueryContext(ctx, `SELECT id FROM skills WHERE status='active' ORDER BY id`)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		ids := []string{}
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				return nil, err
			}
			ids = append(ids, id)
		}
		return ids, rows.Err()
	})
}

// generationPolicy returns the recommendation policy of the generation.
func generationPolicy(ctx context.Context, view *catalog.View) (resolverpkg.Policy, error) {
	return catalog.Memoized(ctx, view, memoPolicyKey, func(database *sql.DB) (resolverpkg.Policy, error) {
		return resolverpkg.LoadPolicy(ctx, database)
	})
}

// memoSkillsCatalog serves the decoded resolver skills of the generation from
// its memo; searching still goes to the database.
type memoSkillsCatalog struct {
	resolverpkg.Catalog
	view *catalog.View
}

func (memo memoSkillsCatalog) Skills(ctx context.Context) ([]resolverpkg.Skill, error) {
	return catalog.Memoized(ctx, memo.view, memoResolverSkillsKey, func(*sql.DB) ([]resolverpkg.Skill, error) {
		return memo.Catalog.Skills(ctx)
	})
}

// skillRecord is the memoized manifest of one active, servable skill. It stores
// metadata only: the entry a client sees and the canonical file each
// distributed file must still match.
type skillRecord struct {
	entry DistributedSkill
	files []recordedFile
}

type recordedFile struct {
	relative string // path inside the skill folder
	internal string // canonical workspace path
	digest   string // pinned digest
}

// current reports whether every distributed file still has its pinned digest
// on the canonical scan the view's freshness check made. The scan reuses a
// digest only for a file whose size, times and identity are unchanged and
// settled, and reads every other file, so an edited file is noticed here.
func (record *skillRecord) current(view *catalog.View) bool {
	for _, file := range record.files {
		if digest, ok := view.CanonicalDigest(file.internal); !ok || digest != file.digest {
			return false
		}
	}
	return true
}

// checkCanonicalDigest verifies a file against its pinned digest on the
// canonical scan of the view, without reading the file.
func checkCanonicalDigest(view *catalog.View, internal, expected string) error {
	digest, ok := view.CanonicalDigest(internal)
	if !ok {
		return fmt.Errorf("%w: resource %s is missing or not regular: %w", ErrResourceContentUnavailable, internal, skill.ErrSnapshotExpired)
	}
	if digest != expected {
		return fmt.Errorf("%w: resource %s digest mismatch (expected %s, got %s): %w", ErrResourceContentUnavailable, internal, expected, digest, skill.ErrResourceDigestMismatch)
	}
	return nil
}
