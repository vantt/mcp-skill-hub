package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/vantt/skillhub-mcp/internal/catalog"
	resolverpkg "github.com/vantt/skillhub-mcp/internal/resolver"
	"github.com/vantt/skillhub-mcp/internal/telemetry"
	"github.com/vantt/skillhub-mcp/internal/workspace"
)

// Routing fields a suggestion can add an entry to.
const (
	FieldExamples        = "examples"
	FieldCounterExamples = "counter_examples"
	FieldNotFor          = "not_for"
)

// Signals that produce a suggestion.
const (
	SignalMissedThenChosen  = "missed_then_chosen"
	SignalRejectedThenOther = "rejected_then_other"
	SignalRepeatedRejection = "repeated_rejection"
)

// Reasons a candidate or suggestion is left out of the report.
const (
	SkipNoText           = "no_text"
	SkipRedacted         = "redacted"
	SkipUnsafeCharacters = "unsafe_characters"
	SkipTooLong          = "too_long"
	SkipSkillUnavailable = "skill_unavailable"
	SkipAlreadyPresent   = "already_present"
	SkipConflict         = "conflict"
	SkipFieldFull        = "field_full"
	SkipDismissed        = "dismissed"
	SkipBelowThreshold   = "below_threshold"
)

const (
	defaultSuggestionWindow = 30 * 24 * time.Hour
	maxSuggestionTextRunes  = 300
	maxRoutingListEntries   = 10
	maxEvidenceIDs          = 10
	notForMinEvents         = 3
	notForMinSessions       = 2
	notForMinTerms          = 2
	phraseClusterOverlap    = 0.5
)

var suggestionIDPattern = regexp.MustCompile(`^rs_[0-9a-f]{20}$`)

// RoutingSuggestionService mines the case journal and usage events for
// routing entries that existing skills are missing. It reads telemetry and the
// served catalog, and writes only the dismissal file, on an explicit dismiss.
type RoutingSuggestionService struct {
	Telemetry TelemetryService
	Clock     Clock
}

// RoutingSuggestionQuery bounds one mining run.
type RoutingSuggestionQuery struct {
	// Since is the start of the window; zero means the last 30 days.
	Since time.Time
	// SkillID limits suggestions to one skill.
	SkillID string
	// MinEvents is the fewest events an examples or counter_examples
	// suggestion needs; zero means 1.
	MinEvents int
}

// RoutingSuggestionWindow is the UTC date range the run covered.
type RoutingSuggestionWindow struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// RoutingSuggestionEvidence says why a suggestion exists. It never carries
// session hashes or the phrasing itself.
type RoutingSuggestionEvidence struct {
	Events           int            `json:"events"`
	Sessions         int            `json:"sessions"`
	FirstSeen        string         `json:"first_seen"`
	LastSeen         string         `json:"last_seen"`
	Kinds            map[string]int `json:"kinds"`
	OtherSkill       string         `json:"other_skill,omitempty"`
	RejectedFeedback int            `json:"rejected_feedback"`
	Clients          []string       `json:"clients"`
	ResolutionIDs    []string       `json:"resolution_ids"`
	CaseIDs          []string       `json:"case_ids"`
	Reason           string         `json:"reason"`
}

// RoutingSuggestion is one routing entry proposed for one skill.
type RoutingSuggestion struct {
	ID       string                    `json:"id"`
	SkillID  string                    `json:"skill_id"`
	Field    string                    `json:"field"`
	Text     string                    `json:"text"`
	Signal   string                    `json:"signal"`
	Evidence RoutingSuggestionEvidence `json:"evidence"`
}

// RoutingConflict is a phrasing that was not suggested because the skill
// already lists it in a contradicting field.
type RoutingConflict struct {
	SkillID       string `json:"skill_id"`
	Field         string `json:"field"`
	Text          string `json:"text"`
	ConflictsWith string `json:"conflicts_with"`
}

// RoutingSuggestionReport is the outcome of one mining run.
type RoutingSuggestionReport struct {
	CaseJournalEnabled bool                    `json:"case_journal_enabled"`
	Window             RoutingSuggestionWindow `json:"window"`
	RawEventsSince     string                  `json:"raw_events_since,omitempty"`
	Suggestions        []RoutingSuggestion     `json:"suggestions"`
	Conflicts          []RoutingConflict       `json:"conflicts,omitempty"`
	Skipped            map[string]int          `json:"skipped"`
}

