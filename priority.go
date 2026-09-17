package main

import "sort"

func prioritizePostings(postings []Posting) {
	// Discovery times survive send failures, the cap, and restarts. Unknown-age
	// baseline jobs remain eligible but must not outrank observed new arrivals.
	sort.SliceStable(postings, func(i, j int) bool {
		if !postings[i].DiscoveredAt.Equal(postings[j].DiscoveredAt) {
			return postings[i].DiscoveredAt.After(postings[j].DiscoveredAt)
		}
		// A deterministic tie-breaker avoids depending on vendor response order.
		return postings[i].Key() < postings[j].Key()
	})
}
