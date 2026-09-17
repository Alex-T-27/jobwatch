package main

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAssessSeason(t *testing.T) {
	tests := []struct {
		name, title, description, employment string
		want                                 seasonDecision
	}{
		{"summer full time", "Summer Software Intern", "", "FullTime", seasonMatch},
		{"summer part time", "Summer Software Intern", "", "PartTime", seasonIgnore},
		{"fall part time", "Fall Software Intern", "", "Part-time", seasonMatch},
		{"fall full time", "Fall Software Intern", "", "Full-time", seasonIgnore},
		{"winter part time", "Winter Software Intern", "", "PartTime", seasonMatch},
		{"winter full time", "Winter Software Intern", "", "FullTime", seasonIgnore},
		{"spring part time", "Spring Software Intern", "", "PartTime", seasonMatch},
		{"spring full time", "Spring Software Intern", "", "FullTime", seasonIgnore},
		{"autumn alias", "Autumn Software Intern", "", "PartTime", seasonMatch},
		{"unicode hyphen", "Summer full‑time Software Intern", "", "", seasonMatch},
		{"unknown season", "Software Intern", "This internship is full-time.", "", seasonReview},
		{"unknown workload", "Summer Software Intern", "", "Intern", seasonReview},
		{"both unknown", "Software Intern", "", "", seasonReview},
		{"JD schedule", "Software Intern", "This internship is full-time during summer.", "Intern", seasonMatch},
		{"JD workload", "Winter Software Intern", "This role is full-time.", "Intern", seasonIgnore},
		{"JD separated schedule", "Software Intern", "This internship takes place in spring. This role is part-time.", "", seasonMatch},
		{"multiple seasons", "Summer or Winter Software Intern", "", "FullTime", seasonMatch},
		{"paired good options", "Software Intern", "This internship is full-time in summer or part-time in winter.", "", seasonMatch},
		{"paired bad options", "Software Intern", "This internship is part-time in summer or full-time in winter.", "", seasonIgnore},
		{"unpaired options", "Software Intern", "This internship is full-time or part-time during summer and winter.", "", seasonReview},
		{"partially paired options", "Software Intern", "This internship is part-time in summer or full-time.", "", seasonReview},
		{"conflicting workloads", "Winter Software Intern", "This role is part-time.", "FullTime", seasonReview},
		{"conflicting seasons", "Summer Software Intern", "This internship runs in winter.", "FullTime", seasonReview},
		{"graduation is not season", "Software Intern", "Intern applicants must graduate in spring 2028.", "FullTime", seasonReview},
		{"graduation does not conflict", "Summer Software Intern", "Intern applicants graduate in spring 2028.", "FullTime", seasonMatch},
		{"conversion not workload", "Winter Software Intern", "This internship can convert to a full-time role.", "", seasonReview},
		{"return offer not workload", "Winter Software Intern", "Interns may receive full-time offers.", "", seasonReview},
		{"future employment not workload", "Winter Software Intern", "Interns may receive full-time employment opportunities.", "", seasonReview},
		{"benefits not workload", "Winter Software Intern", "Interns work with full-time employees who receive benefits.", "", seasonReview},
		{"student status not workload", "Winter Software Intern", "Intern applicants must be full-time students.", "", seasonReview},
		{"enrollment not workload", "Winter Software Intern", "Interns must maintain full-time enrollment.", "", seasonReview},
		{"past season", "Software Intern", "Our interns worked full-time last summer.", "", seasonReview},
		{"negated schedule", "Winter Software Intern", "This internship is not full-time.", "", seasonReview},
		{"negated title", "Winter Software Intern (not part-time)", "", "", seasonReview},
		{"spring boot not season", "Spring Boot Software Intern", "", "FullTime", seasonReview},
		{"hours need review", "Winter Software Intern", "Interns work 40 hours per week.", "Intern", seasonReview},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := assessSeason(Posting{Title: tt.title, Description: tt.description, EmploymentType: tt.employment})
			if got.Decision != tt.want || got.Reason == "" {
				t.Fatalf("got %+v, want %s with a reason", got, tt.want)
			}
		})
	}
}

func TestSeasonFilteringDeliveryAndBoardReport(t *testing.T) {
	var messages []string
	useScanTransport(t, func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		messages = append(messages, string(body))
		return scanResponse(200, "{}"), nil
	})
	postings := []Posting{
		{Vendor: "lever", Company: "fixture", Id: "skip", Title: "Winter Software Engineer Intern", Location: "US", EmploymentType: "FullTime"},
		{Vendor: "lever", Company: "fixture", Id: "review", Title: "Winter Software Engineer Intern", Location: "US"},
		{Vendor: "lever", Company: "fixture", Id: "match", Title: "Summer Software Engineer Intern", Location: "US", EmploymentType: "FullTime"},
	}
	report, _ := compareBoard(target{Vendor: "lever", Company: "fixture"}, postings, boardSnapshot{}, true, time.Now())
	if report.NewJobs != 3 || report.NewMatches != 2 {
		t.Fatalf("schedule skips must not count as eligible matches: %+v", report)
	}
	path := filepath.Join(t.TempDir(), "sent.txt")
	sentLog, err := openSentLog(path)
	if err != nil {
		t.Fatal(err)
	}
	defer sentLog.Close()
	sent := make(map[string]bool)
	if err := deliverPostings(context.Background(), postings, sent, sentLog, false, &deliveryBudget{}); err != nil {
		t.Fatal(err)
	}
	if len(messages) != 2 || len(sent) != 2 || sent[postings[0].Key()] {
		t.Fatalf("only match/review should be sent: messages=%d sent=%v", len(messages), sent)
	}
	if !strings.Contains(messages[0], "REVIEW: season or workload unclear") || !strings.Contains(messages[1], "summer full-time fits") {
		t.Fatalf("alerts must explain schedule decisions: %v", messages)
	}
	saved, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(saved), postings[0].Key()) || !strings.Contains(string(saved), postings[1].Key()) || !strings.Contains(string(saved), postings[2].Key()) {
		t.Fatalf("rejected schedules must not be recorded as delivered: %s", saved)
	}
}
