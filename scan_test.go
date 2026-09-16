package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestCompareBoard(t *testing.T) {
	board := target{Vendor: "lever", Company: "fixture"}
	job := Posting{Vendor: board.Vendor, Company: board.Company, Id: "1", Title: "Software Engineer Intern", Location: "US"}
	baseline, previous := compareBoard(board, []Posting{job, job}, boardSnapshot{}, false)
	if !baseline.Baseline || baseline.NewJobs != 0 || baseline.Total != 1 {
		t.Fatalf("first scan should establish a deduplicated baseline: %+v", baseline)
	}
	if report, _ := compareBoard(board, []Posting{job}, previous, true); report.NewJobs != 0 || report.Baseline {
		t.Fatalf("unchanged board: %+v", report)
	}
	added := job
	added.Id = "2"
	foreign := job
	foreign.Id, foreign.Location = "3", "Canada"
	report, _ := compareBoard(board, []Posting{job, added, added, foreign}, previous, true)
	if report.NewJobs != 2 || report.NewMatches != 1 || report.Total != 3 {
		t.Fatalf("new IDs and eligible new IDs must be counted separately: %+v", report)
	}
	empty, emptySnapshot := compareBoard(board, nil, previous, true)
	if empty.NewJobs != 0 || empty.Baseline || empty.Total != 0 || emptySnapshot.Keys == nil {
		t.Fatalf("successful empty board: %+v %+v", empty, emptySnapshot)
	}
	reopened, _ := compareBoard(board, []Posting{job}, emptySnapshot, true)
	if reopened.NewJobs != 1 {
		t.Fatal("a reappearing ID should count as newly listed since the previous scan")
	}
}

func useScanTransport(t *testing.T, transport roundTripFunc) {
	t.Helper()
	previousClient, previousCache := client, boardsByURL
	t.Cleanup(func() { client, boardsByURL = previousClient, previousCache })
	boardsByURL = make(map[string]cachedBoard)
	client = &http.Client{Transport: transport}
}

func scanResponse(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Status: fmt.Sprint(status), Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
}

func TestScanPersistenceFailureAndRecovery(t *testing.T) {
	status, body := 200, `[{"id":"old","text":"Existing job"}]`
	useScanTransport(t, func(r *http.Request) (*http.Response, error) {
		return scanResponse(status, body), nil
	})
	path := filepath.Join(t.TempDir(), "scan-state.json")
	s := &scanner{state: make(scanState), path: path}
	targets := []target{{Vendor: "lever", Company: "fixture"}}
	_, reports, err := s.scanBatch(context.Background(), targets, false)
	if err != nil || !reports[0].Baseline {
		t.Fatalf("baseline: %v %+v", err, reports)
	}
	saved, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// Reloading simulates a process restart, including an empty HTTP cache.
	s.state, err = loadScanState(path)
	if err != nil {
		t.Fatal(err)
	}
	boardsByURL = make(map[string]cachedBoard)
	status, body = 500, "temporary failure"
	_, reports, err = s.scanBatch(context.Background(), targets, false)
	if err != nil || reports[0].Err == nil {
		t.Fatalf("failure must be reported, not presented as no change: %v %+v", err, reports)
	}
	after, err := os.ReadFile(path)
	if err != nil || string(after) != string(saved) {
		t.Fatal("failed scan changed the saved baseline")
	}
	status, body = 200, `[{"id":"old"},{"id":"new"}]`
	_, reports, err = s.scanBatch(context.Background(), targets, true)
	if err != nil || reports[0].NewJobs != 1 {
		t.Fatalf("preview must compare against the saved history: %v %+v", err, reports)
	}
	after, err = os.ReadFile(path)
	if err != nil || string(after) != string(saved) || len(s.state["lever:fixture"].Keys) != 1 {
		t.Fatal("dry run changed scan history")
	}
	_, reports, err = s.scanBatch(context.Background(), targets, false)
	if err != nil || reports[0].NewJobs != 1 || reports[0].Baseline {
		t.Fatalf("recovery should compare with last successful scan: %v %+v", err, reports)
	}
	status, body = 304, ""
	_, reports, err = s.scanBatch(context.Background(), targets, false)
	if err != nil || reports[0].Err != nil || reports[0].NewJobs != 0 {
		t.Fatalf("cached unchanged board: %v %+v", err, reports)
	}
}

func TestScanBatchIsolationAndWriteFailure(t *testing.T) {
	useScanTransport(t, func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Path, "/bad") {
			return scanResponse(503, "unavailable"), nil
		}
		return scanResponse(200, "[]"), nil
	})
	targets := []target{{Vendor: "lever", Company: "bad"}, {Vendor: "lever", Company: "good"}}
	s := &scanner{state: make(scanState), path: filepath.Join(t.TempDir(), "scan-state.json")}
	_, reports, err := s.scanBatch(context.Background(), targets, false)
	if err != nil || len(reports) != 2 || reports[0].Err == nil || !reports[1].Baseline || len(s.state) != 1 {
		t.Fatalf("one failed board must not stop the rest: %v %+v", err, reports)
	}
	previous := s.state
	// A directory is not a replaceable snapshot file.
	s.path = t.TempDir()
	_, _, err = s.scanBatch(context.Background(), []target{{Vendor: "lever", Company: "other"}}, false)
	if err == nil || !reflect.DeepEqual(previous, s.state) {
		t.Fatal("failed persistence must not advance the in-memory scan history")
	}
}

