package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// loadSent reads path and returns the set of job IDs already sent to Discord.
// A missing file is not an error, it means nothing has been sent yet.
func loadSent(path string) (map[string]bool, error) {
	sent := make(map[string]bool)

	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return sent, nil
		}
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		id := strings.TrimSpace(scanner.Text())
		if id == "" {
			continue
		}
		sent[id] = true
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return sent, nil
}

// openSentLog opens path for appending, creating it if it does not exist.
// The caller owns the file and must close it.
func openSentLog(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
}

// markSent records one job id as sent by appending it to the open log.
// Written straight to the file with no buffering on purpose: a buffer that
// never gets flushed is exactly the lost-state failure dedupe exists to stop.
func markSent(f *os.File, id string) error {
	_, err := fmt.Fprintln(f, id)
	return err
}

func loadScanState(path string) (scanState, error) {
	body, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return make(scanState), nil
	}
	if err != nil {
		return nil, err
	}
	var state scanState
	if err := json.Unmarshal(body, &state); err != nil {
		return nil, fmt.Errorf("decode scan history %q: %w", path, err)
	}
	if state == nil {
		return nil, fmt.Errorf("scan history %q must be an object", path)
	}
	for key, snapshot := range state {
		if snapshot.CheckedAt.IsZero() || snapshot.Keys == nil {
			return nil, fmt.Errorf("invalid scan snapshot for %q", key)
		}
	}
	return state, nil
}

// Replace the snapshot atomically so an interrupted write leaves the previous
// complete history intact. The temporary file must be on the same filesystem.
func saveScanState(path string, state scanState) error {
	body, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".scan-state-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	defer file.Close()
	if _, err := file.Write(body); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), path)
}