// suggestionCandidate is one phrasing proposed for one skill field by one case.
type suggestionCandidate struct {
	skillID string
	field   string
	text    string
	signal  string
	other   string
	// priorResolutionID is the resolution a reformulation turned down.
	priorResolutionID string
	record            telemetry.CaseRecord
	// cluster holds the rejection events a not_for candidate stands for.
	cluster []suggestionCandidate
}

// suggestionGroup collects the candidates that propose the same entry.
type suggestionGroup struct {
	id      string
	skillID string
	field   string
	text    string
	signal  string
	members []suggestionCandidate
}

// rawEvidence indexes the raw events a suggestion cites.
type rawEvidence struct {
	loads    map[string]map[string]bool
	rejected map[string]map[string]string
}

func (s RoutingSuggestionService) now() time.Time {
	if s.Clock != nil {
		return s.Clock.Now().UTC()
	}
	return SystemClock{}.Now().UTC()
}

// Suggest mines the window for routing suggestions. Event level skips (no
// text, redaction, unsafe characters, length, unavailable skill, already
// present, conflict) count once per candidate; the skips decided on a whole
// suggestion (dismissed, field full, below threshold) count once per
// suggestion.
func (s RoutingSuggestionService) Suggest(ctx context.Context, path string, query RoutingSuggestionQuery) (RoutingSuggestionReport, error) {
	return s.suggest(ctx, path, query, true)
}

func (s RoutingSuggestionService) suggest(ctx context.Context, path string, query RoutingSuggestionQuery, applyDismissals bool) (RoutingSuggestionReport, error) {
	now := s.now()
	since := query.Since
	if since.IsZero() {
		since = now.Add(-defaultSuggestionWindow)
	}
	report := RoutingSuggestionReport{
		Window:      RoutingSuggestionWindow{From: since.UTC().Format("2006-01-02"), To: now.Format("2006-01-02")},
		Suggestions: []RoutingSuggestion{},
		Skipped:     map[string]int{},
	}
	root, err := workspace.Discover(path)
	if err != nil {
		return report, err
	}
	report.CaseJournalEnabled = telemetry.IsCaseJournalEnabled(root)
	if !report.CaseJournalEnabled {
		return report, nil
	}
	cases, raw, oldest, err := s.readTelemetry(ctx, root, since, now)
	if err != nil {
		return report, err
	}
	if !oldest.IsZero() {
		report.RawEventsSince = oldest.UTC().Format(time.RFC3339)
	}
	skills, err := servedSkillsByID(ctx, root)
	if err != nil {
		return report, err
	}
	dismissed := map[string]routingDismissal{}
	if applyDismissals {
		if dismissed, err = loadDismissals(root); err != nil {
			return report, err
		}
	}
	miner := routingMiner{query: query, skills: skills, raw: raw, dismissed: dismissed, skipped: report.Skipped}
	report.Suggestions, report.Conflicts = miner.mine(cases)
	return report, nil
}

func (s RoutingSuggestionService) readTelemetry(ctx context.Context, root string, since, now time.Time) ([]telemetry.CaseRecord, rawEvidence, time.Time, error) {
	recorder, err := s.Telemetry.Open(root)
	if err != nil {
		return nil, rawEvidence{}, time.Time{}, err
	}
	defer closeTelemetryRecorder(recorder, new(error))
	cases, err := recorder.Cases(ctx, since, "")
	if err != nil {
		return nil, rawEvidence{}, time.Time{}, storeFailure(err)
	}
	events, err := recorder.RawEvents(ctx, since.UTC().Format("2006-01-02"), now.Format("2006-01-02"))
	if err != nil {
		return nil, rawEvidence{}, time.Time{}, storeFailure(err)
	}
	oldest, _, err := recorder.OldestRawEventTime(ctx)
	if err != nil {
		return nil, rawEvidence{}, time.Time{}, storeFailure(err)
	}
	return cases, indexRawEvidence(events), oldest, nil
}

