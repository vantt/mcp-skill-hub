package evaluation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"sort"

	"github.com/vantt/mcp-skill-hub/internal/resolver"
)

type Runner struct {
	resolver Resolver
	pins     PinValidator
}

func NewRunner(engine Resolver, pins PinValidator) (*Runner, error) {
	if engine == nil || pins == nil {
		return nil, errors.New("resolver and pin validator are required")
	}
	return &Runner{resolver: engine, pins: pins}, nil
}

type record struct {
	outcome Outcome
	test    Case
}

func (runner *Runner) RunCase(ctx context.Context, test Case, manifest Manifest, options RunOptions) (Report, error) {
	if err := validateCase(test); err != nil {
		return Report{}, err
	}
	digest, err := DigestCase(test)
	if err != nil {
		return Report{}, err
	}
	if manifest.CaseDigest != digest {
		return Report{}, fmt.Errorf("%w: case digest", ErrPinnedArtifactUnavailable)
	}
	return runner.run(ctx, []Case{test}, manifest, options)
}
func (runner *Runner) RunSuite(ctx context.Context, suite Suite, manifest Manifest, options RunOptions) (Report, error) {
	if err := validateSuite(suite); err != nil {
		return Report{}, err
	}
	digest, err := DigestSuite(suite)
	if err != nil {
		return Report{}, err
	}
	if manifest.SuiteDigest != digest {
		return Report{}, fmt.Errorf("%w: suite digest", ErrPinnedArtifactUnavailable)
	}
	return runner.run(ctx, suite.Cases, manifest, options)
}
func (runner *Runner) run(ctx context.Context, cases []Case, manifest Manifest, options RunOptions) (Report, error) {
	if err := runner.pins.ValidatePins(ctx, manifest); err != nil {
		return Report{}, err
	}
	if err := ctx.Err(); err != nil {
		return Report{}, err
	}
	bootstrap := options.BootstrapSamples
	if bootstrap == 0 {
		bootstrap = 2000
	}
	if bootstrap < 100 || bootstrap > MaxBootstrapSamples {
		return Report{}, fmt.Errorf("bootstrap samples must be between 100 and %d", MaxBootstrapSamples)
	}
	selected := selectedPartitions(options.Partitions)
	var records []record
	var exclusions []Exclusion
	for _, test := range cases {
		if len(selected) > 0 && !selected[test.Partition] {
			continue
		}
		if test.Exclude != "" {
			exclusions = append(exclusions, Exclusion{CaseID: test.ID, Reason: test.Exclude})
			continue
		}
		response, err := runner.resolver.Resolve(ctx, test.Request)
		if err != nil {
			return Report{}, fmt.Errorf("resolve case %s: %w", test.ID, err)
		}
		if response.CatalogSnapshot != manifest.CatalogSnapshot || response.PolicyRevision != manifest.PolicyRevision || response.SchemaVersion != manifest.ProtocolSchema {
			return Report{}, fmt.Errorf("case %s returned an identity different from the manifest", test.ID)
		}
		measurements := Measurements{}
		if options.Evidence != nil {
			measurements, err = options.Evidence.Measurements(ctx, test, response)
			if err != nil {
				return Report{}, fmt.Errorf("collect measurements for case %s: %w", test.ID, err)
			}
			if err := validateMeasurements(measurements); err != nil {
				return Report{}, fmt.Errorf("case %s measurements: %w", test.ID, err)
			}
		}
		outcome := Outcome{CaseID: test.ID, Partition: test.Partition, Tags: sortedCopy(test.Tags), Status: response.Status, LatencyMS: measurements.LatencyMS, Counters: copyCounters(measurements.Counters)}
		if response.Primary != nil {
			outcome.Primary = response.Primary.ID
		}
		for _, supporting := range response.Supporting {
			outcome.Supporting = append(outcome.Supporting, supporting.ID)
		}
		sort.Strings(outcome.Supporting)
		outcome.Correct = matchesExpected(response, test.Expected)
		if len(test.Counters) > 0 {
			valid := measurementsMatch(test.Counters, measurements.Counters)
			outcome.MeasurementAssertionsValid = &valid
			outcome.Correct = outcome.Correct && valid
		}
		if response.Status == resolver.StatusNeedsContext {
			valid, err := runner.validateClarification(ctx, test, response, manifest)
			if err != nil {
				return Report{}, err
			}
			outcome.ClarificationValid = valid
			outcome.Correct = outcome.Correct && valid
		}
		records = append(records, record{outcome: outcome, test: test})
	}
	sort.Slice(exclusions, func(i, j int) bool { return exclusions[i].CaseID < exclusions[j].CaseID })
	report := Report{SchemaVersion: SchemaVersion, Manifest: manifest, Samples: len(records), Exclusions: exclusions, Outcomes: make([]Outcome, len(records))}
	for i, item := range records {
		report.Outcomes[i] = item.outcome
	}
	report.Metrics = calculateMetrics(records, manifest.Seed, bootstrap)
	report.Slices = buildSlices(records, manifest.Seed, bootstrap)
	return report, nil
}

