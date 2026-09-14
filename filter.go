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

var softwareMetadataPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\bsoftware\b`),
	regexp.MustCompile(`(?i)\bengineering\b`),
	regexp.MustCompile(`(?i)\bbackend\b`),
	regexp.MustCompile(`(?i)\binfrastructure\b`),
	regexp.MustCompile(`(?i)\bplatform\b`),
	regexp.MustCompile(`(?i)\bsite\s+reliability\b`),
	regexp.MustCompile(`(?i)\bDevOps\b`),
	regexp.MustCompile(`(?i)\bcloud\b`),
	regexp.MustCompile(`(?i)\btechnology\b`),
}

var softwareDescriptionPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\b(?:write|writing|develop|developing|design|designing|implement|implementing|build|building|maintain|maintaining)\s+(?:\w+\s+){0,5}(?:code|software|backends?|APIs?|services?|applications?)\b`),
	regexp.MustCompile(`(?i)\bsoftware\s+(?:development|engineering)\b`),
	regexp.MustCompile(`(?i)\b(?:distributed\s+systems?|microservices?|REST(?:ful)?\s+APIs?)\b`),
	regexp.MustCompile(`(?i)\b(?:programming|coding)\s+(?:experience|skills?|languages?)\b`),
}

var nonSoftwareRolePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\b(?:marketing|sales|finance|accounting|legal|recruiting)\b`),
	regexp.MustCompile(`(?i)\b(?:human\s+resources|people\s+operations|customer\s+success)\b`),
	regexp.MustCompile(`(?i)\bproduct\s+(?:manager|management)\b`),
	regexp.MustCompile(`(?i)\b(?:graphic|product|UX|UI)\s+design(?:er)?\b`),
	regexp.MustCompile(`(?i)\bdeveloper\s+relations\b`),
	regexp.MustCompile(`(?i)\b(?:hardware|mechanical|electrical|civil|chemical|manufacturing)\s+engineering\b`),
}

func isInternship(p Posting) bool {
	return internshipTitlePattern.MatchString(p.Title) ||
		internshipTitlePattern.MatchString(p.EmploymentType)
}

type roleDecision string

const (
	roleIgnore roleDecision = "ignore"
	roleReview roleDecision = "review"
	roleMatch  roleDecision = "match"
)

type roleAssessment struct {
	Decision roleDecision
	Reason   string
}

func matchesAny(value string, patterns []*regexp.Regexp) bool {
	for _, pattern := range patterns {
		if pattern.MatchString(value) {
			return true
		}
	}
	return false
}

func assessRole(p Posting) roleAssessment {
	if !isInternship(p) {
		return roleAssessment{Decision: roleIgnore, Reason: "no internship signal"}
	}

	if matchesAny(p.Title, softwareRolePatterns) {
		return roleAssessment{Decision: roleMatch, Reason: "software signal in the title"}
	}

	if matchesAny(p.Title, nonSoftwareRolePatterns) {
		return roleAssessment{Decision: roleIgnore, Reason: "title indicates a non-software role"}
	}

	metadata := p.Department + " " + p.Team
	softwareMetadata := matchesAny(metadata, softwareMetadataPatterns)
	nonSoftwareMetadata := matchesAny(metadata, nonSoftwareRolePatterns)
	softwareDescription := matchesAny(p.Description, softwareDescriptionPatterns)

	if nonSoftwareMetadata {
		return roleAssessment{Decision: roleIgnore, Reason: "department or team indicates a non-software role"}
	}

	if softwareMetadata && softwareDescription {
		return roleAssessment{Decision: roleMatch, Reason: "engineering metadata and software work in the description"}
	}

	if softwareMetadata {
		return roleAssessment{Decision: roleReview, Reason: "engineering metadata, but the description is inconclusive"}
	}

	if softwareDescription {
		return roleAssessment{Decision: roleReview, Reason: "software work appears only in the description"}
	}

	return roleAssessment{Decision: roleReview, Reason: "internship title is too vague to classify safely"}
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
