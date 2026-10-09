package distill

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	insightpkg "github.com/vantt/mcp-skill-hub/internal/insight"
	"github.com/vantt/mcp-skill-hub/internal/source"
	"gopkg.in/yaml.v3"
)

var canonicalLineLocator = regexp.MustCompile(`^L([1-9][0-9]*)(?:-L?([1-9][0-9]*))?$`)

// ValidateWorkspace enforces relationships that cannot be checked from one
// canonical document. It is called for external edits and every catalog rebuild.
func ValidateWorkspace(root string) error {
	skipRuntimePackages := strings.HasPrefix(filepath.Base(root), ".skillhub-validate-")
	paths, _ := filepath.Glob(filepath.Join(root, "distill", "sources", "*", "runs", "*.yaml"))
	hasDistillation := false
	for _, path := range paths {
		if data, readErr := os.ReadFile(path); readErr == nil {
			if _, parseErr := ParseRun(data); parseErr == nil {
				hasDistillation = true
				break
			}
		}
	}
	if !hasDistillation {
		return nil
	}
	sources := map[string]source.Record{}
	runs := map[string]Run{}
	observations := map[string]Observation{}
	comparisons := map[string]Comparison{}
	insights := map[string]Insight{}
	proposals := map[string]insightpkg.ApplicationProposal{}
	incorporations := map[string]insightpkg.Incorporation{}
	receiptAfter := map[string]map[string]string{}
	walk := func(relative string, visit func(string, []byte) error) error {
		base := filepath.Join(root, filepath.FromSlash(relative))
		return filepath.WalkDir(base, func(path string, entry fs.DirEntry, err error) error {
			if errors.Is(err, os.ErrNotExist) {
				return nil
			}
			if err != nil {
				return err
			}
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			return visit(path, data)
		})
	}
	if err := walk("sources/catalog", func(_ string, data []byte) error {
		item, err := source.ParseRecord(data)
		if err == nil {
			sources[item.ID] = item
		}
		return err
	}); err != nil {
		return err
	}
	if err := walk("history/operations", func(_ string, data []byte) error {
		var receipt struct {
			ID      string `yaml:"id"`
			Status  string `yaml:"status"`
			Changes []struct {
				Path  string `yaml:"path"`
				After string `yaml:"after"`
			} `yaml:"changes"`
		}
		if err := yaml.Unmarshal(data, &receipt); err != nil {
			return err
		}
		if receipt.ID == "" || receipt.Status != "applied" {
			return errors.New("invalid operation receipt")
		}
		if _, duplicate := receiptAfter[receipt.ID]; duplicate {
			return fmt.Errorf("duplicate operation receipt %s", receipt.ID)
		}
		receiptAfter[receipt.ID] = map[string]string{}
		for _, change := range receipt.Changes {
			receiptAfter[receipt.ID][change.Path] = change.After
		}
		return nil
	}); err != nil {
		return err
	}
	if err := walk("distill", func(path string, data []byte) error {
		var err error
		slash := filepath.ToSlash(path)
		switch {
		case strings.Contains(slash, "/runs/"):
			var item Run
			item, err = ParseRun(data)
			if err == nil {
				runs[item.ID] = item
			}
		case strings.Contains(slash, "/observations/"), strings.Contains(slash, "/findings/"):
			var item Observation
			item, err = ParseObservation(data)
			if err == nil {
				observations[item.ID] = item
			}
		case strings.Contains(slash, "/comparisons/"):
			var item Comparison
			item, err = ParseComparison(data)
			if err == nil {
				comparisons[item.ID] = item
			}
		case strings.Contains(slash, "/insights/"):
			var item Insight
			item, err = ParseInsight(data)
			if err == nil {
				insights[item.ID] = item
			}
		case strings.Contains(slash, "/proposals/"):
			var item insightpkg.ApplicationProposal
			item, err = insightpkg.ParseApplicationProposal(data)
			if err == nil {
				proposals[item.ID] = item
			}
		case strings.Contains(slash, "/incorporations/"):
			var item insightpkg.Incorporation
			item, err = insightpkg.ParseIncorporation(data)
			if err == nil {
				incorporations[item.ID] = item
			}
		}
		return err
	}); err != nil {
		return err
	}

	latestFinalized := map[string]Run{}
	for _, run := range runs {
		if run.State == "finalized" && (latestFinalized[run.SourceID].FinalizedAt == "" || run.FinalizedAt > latestFinalized[run.SourceID].FinalizedAt) {
			latestFinalized[run.SourceID] = run
		}
		_, ok := sources[run.SourceID]
		if !ok {
			return fmt.Errorf("run %s references missing source %s", run.ID, run.SourceID)
		}
		for _, id := range run.FindingIDs {
			item, ok := observations[id]
			if !ok || item.SourceID != run.SourceID || (item.RunID == run.ID && item.LastSeen != IdentityOf(run.ToRevision)) {
				return fmt.Errorf("run %s declares invalid finding %s", run.ID, id)
			}
		}
		for _, id := range run.ComparisonIDs {
			if _, ok := comparisons[id]; !ok {
				return fmt.Errorf("run %s declares invalid comparison %s", run.ID, id)
			}
		}
		for _, id := range run.InsightIDs {
			if _, ok := insights[id]; !ok {
				return fmt.Errorf("run %s declares invalid insight %s", run.ID, id)
			}
		}
	}
	for sourceID, run := range latestFinalized {
		record := sources[sourceID]
		if record.DistilledRevision == nil || !SameRevision(*record.DistilledRevision, run.ToRevision) {
			return fmt.Errorf("latest finalized run %s target does not equal source cursor", run.ID)
		}
	}
	for _, observation := range observations {
		run, ok := runs[observation.RunID]
		if !ok || run.SourceID != observation.SourceID {
			return fmt.Errorf("observation %s is not declared by its producing run", observation.ID)
		}
		if run.State == "finalized" && observation.Status == "active" && !containsID(run.FindingIDs, observation.ID) {
			return fmt.Errorf("observation %s is not declared by its producing run", observation.ID)
		}
	}
	for _, comparison := range comparisons {
		run, ok := runs[comparison.RunID]
		if !ok {
			return fmt.Errorf("comparison %s is not declared by its producing run", comparison.ID)
		}
		if run.State == "finalized" && !containsID(run.ComparisonIDs, comparison.ID) {
			return fmt.Errorf("comparison %s is not declared by its producing run", comparison.ID)
		}
		stale := false
		for _, id := range comparison.ObservationIDs {
			observation, ok := observations[id]
			if !ok || comparison.BasedOn[id] != observation.LastSeen || observation.Status != "active" {
				stale = true
			}
		}
		if comparison.Stale != stale {
			return fmt.Errorf("comparison %s stale marker is incorrect", comparison.ID)
		}
	}
	for _, insight := range insights {
		run, ok := runs[insight.RunID]
		if !ok || (run.State == "finalized" && insight.Status == "active" && !containsID(run.InsightIDs, insight.ID)) {
			return fmt.Errorf("insight %s is not declared by its producing run", insight.ID)
		}
		active := false
		for _, id := range insight.ObservationIDs {
			observation, ok := observations[id]
			if !ok {
				return fmt.Errorf("insight %s references missing observation %s", insight.ID, id)
			}
			active = active || observation.Status == "active"
		}
		for _, id := range insight.ComparisonIDs {
			comparison, ok := comparisons[id]
			if !ok {
				return fmt.Errorf("insight %s references missing comparison %s", insight.ID, id)
			}
			if insight.Status == "pending" && comparison.Stale {
				return fmt.Errorf("pending insight %s relies on stale comparison %s", insight.ID, id)
			}
		}
		if insight.Status == "pending" && !active {
			return fmt.Errorf("pending insight %s has no active observation support", insight.ID)
		}
	}
	for _, proposal := range proposals {
		if _, ok := insights[proposal.InsightID]; !ok {
			return fmt.Errorf("proposal %s references missing insight %s", proposal.ID, proposal.InsightID)
		}
		for _, pin := range proposal.PathPins {
			_, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(pin.Path)))
			if pin.After == "" {
				if !errors.Is(err, os.ErrNotExist) {
					return fmt.Errorf("proposal %s deleted path %s still exists", proposal.ID, pin.Path)
				}
				continue
			}
			if err != nil {
				return fmt.Errorf("proposal %s artifact %s is missing", proposal.ID, pin.Path)
			}
		}
	}
	for _, incorporation := range incorporations {
		proposal, ok := proposals[incorporation.ProposalID]
		if !ok || proposal.InsightID != incorporation.InsightID {
			return fmt.Errorf("incorporation %s does not resolve to its insight proposal", incorporation.ID)
		}
		item, ok := insights[incorporation.InsightID]
		if !ok || item.Status != "incorporated" {
			return fmt.Errorf("incorporation %s references a missing or non-incorporated insight", incorporation.ID)
		}
		operationChanges, operationExists := receiptAfter[incorporation.OperationID]
		if !operationExists && !skipRuntimePackages {
			return fmt.Errorf("incorporation %s references missing operation %s", incorporation.ID, incorporation.OperationID)
		}
		pins := make(map[string]insightpkg.PathPin, len(proposal.PathPins))
		for _, pin := range proposal.PathPins {
			if operationExists && operationChanges[pin.Path] != pin.After {
				return fmt.Errorf("incorporation %s target digest is not recorded by its operation", incorporation.ID)
			}
			pins[pin.Path] = pin
		}
		expectedObservations := map[string]bool{}
		for _, id := range item.ObservationIDs {
			expectedObservations[id] = true
		}
		for _, comparisonID := range item.ComparisonIDs {
			comparison, exists := comparisons[comparisonID]
			if !exists {
				return fmt.Errorf("incorporation %s references missing comparison %s", incorporation.ID, comparisonID)
			}
			for _, id := range comparison.ObservationIDs {
				expectedObservations[id] = true
			}
		}
		coveredObservations := map[string]bool{}
		coveredTargets := map[string]bool{}
		seenMappings := map[string]bool{}
		for _, mapping := range incorporation.SourceToLocal {
			key := mapping.ObservationID + "\x00" + mapping.ArtifactPath + "\x00" + strings.TrimSpace(mapping.Concept)
			if seenMappings[key] || !expectedObservations[mapping.ObservationID] {
				return fmt.Errorf("incorporation %s contains a dangling or duplicate mapping", incorporation.ID)
			}
			pin, exists := pins[mapping.ArtifactPath]
			if !exists {
				return fmt.Errorf("incorporation %s mapping targets an unpinned artifact", incorporation.ID)
			}
			data, readErr := os.ReadFile(filepath.Join(root, filepath.FromSlash(mapping.ArtifactPath)))
			currentMatches := readErr == nil && source.Digest(data) == pin.After
			historicalMatches := receiptAfter[incorporation.OperationID][mapping.ArtifactPath] == pin.After
			if !currentMatches && !historicalMatches {
				return fmt.Errorf("incorporation %s mapping artifact digest is neither current nor recorded by its operation", incorporation.ID)
			}
			seenMappings[key] = true
			coveredObservations[mapping.ObservationID] = true
			coveredTargets[mapping.ArtifactPath] = true
		}
		if len(coveredObservations) != len(expectedObservations) || len(coveredTargets) != len(proposal.ChangedFiles) || len(incorporation.Targets) != len(proposal.ChangedFiles) {
			return fmt.Errorf("incorporation %s does not fully cover evidence and changed artifacts", incorporation.ID)
		}
		if proposal.Digest != insightpkg.ProposalDigest(item.ID, proposal.BaseCatalogVersion, incorporation.OperationID, proposal.PathPins, incorporation.SourceToLocal) {
			return fmt.Errorf("incorporation %s does not match proposal digest", incorporation.ID)
		}
	}
	return nil
}

