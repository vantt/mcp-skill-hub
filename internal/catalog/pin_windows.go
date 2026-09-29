//go:build windows

package catalog

import (
	"encoding/json"
	"os"

	"golang.org/x/sys/windows"
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
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(metadata.PID))
	if err == nil {
		_ = windows.CloseHandle(handle)
		return false
	}
	if err == windows.ERROR_ACCESS_DENIED {
		return false // The PID exists or cannot be proven abandoned; fail safe.
	}
	return os.Remove(path) == nil
}
