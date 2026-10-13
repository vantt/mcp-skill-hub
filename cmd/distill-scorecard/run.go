package main

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

//go:embed scenarios/test-audit.json
var defaultScenarios []byte

// Family is one scenario: a prompt template, the variants it runs on and its output schema.
type Family struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Variants    []string          `json:"variants"`
	Repetitions int               `json:"repetitions"`
	OnlyKeys    []string          `json:"only_keys"`
	Missing     map[string]string `json:"missing"`
	Prompt      string            `json:"prompt"`
	Schema      json.RawMessage   `json:"schema"`
}

// Judge scores the F3 answers blind to the variant that produced them.
type Judge struct {
	NoiseRuns int             `json:"noise_runs"`
	Prompt    string          `json:"prompt"`
	Schema    json.RawMessage `json:"schema"`
}

// Scenarios is the content of scenarios/test-audit.json.
type Scenarios struct {
	Families []Family `json:"families"`
	Judge    Judge    `json:"judge"`
}

func loadScenarios(path string) (Scenarios, error) {
	data := defaultScenarios
	if path != "" {
		b, err := os.ReadFile(path)
		if err != nil {
			return Scenarios{}, err
		}
		data = b
	}
	var sc Scenarios
	if err := json.Unmarshal(data, &sc); err != nil {
		return Scenarios{}, fmt.Errorf("scenarios: %w", err)
	}
	return sc, nil
}

// plannedRun is one `claude -p` call.
type plannedRun struct {
	ID       string
	Family   string
	Variant  string
	Rep      int
	Judge    bool
	Prompt   string
	Schema   json.RawMessage
	Expected []string // lesson keys the output must cover
}

// inputs holds everything a prompt is built from, read from the sandbox.
type inputs struct {
	goal, skill, campaign, rubric string
	gold                          []goldLesson
}

func readInputs(sandbox string) (inputs, error) {
	var in inputs
	read := func(name string) (string, error) {
		b, err := os.ReadFile(filepath.Join(sandbox, "inputs", name))
		return string(b), err
	}
	var err error
	if in.goal, err = read("goal.json"); err != nil {
		return in, err
	}
	if in.skill, err = read("SKILL.md"); err != nil {
		return in, err
	}
	if in.campaign, err = read("CAMPAIGN.md"); err != nil {
		return in, err
	}
	if in.rubric, err = read("rubric.txt"); err != nil {
		return in, err
	}
	goldRaw, err := read("gold.json")
	if err != nil {
		return in, err
	}
	return in, json.Unmarshal([]byte(goldRaw), &in.gold)
}

func fill(tmpl string, vars map[string]string) string {
	pairs := make([]string, 0, len(vars)*2)
	for k, v := range vars {
		pairs = append(pairs, "{{"+k+"}}", v)
	}
	return strings.NewReplacer(pairs...).Replace(tmpl)
}