func TestScanStateValidation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "scan-state.json")
	if state, err := loadScanState(path); err != nil || len(state) != 0 {
		t.Fatalf("missing state: %v %v", state, err)
	}
	for _, body := range []string{"", "{", "null", "[]", `{"lever:fixture":{}}`} {
		if err := os.WriteFile(path, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := loadScanState(path); err == nil {
			t.Fatalf("corrupt history %q must not silently reset to baseline", body)
		}
	}
}

func TestBatchSweepCapAndUnsentBacklog(t *testing.T) {
	gets, sends := 0, 0
	useScanTransport(t, func(r *http.Request) (*http.Response, error) {
		if r.Method == http.MethodPost {
			sends++
			return scanResponse(200, "{}"), nil
		}
		gets++
		if gets%16 == 0 && sends != (gets/16)*maxPerRun {
			t.Fatal("the first batch was not processed before fetching the next batch")
		}
		return scanResponse(200, `[{"id":"intern","text":"Software Engineer Intern","categories":{"location":"US"}}]`), nil
	})
	var targets []target
	for i := 0; i < 16; i++ {
		targets = append(targets, target{Vendor: "lever", Company: fmt.Sprintf("company%d", i)})
	}
	dir := t.TempDir()
	s := &scanner{state: make(scanState), path: filepath.Join(dir, "scan-state.json")}
	sentLog, err := openSentLog(filepath.Join(dir, "sent.txt"))
	if err != nil {
		t.Fatal(err)
	}
	defer sentLog.Close()
	sent := make(map[string]bool)
	if err := runOnce(context.Background(), s, targets, sent, sentLog, false); err != nil {
		t.Fatal(err)
	}
	if gets != 16 || sends != maxPerRun || len(sent) != maxPerRun || len(s.state) != 16 {
		t.Fatalf("all batches must scan but share one send cap: gets=%d sends=%d sent=%d scans=%d", gets, sends, len(sent), len(s.state))
	}
	// All jobs are already scanned, but undelivered jobs must still be sent.
	if err := runOnce(context.Background(), s, targets, sent, sentLog, false); err != nil {
		t.Fatal(err)
	}
	if gets != 32 || sends != 2*maxPerRun || len(sent) != 2*maxPerRun {
		t.Fatalf("scan history suppressed delivery backlog: gets=%d sends=%d sent=%d", gets, sends, len(sent))
	}
}

func TestScanDoesNotSuppressFailedAlert(t *testing.T) {
	posts, sendStatus := 0, 503
	useScanTransport(t, func(r *http.Request) (*http.Response, error) {
		if r.Method == http.MethodPost {
			posts++
			return scanResponse(sendStatus, "{}"), nil
		}
		return scanResponse(200, `[{"id":"intern","text":"Software Engineer Intern","categories":{"location":"US"}}]`), nil
	})
	dir := t.TempDir()
	s := &scanner{state: make(scanState), path: filepath.Join(dir, "scan-state.json")}
	sentLog, err := openSentLog(filepath.Join(dir, "sent.txt"))
	if err != nil {
		t.Fatal(err)
	}
	defer sentLog.Close()
	sent := make(map[string]bool)
	targets := []target{{Vendor: "lever", Company: "fixture"}}
	if err := runOnce(context.Background(), s, targets, sent, sentLog, false); err != nil {
		t.Fatal(err)
	}
	if posts != 1 || len(sent) != 0 || len(s.state) != 1 {
		t.Fatalf("failed alert must be scanned but not recorded as sent: posts=%d sent=%v scans=%v", posts, sent, s.state)
	}
	sendStatus = 200
	if err := runOnce(context.Background(), s, targets, sent, sentLog, false); err != nil {
		t.Fatal(err)
	}
	if posts != 2 || !sent["lever:fixture:intern"] {
		t.Fatal("failed Discord alert was not retried on the unchanged board")
	}
}

func TestCanceledScanMakesNoRequests(t *testing.T) {
	useScanTransport(t, func(r *http.Request) (*http.Response, error) {
		t.Fatal("canceled scanner made a request")
		return nil, errors.New("unexpected request")
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	s := &scanner{state: make(scanState), path: filepath.Join(t.TempDir(), "scan-state.json")}
	err := runOnce(ctx, s, []target{{Vendor: "lever", Company: "fixture"}}, nil, nil, false)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v, want cancellation", err)
	}
}

func TestBoardReportLabels(t *testing.T) {
	for _, tt := range []struct {
		report boardReport
		want   string
	}{
		{boardReport{Baseline: true, Total: 12}, "Baseline: 12 existing jobs"},
		{boardReport{}, "No new jobs"},
		{boardReport{NewJobs: 3, NewMatches: 1}, "3 new jobs (1 pass alert filters"},
		{boardReport{Err: errors.New("timeout")}, "Check failed; retry next cycle"},
	} {
		if got := formatBoardReport(tt.report); !strings.Contains(got, tt.want) {
			t.Fatalf("got %q, want %q", got, tt.want)
		}
	}
}
