package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const defaultProcessEventPath = "data/process-events.jsonl"

var processEventMu sync.Mutex

type processEventRecord struct {
	At      time.Time      `json:"at"`
	Process string         `json:"process"`
	Venue   string         `json:"venue,omitempty"`
	RunID   string         `json:"run_id,omitempty"`
	Event   string         `json:"event"`
	Error   string         `json:"error,omitempty"`
	Details map[string]any `json:"details,omitempty"`
}

func emitProcessEvent(process, venue, runID, event, errorText string, details map[string]any) {
	path := processEventPath()
	if path == "" {
		return
	}
	record := processEventRecord{
		At:      time.Now().UTC(),
		Process: process,
		Venue:   venue,
		RunID:   runID,
		Event:   event,
		Error:   errorText,
		Details: details,
	}
	if record.Process == "" || record.Event == "" {
		return
	}
	data, err := json.Marshal(record)
	if err != nil {
		return
	}

	processEventMu.Lock()
	defer processEventMu.Unlock()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer file.Close()
	_, _ = file.Write(append(data, '\n'))
}

func processEventPath() string {
	path := strings.TrimSpace(os.Getenv("PERPS_BENCH_PROCESS_EVENTS"))
	switch strings.ToLower(path) {
	case "off", "none", "disabled":
		return ""
	case "":
		return defaultProcessEventPath
	default:
		return path
	}
}