func lessonsPrompt(sandbox, variant string, rep int, only []string) (string, []string, error) {
	b, err := os.ReadFile(filepath.Join(sandbox, "variants", variant, fmt.Sprintf("rep%d.json", rep)))
	if err != nil {
		return "", nil, err
	}
	var ls []map[string]any
	if err := json.Unmarshal(b, &ls); err != nil {
		return "", nil, err
	}
	var kept []map[string]any
	var keys []string
	for _, l := range ls {
		key, _ := l["key"].(string)
		if len(only) > 0 && !contains(only, key) {
			continue
		}
		kept = append(kept, l)
		keys = append(keys, key)
	}
	out, err := json.MarshalIndent(kept, "", "  ")
	return string(out), keys, err
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// planRuns lists every non-judge run, in a stable order.
func planRuns(sc Scenarios, sandbox string) ([]plannedRun, error) {
	in, err := readInputs(sandbox)
	if err != nil {
		return nil, err
	}
	var runs []plannedRun
	for _, f := range sc.Families {
		for _, v := range f.Variants {
			for rep := 1; rep <= f.Repetitions; rep++ {
				lessons, keys, err := lessonsPrompt(sandbox, v, rep, f.OnlyKeys)
				if err != nil {
					return nil, err
				}
				prompt := fill(f.Prompt, map[string]string{
					"GOAL": in.goal, "SKILL": in.skill, "CAMPAIGN": in.campaign, "RUBRIC": in.rubric,
					"LESSONS": lessons, "MISSING": f.Missing[v],
				})
				runs = append(runs, plannedRun{
					ID: fmt.Sprintf("%s-%s-r%d", strings.ToLower(f.ID), v, rep), Family: f.ID, Variant: v, Rep: rep,
					Prompt: prompt, Schema: f.Schema, Expected: keys,
				})
			}
		}
	}
	return runs, nil
}

// judgeRun builds the blinded grading prompt for one F3 answer set. The answers are shown in a
// shuffled order and never name the variant; the judge sees what and where, never notable.
// judgeSpec says which F3 run to grade, from which answers, under which run id and shuffle seed.
type judgeSpec struct {
	target  plannedRun
	answers string
	id      string
	seed    int64
}

func judgeRun(sc Scenarios, in inputs, sandbox string, spec judgeSpec) (plannedRun, error) {
	f3, answers, id, seed := spec.target, spec.answers, spec.id, spec.seed
	var parsed struct {
		Answers []map[string]any `json:"answers"`
	}
	if err := json.Unmarshal([]byte(answers), &parsed); err != nil {
		return plannedRun{}, err
	}
	rand.New(rand.NewSource(seed)).Shuffle(len(parsed.Answers), func(i, j int) {
		parsed.Answers[i], parsed.Answers[j] = parsed.Answers[j], parsed.Answers[i]
	})
	shuffled, _ := json.MarshalIndent(parsed.Answers, "", "  ")
	var items []map[string]any
	rawLessons, _, err := lessonsPrompt(sandbox, variantFull, 1, f3.Expected)
	if err != nil {
		return plannedRun{}, err
	}
	var full []map[string]any
	if err := json.Unmarshal([]byte(rawLessons), &full); err != nil {
		return plannedRun{}, err
	}
	for _, l := range full {
		items = append(items, map[string]any{"key": l["key"], "what": l["what"], "where": l["where"]})
	}
	itemsJSON, _ := json.MarshalIndent(items, "", "  ")
	prompt := fill(sc.Judge.Prompt, map[string]string{
		"GOAL": in.goal, "SKILL": in.skill, "ITEMS": string(itemsJSON), "ANSWERS": string(shuffled),
	})
	return plannedRun{ID: id, Family: "JUDGE", Variant: f3.Variant, Rep: f3.Rep, Judge: true,
		Prompt: prompt, Schema: sc.Judge.Schema, Expected: f3.Expected}, nil
}

type runOptions struct {
	sandbox, claude, model, budget, credentialEnv string
	dryRun                                        bool
	maxFailureRate                                float64
	timeout                                       time.Duration
}

// runMeta is stored next to each run's output.
type runMeta struct {
	ID       string  `json:"id"`
	Family   string  `json:"family"`
	Variant  string  `json:"variant"`
	Rep      int     `json:"rep"`
	Status   string  `json:"status"` // ok | failed
	Error    string  `json:"error,omitempty"`
	ExitCode int     `json:"exit_code"`
	Seconds  float64 `json:"seconds"`
}

func (o runOptions) commandArgs(schema json.RawMessage) []string {
	return []string{"-p", "--model", o.model, "--tools", "", "--strict-mcp-config", "--setting-sources", "",
		"--no-session-persistence", "--output-format", "json", "--json-schema", string(compactJSON(schema)),
		"--max-budget-usd", o.budget}
}

func compactJSON(raw json.RawMessage) []byte {
	var b bytes.Buffer
	if err := json.Compact(&b, raw); err != nil {
		return raw
	}
	return b.Bytes()
}

// environment is the only environment a run gets: an isolated HOME and XDG folders inside the
// sandbox, PATH, and the one credential variable the user approved.
func (o runOptions) environment() []string {
	home := filepath.Join(o.sandbox, "home")
	env := []string{
		"HOME=" + home, "XDG_CONFIG_HOME=" + filepath.Join(home, ".config"),
		"XDG_STATE_HOME=" + filepath.Join(home, ".local", "state"),
		"XDG_CACHE_HOME=" + filepath.Join(home, ".cache"),
		"XDG_DATA_HOME=" + filepath.Join(home, ".local", "share"),
		"PATH=" + os.Getenv("PATH"),
	}
	if o.credentialEnv != "" {
		if v, ok := os.LookupEnv(o.credentialEnv); ok {
			env = append(env, o.credentialEnv+"="+v)
		}
	}
	return env
}

func (o runOptions) runDir(id string) string { return filepath.Join(o.sandbox, "runs", id) }

func readMeta(dir string) (runMeta, bool) {
	b, err := os.ReadFile(filepath.Join(dir, "meta.json"))
	if err != nil {
		return runMeta{}, false
	}
	var m runMeta
	return m, json.Unmarshal(b, &m) == nil
}

// execute runs one planned run, or skips it when an earlier invocation already finished it.
func (o runOptions) execute(ctx context.Context, r plannedRun, stdout io.Writer) (runMeta, error) {
	dir := o.runDir(r.ID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return runMeta{}, err
	}
	if err := os.WriteFile(filepath.Join(dir, "prompt.txt"), []byte(r.Prompt), 0o644); err != nil {
		return runMeta{}, err
	}
	if m, ok := readMeta(dir); ok && m.Status == "ok" {
		return m, nil
	}
	args := o.commandArgs(r.Schema)
	if o.dryRun {
		if o.model == "" {
			args[2] = "<model>"
		}
		fmt.Fprintf(stdout, "%s: %s %s < %s  (prompt %d bytes)\n", r.ID, o.claude, quoteArgs(args), filepath.Join(dir, "prompt.txt"), len(r.Prompt))
		return runMeta{ID: r.ID, Status: "dry-run"}, nil
	}
	work := filepath.Join(o.sandbox, "work", r.ID)
	if err := os.MkdirAll(work, 0o755); err != nil {
		return runMeta{}, err
	}
	cctx, cancel := context.WithTimeout(ctx, o.timeout)
	defer cancel()
	cmd := exec.CommandContext(cctx, o.claude, args...)
	cmd.Dir = work
	cmd.Env = o.environment()
	cmd.Stdin = strings.NewReader(r.Prompt)
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout, cmd.Stderr = &outBuf, &errBuf
	start := time.Now()
	runErr := cmd.Run()
	meta := runMeta{ID: r.ID, Family: r.Family, Variant: r.Variant, Rep: r.Rep, Status: "ok", Seconds: time.Since(start).Seconds()}
	var exitErr *exec.ExitError
	switch {
	case errors.As(runErr, &exitErr):
		meta.ExitCode = exitErr.ExitCode()
		meta.Status, meta.Error = "failed", fmt.Sprintf("exit code %d", meta.ExitCode)
	case runErr != nil:
		meta.Status, meta.Error = "failed", runErr.Error()
	}
	_ = os.WriteFile(filepath.Join(dir, "stdout.json"), outBuf.Bytes(), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "stderr.txt"), errBuf.Bytes(), 0o644)
	if meta.Status == "ok" {
		payload, err := structuredOutput(outBuf.Bytes())
		if err == nil {
			err = validateOutput(r, payload)
		}
		if err != nil {
			meta.Status, meta.Error = "failed", err.Error()
		} else {
			_ = os.WriteFile(filepath.Join(dir, "output.json"), payload, 0o644)
		}
	}
	mb, _ := json.MarshalIndent(meta, "", "  ")
	return meta, os.WriteFile(filepath.Join(dir, "meta.json"), mb, 0o644)
}

