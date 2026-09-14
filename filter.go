package main

import (
	"html"
	"regexp"
	"strings"
)

var internshipTitlePattern = regexp.MustCompile(`(?i)\b(intern(ship)?|co[- ]?op)\b`)

var softwareRolePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\bsoftware\s+(?:engineer(?:ing)?|developer|development)\b`),
	regexp.MustCompile(`(?i)\b(?:backend|back-end)\b`),
	regexp.MustCompile(`(?i)\binfrastructure\b`),
	regexp.MustCompile(`(?i)\bplatform\s+(?:engineer(?:ing)?|developer|development)\b`),
	regexp.MustCompile(`(?i)\bsite\s+reliability\b`),
	regexp.MustCompile(`(?i)\bSRE\b`),
	regexp.MustCompile(`(?i)\bDevOps\b`),
	regexp.MustCompile(`(?i)\bcloud\s+(?:engineer(?:ing)?|developer|development)\b`),
	regexp.MustCompile(`(?i)\bfull[- ]?stack\b`),
}

// Internship and software-role filters use the title as the high-signal field.
// Keeping non-matches out of sent.txt lets a posting through later if its title
// changes to match either filter.
func isInternship(p Posting) bool {
	return internshipTitlePattern.MatchString(p.Title)
}

func isSoftwareRole(p Posting) bool {
	for _, pattern := range softwareRolePatterns {
		if pattern.MatchString(p.Title) {
			return true
		}
	}
	return false
}

type sponsorshipAssessment struct {
	LikelyBlocked bool
	Reason        string
}

type sponsorshipRule struct {
	pattern *regexp.Regexp
	exclude *regexp.Regexp
	reason  string
}

var sponsorshipRules = []sponsorshipRule{
	{
		pattern: regexp.MustCompile(`(?i)\b(?:no|without)\s+(?:current\s+or\s+future\s+)?(?:employment\s+|visa\s+)?sponsorship\b`),
		reason:  "the description says sponsorship is unavailable",
	},
	{
		pattern: regexp.MustCompile(`(?i)\b(?:will|do(?:es)?|can)\s+not(?:\s+\w+){0,5}\s+sponsor(?:ship)?\b`),
		reason:  "the description says the company will not sponsor",
	},
	{
		pattern: regexp.MustCompile(`(?i)\bunable\s+to(?:\s+\w+){0,3}\s+sponsor(?:ship)?\b`),
		reason:  "the description says the company cannot sponsor",
	},
	{
		pattern: regexp.MustCompile(`(?i)\b(?:must\s+(?:be\s+(?:an?\s+)?u\.?s\.?\s+citizens?|have\s+u\.?s\.?\s+citizenship)|requires?\s+u\.?s\.?\s+citizenship|u\.?s\.?\s+citizenship\s+(?:is\s+)?required)\b`),
		reason:  "the description requires U.S. citizenship",
	},
	{
		pattern: regexp.MustCompile(`(?i)\b(?:must\s+be\s+(?:an?\s+)?u\.?s\.?\s+persons?|requires?\s+(?:an?\s+)?u\.?s\.?\s+person(?:s|\s+status)?|u\.?s\.?\s+person\s+status\s+(?:is\s+)?required)\b`),
		reason:  "the description requires U.S. Person status",
	},
	{
		pattern: regexp.MustCompile(`(?i)\bITAR\b`),
		reason:  "the description mentions ITAR",
	},
	{
		pattern: regexp.MustCompile(`(?i)\b(?:(?:active|current)\s+(?:\w+\s+){0,3}security\s+clearance|(?:obtain|maintain|hold|possess)\s+(?:an?\s+)?(?:\w+\s+){0,3}security\s+clearance|eligible\s+for\s+(?:an?\s+)?security\s+clearance|security\s+clearance\s+(?:is\s+)?required)\b`),
		exclude: regexp.MustCompile(`(?i)\b(?:no\s+security\s+clearance\s+(?:is\s+)?required|security\s+clearance\s+(?:is\s+)?not\s+required)\b`),
		reason:  "the description requires security-clearance eligibility",
	},
}

var htmlTagPattern = regexp.MustCompile(`(?s)<[^>]*>`)

// plainText normalizes Greenhouse's HTML description into the same plain-text
// shape returned directly by Ashby and Lever.
func plainText(value string) string {
	withoutTags := htmlTagPattern.ReplaceAllString(html.UnescapeString(value), " ")
	return strings.Join(strings.Fields(withoutTags), " ")
}

func assessSponsorship(description string) sponsorshipAssessment {
	for _, rule := range sponsorshipRules {
		if rule.pattern.MatchString(description) &&
			(rule.exclude == nil || !rule.exclude.MatchString(description)) {
			return sponsorshipAssessment{
				LikelyBlocked: true,
				Reason:        rule.reason,
			}
		}
	}

	return sponsorshipAssessment{}
}