func (runner *Runner) validateClarification(ctx context.Context, test Case, issued resolver.Response, manifest Manifest) (bool, error) {
	expected := test.Expected
	if expected.Question == nil || issued.Question == nil || issued.Question.Field != expected.Question.Field || (expected.Question.ID != "" && issued.Question.ID != expected.Question.ID) {
		return false, nil
	}
	if len(expected.Branches) == 0 {
		return true, nil
	}
	choices := map[string]bool{}
	for _, choice := range issued.Question.Choices {
		choices[choice] = true
	}
	for _, answer := range sortedKeys(expected.Branches) {
		if !choices[answer] {
			return false, nil
		}
		request := test.Request
		request.RequestID = test.Request.RequestID + "-branch-" + answer
		request.Prior = &resolver.Prior{ResolutionID: issued.ResolutionID, ContextRevision: issued.ContextRevision, Kind: "clarification", QuestionID: issued.Question.ID, Answer: answer, Basis: "evaluation-fixture"}
		response, err := runner.resolver.Resolve(ctx, request)
		if err != nil {
			return false, fmt.Errorf("resolve case %s branch %s: %w", test.ID, answer, err)
		}
		if response.CatalogSnapshot != manifest.CatalogSnapshot || response.PolicyRevision != manifest.PolicyRevision || response.SchemaVersion != manifest.ProtocolSchema {
			return false, fmt.Errorf("case %s branch %s returned an identity different from the manifest", test.ID, answer)
		}
		branch := expected.Branches[answer]
		branchExpected := Expected{AcceptableStatuses: branch.AcceptableStatuses, AcceptablePrimary: branch.AcceptablePrimary, NoSkill: branch.NoSkill}
		if !matchesExpected(response, branchExpected) {
			return false, nil
		}
	}
	return true, nil
}
func matchesExpected(response resolver.Response, expected Expected) bool {
	if !containsStatus(expected.AcceptableStatuses, response.Status) {
		return false
	}
	if len(expected.Supporting) > 0 && !sameStrings(expected.Supporting, supportingIDs(response.Supporting)) {
		return false
	}
	if response.Status == resolver.StatusResolved {
		return response.Primary != nil && containsString(expected.AcceptablePrimary, response.Primary.ID) && !containsString(expected.UnacceptablePrimary, response.Primary.ID)
	}
	if response.Status == resolver.StatusNoSkill {
		return response.NoSkill != nil
	}
	if response.Status == resolver.StatusNeedsContext {
		return response.Question != nil
	}
	return response.Status == resolver.StatusAlreadyCovered && response.CoveredBy != ""
}
func containsString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
func sortedCopy(values []string) []string {
	result := append([]string(nil), values...)
	sort.Strings(result)
	return result
}

func supportingIDs(values []resolver.Supporting) []string {
	result := make([]string, len(values))
	for index, value := range values {
		result[index] = value.ID
	}
	return result
}

func sameStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	left = sortedCopy(left)
	right = sortedCopy(right)
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func measurementsMatch(expected, observed map[string]float64) bool {
	if len(expected) != len(observed) {
		return false
	}
	for key, value := range expected {
		if observed[key] != value {
			return false
		}
	}
	return true
}

func copyCounters(values map[string]float64) map[string]float64 {
	if len(values) == 0 {
		return nil
	}
	result := make(map[string]float64, len(values))
	for key, value := range values {
		result[key] = value
	}
	return result
}

func calculateMetrics(records []record, seed int64, samples int) Metrics {
	metrics := calculatePointMetrics(records)
	selectors := []struct {
		metric *Metric
		kind   int
	}{{&metrics.AcceptableTop1, 0}, {&metrics.NoSkillPrecision, 1}, {&metrics.NoSkillRecall, 2}, {&metrics.NoSkillFalsePositive, 3}, {&metrics.ClarificationValidity, 4}}
	for _, entry := range selectors {
		entry.metric.CI95 = bootstrapInterval(records, seed+int64(entry.kind)*7919, samples, entry.kind)
	}
	return metrics
}
func calculatePointMetrics(records []record) Metrics {
	var topCorrect, topN, predNo, expectedNo, trueNo, falsePositive, clarifyN, clarifyValid int
	latencies := make([]float64, 0, len(records))
	counterSums := map[string]float64{}
	counterCounts := map[string]int{}
	for _, item := range records {
		o := item.outcome
		e := item.test.Expected
		// A different acceptable status (for example no_skill) is not a top-1 miss.
		if containsStatus(e.AcceptableStatuses, resolver.StatusResolved) && (o.Status == resolver.StatusResolved || !containsStatus(e.AcceptableStatuses, o.Status)) {
			topN++
			if o.Status == resolver.StatusResolved && containsString(e.AcceptablePrimary, o.Primary) && !containsString(e.UnacceptablePrimary, o.Primary) {
				topCorrect++
			}
		}
		expectsNo := expectsOnlyNoSkill(e.AcceptableStatuses)
		if o.Status == resolver.StatusNoSkill {
			predNo++
			if expectsNo {
				trueNo++
			}
		}
		if expectsNo {
			expectedNo++
			if o.Status == resolver.StatusResolved && !containsStatus(e.AcceptableStatuses, resolver.StatusResolved) {
				falsePositive++
			}
		}
		if containsStatus(e.AcceptableStatuses, resolver.StatusNeedsContext) {
			clarifyN++
			if o.Status == resolver.StatusNeedsContext && o.ClarificationValid {
				clarifyValid++
			}
		}
		if o.LatencyMS != nil {
			latencies = append(latencies, *o.LatencyMS)
		}
		for key, value := range o.Counters {
			counterSums[key] += value
			counterCounts[key]++
		}
	}
	counters := map[string]CounterMetric{}
	for _, key := range sortedKeys(counterSums) {
		count := counterCounts[key]
		counters[key] = CounterMetric{Samples: count, Sum: counterSums[key], Mean: safeRateFloat(counterSums[key], count)}
	}
	return Metrics{AcceptableTop1: newMetric(topCorrect, topN), NoSkillPrecision: newMetric(trueNo, predNo), NoSkillRecall: newMetric(trueNo, expectedNo), NoSkillFalsePositive: newMetric(falsePositive, expectedNo), ClarificationValidity: newMetric(clarifyValid, clarifyN), Latency: latencyMetrics(latencies), Counters: counters}
}

