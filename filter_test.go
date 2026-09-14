package main

import "testing"

func TestAssessRole(t *testing.T) {
	tests := []struct {
		name    string
		posting Posting
		want    roleDecision
	}{
		{
			name:    "software title",
			posting: Posting{Title: "Software Engineering Intern"},
			want:    roleMatch,
		},
		{
			name: "vague title confirmed by metadata and description",
			posting: Posting{
				Title:       "Summer Intern",
				Department:  "Engineering",
				Team:        "Platform",
				Description: "You will build backend services and REST APIs.",
			},
			want: roleMatch,
		},
		{
			name: "employment type supplies internship signal",
			posting: Posting{
				Title:          "Technology Analyst",
				EmploymentType: "Intern",
				Department:     "Engineering",
				Description:    "You will write production code for backend services.",
			},
			want: roleMatch,
		},
		{
			name: "description only goes to review",
			posting: Posting{
				Title:       "Technology Intern",
				Description: "You will implement REST APIs.",
			},
			want: roleReview,
		},
		{
			name: "vague internship goes to review",
			posting: Posting{
				Title: "Summer Intern",
			},
			want: roleReview,
		},
		{
			name:    "marketing internship",
			posting: Posting{Title: "Marketing Intern"},
			want:    roleIgnore,
		},
		{
			name: "non-software department",
			posting: Posting{
				Title:      "Summer Intern",
				Department: "Finance",
			},
			want: roleIgnore,
		},
		{
			name:    "full-time software role",
			posting: Posting{Title: "Backend Engineer", EmploymentType: "FullTime"},
			want:    roleIgnore,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := assessRole(tt.posting)
			if got.Decision != tt.want {
				t.Errorf("got decision %q, want %q", got.Decision, tt.want)
			}
		})
	}
}

func TestAssessSponsorship(t *testing.T) {
	tests := []struct {
		name          string
		description   string
		likelyBlocked bool
	}{
		{
			name:          "explicitly no sponsorship",
			description:   "Candidates must work without current or future sponsorship.",
			likelyBlocked: true,
		},
		{
			name:          "unable to sponsor",
			description:   "We are unable to provide visa sponsorship for this role.",
			likelyBlocked: true,
		},
		{
			name:          "US person",
			description:   "You must be a U.S. Person under export laws.",
			likelyBlocked: true,
		},
		{
			name:          "US citizenship",
			description:   "U.S. citizenship is required for this position.",
			likelyBlocked: true,
		},
		{
			name:          "ITAR",
			description:   "Employment is subject to ITAR restrictions.",
			likelyBlocked: true,
		},
		{
			name:          "security clearance",
			description:   "Candidates must be able to obtain a security clearance.",
			likelyBlocked: true,
		},
		{
			name:          "sponsorship offered",
			description:   "Visa sponsorship is available for qualified candidates.",
			likelyBlocked: false,
		},
		{
			name:          "ambiguous work authorization",
			description:   "You must be legally authorized to work in the United States.",
			likelyBlocked: false,
		},
		{
			name:          "clearance explicitly not required",
			description:   "No security clearance is required for this role.",
			likelyBlocked: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := assessSponsorship(tt.description)
			if got.LikelyBlocked != tt.likelyBlocked {
				t.Errorf("got LikelyBlocked %t, want %t", got.LikelyBlocked, tt.likelyBlocked)
			}
		})
	}
}
