package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestDiscoveryTimestamps(t *testing.T) {
	board := target{Vendor: "lever", Company: "fixture"}
	old := Posting{Vendor: "lever", Company: "fixture", Id: "old"}
	added := old
	added.Id = "new"
	first := time.Date(2026, 9, 17, 1, 0, 0, 0, time.UTC)
	later := first.Add(time.Minute)
	_, baseline := compareBoard(board, []Posting{old}, boardSnapshot{}, false, first)
	if len(baseline.DiscoveredAt) != 0 {
		t.Fatal("a first scan must not claim existing jobs are fresh discoveries")
	}
	_, snapshot := compareBoard(board, []Posting{old, added}, baseline, true, later)
	if len(snapshot.DiscoveredAt) != 1 || !snapshot.DiscoveredAt[added.Key()].Equal(later) {
		t.Fatalf("only observed arrivals should get timestamps: %+v", snapshot)
	}
	_, unchanged := compareBoard(board, []Posting{added, old}, snapshot, true, later.Add(time.Minute))
	if !reflect.DeepEqual(unchanged.DiscoveredAt, snapshot.DiscoveredAt) {
		t.Fatal("rechecking a job refreshed its priority")
	}
	_, closed := compareBoard(board, []Posting{old}, unchanged, true, later.Add(2*time.Minute))
	if len(closed.DiscoveredAt) != 0 || len(unchanged.DiscoveredAt) != 1 {
		t.Fatal("closed job timestamps must be pruned without mutating previous snapshots")
	}
	reopenedAt := later.Add(3 * time.Minute)
	_, reopened := compareBoard(board, []Posting{added, old}, closed, true, reopenedAt)
	if !reopened.DiscoveredAt[added.Key()].Equal(reopenedAt) {
		t.Fatal("reappearing jobs should use their newly observed arrival time")
	}
}

func TestPrioritizePostings(t *testing.T) {
	first := time.Date(2026, 9, 17, 1, 0, 0, 0, time.UTC)
	postings := []Posting{
		{Vendor: "lever", Company: "a", Id: "baseline"},
		{Vendor: "lever", Company: "a", Id: "earlier", DiscoveredAt: first},
		{Vendor: "lever", Company: "z", Id: "new-b", DiscoveredAt: first.Add(time.Minute)},
		{Vendor: "lever", Company: "z", Id: "new-a", DiscoveredAt: first.Add(time.Minute)},
	}
	prioritizePostings(postings)
	var ids []string
	for _, p := range postings {
		ids = append(ids, p.Id)
	}
	if !reflect.DeepEqual(ids, []string{"new-a", "new-b", "earlier", "baseline"}) {
		t.Fatalf("wrong priority order: %v", ids)
	}
	prioritizePostings(nil)
	prioritizePostings(postings[:1])
}

func TestDiscoveryStateCompatibilityAndValidation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "scan-state.json")
	legacy := `{"lever:fixture":{"checked_at":"2026-09-17T01:00:00Z","keys":["lever:fixture:old"]}}`
	if err := os.WriteFile(path, []byte(legacy), 0600); err != nil {
		t.Fatal(err)
	}
	state, err := loadScanState(path)
	if err != nil || len(state["lever:fixture"].DiscoveredAt) != 0 {
		t.Fatalf("legacy snapshots must load without inventing timestamps: %v %v", state, err)
	}
	for _, timestamps := range []string{
		`{"lever:fixture:old":"0001-01-01T00:00:00Z"}`,
		`{"lever:fixture:missing":"2026-09-17T01:00:00Z"}`,
		`{"lever:fixture:old":"invalid"}`,
	} {
		body := `{"lever:fixture":{"checked_at":"2026-09-17T01:00:00Z","keys":["lever:fixture:old"],"discovered_at":` + timestamps + `}}`
		if err := os.WriteFile(path, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := loadScanState(path); err == nil {
			t.Fatalf("invalid priority history accepted: %s", body)
		}
	}
}