// expectsOnlyNoSkill reports whether no_skill is the sole acceptable status, the
// only case that is ground truth for no-skill recall and false positives.
func expectsOnlyNoSkill(statuses []resolver.Status) bool {
	for _, value := range statuses {
		if value != resolver.StatusNoSkill {
			return false
		}
	}
	return len(statuses) > 0
}

func newMetric(numerator, denominator int) Metric {
	metric := Metric{Numerator: numerator, Denominator: denominator}
	if denominator > 0 {
		value := float64(numerator) / float64(denominator)
		metric.Value = &value
	}
	return metric
}
func safeRateFloat(sum float64, count int) float64 {
	if count == 0 {
		return 0
	}
	return sum / float64(count)
}
func latencyMetrics(values []float64) LatencyMetrics {
	if len(values) == 0 {
		return LatencyMetrics{}
	}
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	mean := 0.0
	for _, v := range sorted {
		mean += v
	}
	mean /= float64(len(sorted))
	variance := 0.0
	for _, v := range sorted {
		d := v - mean
		variance += d * d
	}
	variance /= float64(len(sorted))
	p50, p95, p99 := percentile(sorted, .50), percentile(sorted, .95), percentile(sorted, .99)
	return LatencyMetrics{Samples: len(sorted), P50MS: &p50, P95MS: &p95, P99MS: &p99, MeanMS: &mean, VarianceMS2: &variance}
}
func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	index := int(math.Ceil(p*float64(len(sorted)))) - 1
	if index < 0 {
		index = 0
	}
	return sorted[index]
}
func metricValue(records []record, kind int) *float64 {
	m := calculatePointMetrics(records)
	switch kind {
	case 0:
		return m.AcceptableTop1.Value
	case 1:
		return m.NoSkillPrecision.Value
	case 2:
		return m.NoSkillRecall.Value
	case 3:
		return m.NoSkillFalsePositive.Value
	default:
		return m.ClarificationValidity.Value
	}
}
func bootstrapInterval(records []record, seed int64, samples, kind int) *Interval {
	if len(records) == 0 {
		return nil
	}
	rng := rand.New(rand.NewSource(seed))
	values := make([]float64, 0, samples)
	resampled := make([]record, len(records))
	for i := 0; i < samples; i++ {
		for j := range resampled {
			resampled[j] = records[rng.Intn(len(records))]
		}
		if value := metricValue(resampled, kind); value != nil {
			values = append(values, *value)
		}
	}
	if len(values) == 0 {
		return nil
	}
	sort.Float64s(values)
	return &Interval{Low: quantile(values, .025), High: quantile(values, .975), Samples: len(values)}
}
func quantile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	position := p * float64(len(sorted)-1)
	low := int(math.Floor(position))
	high := int(math.Ceil(position))
	if low == high {
		return sorted[low]
	}
	weight := position - float64(low)
	return sorted[low]*(1-weight) + sorted[high]*weight
}

func buildSlices(records []record, seed int64, samples int) []Slice {
	groups := map[string][]record{}
	for _, item := range records {
		groups["partition:"+string(item.outcome.Partition)] = append(groups["partition:"+string(item.outcome.Partition)], item)
		for _, tag := range item.outcome.Tags {
			groups["tag:"+tag] = append(groups["tag:"+tag], item)
		}
	}
	result := make([]Slice, 0, len(groups))
	for _, name := range sortedKeys(groups) {
		items := groups[name]
		result = append(result, Slice{Name: name, Samples: len(items), Metrics: calculateMetrics(items, seed+int64(len(name))*104729, samples)})
	}
	return result
}

func MarshalReport(report Report) ([]byte, error) {
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}
