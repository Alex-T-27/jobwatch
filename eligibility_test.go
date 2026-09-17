package main

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAssessEligibility(t *testing.T) {
	tests := []struct {
		name, description string
		want              eligibilityDecision
	}{
		{"citizen requirement", "You must be a U.S. citizen.", eligibilityIgnore},
		{"citizenship required", "Build APIs. U.S. citizenship is required for this position. Apply today.", eligibilityIgnore},
		{"citizenship shorthand", "US citizenship required.", eligibilityIgnore},
		{"citizenship spelled out", "Applicants must have United States citizenship.", eligibilityIgnore},
		{"citizen only", "US citizens only.", eligibilityIgnore},
		{"person requirement", "You must be a U.S. Person under export laws.", eligibilityIgnore},
		{"person status required", "U.S. Person status is required for this position.", eligibilityIgnore},
		{"person only", "U.S. persons only.", eligibilityIgnore},
		{"role requirement", "This internship requires US citizenship.", eligibilityIgnore},
		{"clearance required", "An active security clearance is required.", eligibilityIgnore},
		{"obtain clearance", "Candidates must be able to obtain a security clearance.", eligibilityIgnore},
		{"maintain clearance", "You must maintain a Top Secret security clearance.", eligibilityIgnore},
		{"hold clearance", "Must hold an active TS/SCI clearance.", eligibilityIgnore},
		{"role clearance", "This position requires a Secret clearance.", eligibilityIgnore},
		{"nonsecurity clearance", "Medical clearance is required.", eligibilityReview},
		{"background check", "Candidates must pass a background check.", eligibilityReview},
		{"no citizenship", "U.S. citizenship is not required.", eligibilityReview},
		{"no citizenship prefix", "No U.S. citizenship required.", eligibilityReview},
		{"no citizen restriction", "You do not have to be a U.S. citizen.", eligibilityReview},
		{"negated requirement", "It is not true that US citizenship is required.", eligibilityReview},
		{"no clearance", "No security clearance is required.", eligibilityReview},
		{"clearance not needed", "Security clearance is not required.", eligibilityReview},
		{"optional clearance", "Active security clearance preferred.", eligibilityReview},
		{"optional citizen", "US citizenship is preferred.", eligibilityReview},
		{"alternative pathway", "You must be a US citizen or have work authorization.", eligibilityReview},
		{"export alternative", "Candidates must be a U.S. Person or eligible for an export license.", eligibilityReview},
		{"conditional clearance", "If assigned to certain projects, you must obtain a security clearance.", eligibilityReview},
		{"waiver", "US citizenship is required, but exceptions are available.", eligibilityReview},
		{"other roles", "For some roles, U.S. citizenship is required.", eligibilityReview},
		{"customer restriction", "Our customers require US citizenship for their own staff.", eligibilityReview},
		{"customer clearance", "Our clients must maintain a security clearance.", eligibilityReview},
		{"ITAR alone", "We work on ITAR projects.", eligibilityReview},
		{"ITAR unrestricted", "This role is not subject to ITAR restrictions.", eligibilityReview},
		{"no sponsorship", "We cannot provide visa sponsorship for this role.", eligibilityReview},
		{"no future sponsorship", "Candidates must work without current or future sponsorship.", eligibilityReview},
		{"sponsorship available", "Visa sponsorship is available for qualified applicants.", eligibilityReview},
		{"work authorization", "You must be legally authorized to work in the United States.", eligibilityReview},
		{"no information", "Build software and learn from the team.", eligibilityReview},
		{"empty JD", "", eligibilityReview},
		{"contradictory citizenship", "U.S. citizenship is required. US citizenship is not required.", eligibilityReview},
		{"contradictory person", "You must be a U.S. Person. US person status is optional.", eligibilityReview},
		{"contradictory clearance", "You must obtain a security clearance. No security clearance is required.", eligibilityReview},
		{"contradictory clearance reversed", "You are not required to obtain a security clearance. You must hold a security clearance.", eligibilityReview},
		{"contradictory citizen status", "You must be a US citizen. You don't have to be a US citizen.", eligibilityReview},
		{"removed citizenship requirement", "US citizenship is required. US citizenship is no longer required.", eligibilityReview},
		{"non US restriction", "Non-US citizenship is required.", eligibilityReview},
		{"independent blocker", "No security clearance is required. You must be a US citizen.", eligibilityIgnore},
		{"independent blocker after conflict", "US citizenship is required. US citizenship is not required. Candidates must hold a security clearance.", eligibilityIgnore},
		{"unrelated negation", "No prior experience needed. Candidates must be a U.S. citizen.", eligibilityIgnore},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := assessEligibility(tt.description)
			if got.Decision != tt.want || got.Reason == "" {
				t.Fatalf("got %+v, want %s with reason", got, tt.want)
			}
			if got.Decision == eligibilityIgnore && (got.Evidence == "" || !strings.Contains(tt.description, got.Evidence)) {
				t.Fatalf("blocked decision needs original JD evidence: %+v", got)
			}
		})
	}
}

