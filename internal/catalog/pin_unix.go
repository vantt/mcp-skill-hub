//go:build !windows

package catalog

import (
	"encoding/json"
	"os"
	"syscall"
)

func reclaimAbandonedPin(path string) bool {
	contents, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var metadata struct {
		PID int `json:"pid"`
	}
	if json.Unmarshal(contents, &metadata) != nil || metadata.PID <= 0 {
		return false
	}
	process, err := os.FindProcess(metadata.PID)
	if err == nil {
		err = process.Signal(syscall.Signal(0))
	}
	if err == nil || err == syscall.EPERM {
		return false
	}
	removeErr := os.Remove(path)
	return removeErr == nil || os.IsNotExist(removeErr)
}
