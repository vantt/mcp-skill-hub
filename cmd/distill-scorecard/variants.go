package main

import (
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/vantt/skillhub-mcp/internal/distill"
)

const (
	variantFull       = "full"
	variantNoNotable  = "no-notable"
	variantNoContrast = "no-contrast"
	variantNoScore    = "no-score"
	variantFullSorted = "full-sorted"
)

// goldLesson is the stored judgement the runs are compared with; it never enters a prompt.
type goldLesson struct {
	Key       string   `json:"key"`
	Contrast  string   `json:"contrast"`
	State     string   `json:"state"`
	Relevance int      `json:"relevance"`
	Facts     []string `json:"facts"`
	Impact    int      `json:"impact"`
	Evidence  int      `json:"evidence"`
	Effort    int      `json:"effort"`
	Final     float64  `json:"final_score"`
}

// lessonMap turns a lesson into the object an agent sees: the decision never appears, because
// it would leak the answer.
func lessonMap(l distill.Lesson) (map[string]any, error) {
	raw, err := json.Marshal(l)
	if err != nil {
		return nil, err
	}
	m := map[string]any{}
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	for _, k := range []string{"decision", "status", "found_by"} {
		delete(m, k)
	}
	return m, nil
}

// variantLessons returns the lessons of one variant, with the field under test removed and the
// order shuffled by seed (except full-sorted, which keeps the stored priority order).
func variantLessons(lessons []distill.Lesson, variant string, seed int64) ([]map[string]any, error) {
	out := make([]map[string]any, 0, len(lessons))
	for _, l := range lessons {
		m, err := lessonMap(l)
		if err != nil {
			return nil, err
		}
		switch variant {
		case variantNoNotable:
			delete(m, "notable")
		case variantNoContrast:
			delete(m, "contrast")
		case variantNoScore:
			delete(m, "score")
			delete(m, "final_score")
		case variantFull, variantFullSorted:
		default:
			return nil, fmt.Errorf("unknown variant %q", variant)
		}
		out = append(out, m)
	}
	if variant != variantFullSorted {
		r := rand.New(rand.NewSource(seed))
		r.Shuffle(len(out), func(i, j int) { out[i], out[j] = out[j], out[i] })
	}
	return out, nil
}

var rubricStart = regexp.MustCompile(`(?m)^## Record everything, then weigh it\s*$`)
var rubricEnd = regexp.MustCompile(`(?m)^## Data \(the only files you write\)\s*$`)

// extractRubric copies the scoring rubric of distill-lab verbatim from its SKILL.md.
func extractRubric(skillMD string) (string, error) {
	s := rubricStart.FindStringIndex(skillMD)
	e := rubricEnd.FindStringIndex(skillMD)
	if s == nil || e == nil || e[0] <= s[0] {
		return "", fmt.Errorf("the rubric headings were not found in the distill-lab SKILL.md")
	}
	return strings.TrimSpace(skillMD[s[0]:e[0]]) + "\n", nil
}

func sha(b []byte) string { return fmt.Sprintf("%x", sha256.Sum256(b)) }

func writeFile(path string, data []byte, manifest map[string]string, root string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return err
	}
	rel, _ := filepath.Rel(root, path)
	manifest[filepath.ToSlash(rel)] = sha(data)
	return nil
}

func readInput(path string) ([]byte, error) { return os.ReadFile(path) }

func runVariants(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("variants", flag.ContinueOnError)
	fs.SetOutput(stderr)
	in := fs.String("in", "", "distill.yaml to read (read-only)")
	skillDir := fs.String("skill-dir", "", "folder holding the skill's SKILL.md and CAMPAIGN.md (read-only)")
	rubricFrom := fs.String("rubric", "internal/systemskills/distill-lab/SKILL.md", "distill-lab SKILL.md to take the rubric from")
	out := fs.String("out", "", "sandbox folder to write the variants into")
	reps := fs.Int("reps", 5, "repetitions (seeds 1..N)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *in == "" || *skillDir == "" || *out == "" {
		return fmt.Errorf("--in, --skill-dir and --out are required")
	}
	if err := checkOutsideHub(*out); err != nil {
		return err
	}
	if err := checkOutputPath(filepath.Join(*out, "manifest.json"), *in); err != nil {
		return err
	}
	doc, err := distill.LoadDocument(*in)
	if err != nil {
		return fmt.Errorf("load %s: %w", *in, err)
	}
	manifest := map[string]string{}
	inputs := filepath.Join(*out, "inputs")
	for _, name := range []string{"SKILL.md", "CAMPAIGN.md"} {
		b, err := readInput(filepath.Join(*skillDir, name))
		if err != nil {
			return err
		}
		if err := writeFile(filepath.Join(inputs, name), b, manifest, *out); err != nil {
			return err
		}
	}
	rubricSrc, err := readInput(*rubricFrom)
	if err != nil {
		return err
	}
	rubric, err := extractRubric(string(rubricSrc))
	if err != nil {
		return err
	}
	goal, _ := json.MarshalIndent(doc.Goal, "", "  ")
	gold := make([]goldLesson, 0, len(doc.Lessons))
	for _, l := range doc.Lessons {
		gold = append(gold, goldLesson{l.Key, l.Contrast, l.Decision.State, l.Score.Relevance, l.Score.Facts,
			distill.ImpactOf(l.Score.Facts), l.Score.Evidence, l.Score.Effort, finalOf(l)})
	}
	goldJSON, _ := json.MarshalIndent(gold, "", "  ")
	for name, data := range map[string][]byte{"rubric.txt": []byte(rubric), "goal.json": goal, "gold.json": goldJSON} {
		if err := writeFile(filepath.Join(inputs, name), data, manifest, *out); err != nil {
			return err
		}
	}
	for _, v := range []string{variantFull, variantNoNotable, variantNoContrast, variantNoScore, variantFullSorted} {
		for rep := 1; rep <= *reps; rep++ {
			ls, err := variantLessons(doc.Lessons, v, int64(rep))
			if err != nil {
				return err
			}
			data, _ := json.MarshalIndent(ls, "", "  ")
			if err := writeFile(filepath.Join(*out, "variants", v, fmt.Sprintf("rep%d.json", rep)), data, manifest, *out); err != nil {
				return err
			}
		}
	}
	keys := make([]string, 0, len(manifest))
	for k := range manifest {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	mdata, _ := json.MarshalIndent(map[string]any{"sha256": manifest, "files": len(keys)}, "", "  ")
	if err := os.WriteFile(filepath.Join(*out, "manifest.json"), append(mdata, '\n'), 0o644); err != nil {
		return err
	}
	_, err = fmt.Fprintf(stdout, "wrote %d files under %s\n", len(keys), *out)
	return err
}
