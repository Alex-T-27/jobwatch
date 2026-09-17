package main

import (
	"context"
	"fmt"
	"log"
	"time"
)

const scanBatchSize = 15
const scanPath = "scan-state.json"

type boardSnapshot struct {
	CheckedAt time.Time `json:"checked_at"`
	Keys      []string  `json:"keys"`
}

type scanState map[string]boardSnapshot

type boardReport struct {
	Target     target
	Baseline   bool
	Total      int
	NewJobs    int
	NewMatches int
	Err        error
}

type scanner struct {
	state scanState
	path  string
}

// Scan history answers what appeared, while sent.txt answers what was delivered.
// Saving a scan must never prevent an unsent alert from being retried.
func compareBoard(t target, jobs []Posting, previous boardSnapshot, known bool) (boardReport, boardSnapshot) {
	report := boardReport{Target: t, Baseline: !known}
	snapshot := boardSnapshot{CheckedAt: time.Now().UTC(), Keys: make([]string, 0, len(jobs))}
	old := make(map[string]bool, len(previous.Keys))
	for _, key := range previous.Keys {
		old[key] = true
	}
	current := make(map[string]bool, len(jobs))
	for _, p := range jobs {
		key := p.Key()
		if current[key] {
			continue
		}
		current[key] = true
		snapshot.Keys = append(snapshot.Keys, key)
		if known && !old[key] {
			report.NewJobs++
			if assessRole(p).Decision != roleIgnore && assessLocation(p).Decision != locationIgnore &&
				assessSeason(p).Decision != seasonIgnore && assessEligibility(p.Description).Decision != eligibilityIgnore {
				report.NewMatches++
			}
		}
	}
	report.Total = len(current)
	return report, snapshot
}

func (s *scanner) scanBatch(ctx context.Context, targets []target, dryRun bool) ([]Posting, []boardReport, error) {
	next := make(scanState, len(s.state)+len(targets))
	for key, snapshot := range s.state {
		next[key] = snapshot
	}
	var postings []Posting
	var reports []boardReport
	changed := false
	for _, t := range targets {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		jobs, err := fetchJobs(ctx, t.Vendor, t.Company)
		if err != nil {
			log.Printf("%s/%s failed: %v", t.Vendor, t.Company, err)
			reports = append(reports, boardReport{Target: t, Err: err})
			continue
		}
		key := t.Vendor + ":" + t.Company
		previous, known := s.state[key]
		report, snapshot := compareBoard(t, jobs, previous, known)
		next[key] = snapshot
		changed = true
		reports = append(reports, report)
		postings = append(postings, jobs...)
	}
	if !dryRun && changed {
		if err := saveScanState(s.path, next); err != nil {
			return nil, nil, fmt.Errorf("save scan history: %w", err)
		}
		s.state = next
	}
	return postings, reports, nil
}

func formatBoardReport(r boardReport) string {
	prefix := fmt.Sprintf("%-11s %-16s", r.Target.Vendor, r.Target.Company)
	switch {
	case r.Err != nil:
		return prefix + " Check failed; retry next cycle"
	case r.Baseline:
		return fmt.Sprintf("%s Baseline: %d existing jobs", prefix, r.Total)
	case r.NewJobs == 0:
		return prefix + " No new jobs"
	default:
		return fmt.Sprintf("%s %d new jobs (%d pass alert filters, including review)", prefix, r.NewJobs, r.NewMatches)
	}
}