// indexRawEvidence keeps the server-observed loads and the rejection feedback
// per resolution.
func indexRawEvidence(events []telemetry.Event) rawEvidence {
	index := rawEvidence{loads: map[string]map[string]bool{}, rejected: map[string]map[string]string{}}
	for _, event := range events {
		switch event.Type {
		case telemetry.EventSkillLoaded:
			load, ok := parseChainLoad(event)
			if !ok || load.ResolutionID == "" || load.SkillID == "" {
				continue
			}
			if index.loads[load.ResolutionID] == nil {
				index.loads[load.ResolutionID] = map[string]bool{}
			}
			index.loads[load.ResolutionID][load.SkillID] = true
		case telemetry.EventActivationRejected:
			skillID, _ := event.Payload["skill_id"].(string)
			if event.ResolutionID == "" || skillID == "" {
				continue
			}
			if index.rejected[event.ResolutionID] == nil {
				index.rejected[event.ResolutionID] = map[string]string{}
			}
			index.rejected[event.ResolutionID][event.ID] = skillID
		}
	}
	return index
}

// servedSkillsByID reads the skills of the generation agents are served now.
func servedSkillsByID(ctx context.Context, root string) (map[string]resolverpkg.Skill, error) {
	view, err := catalog.ObservePublishedLocked(ctx, root)
	if err != nil {
		return nil, fmt.Errorf("read the skill catalog: %w", err)
	}
	defer view.Close()
	served, err := servedSkillCatalog(ctx, view)
	if err != nil {
		return nil, fmt.Errorf("read the skill catalog: %w", err)
	}
	all, err := served.Skills(ctx)
	if err != nil {
		return nil, fmt.Errorf("read the skill catalog: %w", err)
	}
	skills := make(map[string]resolverpkg.Skill, len(all))
	for _, skill := range activeLintSkills(all) {
		skills[skill.ID] = skill
	}
	return skills, nil
}

func loadDismissals(root string) (map[string]routingDismissal, error) {
	handle, err := os.OpenRoot(root)
	if err != nil {
		return nil, err
	}
	defer handle.Close()
	state, err := readRoutingDismissals(handle)
	return state.Dismissed, err
}

// Dismiss hides a suggestion ID from later runs. An ID that is not suggested
// now is accepted, so a curator can dismiss from a saved listing, and a second
// dismissal changes nothing.
func (s RoutingSuggestionService) Dismiss(ctx context.Context, path, id string) error {
	if !suggestionIDPattern.MatchString(id) {
		return fmt.Errorf("suggestion ID %q is not valid", id)
	}
	root, err := workspace.Discover(path)
	if err != nil {
		return err
	}
	entry := routingDismissal{}
	if report, err := s.suggest(ctx, root, RoutingSuggestionQuery{}, false); err == nil {
		for _, suggestion := range report.Suggestions {
			if suggestion.ID == id {
				entry = routingDismissal{SkillID: suggestion.SkillID, Field: suggestion.Field}
			}
		}
	}
	handle, err := os.OpenRoot(root)
	if err != nil {
		return err
	}
	defer handle.Close()
	state, err := readRoutingDismissals(handle)
	if err != nil {
		return err
	}
	if _, ok := state.Dismissed[id]; ok {
		return nil
	}
	addRoutingDismissal(&state, id, entry, s.now())
	return writeRoutingDismissals(handle, state)
}

// routingMiner turns cases into suggestions for one run.
type routingMiner struct {
	query     RoutingSuggestionQuery
	skills    map[string]resolverpkg.Skill
	raw       rawEvidence
	dismissed map[string]routingDismissal
	skipped   map[string]int
	conflicts []RoutingConflict
}

func (m *routingMiner) mine(cases []telemetry.CaseRecord) ([]RoutingSuggestion, []RoutingConflict) {
	var candidates []suggestionCandidate
	for _, record := range cases {
		candidates = append(candidates, m.caseCandidates(record)...)
	}
	sortCandidates(candidates)
	candidates = m.availableCandidates(candidates)
	groups := m.groupsFor(m.clusterNotFor(candidates))
	suggestions := m.finalize(groups)
	sort.Slice(suggestions, func(i, j int) bool {
		a, b := suggestions[i], suggestions[j]
		if a.Evidence.Events != b.Evidence.Events {
			return a.Evidence.Events > b.Evidence.Events
		}
		if a.Evidence.Sessions != b.Evidence.Sessions {
			return a.Evidence.Sessions > b.Evidence.Sessions
		}
		if a.Evidence.LastSeen != b.Evidence.LastSeen {
			return a.Evidence.LastSeen > b.Evidence.LastSeen
		}
		return a.ID < b.ID
	})
	return suggestions, m.conflicts
}

