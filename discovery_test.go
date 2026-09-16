package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestTargetFromBoardURL(t *testing.T) {
	for _, tt := range []struct {
		url  string
		want target
	}{
		{"https://jobs.ashbyhq.com/Deepgram/job-id?utm_source=test", target{"ashby", "Deepgram"}},
		{"https://jobs.ashbyhq.com/tldr.tech", target{"ashby", "tldr.tech"}},
		{"https://jobs.lever.co/spotify/id/apply", target{"lever", "spotify"}},
		{"https://job-boards.greenhouse.io/stripe/jobs/123", target{"greenhouse", "stripe"}},
		{"https://boards.greenhouse.io/stripe", target{"greenhouse", "stripe"}},
		{"https://boards.greenhouse.io/embed/job_board?for=stripe", target{"greenhouse", "stripe"}},
		{"https://boards.greenhouse.io/embed/job_app?for=stripe&token=123", target{"greenhouse", "stripe"}},
		{"https://JOBS.ASHBYHQ.COM/notion", target{"ashby", "notion"}},
	} {
		got, ok := targetFromBoardURL(tt.url)
		if !ok || got != tt.want {
			t.Errorf("%s: got %+v, %v; want %+v", tt.url, got, ok, tt.want)
		}
	}
	for _, raw := range []string{
		"", "not a URL", "http://jobs.ashbyhq.com/notion", "https://jobs.ashbyhq.com/",
		"https://jobs.ashbyhq.com.evil.test/notion", "https://evil.test/jobs.ashbyhq.com/notion",
		"https://jobs.ashbyhq.com@evil.test/notion", "https://user:pass@jobs.ashbyhq.com/notion",
		"https://jobs.ashbyhq.com:443/notion", "https://jobs.eu.lever.co/example",
		"https://jobs.ashbyhq.com/../etc", "https://jobs.ashbyhq.com/%2e%2e/etc",
		"https://jobs.ashbyhq.com/foo%2fbar", "https://jobs.ashbyhq.com/foo%3fbar",
		"https://jobs.ashbyhq.com/foo%23bar", "https://jobs.ashbyhq.com/foo%0abar",
		"https://boards.greenhouse.io/embed", "https://boards.greenhouse.io/embed/job_board",
		"https://boards.greenhouse.io/embed/job_board?for=../other", "https://boards.greenhouse.io/embed/unknown?for=stripe",
		"https://jobs.ashbyhq.com/" + strings.Repeat("a", 101),
	} {
		if got, ok := targetFromBoardURL(raw); ok {
			t.Errorf("accepted unsafe/unsupported URL %q as %+v", raw, got)
		}
	}
}

func discoveryFixture(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "companies.json")
	if err := saveJSONAtomic(path, config{Targets: []target{{Vendor: "ashby", Company: "Deepgram"}}}); err != nil {
		t.Fatal(err)
	}
	return path
}

func discoverySearchResponse(urls ...string) *http.Response {
	results := make([]any, 0, len(urls))
	for _, u := range urls {
		results = append(results, map[string]any{"page": map[string]string{"url": u}})
	}
	body, _ := json.Marshal(map[string]any{"results": results})
	return scanResponse(200, string(body))
}