func ValidateLocator(path, locator string, contents []byte) error {
	if locator == path {
		return nil
	}
	if !strings.HasPrefix(locator, path+"#") {
		return errors.New("evidence locator is outside its resource")
	}
	fragment := strings.TrimPrefix(locator, path+"#")
	if match := canonicalLineLocator.FindStringSubmatch(fragment); match != nil {
		start, _ := strconv.Atoi(match[1])
		end := start
		if match[2] != "" {
			end, _ = strconv.Atoi(match[2])
		}
		lines := 1 + strings.Count(string(contents), "\n")
		if start <= end && end <= lines {
			return nil
		}
		return errors.New("evidence line locator is outside pinned bytes")
	}
	if !strings.HasSuffix(strings.ToLower(path), ".md") && !strings.HasSuffix(strings.ToLower(path), ".markdown") {
		return errors.New("named evidence anchors require Markdown")
	}
	for _, line := range strings.Split(string(contents), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") && anchor(strings.TrimSpace(strings.TrimLeft(trimmed, "#"))) == strings.ToLower(fragment) {
			return nil
		}
	}
	return fmt.Errorf("evidence anchor %q does not resolve against pinned bytes", fragment)
}

func anchor(value string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(strings.TrimSpace(value)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r > 127:
			b.WriteRune(r)
			dash = false
		case r == ' ' || r == '-':
			if b.Len() > 0 && !dash {
				b.WriteByte('-')
				dash = true
			}
		}
	}
	return strings.TrimSuffix(b.String(), "-")
}
func containsID(values []string, id string) bool {
	for _, value := range values {
		if value == id {
			return true
		}
	}
	return false
}