func (m *routingMiner) skip(reason string) { m.skipped[reason]++ }

func stringField(values map[string]any, key string) string {
	value, _ := values[key].(string)
	return value
}

// caseCandidates extracts the candidates of one case after the text filters.
// The ignored case kinds produce none.
func (m *routingMiner) caseCandidates(record telemetry.CaseRecord) []suggestionCandidate {
	var text, x, y string
	switch record.Kind {
	case "after_no_skill", "after_needs_context":
		text, y = stringField(record.Task, "description"), record.Chosen
		return m.withText(record, text, []suggestionCandidate{
			{skillID: y, field: FieldExamples, signal: SignalMissedThenChosen, record: record},
		})
	case "verified_reformulation":
		text = stringField(record.Task, "prior_description")
		x, y = stringField(record.Resolver, "prior_primary"), record.Chosen
		if x == "" || y == "" {
			return m.withText(record, "", nil)
		}
		prior, _ := record.Request["prior"].(map[string]any)
		candidates := []suggestionCandidate{{skillID: x, field: FieldCounterExamples, signal: SignalRejectedThenOther, other: y, priorResolutionID: stringField(prior, "resolution_id"), record: record}}
		if m.loadedOn(record.ResolutionID, y) {
			candidates = append(candidates, suggestionCandidate{skillID: y, field: FieldExamples, signal: SignalRejectedThenOther, other: x, record: record})
		}
		return m.withText(record, text, candidates)
	case "override":
		text = stringField(record.Task, "description")
		x, y = stringField(record.Resolver, "primary"), record.Chosen
		if x == "" || y == "" {
			return m.withText(record, "", nil)
		}
		return m.withText(record, text, []suggestionCandidate{
			{skillID: x, field: FieldCounterExamples, signal: SignalRejectedThenOther, other: y, record: record},
			{skillID: y, field: FieldExamples, signal: SignalRejectedThenOther, other: x, record: record},
		})
	}
	return nil
}

// loadedOn reports whether the hub saw skill id loaded on a resolution.
func (m *routingMiner) loadedOn(resolutionID, id string) bool {
	return resolutionID != "" && m.raw.loads[resolutionID][id]
}

// withText applies the text filters once for the case and stamps the
// normalized text on the candidates that remain. Candidates that name the same
// skill on both sides, or another skill than the query's, are dropped.
func (m *routingMiner) withText(record telemetry.CaseRecord, text string, candidates []suggestionCandidate) []suggestionCandidate {
	text = normalizeSuggestionText(text)
	if reason := textSkipReason(text); reason != "" {
		m.skip(reason)
		return nil
	}
	var kept []suggestionCandidate
	for _, candidate := range candidates {
		if candidate.skillID == "" || candidate.skillID == candidate.other {
			continue
		}
		if m.query.SkillID != "" && candidate.skillID != m.query.SkillID {
			continue
		}
		candidate.text = text
		kept = append(kept, candidate)
	}
	return kept
}

// normalizeSuggestionText trims the text and collapses whitespace.
func normalizeSuggestionText(text string) string {
	return strings.Join(strings.Fields(text), " ")
}

func textSkipReason(text string) string {
	switch {
	case text == "":
		return SkipNoText
	case telemetry.ContainsRedaction(text):
		return SkipRedacted
	case strings.IndexFunc(text, func(r rune) bool { return unicode.Is(unicode.Cc, r) || unicode.Is(unicode.Cf, r) }) >= 0:
		return SkipUnsafeCharacters
	case utf8.RuneCountInString(text) > maxSuggestionTextRunes:
		return SkipTooLong
	}
	return ""
}

func sortCandidates(candidates []suggestionCandidate) {
	sort.SliceStable(candidates, func(i, j int) bool {
		a, b := candidates[i].record, candidates[j].record
		if !a.OccurredAt.Equal(b.OccurredAt) {
			return a.OccurredAt.After(b.OccurredAt)
		}
		return a.CaseID < b.CaseID
	})
}