func TestDiscoveryImportAndDryRun(t *testing.T) {
	path := discoveryFixture(t)
	before, _ := os.ReadFile(path)
	searches, validations := 0, 0
	useScanTransport(t, func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodGet {
			t.Fatal("discovery must not send Discord messages or submit scans")
		}
		if r.Header.Get("User-Agent") == "" {
			t.Fatal("missing identifying User-Agent")
		}
		if r.URL.Host == "urlscan.io" {
			searches++
			if r.URL.Query().Get("size") != "100" || !strings.Contains(r.URL.Query().Get("q"), "date:>now-30d") {
				t.Fatal("search is not bounded")
			}
			return discoverySearchResponse(
				"https://jobs.ashbyhq.com/deepgram/old-job",
				"https://jobs.ashbyhq.com/newco/job-a",
				"https://jobs.ashbyhq.com/newco/job-b",
				"https://jobs.ashbyhq.com/NEWCO/job-c",
				"https://jobs.ashbyhq.com/emptyco",
				"https://jobs.ashbyhq.com/deadco",
				"https://evil.test/fake",
			), nil
		}
		if r.URL.Host != "api.ashbyhq.com" {
			t.Fatalf("discovery fetched an untrusted host: %s", r.URL.Host)
		}
		validations++
		switch {
		case strings.HasSuffix(r.URL.Path, "/newco"):
			return scanResponse(200, `{"jobs":[{"id":"1","title":"Software Engineer"}]}`), nil
		case strings.HasSuffix(r.URL.Path, "/emptyco"):
			return scanResponse(200, `{"jobs":[]}`), nil
		case strings.HasSuffix(r.URL.Path, "/deadco"):
			return scanResponse(404, "not found"), nil
		default:
			t.Fatalf("known or duplicate board was revalidated: %s", r.URL.Path)
			return nil, errors.New("unexpected board")
		}
	})
	targets, report, err := discoverCompanies(context.Background(), path, true)
	if err != nil || len(targets) != 2 || len(report.Added) != 1 || report.Checked != 3 || len(report.Warnings) != 2 {
		t.Fatalf("preview: %v %+v targets=%v", err, report, targets)
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) || searches != 4 || validations != 3 {
		t.Fatalf("preview wrote config or made excessive requests: searches=%d validations=%d", searches, validations)
	}
	targets, report, err = discoverCompanies(context.Background(), path, false)
	if err != nil || len(report.Added) != 1 {
		t.Fatalf("import: %v %+v", err, report)
	}
	saved, err := loadTargets(path)
	if err != nil || !reflect.DeepEqual(targets, saved) || saved[0].Company != "Deepgram" {
		t.Fatalf("existing keys/order were not preserved: %v %v", saved, err)
	}
	_, report, err = discoverCompanies(context.Background(), path, false)
	if err != nil || len(report.Added) != 0 || report.Checked != 2 {
		t.Fatalf("repeated import is not idempotent: %v %+v", err, report)
	}
}

func TestDiscoveryLimits(t *testing.T) {
	for _, valid := range []bool{true, false} {
		t.Run(fmt.Sprint(valid), func(t *testing.T) {
			path := discoveryFixture(t)
			validations := 0
			useScanTransport(t, func(r *http.Request) (*http.Response, error) {
				if r.URL.Host == "urlscan.io" {
					var urls []string
					for i := 0; i < 40; i++ {
						urls = append(urls, fmt.Sprintf("https://jobs.ashbyhq.com/company%d", i))
					}
					return discoverySearchResponse(urls...), nil
				}
				validations++
				if valid {
					return scanResponse(200, `{"jobs":[{"id":"1","title":"Engineer"}]}`), nil
				}
				return scanResponse(404, "not found"), nil
			})
			_, report, err := discoverCompanies(context.Background(), path, false)
			if err != nil {
				t.Fatal(err)
			}
			if valid && (len(report.Added) != maxDiscoveryAdds || validations != maxDiscoveryAdds) {
				t.Fatalf("addition limit: %+v validations=%d", report, validations)
			}
			if !valid && (len(report.Added) != 0 || validations != maxDiscoveryChecks) {
				t.Fatalf("validation limit: %+v validations=%d", report, validations)
			}
		})
	}
}

func TestDiscoveryUnavailableSource(t *testing.T) {
	for _, status := range []int{401, 403, 429, 500} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			path := discoveryFixture(t)
			before, _ := os.ReadFile(path)
			requests := 0
			useScanTransport(t, func(r *http.Request) (*http.Response, error) {
				requests++
				if r.URL.Host != "urlscan.io" {
					t.Fatal("validated a board without a source candidate")
				}
				return scanResponse(status, "unavailable"), nil
			})
			targets, report, err := discoverCompanies(context.Background(), path, false)
			if err != nil || len(targets) != 1 || len(report.Added) != 0 || len(report.Warnings) == 0 {
				t.Fatalf("failed discovery must preserve current targets: %v %+v", err, report)
			}
			if status != 500 && requests != 1 {
				t.Fatal("continued querying after rate limit/access denial")
			}
			after, _ := os.ReadFile(path)
			if string(before) != string(after) {
				t.Fatal("source failure changed the config")
			}
		})
	}
}