func TestFreshJobsAcrossBatchesAndRestart(t *testing.T) {
	gets := 0
	var delivered []string
	useScanTransport(t, func(r *http.Request) (*http.Response, error) {
		if r.Method == http.MethodPost {
			if gets != (len(delivered)/maxPerRun+1)*16 {
				t.Fatal("sending started before the full sweep finished")
			}
			var message Data
			if err := json.NewDecoder(r.Body).Decode(&message); err != nil {
				t.Fatal(err)
			}
			delivered = append(delivered, message.Content)
			return scanResponse(200, "{}"), nil
		}
		gets++
		company := filepath.Base(r.URL.Path)
		var jobs []map[string]any
		ids := []string{"old"}
		if company == "company15" {
			ids = nil
			for i := 0; i < 8; i++ {
				ids = append(ids, fmt.Sprintf("fresh-%d", i))
			}
		}
		for _, id := range ids {
			jobs = append(jobs, map[string]any{
				"id": id, "text": "Software Engineer Intern",
				"categories": map[string]string{"location": "US"},
				"hostedUrl":  "https://example.test/" + company + "/" + id,
			})
		}
		body, err := json.Marshal(jobs)
		if err != nil {
			t.Fatal(err)
		}
		return scanResponse(200, string(body)), nil
	})
	dir := t.TempDir()
	s := &scanner{state: make(scanState), path: filepath.Join(dir, "scan-state.json")}
	var targets []target
	for i := 0; i < 16; i++ {
		company := fmt.Sprintf("company%02d", i)
		targets = append(targets, target{Vendor: "lever", Company: company})
		keys := []string{"lever:" + company + ":old"}
		if i == 15 {
			keys = []string{}
		}
		s.state["lever:"+company] = boardSnapshot{CheckedAt: time.Now().Add(-time.Hour), Keys: keys}
	}
	sentPath := filepath.Join(dir, "sent.txt")
	sentLog, err := openSentLog(sentPath)
	if err != nil {
		t.Fatal(err)
	}
	defer sentLog.Close()
	sent := make(map[string]bool)
	if err := runOnce(context.Background(), s, targets, sent, sentLog, false); err != nil {
		t.Fatal(err)
	}
	if len(delivered) != maxPerRun || len(sent) != maxPerRun {
		t.Fatalf("sweep cap changed: delivered=%d sent=%d", len(delivered), len(sent))
	}
	for i, message := range delivered {
		if !strings.Contains(message, fmt.Sprintf("https://example.test/company15/fresh-%d", i)) {
			t.Fatalf("later batch's fresh jobs did not beat earlier backlog: %s", message)
		}
	}
	discoveries := s.state["lever:company15"].DiscoveredAt
	if len(discoveries) != 8 {
		t.Fatalf("all eight arrivals must retain priority, not just those sent: %v", discoveries)
	}
	// Restart both kinds of state, and reverse target order to check that neither
	// scan order nor having hit the cap changes the pending priority.
	reloaded, err := loadScanState(s.path)
	if err != nil {
		t.Fatal(err)
	}
	s = &scanner{state: reloaded, path: s.path}
	sent, err = loadSent(sentPath)
	if err != nil {
		t.Fatal(err)
	}
	for i, j := 0, len(targets)-1; i < j; i, j = i+1, j-1 {
		targets[i], targets[j] = targets[j], targets[i]
	}
	if err := runOnce(context.Background(), s, targets, sent, sentLog, false); err != nil {
		t.Fatal(err)
	}
	if len(delivered) != 2*maxPerRun || len(sent) != 2*maxPerRun {
		t.Fatalf("remaining alerts did not drain: delivered=%d sent=%d", len(delivered), len(sent))
	}
	for i := 5; i < 8; i++ {
		if !strings.Contains(delivered[i], fmt.Sprintf("https://example.test/company15/fresh-%d", i)) {
			t.Fatalf("restart lost fresh backlog priority: %s", delivered[i])
		}
	}
	if !strings.Contains(delivered[8], "/company00/old") || !strings.Contains(delivered[9], "/company01/old") {
		t.Fatal("existing backlog should fill slots after fresh jobs")
	}
	if !reflect.DeepEqual(discoveries, s.state["lever:company15"].DiscoveredAt) {
		t.Fatal("restart or unchanged scan refreshed discovery timestamps")
	}
}