// availableCandidates drops the candidates whose skill is not an active skill.
func (m *routingMiner) availableCandidates(candidates []suggestionCandidate) []suggestionCandidate {
	var kept []suggestionCandidate
	for _, candidate := range candidates {
		if _, ok := m.skills[candidate.skillID]; !ok {
			m.skip(SkipSkillUnavailable)
			continue
		}
		kept = append(kept, candidate)
	}
	return kept
}

func foldedKey(text string) string { return strings.ToLower(normalizeSuggestionText(text)) }

// suggestionID is stable for the same skill, field and text, ignoring case.
func suggestionID(skillID, field, text string) string {
	sum := sha256.Sum256([]byte(skillID + "\x00" + field + "\x00" + foldedKey(text)))
	return "rs_" + hex.EncodeToString(sum[:])[:20]
}

func listContains(entries []string, text string) bool {
	key := foldedKey(text)
	for _, entry := range entries {
		if foldedKey(entry) == key {
			return true
		}
	}
	return false
}

func skillField(skill resolverpkg.Skill, field string) []string {
	switch field {
	case FieldExamples:
		return skill.Examples
	case FieldCounterExamples:
		return skill.CounterExamples
	}
	return skill.NotFor
}

// contradicting returns the fields of the skill that contradict an entry in
// field: an example conflicts with a counter example or a not_for entry, and
// the reverse.
func contradicting(field string) []string {
	if field == FieldExamples {
		return []string{FieldCounterExamples, FieldNotFor}
	}
	return []string{FieldExamples}
}

// groupsFor applies the per-candidate field filters and groups what is left by
// suggestion ID, in a stable order.
func (m *routingMiner) groupsFor(candidates []suggestionCandidate) []*suggestionGroup {
	byID := map[string]*suggestionGroup{}
	var order []*suggestionGroup
	for _, candidate := range candidates {
		skill := m.skills[candidate.skillID]
		if listContains(skillField(skill, candidate.field), candidate.text) {
			m.skip(SkipAlreadyPresent)
			continue
		}
		if with := conflictingField(skill, candidate); with != "" {
			m.skip(SkipConflict)
			m.addConflict(candidate, with)
			continue
		}
		id := suggestionID(candidate.skillID, candidate.field, candidate.text)
		group := byID[id]
		if group == nil {
			group = &suggestionGroup{id: id, skillID: candidate.skillID, field: candidate.field, text: candidate.text, signal: candidate.signal}
			byID[id] = group
			order = append(order, group)
		}
		members := []suggestionCandidate{candidate}
		if candidate.cluster != nil {
			members = candidate.cluster
		}
		group.members = append(group.members, members...)
	}
	return order
}

func conflictingField(skill resolverpkg.Skill, candidate suggestionCandidate) string {
	for _, field := range contradicting(candidate.field) {
		if listContains(skillField(skill, field), candidate.text) {
			return field
		}
	}
	return ""
}

func (m *routingMiner) addConflict(candidate suggestionCandidate, with string) {
	for _, existing := range m.conflicts {
		if existing.SkillID == candidate.skillID && existing.Field == candidate.field && foldedKey(existing.Text) == foldedKey(candidate.text) {
			return
		}
	}
	m.conflicts = append(m.conflicts, RoutingConflict{SkillID: candidate.skillID, Field: candidate.field, Text: candidate.text, ConflictsWith: with})
}

// finalize applies the filters decided on a whole suggestion and builds the
// evidence of the ones that stay.
func (m *routingMiner) finalize(groups []*suggestionGroup) []RoutingSuggestion {
	minEvents := m.query.MinEvents
	if minEvents <= 0 {
		minEvents = 1
	}
	suggestions := []RoutingSuggestion{}
	for _, group := range groups {
		if _, ok := m.dismissed[group.id]; ok {
			m.skip(SkipDismissed)
			continue
		}
		if group.field != FieldNotFor {
			if len(skillField(m.skills[group.skillID], group.field)) >= maxRoutingListEntries {
				m.skip(SkipFieldFull)
				continue
			}
			if len(group.members) < minEvents {
				m.skip(SkipBelowThreshold)
				continue
			}
		}
		suggestions = append(suggestions, RoutingSuggestion{
			ID: group.id, SkillID: group.skillID, Field: group.field, Text: group.text, Signal: group.signal,
			Evidence: m.evidenceFor(group),
		})
	}
	return suggestions
}