func TestEligibilityEvidence(t *testing.T) {
	description := "Build APIs. U.S. citizenship is required for this position. Apply today."
	got := assessEligibility(description)
	if got.Evidence != "U.S. citizenship is required for this position." {
		t.Fatalf("lost sentence boundaries or changed JD text: %+v", got)
	}
	description = "Build APIs. We are unable to provide visa sponsorship. Apply today."
	got = assessEligibility(description)
	if got.Decision != eligibilityReview || got.Evidence != "We are unable to provide visa sponsorship." {
		t.Fatalf("missing sponsorship review evidence: %+v", got)
	}
	got = assessEligibility("US citizenship is required. US citizenship is not required.")
	if !strings.Contains(got.Reason, "conflicting") || !strings.Contains(got.Evidence, "is required") || !strings.Contains(got.Evidence, "not required") {
		t.Fatalf("conflict must show both statements: %+v", got)
	}
	formatted := formatEligibility(eligibilityAssessment{Decision: eligibilityReview, Reason: "unknown", Evidence: strings.Repeat("語", 1000)})
	if len([]rune(formatted)) > 500 || !strings.HasSuffix(formatted, "…") {
		t.Fatal("long JD evidence must be visibly truncated without breaking Unicode")
	}
	boilerplate := "We do not discriminate on the basis of race, religion, national origin, citizenship, or any other characteristic protected by law."
	if got := assessEligibility(boilerplate); got.Decision != eligibilityReview || got.Evidence != "" {
		t.Fatalf("equal-opportunity boilerplate is not eligibility evidence: %+v", got)
	}
	statement := "We cannot provide visa sponsorship."
	if got := assessEligibility(boilerplate + " " + statement); got.Evidence != statement {
		t.Fatalf("boilerplate obscured the actual sponsorship statement: %+v", got)
	}
	statement = "For US based roles only, please note the Company may not be able to employ candidates for this role who have United States work authorization related to certain U.S. visa categories, or support future H-1B sponsorship at this time."
	if got := assessEligibility(statement); got.Decision != eligibilityReview || got.Evidence != statement {
		t.Fatalf("conditional visa wording must be preserved for review: %+v", got)
	}
	statement = "To conform to US export control regulations, some of these roles may require candidates to be eligible for any required authorizations from the US government."
	if got := assessEligibility(statement); got.Decision != eligibilityReview || got.Evidence != statement {
		t.Fatalf("conditional export wording must be preserved for review: %+v", got)
	}
}

func TestEligibilityDeliveryAndUpdatedPosting(t *testing.T) {
	var messages []string
	useScanTransport(t, func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL)
		}
		var message Data
		if err := json.NewDecoder(r.Body).Decode(&message); err != nil {
			t.Fatal(err)
		}
		messages = append(messages, message.Content)
		return scanResponse(200, "{}"), nil
	})
	blocked := Posting{Vendor: "lever", Company: "fixture", Id: "blocked", Title: "Summer Software Engineer Intern", Location: "US", EmploymentType: "FullTime", Description: "U.S. citizenship is required."}
	review := blocked
	review.Id, review.Description = "review", "We cannot provide visa sponsorship."
	postings := []Posting{blocked, review}
	report, snapshot := compareBoard(target{Vendor: "lever", Company: "fixture"}, postings, boardSnapshot{}, true)
	if report.NewJobs != 2 || report.NewMatches != 1 {
		t.Fatalf("blocked job counted as eligible: %+v", report)
	}
	sent := make(map[string]bool)
	if err := deliverPostings(context.Background(), postings, sent, nil, true, &deliveryBudget{}); err != nil {
		t.Fatal(err)
	}
	if len(messages) != 0 || len(sent) != 0 {
		t.Fatal("dry run sent an alert or changed delivery state")
	}
	path := filepath.Join(t.TempDir(), "sent.txt")
	sentLog, err := openSentLog(path)
	if err != nil {
		t.Fatal(err)
	}
	defer sentLog.Close()
	if err := deliverPostings(context.Background(), postings, sent, sentLog, false, &deliveryBudget{}); err != nil {
		t.Fatal(err)
	}
	if len(messages) != 1 || !sent[review.Key()] || sent[blocked.Key()] {
		t.Fatalf("wrong delivery state: sent=%v messages=%v", sent, messages)
	}
	if !strings.Contains(messages[0], "**Eligibility:** REVIEW") || !strings.Contains(messages[0], review.Description) {
		t.Fatalf("review alert must include JD evidence: %s", messages[0])
	}
	data, err := os.ReadFile(path)
	if err != nil || strings.Contains(string(data), blocked.Key()) {
		t.Fatalf("blocked key persisted: %s, %v", data, err)
	}
	// A revised JD has the same ID. Scan history must not stop its first alert.
	blocked.Description = "U.S. citizenship is not required."
	postings[0] = blocked
	report, _ = compareBoard(target{Vendor: "lever", Company: "fixture"}, postings, snapshot, true)
	if report.NewJobs != 0 {
		t.Fatalf("a changed JD is not a new ID: %+v", report)
	}
	if err := deliverPostings(context.Background(), postings, sent, sentLog, false, &deliveryBudget{}); err != nil {
		t.Fatal(err)
	}
	if len(messages) != 2 || !sent[blocked.Key()] {
		t.Fatal("previously blocked job was not delivered after its restriction changed")
	}
}