func TestPriorityPreviewAndFailedSnapshot(t *testing.T) {
	useScanTransport(t, func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodGet {
			t.Fatal("preview attempted to send")
		}
		return scanResponse(200, `[{"id":"new"}]`), nil
	})
	path := filepath.Join(t.TempDir(), "scan-state.json")
	s := &scanner{state: scanState{"lever:fixture": {CheckedAt: time.Now(), Keys: []string{}}}, path: path}
	if err := saveScanState(path, s.state); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	when := time.Now().UTC()
	targets := []target{{Vendor: "lever", Company: "fixture"}}
	jobs, _, err := s.scanBatch(context.Background(), targets, true, when)
	if err != nil || len(jobs) != 1 || !jobs[0].DiscoveredAt.Equal(when) {
		t.Fatalf("preview did not assign transient priority: %+v %v", jobs, err)
	}
	after, err := os.ReadFile(path)
	if err != nil || string(after) != string(before) || len(s.state["lever:fixture"].DiscoveredAt) != 0 {
		t.Fatal("preview persisted discovery timestamps")
	}
	s.path = t.TempDir()
	if _, _, err := s.scanBatch(context.Background(), targets, false, when); err == nil {
		t.Fatal("snapshot write failure was ignored")
	}
	if len(s.state["lever:fixture"].DiscoveredAt) != 0 {
		t.Fatal("failed save advanced in-memory priority history")
	}
}

func TestSweepUsesOneDiscoveryTime(t *testing.T) {
	useScanTransport(t, func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodGet {
			t.Fatal("non-internship must not send")
		}
		return scanResponse(200, `[{"id":"new","text":"Other job"}]`), nil
	})
	s := &scanner{state: make(scanState), path: filepath.Join(t.TempDir(), "scan-state.json")}
	var targets []target
	for i := 0; i < 16; i++ {
		company := fmt.Sprintf("company%02d", i)
		targets = append(targets, target{Vendor: "lever", Company: company})
		s.state["lever:"+company] = boardSnapshot{CheckedAt: time.Now(), Keys: []string{}}
	}
	if err := runOnce(context.Background(), s, targets, nil, nil, false); err != nil {
		t.Fatal(err)
	}
	first := s.state["lever:company00"].DiscoveredAt["lever:company00:new"]
	if first.IsZero() {
		t.Fatal("missing discovery time")
	}
	for _, target := range targets {
		boardKey := target.Vendor + ":" + target.Company
		if !s.state[boardKey].DiscoveredAt[boardKey+":new"].Equal(first) {
			t.Fatal("scan order gave a later board higher priority")
		}
	}
}

func TestFailedFreshAlertKeepsPriority(t *testing.T) {
	status := 503
	var attempts []string
	useScanTransport(t, func(r *http.Request) (*http.Response, error) {
		if r.Method == http.MethodPost {
			var message Data
			if err := json.NewDecoder(r.Body).Decode(&message); err != nil {
				t.Fatal(err)
			}
			attempts = append(attempts, message.Content)
			return scanResponse(status, "{}"), nil
		}
		company := filepath.Base(r.URL.Path)
		return scanResponse(200, fmt.Sprintf(`[{"id":"intern","text":"Software Engineer Intern","categories":{"location":"US"},"hostedUrl":"https://example.test/%s"}]`, company)), nil
	})
	s := &scanner{state: scanState{
		"lever:old": {CheckedAt: time.Now(), Keys: []string{"lever:old:intern"}},
		"lever:new": {CheckedAt: time.Now(), Keys: []string{}},
	}, path: filepath.Join(t.TempDir(), "scan-state.json")}
	targets := []target{{Vendor: "lever", Company: "old"}, {Vendor: "lever", Company: "new"}}
	sent := make(map[string]bool)
	sentLog, err := openSentLog(filepath.Join(t.TempDir(), "sent.txt"))
	if err != nil {
		t.Fatal(err)
	}
	defer sentLog.Close()
	if err := runOnce(context.Background(), s, targets, sent, sentLog, false); err != nil {
		t.Fatal(err)
	}
	if len(attempts) != 1 || len(sent) != 0 || !strings.Contains(attempts[0], "https://example.test/new") {
		t.Fatalf("fresh job must be attempted first, without recording a failed send: %v %v", attempts, sent)
	}
	first := s.state["lever:new"].DiscoveredAt["lever:new:intern"]
	state, err := loadScanState(s.path)
	if err != nil {
		t.Fatal(err)
	}
	s = &scanner{state: state, path: s.path}
	status = 200
	if err := runOnce(context.Background(), s, targets, sent, sentLog, false); err != nil {
		t.Fatal(err)
	}
	if len(attempts) != 3 || !strings.Contains(attempts[1], "https://example.test/new") || !strings.Contains(attempts[2], "https://example.test/old") {
		t.Fatalf("failed alert lost its place: %v", attempts)
	}
	if !s.state["lever:new"].DiscoveredAt["lever:new:intern"].Equal(first) {
		t.Fatal("retry changed discovery time")
	}
}
