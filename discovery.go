package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const discoveryInterval = 24 * time.Hour
const discoveryTimeout = 45 * time.Second
const maxDiscoveryChecks = 20
const maxDiscoveryAdds = 10
const discoverySearchURL = "https://urlscan.io/api/v1/search/"

var discoveryHosts = []string{
	"jobs.ashbyhq.com",
	"job-boards.greenhouse.io",
	"jobs.lever.co",
	"boards.greenhouse.io",
}

var boardSlugPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,99}$`)

func discoveryDue(now, next time.Time) bool { return !now.Before(next) }

// Keep the spelling already in the config: changing it would change sent keys.
func discoveryKey(t target) string { return t.Vendor + ":" + strings.ToLower(t.Company) }

type discoveryReport struct {
	Candidates int
	Checked    int
	Added      []target
	Warnings   []string
}

// Only extract identifiers. Never fetch a URL supplied by an index result:
// the vendor adapters construct requests to their fixed public API hosts.
func targetFromBoardURL(raw string) (target, bool) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" {
		return target{}, false
	}
	var vendor string
	switch strings.ToLower(u.Host) {
	case "jobs.ashbyhq.com":
		vendor = "ashby"
	case "job-boards.greenhouse.io", "boards.greenhouse.io":
		vendor = "greenhouse"
	case "jobs.lever.co":
		vendor = "lever"
	default:
		return target{}, false
	}
	// Split before decoding: an encoded slash must not turn one identifier
	// into a different board name plus a path segment.
	parts := strings.Split(strings.TrimPrefix(u.EscapedPath(), "/"), "/")
	slug, err := url.PathUnescape(parts[0])
	if err != nil {
		return target{}, false
	}
	if vendor == "greenhouse" && slug == "embed" {
		if len(parts) != 2 || (parts[1] != "job_board" && parts[1] != "job_app") {
			return target{}, false
		}
		slug = u.Query().Get("for")
	}
	if !boardSlugPattern.MatchString(slug) {
		return target{}, false
	}
	return target{Vendor: vendor, Company: slug}, true
}

func searchBoardCandidates(ctx context.Context) ([]target, []string) {
	var candidates []target
	var warnings []string
	seen := make(map[string]bool)
	for _, host := range discoveryHosts {
		if ctx.Err() != nil {
			break
		}
		query := url.Values{
			"q":    {"page.domain:" + host + " AND date:>now-30d"},
			"size": {"100"},
		}
		var response struct {
			Results []struct {
				Page struct {
					URL string `json:"url"`
				} `json:"page"`
			} `json:"results"`
		}
		requestCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		err := getJSON(requestCtx, discoverySearchURL+"?"+query.Encode(), &response)
		cancel()
		if err == nil && response.Results == nil {
			err = errors.New("search response is missing its results array")
		}
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("search %s: %v", host, err))
			var status *httpStatusError
			if errors.As(err, &status) && (status.Code == http.StatusTooManyRequests || status.Code == http.StatusUnauthorized || status.Code == http.StatusForbidden) {
				// Quotas are shared by all four searches. Do not keep asking after
				// the index has denied access or told us to slow down.
				break
			}
			continue
		}
		for _, result := range response.Results[:min(len(response.Results), 100)] {
			if t, ok := targetFromBoardURL(result.Page.URL); ok && !seen[discoveryKey(t)] {
				seen[discoveryKey(t)] = true
				candidates = append(candidates, t)
			}
		}
	}
	return candidates, warnings
}

// Validation uses the same adapters as polling. A decodable response with real
// IDs and titles is required; an empty or malformed board can be tried later.
func hasValidPostings(jobs []Posting) bool {
	for _, p := range jobs {
		if strings.TrimSpace(p.Id) != "" && strings.TrimSpace(p.Title) != "" {
			return true
		}
	}
	return false
}

func discoverCompanies(ctx context.Context, path string, dryRun bool) ([]target, discoveryReport, error) {
	report := discoveryReport{}
	targets, err := loadTargets(path)
	if err != nil {
		return nil, report, err
	}
	ctx, cancel := context.WithTimeout(ctx, discoveryTimeout)
	defer cancel()
	candidates, warnings := searchBoardCandidates(ctx)
	report.Candidates, report.Warnings = len(candidates), warnings
	known := make(map[string]bool, len(targets))
	for _, t := range targets {
		known[discoveryKey(t)] = true
	}
	var additions []target
	for _, t := range candidates {
		if ctx.Err() != nil || report.Checked >= maxDiscoveryChecks || len(additions) >= maxDiscoveryAdds {
			break
		}
		if known[discoveryKey(t)] {
			continue
		}
		report.Checked++
		requestCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		jobs, err := fetchJobs(requestCtx, t.Vendor, t.Company)
		cancel()
		if err != nil {
			report.Warnings = append(report.Warnings, fmt.Sprintf("validate %s/%s: %v", t.Vendor, t.Company, err))
			var status *httpStatusError
			if errors.As(err, &status) && status.Code == http.StatusTooManyRequests {
				break
			}
			continue
		}
		if !hasValidPostings(jobs) {
			report.Warnings = append(report.Warnings, fmt.Sprintf("defer %s/%s: no valid postings to verify", t.Vendor, t.Company))
			continue
		}
		known[discoveryKey(t)] = true
		additions = append(additions, t)
	}
	if err := ctx.Err(); err != nil {
		return targets, report, fmt.Errorf("discovery interrupted; company config unchanged: %w", err)
	}
	// Re-read before merging so edits made while network requests were running
	// are preserved. If the edited file is invalid, leave it untouched.
	targets, err = loadTargets(path)
	if err != nil {
		return nil, report, err
	}
	known = make(map[string]bool, len(targets))
	for _, t := range targets {
		known[discoveryKey(t)] = true
	}
	for _, t := range additions {
		if !known[discoveryKey(t)] {
			targets = append(targets, t)
			known[discoveryKey(t)] = true
			report.Added = append(report.Added, t)
		}
	}
	if !dryRun && len(report.Added) > 0 {
		if err := saveJSONAtomic(path, config{Targets: targets}); err != nil {
			return nil, report, fmt.Errorf("save discovered companies: %w", err)
		}
	}
	return targets, report, nil
}

func printDiscoveryReport(report discoveryReport, dryRun bool) {
	action := "added"
	if dryRun {
		action = "would add"
	}
	fmt.Printf("Discovery: %d candidate boards, %d checked, %s %d companies\n", report.Candidates, report.Checked, action, len(report.Added))
	for _, t := range report.Added {
		fmt.Printf("  %s %s/%s\n", action, t.Vendor, t.Company)
	}
	for _, warning := range report.Warnings {
		fmt.Printf("  discovery warning: %s\n", warning)
	}
}
