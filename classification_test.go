package main

import (
	"strings"
	"testing"
)

func TestRoleEvidence(t *testing.T) {
	tests := []struct {
		name    string
		posting Posting
		want    roleDecision
	}{
		{"JD identifies internship", Posting{Title: "Technology Summer Analyst", Department: "Engineering", Description: "This role is a summer internship. You will build backend services."}, roleMatch},
		{"mentor interns is not an internship", Posting{Title: "Software Engineer", Description: "You will write code and mentor interns."}, roleIgnore},
		{"senior with internship boilerplate", Posting{Title: "Senior Software Engineer", Description: "Our company welcomes students. This internship is a great opportunity. Build backend services."}, roleIgnore},
		{"SWE abbreviation with JD", Posting{Title: "SWE Intern", Description: "You will write production code."}, roleMatch},
		{"conflicting title needs review", Posting{Title: "Software Engineering Recruiting Intern", Description: "Manage candidate interviews."}, roleReview},
		{"finance team coding duties", Posting{Title: "Summer Intern", Department: "Finance", Description: "You will develop software for financial systems."}, roleReview},
		{"marketing software mention", Posting{Title: "Marketing Intern", Department: "Engineering", Description: "Partner with software engineering to promote our services."}, roleIgnore},
		{"generic services are not coding", Posting{Title: "Summer Intern", Department: "Engineering", Description: "Develop customer services and maintain business applications."}, roleReview},
		{"missing JD", Posting{Title: "Software Developer Intern", Department: "Engineering"}, roleReview},
		{"confirmed JD", Posting{Title: "Software Developer Intern", Description: "You will implement REST APIs."}, roleMatch},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := assessRole(tt.posting)
			if got.Decision != tt.want {
				t.Fatalf("got %+v, want %s", got, tt.want)
			}
			if tt.want == roleMatch && !strings.Contains(got.Reason, "JD:") {
				t.Errorf("missing JD evidence: %s", got.Reason)
			}
		})
	}
}