// clusterNotFor replaces the rejection events of a skill that repeat across
// sessions with one not_for candidate for the cluster. Single-link clustering
// joins phrasings that overlap by at least phraseClusterOverlap.
func (m *routingMiner) clusterNotFor(candidates []suggestionCandidate) []suggestionCandidate {
	bySkill := map[string][]int{}
	for i, candidate := range candidates {
		if candidate.field == FieldCounterExamples {
			bySkill[candidate.skillID] = append(bySkill[candidate.skillID], i)
		}
	}
	replaced := map[int]bool{}
	var notFor []suggestionCandidate
	for _, indexes := range bySkill {
		for _, component := range phraseComponents(candidates, indexes) {
			members := make([]suggestionCandidate, len(component))
			for i, index := range component {
				members[i] = candidates[index]
			}
			if len(members) < notForMinEvents || distinctSessions(members) < notForMinSessions {
				continue
			}
			for _, index := range component {
				replaced[index] = true
			}
			notFor = append(notFor, notForCandidate(members))
		}
	}
	var kept []suggestionCandidate
	for i, candidate := range candidates {
		if !replaced[i] {
			kept = append(kept, candidate)
		}
	}
	kept = append(kept, notFor...)
	sortCandidates(kept)
	return kept
}

// phraseComponents groups the indexed candidates into single-link components
// of overlapping phrasings. Each component keeps the order of its indexes.
func phraseComponents(candidates []suggestionCandidate, indexes []int) [][]int {
	parent := make([]int, len(indexes))
	for i := range parent {
		parent[i] = i
	}
	var find func(int) int
	find = func(i int) int {
		if parent[i] != i {
			parent[i] = find(parent[i])
		}
		return parent[i]
	}
	for i := range indexes {
		for j := i + 1; j < len(indexes); j++ {
			if resolverpkg.PhraseOverlap(candidates[indexes[i]].text, candidates[indexes[j]].text) >= phraseClusterOverlap {
				parent[find(j)] = find(i)
			}
		}
	}
	grouped := map[int][]int{}
	var roots []int
	for i, index := range indexes {
		root := find(i)
		if _, ok := grouped[root]; !ok {
			roots = append(roots, root)
		}
		grouped[root] = append(grouped[root], index)
	}
	components := make([][]int, 0, len(roots))
	for _, root := range roots {
		components = append(components, grouped[root])
	}
	return components
}

func distinctSessions(members []suggestionCandidate) int {
	sessions := map[string]bool{}
	for _, member := range members {
		if member.record.SessionHash != "" {
			sessions[member.record.SessionHash] = true
		}
	}
	return len(sessions)
}

// notForCandidate builds the not_for entry for a cluster: the terms every
// member shares, in the order of the medoid phrasing, or the medoid phrasing
// when fewer than notForMinTerms terms are shared.
func notForCandidate(members []suggestionCandidate) suggestionCandidate {
	medoid := clusterMedoid(members)
	text := sharedTerms(medoid.text, members)
	if text == "" {
		text = medoid.text
	}
	return suggestionCandidate{
		skillID: medoid.skillID, field: FieldNotFor, text: text, signal: SignalRepeatedRejection,
		other: medoid.other, record: members[0].record, cluster: members,
	}
}

// clusterMedoid returns the member that overlaps the others most. Ties go to
// the smaller text, so the choice does not depend on arrival order.
func clusterMedoid(members []suggestionCandidate) suggestionCandidate {
	best, bestScore := members[0], -1.0
	for _, member := range members {
		score := 0.0
		for _, other := range members {
			score += resolverpkg.PhraseOverlap(member.text, other.text)
		}
		if score > bestScore || (score == bestScore && member.text < best.text) {
			best, bestScore = member, score
		}
	}
	return best
}

// sharedTerms keeps the words of the medoid phrasing that also occur in every
// other member, or returns "" when there are fewer than notForMinTerms. A word
// is shared when the resolver's overlap between it and the member is not zero.
func sharedTerms(medoid string, members []suggestionCandidate) string {
	var terms []string
	seen := map[string]bool{}
	for _, word := range strings.Fields(medoid) {
		word = strings.TrimFunc(word, func(r rune) bool { return unicode.IsPunct(r) || unicode.IsSymbol(r) })
		key := strings.ToLower(word)
		if word == "" || seen[key] {
			continue
		}
		shared := true
		for _, member := range members {
			if resolverpkg.PhraseOverlap(word, member.text) == 0 {
				shared = false
				break
			}
		}
		if shared {
			seen[key] = true
			terms = append(terms, word)
		}
	}
	if len(terms) < notForMinTerms {
		return ""
	}
	return strings.Join(terms, " ")
}

