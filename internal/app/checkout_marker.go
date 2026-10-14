package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// CheckoutMarkerName is the file inside a checked-out skill folder that says
// which hub version the folder started from. It is never skill content.
const CheckoutMarkerName = ".skillhub-checkout.json"

const checkoutMarkerSchemaVersion = 1

// CheckoutMarker records where a project folder came from, so a later commit
// can tell what the hub changed in the meantime.
type CheckoutMarker struct {
	SchemaVersion int    `json:"schema_version"`
	SkillID       string `json:"skill_id"`
	// Hub is the workspace path or hub URL the skill was copied from.
	Hub string `json:"hub"`
	// BaseVersion is the digest of the skill's content files at checkout.
	BaseVersion string `json:"base_version"`
	// BaseCommit is the hub commit the folder matches. It is empty when the
	// skill had uncommitted edits at checkout.
	BaseCommit   string `json:"base_commit,omitempty"`
	CheckedOutAt string `json:"checked_out_at"`
	// BundleDigest is the manifest digest of the bundle a checkout from a
	// remote hub was made from.
	BundleDigest string `json:"bundle_digest,omitempty"`
	// BaseFiles lists the digest of each file a remote checkout wrote, so a
	// later commit can say which files changed without the hub's history.
	BaseFiles map[string]string `json:"base_files,omitempty"`
}

// ReadCheckoutMarker reads the marker of a skill folder. The bool is false when
// the folder has none; a marker this version cannot read is an error.
func ReadCheckoutMarker(dir string) (CheckoutMarker, bool, error) {
	data, err := os.ReadFile(filepath.Join(dir, CheckoutMarkerName))
	if errors.Is(err, os.ErrNotExist) {
		return CheckoutMarker{}, false, nil
	}
	if err != nil {
		return CheckoutMarker{}, false, err
	}
	var marker CheckoutMarker
	if err := json.Unmarshal(data, &marker); err != nil {
		return CheckoutMarker{}, false, fmt.Errorf("%s is not valid JSON: %w", CheckoutMarkerName, err)
	}
	if marker.SchemaVersion != checkoutMarkerSchemaVersion {
		return CheckoutMarker{}, false, fmt.Errorf("%s has schema_version %d; this skillhub reads version %d", CheckoutMarkerName, marker.SchemaVersion, checkoutMarkerSchemaVersion)
	}
	return marker, true, nil
}

// WriteCheckoutMarker writes the marker next to the skill files.
func WriteCheckoutMarker(dir string, marker CheckoutMarker) error {
	marker.SchemaVersion = checkoutMarkerSchemaVersion
	data, err := json.MarshalIndent(marker, "", "  ")
	if err != nil {
		return err
	}
	temporary := filepath.Join(dir, CheckoutMarkerName+".tmp")
	if err := os.WriteFile(temporary, append(data, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(temporary, filepath.Join(dir, CheckoutMarkerName))
}