func TestDiscoveryPreservesConcurrentConfigEdit(t *testing.T) {
	path := discoveryFixture(t)
	useScanTransport(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Host == "urlscan.io" {
			return discoverySearchResponse("https://jobs.ashbyhq.com/newco"), nil
		}
		if err := saveJSONAtomic(path, config{Targets: []target{{"ashby", "Deepgram"}, {"lever", "manual"}}}); err != nil {
			t.Fatal(err)
		}
		return scanResponse(200, `{"jobs":[{"id":"1","title":"Engineer"}]}`), nil
	})
	targets, _, err := discoverCompanies(context.Background(), path, false)
	if err != nil || len(targets) != 3 || targets[1].Company != "manual" {
		t.Fatalf("lost a manual edit during validation: %v %v", targets, err)
	}
}

func TestDiscoveryCancellation(t *testing.T) {
	path := discoveryFixture(t)
	before, _ := os.ReadFile(path)
	useScanTransport(t, func(r *http.Request) (*http.Response, error) {
		<-r.Context().Done()
		return nil, r.Context().Err()
	})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, _, err := discoverCompanies(ctx, path, false)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("request did not honor deadline: %v", err)
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatal("interrupted discovery wrote its partial results")
	}
}

func TestDiscoverySchedule(t *testing.T) {
	now := time.Date(2026, 9, 15, 0, 0, 0, 0, time.UTC)
	if !discoveryDue(now, time.Time{}) {
		t.Fatal("discovery must run at startup")
	}
	next := now.Add(discoveryInterval)
	if discoveryDue(now.Add(time.Hour), next) || discoveryDue(next.Add(-time.Nanosecond), next) {
		t.Fatal("discovery ran before the 24-hour interval")
	}
	if !discoveryDue(next, next) || !discoveryDue(next.Add(time.Minute), next) {
		t.Fatal("discovery did not become due after 24 hours")
	}
}

func TestDiscoveryStopsValidationOnThrottle(t *testing.T) {
	path := discoveryFixture(t)
	validations := 0
	useScanTransport(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Host == "urlscan.io" {
			return discoverySearchResponse("https://jobs.ashbyhq.com/first", "https://jobs.ashbyhq.com/second"), nil
		}
		validations++
		return scanResponse(429, "slow down"), nil
	})
	targets, report, err := discoverCompanies(context.Background(), path, false)
	if err != nil || validations != 1 || len(targets) != 1 || len(report.Added) != 0 {
		t.Fatalf("continued validation after throttling: %v %+v requests=%d", err, report, validations)
	}
}

func TestDiscoveryRejectsMalformedSearch(t *testing.T) {
	for _, body := range []string{`{}`, `{"results":null}`, `{"results":"wrong type"}`} {
		t.Run(body, func(t *testing.T) {
			path := discoveryFixture(t)
			useScanTransport(t, func(r *http.Request) (*http.Response, error) {
				if r.URL.Host != "urlscan.io" {
					t.Fatal("malformed search triggered board validation")
				}
				return scanResponse(200, body), nil
			})
			targets, report, err := discoverCompanies(context.Background(), path, false)
			if err != nil || len(targets) != 1 || report.Checked != 0 || len(report.Warnings) != 4 {
				t.Fatalf("malformed search was silently accepted: %v %+v", err, report)
			}
		})
	}
}

func TestDiscoveryKeepsInvalidManualEdit(t *testing.T) {
	path := discoveryFixture(t)
	useScanTransport(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Host == "urlscan.io" {
			return discoverySearchResponse("https://jobs.ashbyhq.com/newco"), nil
		}
		if err := os.WriteFile(path, []byte("unfinished manual edit"), 0600); err != nil {
			t.Fatal(err)
		}
		return scanResponse(200, `{"jobs":[{"id":"1","title":"Engineer"}]}`), nil
	})
	if _, _, err := discoverCompanies(context.Background(), path, false); err == nil {
		t.Fatal("invalid manual edit should prevent import")
	}
	body, _ := os.ReadFile(path)
	if string(body) != "unfinished manual edit" {
		t.Fatal("overwrote an invalid manual edit")
	}
}

func TestValidDiscoveryPosting(t *testing.T) {
	for _, jobs := range [][]Posting{nil, {}, {{Id: "1"}}, {{Title: "Engineer"}}, {{Id: " ", Title: "Engineer"}}} {
		if hasValidPostings(jobs) {
			t.Fatalf("accepted unverifiable postings: %+v", jobs)
		}
	}
	if !hasValidPostings([]Posting{{Id: "1", Title: "Engineer"}}) {
		t.Fatal("rejected valid posting")
	}
}
