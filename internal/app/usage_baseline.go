package app

import (
	"context"
	"sort"
	"time"

	"github.com/vantt/mcp-skill-hub/internal/telemetry"
	"github.com/vantt/mcp-skill-hub/internal/workspace"
)

// BaselineQuery defines parameters for querying the baseline report.
type BaselineQuery struct {
	Since     time.Time `json:"since"`
	Until     time.Time `json:"until"`
	MinChains int       `json:"min_chains"`
}

// BucketSufficiency describes whether a bucket meets the minimum chains criterion.
type BucketSufficiency struct {
	Verdict         string `json:"verdict"` // "sufficient" or "insufficient"
	ResolvedChains  int64  `json:"resolved_chains"`
	NoSkillChains   int64  `json:"no_skill_chains"`
	MinChains       int    `json:"min_chains"`
	MissingResolved int64  `json:"missing_resolved"`
	MissingNoSkill  int64  `json:"missing_no_skill"`
}

// BaselineBucket contains metrics and sufficiency status for a (catalog_snapshot, client) pair.
type BaselineBucket struct {
	CatalogSnapshot  string            `json:"catalog_snapshot"`
	Client           string            `json:"client"`
	Status           string            `json:"status"` // "baseline" or "superseded"
	IsBaselineWindow bool              `json:"is_baseline_window"`
	Sufficiency      BucketSufficiency `json:"sufficiency"`
	Metrics          ChainMetrics      `json:"metrics"`
	TotalLoads       int64             `json:"total_loads"`
	UnsolicitedLoads int64             `json:"unsolicited_loads"`
	UnsolicitedShare RateMetric        `json:"unsolicited_share"`
	FirstValidDay    string            `json:"first_valid_day,omitempty"`
}

// BaselineReport is the aggregated O7 baseline evaluation.
type BaselineReport struct {
	Since              string           `json:"since"`
	Until              string           `json:"until"`
	GeneratedAt        string           `json:"generated_at"`
	MinChains          int              `json:"min_chains"`
	RawRetentionPruned bool             `json:"raw_retention_pruned"`
	ActiveSnapshot     string           `json:"active_snapshot,omitempty"`
	Buckets            []BaselineBucket `json:"buckets"`
}

type rawRetentionInfo struct {
	retention  time.Duration
	oldestTime time.Time
	hasOldest  bool
}
type bucketKey struct {
	snapshot string
	client   string
}

type baselineContext struct {
	counts           map[string]int64
	firstValid       map[string]string
	totalLoads       map[string]int64
	unsolicitedLoads map[string]int64
	activeSnapshot   string
	until            time.Time
	minChains        int
}

// Baseline produces an O7 baseline report grouped per catalog_snapshot x client.
func (service UsageService) Baseline(ctx context.Context, path string, q BaselineQuery) (BaselineReport, error) {
	root, err := workspace.Discover(path)
	if err != nil {
		return BaselineReport{}, err
	}

	until := q.Until
	if until.IsZero() {
		until = time.Now().UTC()
	}
	minChains := q.MinChains
	if minChains <= 0 {
		minChains = 30
	}
	since := q.Since
	if since.IsZero() {
		since = until.Add(-30 * 24 * time.Hour)
	}

	recorder, err := service.Telemetry.Open(root)
	if err != nil {
		return BaselineReport{}, err
	}
	defer recorder.Close(ctx)

	fromStr := since.Format("2006-01-02")
	toStr := until.Format("2006-01-02")
	rawEvents, err := recorder.RawEvents(ctx, fromStr, toStr)
	if err != nil {
		return BaselineReport{}, err
	}
	retention := recorder.Retention()
	oldestTime, hasOldest, _ := recorder.OldestRawEventTime(ctx)
	rInfo := rawRetentionInfo{
		retention:  retention,
		oldestTime: oldestTime,
		hasOldest:  hasOldest,
	}
	report := service.compileBaseline(rawEvents, since, until, minChains, rInfo)
	return report, nil
}

func parseBaselineLoads(rawEvents []telemetry.Event) (map[string]int64, map[string]int64) {
	totalLoads := make(map[string]int64)
	unsolicitedLoads := make(map[string]int64)
	for _, event := range rawEvents {
		if event.Type != telemetry.EventSkillLoaded {
			continue
		}
		basis, _ := event.Payload["basis"].(string)
		status, _ := event.Payload["status"].(string)
		if basis != telemetry.LoadBasisServerObserved || status == "review_required" {
			continue
		}
		snap := event.CatalogSnapshot
		if snap == "" {
			snap = "unknown"
		}
		client := event.Client.Name
		if client == "" {
			client = "skillhub"
		}
		key := snap + "\x00" + client
		totalLoads[key]++
		if attr, ok := event.Payload["attribution"].(string); ok && attr == "unsolicited" {
			unsolicitedLoads[key]++
		}
	}
	return totalLoads, unsolicitedLoads
}

