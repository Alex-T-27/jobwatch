package main

import "regexp"

var internshipTitlePattern = regexp.MustCompile(`(?i)\b(intern(ship)?|co[- ]?op)\b`)

// isInternship uses the title because descriptions are not part of Posting yet.
// Keeping non-matches out of sent.txt lets a posting through later if its title
// changes to match the filter.
func isInternship(p Posting) bool {
	return internshipTitlePattern.MatchString(p.Title)
}