func quoteArgs(args []string) string {
	parts := make([]string, len(args))
	for i, a := range args {
		if a == "" || strings.ContainsAny(a, " \t\n\"'{}[]") {
			a = "'" + strings.ReplaceAll(a, "'", `'\''`) + "'"
		}
		parts[i] = a
	}
	return strings.Join(parts, " ")
}

// structuredOutput extracts the agent's JSON answer from `--output-format json` output: the
// structured_output field when present, otherwise the result text.
func structuredOutput(stdout []byte) (json.RawMessage, error) {
	var env struct {
		IsError          bool            `json:"is_error"`
		Result           json.RawMessage `json:"result"`
		StructuredOutput json.RawMessage `json:"structured_output"`
	}
	if err := json.Unmarshal(stdout, &env); err != nil {
		return nil, fmt.Errorf("stdout is not JSON: %w", err)
	}
	if env.IsError {
		return nil, errors.New("claude reported an error result")
	}
	if len(env.StructuredOutput) > 0 && string(env.StructuredOutput) != "null" {
		return env.StructuredOutput, nil
	}
	var text string
	if err := json.Unmarshal(env.Result, &text); err != nil {
		return nil, errors.New("no structured_output and no result text in the output")
	}
	text = strings.TrimSpace(text)
	text = strings.TrimPrefix(strings.TrimPrefix(text, "```json"), "```")
	text = strings.TrimSuffix(text, "```")
	if !json.Valid([]byte(text)) {
		return nil, errors.New("the result text is not valid JSON")
	}
	return json.RawMessage(strings.TrimSpace(text)), nil
}

