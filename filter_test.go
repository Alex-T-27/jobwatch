package main

import "testing"

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