func groupBaselineChains(allChains []*rawEventChain, totalLoads map[string]int64) (map[bucketKey][]*rawEventChain, map[string]time.Time) {
	groupedChains := make(map[bucketKey][]*rawEventChain)
	snapshotLatestTime := make(map[string]time.Time)

	for _, ch := range allChains {
		if len(ch.Resolutions) == 0 {
			continue
		}
		newest := ch.Resolutions[len(ch.Resolutions)-1]
		snap := newest.CatalogSnapshot
		if snap == "" {
			snap = "unknown"
		}
		client := newest.Client
		if client == "" {
			client = "skillhub"
		}
		bk := bucketKey{snapshot: snap, client: client}
		groupedChains[bk] = append(groupedChains[bk], ch)

		if newest.OccurredAt.After(snapshotLatestTime[snap]) {
			snapshotLatestTime[snap] = newest.OccurredAt
		}
	}

	for key := range totalLoads {
		for i := range len(key) {
			if key[i] == 0 {
				snap := key[:i]
				client := key[i+1:]
				bk := bucketKey{snapshot: snap, client: client}
				if _, exists := groupedChains[bk]; !exists {
					groupedChains[bk] = []*rawEventChain{}
				}
				break
			}
		}
	}
	return groupedChains, snapshotLatestTime
}

func (bc *baselineContext) buildBucket(bk bucketKey, bChains []*rawEventChain) BaselineBucket {
	bucketMetrics := calculateChainMetrics(bChains, bc.counts, bc.firstValid, bc.until, "")
	resolved := bucketMetrics.ChainsResolved
	noSkill := bucketMetrics.ChainsNoSkill

	suff := BucketSufficiency{
		ResolvedChains: resolved,
		NoSkillChains:  noSkill,
		MinChains:      bc.minChains,
	}
	if resolved >= int64(bc.minChains) && noSkill >= int64(bc.minChains) {
		suff.Verdict = "sufficient"
	} else {
		suff.Verdict = "insufficient"
		if resolved < int64(bc.minChains) {
			suff.MissingResolved = int64(bc.minChains) - resolved
		}
		if noSkill < int64(bc.minChains) {
			suff.MissingNoSkill = int64(bc.minChains) - noSkill
		}
	}

	status := "superseded"
	isBaseline := false
	if bk.snapshot == bc.activeSnapshot {
		status = "baseline"
		isBaseline = true
	}

	loadKey := bk.snapshot + "\x00" + bk.client
	totLoads := bc.totalLoads[loadKey]
	unsolLoads := bc.unsolicitedLoads[loadKey]

	firstDay := ""
	if len(bChains) > 0 {
		earliest := bChains[0].Resolutions[0].OccurredAt
		for _, ch := range bChains {
			for _, res := range ch.Resolutions {
				if res.OccurredAt.Before(earliest) {
					earliest = res.OccurredAt
				}
			}
		}
		firstDay = earliest.UTC().Format("2006-01-02")
	}

	return BaselineBucket{
		CatalogSnapshot:  bk.snapshot,
		Client:           bk.client,
		Status:           status,
		IsBaselineWindow: isBaseline,
		Sufficiency:      suff,
		Metrics:          bucketMetrics,
		TotalLoads:       totLoads,
		UnsolicitedLoads: unsolLoads,
		UnsolicitedShare: makeRate(unsolLoads, totLoads),
		FirstValidDay:    firstDay,
	}
}

func (service UsageService) compileBaseline(rawEvents []telemetry.Event, since, until time.Time, minChains int, rInfo rawRetentionInfo) BaselineReport {
	allChains, counts, firstValid := service.buildChains(rawEvents)
	totalLoads, unsolicitedLoads := parseBaselineLoads(rawEvents)
	groupedChains, snapshotLatestTime := groupBaselineChains(allChains, totalLoads)

	var activeSnapshot string
	var latestTime time.Time
	for snap, t := range snapshotLatestTime {
		if activeSnapshot == "" || t.After(latestTime) {
			activeSnapshot = snap
			latestTime = t
		}
	}

	bCtx := baselineContext{
		counts:           counts,
		firstValid:       firstValid,
		totalLoads:       totalLoads,
		unsolicitedLoads: unsolicitedLoads,
		activeSnapshot:   activeSnapshot,
		until:            until,
		minChains:        minChains,
	}

	buckets := make([]BaselineBucket, 0, len(groupedChains))
	for bk, bChains := range groupedChains {
		buckets = append(buckets, bCtx.buildBucket(bk, bChains))
	}

	sort.Slice(buckets, func(i, j int) bool {
		if buckets[i].IsBaselineWindow != buckets[j].IsBaselineWindow {
			return buckets[i].IsBaselineWindow
		}
		if buckets[i].CatalogSnapshot != buckets[j].CatalogSnapshot {
			ti := snapshotLatestTime[buckets[i].CatalogSnapshot]
			tj := snapshotLatestTime[buckets[j].CatalogSnapshot]
			if !ti.Equal(tj) {
				return ti.After(tj)
			}
			return buckets[i].CatalogSnapshot < buckets[j].CatalogSnapshot
		}
		return buckets[i].Client < buckets[j].Client
	})

	now := time.Now().UTC()
	isPruned := isWindowRetentionPruned(since, until, now, rInfo)
	return BaselineReport{
		Since:              since.UTC().Format(time.RFC3339),
		Until:              until.UTC().Format(time.RFC3339),
		GeneratedAt:        now.Format(time.RFC3339),
		MinChains:          minChains,
		RawRetentionPruned: isPruned,
		ActiveSnapshot:     activeSnapshot,
		Buckets:            buckets,
	}
}

func isWindowRetentionPruned(since, until, now time.Time, rInfo rawRetentionInfo) bool {
	if rInfo.retention <= 0 {
		return false
	}
	if now.Sub(since) > rInfo.retention {
		return true
	}
	if until.Sub(since) > rInfo.retention {
		return true
	}
	if rInfo.hasOldest && since.Before(rInfo.oldestTime) && now.Sub(rInfo.oldestTime) >= rInfo.retention {
		return true
	}
	return false
}
