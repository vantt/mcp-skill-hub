// Command distill-scorecard measures the experimental fields of a distill.yaml file.
// It reads one file, uses no network and writes only to scratch folders.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/vantt/skillhub-mcp/internal/distill"
)

func itoa(i int) string { return strconv.Itoa(i) }

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: distill-scorecard stats|variants|run|report (see each command's flags)")
	}
	switch args[0] {
	case "stats":
		return runStats(args[1:], stdout, stderr)
	case "variants":
		return runVariants(args[1:], stdout, stderr)
	case "run":
		return runCommand(args[1:], stdout, stderr)
	case "report":
		return runReport(args[1:], stdout, stderr)
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func runStats(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("stats", flag.ContinueOnError)
	fs.SetOutput(stderr)
	in := fs.String("in", "", "distill.yaml to read (read-only)")
	asJSON := fs.Bool("json", false, "print JSON instead of a text summary")
	out := fs.String("out", "", "write the result to this file instead of stdout")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *in == "" {
		return fmt.Errorf("--in is required")
	}
	if *out != "" {
		if err := checkOutputPath(*out, *in); err != nil {
			return err
		}
	}
	doc, err := distill.LoadDocument(*in)
	if err != nil {
		return fmt.Errorf("load %s: %w", *in, err)
	}
	res := computeStats(doc.Lessons)
	var body []byte
	if *asJSON {
		body, err = json.MarshalIndent(res, "", "  ")
		if err != nil {
			return err
		}
		body = append(body, '\n')
	} else {
		body = []byte(renderText(res))
	}
	if *out == "" {
		_, err = stdout.Write(body)
		return err
	}
	return os.WriteFile(*out, body, 0o644)
}

func renderText(res Result) string {
	var b strings.Builder
	names := make([]string, 0, len(res))
	for n := range res {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		fmt.Fprintf(&b, "[%s]\n", n)
		js, _ := json.MarshalIndent(res[n], "  ", "  ")
		b.WriteString("  " + string(js) + "\n\n")
	}
	return b.String()
}
