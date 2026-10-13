package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// checkOutputPath refuses an output file that is the input, or that lies inside a Skill Hub
// hub (any folder holding .skillhub/schema-version). The tool only writes to scratch folders.
func checkOutputPath(out, in string) error {
	absOut, err := filepath.Abs(out)
	if err != nil {
		return err
	}
	absIn, err := filepath.Abs(in)
	if err != nil {
		return err
	}
	if absOut == absIn {
		return errors.New("--out must not be the input file")
	}
	if a, errA := os.Stat(absOut); errA == nil {
		if b, errB := os.Stat(absIn); errB == nil && os.SameFile(a, b) {
			return errors.New("--out must not be the input file")
		}
	}
	return checkOutsideHub(filepath.Dir(absOut))
}

// checkOutsideHub refuses a directory that is, or is inside, a hub.
func checkOutsideHub(dir string) error {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	// Resolve the deepest existing ancestor so a symlink cannot hide a hub.
	existing := abs
	for {
		if _, statErr := os.Stat(existing); statErr == nil {
			break
		}
		parent := filepath.Dir(existing)
		if parent == existing {
			break
		}
		existing = parent
	}
	if resolved, evalErr := filepath.EvalSymlinks(existing); evalErr == nil {
		existing = resolved
	}
	for cur := existing; ; {
		marker := filepath.Join(cur, ".skillhub", "schema-version")
		if _, statErr := os.Stat(marker); statErr == nil {
			return fmt.Errorf("refusing to write inside the hub at %s", cur)
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return nil
		}
		cur = parent
	}
}
