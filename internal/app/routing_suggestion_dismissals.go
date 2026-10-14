package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"sort"
	"time"
)

const (
	dismissalFile       = "runtime/routing_suggestions.json"
	dismissalDirectory  = "runtime"
	maxDismissals       = 1000
	maxDismissalFileLen = 1 << 20
)

// routingDismissal records one dismissed suggestion. The skill and field are
// empty when the curator dismissed an ID the current mining does not suggest.
type routingDismissal struct {
	At      string `json:"at"`
	SkillID string `json:"skill_id,omitempty"`
	Field   string `json:"field,omitempty"`
}

type routingDismissals struct {
	Version   int                         `json:"version"`
	Dismissed map[string]routingDismissal `json:"dismissed"`
}

// readRoutingDismissals loads the dismissal file under the workspace root. A
// missing file means nothing is dismissed.
func readRoutingDismissals(root *os.Root) (routingDismissals, error) {
	empty := routingDismissals{Version: 1, Dismissed: map[string]routingDismissal{}}
	info, err := root.Lstat(dismissalFile)
	if errors.Is(err, fs.ErrNotExist) {
		return empty, nil
	}
	if err != nil {
		return empty, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Size() > maxDismissalFileLen {
		return empty, fmt.Errorf("unsafe dismissal file: %s", dismissalFile)
	}
	data, err := root.ReadFile(dismissalFile)
	if err != nil {
		return empty, err
	}
	var state routingDismissals
	if err := json.Unmarshal(data, &state); err != nil || state.Version != 1 {
		return empty, fmt.Errorf("dismissal file %s is not valid; delete it to restore every suggestion", dismissalFile)
	}
	if state.Dismissed == nil {
		state.Dismissed = map[string]routingDismissal{}
	}
	return state, nil
}

// writeRoutingDismissals replaces the dismissal file through a temporary file
// and a rename, keeping at most maxDismissals entries (the newest).
func writeRoutingDismissals(root *os.Root, state routingDismissals) error {
	if err := root.MkdirAll(dismissalDirectory, 0o700); err != nil {
		return fmt.Errorf("create runtime directory: %w", err)
	}
	info, err := root.Lstat(dismissalDirectory)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("unsafe runtime path: %s", dismissalDirectory)
	}
	trimRoutingDismissals(&state)
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	temporary := dismissalFile + ".tmp"
	_ = root.Remove(temporary)
	file, err := root.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create dismissal file: %w", err)
	}
	if _, err = file.Write(data); err == nil {
		err = file.Sync()
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		_ = root.Remove(temporary)
		return err
	}
	if err := root.Rename(temporary, dismissalFile); err != nil {
		_ = root.Remove(temporary)
		return fmt.Errorf("publish dismissal file: %w", err)
	}
	return nil
}

// trimRoutingDismissals drops the oldest entries beyond the cap.
func trimRoutingDismissals(state *routingDismissals) {
	if len(state.Dismissed) <= maxDismissals {
		return
	}
	ids := make([]string, 0, len(state.Dismissed))
	for id := range state.Dismissed {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		a, b := state.Dismissed[ids[i]].At, state.Dismissed[ids[j]].At
		if a != b {
			return a < b
		}
		return ids[i] < ids[j]
	})
	for _, id := range ids[:len(ids)-maxDismissals] {
		delete(state.Dismissed, id)
	}
}

// addRoutingDismissal marks id dismissed at now. An ID that is already
// dismissed keeps its first record, so repeating a dismissal changes nothing.
func addRoutingDismissal(state *routingDismissals, id string, entry routingDismissal, now time.Time) {
	if _, ok := state.Dismissed[id]; ok {
		return
	}
	entry.At = now.UTC().Format(time.RFC3339)
	state.Dismissed[id] = entry
}
