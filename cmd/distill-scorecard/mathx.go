package main

import (
	"bytes"
	"compress/gzip"
	"math"
	"regexp"
	"sort"
	"strings"
)

// round4 keeps printed numbers short and deterministic.
func round4(v float64) float64 { return math.Round(v*1e4) / 1e4 }

// optional returns nil for a value that is not defined, so JSON never holds NaN.
func optional(v float64, ok bool) any {
	if !ok || math.IsNaN(v) || math.IsInf(v, 0) {
		return nil
	}
	return round4(v)
}

// midRanks returns 1-based ranks where tied values share the mean of their positions.
// With descending set, the largest value gets rank 1.
func midRanks(values []float64, descending bool) []float64 {
	n := len(values)
	idx := make([]int, n)
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(a, b int) bool {
		if descending {
			return values[idx[a]] > values[idx[b]]
		}
		return values[idx[a]] < values[idx[b]]
	})
	ranks := make([]float64, n)
	for start := 0; start < n; {
		end := start
		for end+1 < n && values[idx[end+1]] == values[idx[start]] {
			end++
		}
		mean := float64(start+end)/2 + 1
		for k := start; k <= end; k++ {
			ranks[idx[k]] = mean
		}
		start = end + 1
	}
	return ranks
}

func mean(v []float64) float64 {
	if len(v) == 0 {
		return math.NaN()
	}
	s := 0.0
	for _, x := range v {
		s += x
	}
	return s / float64(len(v))
}

func median(v []float64) float64 {
	if len(v) == 0 {
		return math.NaN()
	}
	c := append([]float64(nil), v...)
	sort.Float64s(c)
	m := len(c) / 2
	if len(c)%2 == 1 {
		return c[m]
	}
	return (c[m-1] + c[m]) / 2
}

// spearman is the Pearson correlation of mid-ranks; it is undefined when either side is constant.
func spearman(x, y []float64) (float64, bool) {
	rx, ry := midRanks(x, false), midRanks(y, false)
	mx, my := mean(rx), mean(ry)
	var sxy, sxx, syy float64
	for i := range rx {
		sxy += (rx[i] - mx) * (ry[i] - my)
		sxx += (rx[i] - mx) * (rx[i] - mx)
		syy += (ry[i] - my) * (ry[i] - my)
	}
	if sxx == 0 || syy == 0 {
		return 0, false
	}
	return sxy / math.Sqrt(sxx*syy), true
}

// kendallTauB is Kendall's tau-b, which corrects for ties on both sides.
func kendallTauB(x, y []float64) (float64, bool) {
	var conc, disc, tiesX, tiesY float64
	for i := 0; i < len(x); i++ {
		for j := i + 1; j < len(x); j++ {
			dx, dy := x[i]-x[j], y[i]-y[j]
			switch {
			case dx == 0 && dy == 0:
			case dx == 0:
				tiesX++
			case dy == 0:
				tiesY++
			case (dx > 0) == (dy > 0):
				conc++
			default:
				disc++
			}
		}
	}
	denom := math.Sqrt((conc + disc + tiesX) * (conc + disc + tiesY))
	if denom == 0 {
		return 0, false
	}
	return (conc - disc) / denom, true
}

// entropy is the Shannon entropy in bits of a count table.
func entropy(counts map[string]int) float64 {
	total := 0
	for _, c := range counts {
		total += c
	}
	h := 0.0
	for _, c := range counts {
		if c == 0 {
			continue
		}
		p := float64(c) / float64(total)
		h -= p * math.Log2(p)
	}
	return h
}

// conditionalEntropy is H(target | given) = H(joint) - H(given), from paired labels.
func conditionalEntropy(target, given []string) float64 {
	joint, marginal := map[string]int{}, map[string]int{}
	for i := range target {
		joint[given[i]+"\x00"+target[i]]++
		marginal[given[i]]++
	}
	return entropy(joint) - entropy(marginal)
}

// choose returns C(n, k) as a float; the inputs here are at most a few hundred.
func choose(n, k int) float64 {
	if k < 0 || k > n {
		return 0
	}
	r := 1.0
	for i := 1; i <= k; i++ {
		r = r * float64(n-k+i) / float64(i)
	}
	return r
}

// jaccard is the Jaccard index of two word sets.
func jaccard(a, b map[string]bool) float64 {
	inter := 0
	for w := range a {
		if b[w] {
			inter++
		}
	}
	union := len(a) + len(b) - inter
	if union == 0 {
		return 0
	}
	return float64(inter) / float64(union)
}

var wordPattern = regexp.MustCompile(`[a-z0-9]+(?:['-][a-z0-9]+)*`)

var stopWords = map[string]bool{}

func init() {
	for _, w := range strings.Fields(`a about after all also an and any are as at be because been but by can could did do does
	for from had has have how if in into is it its no not of on one or other our so some than that the their them then there
	these they this to was we were what when which while who will with would you your`) {
		stopWords[w] = true
	}
}

func words(s string) []string { return wordPattern.FindAllString(strings.ToLower(s), -1) }

func contentWordSet(s string) map[string]bool {
	set := map[string]bool{}
	for _, w := range words(s) {
		if !stopWords[w] {
			set[w] = true
		}
	}
	return set
}

// trigrams returns the distinct word trigrams of a text.
func trigrams(s string) map[string]bool {
	w := words(s)
	out := map[string]bool{}
	for i := 0; i+2 < len(w); i++ {
		out[w[i]+" "+w[i+1]+" "+w[i+2]] = true
	}
	return out
}

// sharedTrigrams counts trigrams that occur in at least minLessons different texts.
func sharedTrigrams(texts []string, minLessons int) int {
	seen := map[string]int{}
	for _, t := range texts {
		for g := range trigrams(t) {
			seen[g]++
		}
	}
	n := 0
	for _, c := range seen {
		if c >= minLessons {
			n++
		}
	}
	return n
}

func gzipSize(s string) int {
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	_, _ = zw.Write([]byte(s))
	_ = zw.Close()
	return buf.Len()
}

// wordSet is the set of all words of a text, stop words included.
func wordSet(s string) map[string]bool {
	set := map[string]bool{}
	for _, w := range words(s) {
		set[w] = true
	}
	return set
}