// evidenceFor summarizes the members of a suggestion. It never holds session
// hashes or the phrasing.
func (m *routingMiner) evidenceFor(group *suggestionGroup) RoutingSuggestionEvidence {
	evidence := RoutingSuggestionEvidence{Events: len(group.members), Sessions: distinctSessions(group.members), Kinds: map[string]int{}}
	clients, others := map[string]bool{}, map[string]int{}
	var first, last time.Time
	var resolutions []string
	for _, member := range group.members {
		record := member.record
		evidence.Kinds[record.Kind]++
		if member.other != "" {
			others[member.other]++
		}
		if record.Client.Name != "" {
			clients[record.Client.Name] = true
		}
		when := record.OccurredAt.UTC()
		if first.IsZero() || when.Before(first) {
			first = when
		}
		if when.After(last) {
			last = when
		}
		evidence.CaseIDs = appendUniqueLimited(evidence.CaseIDs, record.CaseID)
		resolutions = appendUniqueLimited(resolutions, record.ResolutionID)
	}
	evidence.ResolutionIDs = resolutions
	evidence.FirstSeen, evidence.LastSeen = first.Format("2006-01-02"), last.Format("2006-01-02")
	evidence.Clients = sortedKeys(clients)
	evidence.OtherSkill = mostCommon(others)
	evidence.RejectedFeedback = m.rejectedFeedback(group)
	evidence.Reason = suggestionReason(group, evidence)
	return evidence
}

func appendUniqueLimited(list []string, value string) []string {
	if value == "" || len(list) >= maxEvidenceIDs {
		return list
	}
	for _, existing := range list {
		if existing == value {
			return list
		}
	}
	return append(list, value)
}

func sortedKeys(set map[string]bool) []string {
	keys := make([]string, 0, len(set))
	for key := range set {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// mostCommon returns the most frequent key, the smaller one on a tie.
func mostCommon(counts map[string]int) string {
	best, bestCount := "", 0
	for key, count := range counts {
		if count > bestCount || (count == bestCount && key < best) {
			best, bestCount = key, count
		}
	}
	return best
}

// rejectedFeedback counts the explicit rejections of the suggestion's skill
// on the resolutions behind a counter example or not_for entry.
func (m *routingMiner) rejectedFeedback(group *suggestionGroup) int {
	if group.field == FieldExamples {
		return 0
	}
	events := map[string]bool{}
	for _, member := range group.members {
		for _, resolutionID := range []string{member.record.ResolutionID, member.priorResolutionID} {
			for eventID, skillID := range m.raw.rejected[resolutionID] {
				if skillID == group.skillID {
					events[eventID] = true
				}
			}
		}
	}
	return len(events)
}

func plural(count int, singular, many string) string {
	if count == 1 {
		return fmt.Sprintf("1 %s", singular)
	}
	return fmt.Sprintf("%d %s", count, many)
}

// suggestionReason is a fixed sentence per signal. It names skills and counts
// only, never the phrasing.
func suggestionReason(group *suggestionGroup, evidence RoutingSuggestionEvidence) string {
	events := plural(evidence.Events, "event", "events")
	switch {
	case group.signal == SignalMissedThenChosen:
		return fmt.Sprintf("%s: a resolution found no skill or needed more context, and the host then loaded %s.", events, group.skillID)
	case group.signal == SignalRepeatedRejection:
		return fmt.Sprintf("%s from %s were routed to %s and turned down in favour of %s.", events, plural(evidence.Sessions, "session", "sessions"), group.skillID, orUnknown(evidence.OtherSkill))
	case group.field == FieldCounterExamples:
		return fmt.Sprintf("%s: %s was recommended and the caller then used %s instead.", events, group.skillID, orUnknown(evidence.OtherSkill))
	}
	return fmt.Sprintf("%s: the task was first routed to %s, and the caller then used %s.", events, orUnknown(evidence.OtherSkill), group.skillID)
}

func orUnknown(skillID string) string {
	if skillID == "" {
		return "another skill"
	}
	return skillID
}