func runCommand(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.SetOutput(stderr)
	o := runOptions{}
	fs.StringVar(&o.sandbox, "sandbox", "", "sandbox folder made by `variants`")
	fs.StringVar(&o.claude, "claude", "claude", "claude executable (a stub in tests)")
	fs.StringVar(&o.model, "model", "", "pinned model id (required for a real run)")
	fs.StringVar(&o.budget, "max-budget-usd", "0.50", "spending cap per run")
	fs.StringVar(&o.credentialEnv, "credential-env", "", "name of the one credential variable to pass through")
	fs.BoolVar(&o.dryRun, "dry-run", false, "write prompts and print commands without running")
	fs.Float64Var(&o.maxFailureRate, "max-failure-rate", 0.2, "stop a family when more than this share of its runs failed")
	fs.DurationVar(&o.timeout, "timeout", 15*time.Minute, "limit for one run")
	scenariosPath := fs.String("scenarios", "", "scenario file (default: the embedded one)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if o.sandbox == "" {
		return errors.New("--sandbox is required")
	}
	if !o.dryRun && o.model == "" {
		return errors.New("--model is required unless --dry-run")
	}
	if err := checkOutsideHub(o.sandbox); err != nil {
		return err
	}
	sc, err := loadScenarios(*scenariosPath)
	if err != nil {
		return err
	}
	return o.runAll(context.Background(), sc, stdout)
}

func (o runOptions) runAll(ctx context.Context, sc Scenarios, stdout io.Writer) error {
	in, err := readInputs(o.sandbox)
	if err != nil {
		return err
	}
	runs, err := planRuns(sc, o.sandbox)
	if err != nil {
		return err
	}
	counts := map[string][2]int{} // family -> {failed, total}
	failedFamilies := map[string]bool{}
	planned := map[string]int{}
	for _, r := range runs {
		planned[r.Family]++
	}
	for _, r := range runs {
		if failedFamilies[r.Family] {
			continue
		}
		meta, err := o.execute(ctx, r, stdout)
		if err != nil {
			return err
		}
		c := counts[r.Family]
		c[1]++
		if meta.Status == "failed" {
			c[0]++
			fmt.Fprintf(stdout, "%s failed: %s\n", r.ID, meta.Error)
		}
		counts[r.Family] = c
		if float64(c[0])/float64(planned[r.Family]) > o.maxFailureRate {
			failedFamilies[r.Family] = true
			fmt.Fprintf(stdout, "stopping family %s: %d of %d planned runs failed\n", r.Family, c[0], planned[r.Family])
		}
	}
	if failedFamilies["F3"] {
		return errors.New("F3 stopped on failures; judge runs were not started")
	}
	return o.runJudges(ctx, sc, in, runs, stdout)
}

// answersFor returns the answers to grade for one F3 run: its stored output, or a stand-in for
// a dry run. ok is false when the run has no output (it failed) and so has nothing to grade.
func (o runOptions) answersFor(r plannedRun) (string, bool) {
	if o.dryRun {
		return fmt.Sprintf("{\"answers\": [{\"key\": \"(answers of %s)\"}]}", r.ID), true
	}
	b, err := os.ReadFile(filepath.Join(o.runDir(r.ID), "output.json"))
	return string(b), err == nil
}

// runJudges grades every F3 run once, and re-grades one run several times to measure judge noise.
func (o runOptions) runJudges(ctx context.Context, sc Scenarios, in inputs, runs []plannedRun, stdout io.Writer) error {
	var noiseTarget *plannedRun
	for i := range runs {
		r := runs[i]
		if r.Family != "F3" {
			continue
		}
		answers, ok := o.answersFor(r)
		if !ok {
			continue
		}
		j, err := judgeRun(sc, in, o.sandbox, judgeSpec{r, answers, "judge-" + r.ID, int64(r.Rep)})
		if err != nil {
			return err
		}
		if _, err := o.execute(ctx, j, stdout); err != nil {
			return err
		}
		if r.Variant == variantFull && r.Rep == 1 {
			noiseTarget = &runs[i]
		}
	}
	if noiseTarget == nil {
		return nil
	}
	answers, ok := o.answersFor(*noiseTarget)
	if !ok {
		return nil
	}
	for n := 1; n <= sc.Judge.NoiseRuns; n++ {
		j, err := judgeRun(sc, in, o.sandbox, judgeSpec{*noiseTarget, answers, fmt.Sprintf("judge-noise-%d", n), int64(100 + n)})
		if err != nil {
			return err
		}
		if _, err := o.execute(ctx, j, stdout); err != nil {
			return err
		}
	}
	return nil
}
