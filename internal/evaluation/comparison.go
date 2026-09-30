package evaluation

import (
	"errors"
	"fmt"
	"math/rand"
	"sort"
)

func ComparePaired(baseline, candidate Report, seed int64, bootstrapSamples int) (VariantComparison, error) {
	if baseline.Manifest.Variant == "" || candidate.Manifest.Variant == "" {
		return VariantComparison{}, errors.New("both reports require variant identities")
	}
	if baseline.Manifest.SuiteDigest != candidate.Manifest.SuiteDigest || baseline.Manifest.CaseDigest != candidate.Manifest.CaseDigest {
		return VariantComparison{}, errors.New("paired reports do not use the same case or suite digest")
	}
	if bootstrapSamples == 0 {
		bootstrapSamples = 2000
	}
	if bootstrapSamples < 100 || bootstrapSamples > MaxBootstrapSamples {
		return VariantComparison{}, fmt.Errorf("bootstrap samples must be between 100 and %d", MaxBootstrapSamples)
	}
	base := map[string]Outcome{}
	for _, outcome := range baseline.Outcomes {
		base[outcome.CaseID] = outcome
	}
	cand := map[string]Outcome{}
	for _, outcome := range candidate.Outcomes {
		cand[outcome.CaseID] = outcome
	}
	if len(base) != len(cand) {
		return VariantComparison{}, errors.New("paired reports have different sample counts")
	}
	ids := sortedKeys(base)
	correctnessDiff := make([]float64, 0, len(ids))
	latencyDiff := make([]float64, 0, len(ids))
	baseCorrect := make([]float64, 0, len(ids))
	candidateCorrect := make([]float64, 0, len(ids))
	for _, id := range ids {
		left := base[id]
		right, ok := cand[id]
		if !ok {
			return VariantComparison{}, fmt.Errorf("candidate report is missing case %s", id)
		}
		b := boolFloat(left.Correct)
		c := boolFloat(right.Correct)
		baseCorrect = append(baseCorrect, b)
		candidateCorrect = append(candidateCorrect, c)
		correctnessDiff = append(correctnessDiff, c-b)
		if left.LatencyMS != nil && right.LatencyMS != nil {
			latencyDiff = append(latencyDiff, *right.LatencyMS-*left.LatencyMS)
		}
	}
	return VariantComparison{
		SchemaVersion: SchemaVersion, Baseline: baseline.Manifest.Variant, Candidate: candidate.Manifest.Variant,
		PairedSamples: len(ids), CorrectnessDelta: meanValue(correctnessDiff), CorrectnessDeltaCI95: pairedBootstrap(correctnessDiff, seed, bootstrapSamples),
		BaselineCorrectnessVariance: varianceValue(baseCorrect), CandidateCorrectnessVariance: varianceValue(candidateCorrect),
		LatencyPairedSamples: len(latencyDiff), LatencyDeltaMeanMS: meanValue(latencyDiff), LatencyDeltaVarianceMS2: varianceValue(latencyDiff),
	}, nil
}
func boolFloat(value bool) float64 {
	if value {
		return 1
	}
	return 0
}
func mean(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sum := 0.0
	for _, value := range values {
		sum += value
	}
	return sum / float64(len(values))
}
func meanValue(values []float64) *float64 {
	if len(values) == 0 {
		return nil
	}
	value := mean(values)
	return &value
}
func varianceValue(values []float64) *float64 {
	if len(values) == 0 {
		return nil
	}
	value := variance(values)
	return &value
}
func variance(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	average := mean(values)
	sum := 0.0
	for _, value := range values {
		delta := value - average
		sum += delta * delta
	}
	return sum / float64(len(values))
}
func pairedBootstrap(differences []float64, seed int64, samples int) *Interval {
	if len(differences) == 0 {
		return nil
	}
	rng := rand.New(rand.NewSource(seed))
	values := make([]float64, samples)
	for i := range values {
		sum := 0.0
		for range differences {
			sum += differences[rng.Intn(len(differences))]
		}
		values[i] = sum / float64(len(differences))
	}
	sort.Float64s(values)
	return &Interval{Low: quantile(values, .025), High: quantile(values, .975), Samples: len(values)}
}
