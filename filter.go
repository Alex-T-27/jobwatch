package main

import (
	"html"
	"regexp"
	"strings"
)

var internshipTitlePattern = regexp.MustCompile(`(?i)\b(intern(ship)?|co[- ]?op)\b`)

var softwareRolePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\b(?:SWE|SDE)\b`),
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
	regexp.MustCompile(`(?i)\b(?:write|writing|develop|developing|design|designing|implement|implementing|build|building|maintain|maintaining)\s+(?:\w+\s+){0,4}(?:code|software|APIs?|microservices|backend\s+services|distributed\s+systems|web\s+applications|mobile\s+applications)\b`),
}

// These describe the advertised position. A generic mention of interns could
// describe mentoring duties or a company's benefits, even on a senior role.
var internshipDescriptionPattern = regexp.MustCompile(`(?i)\b(?:(?:this|the|our)\s+(?:role|position|opportunity)\s+is\s+(?:an?\s+)?(?:\w+\s+){0,2}internship|(?:this|the)\s+internship\s+(?:is|will|offers|provides)|as\s+(?:an?|our)\s+(?:\w+\s+){0,3}intern\b)`)
var seniorTitlePattern = regexp.MustCompile(`(?i)\b(?:senior|sr\.?|staff|principal|director|manager|lead)\b`)

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
		internshipTitlePattern.MatchString(p.EmploymentType) ||
		(!seniorTitlePattern.MatchString(p.Title) && internshipDescriptionPattern.MatchString(p.Description))
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

	metadata := p.Department + " " + p.Team
	softwareTitle := matchesAny(p.Title, softwareRolePatterns)
	nonSoftwareTitle := matchesAny(p.Title, nonSoftwareRolePatterns)
	softwareMetadata := matchesAny(metadata, softwareMetadataPatterns)
	nonSoftwareMetadata := matchesAny(metadata, nonSoftwareRolePatterns)
	jdEvidence := ""
	for _, pattern := range softwareDescriptionPatterns {
		if match := pattern.FindString(p.Description); match != "" {
			jdEvidence = strings.Join(strings.Fields(match), " ")
			break
		}
	}
	softwareDescription := jdEvidence != ""

	if nonSoftwareTitle || nonSoftwareMetadata {
		if softwareTitle || softwareDescription {
			return roleAssessment{Decision: roleReview, Reason: "conflicting software and non-software signals; JD: " + jdEvidence}
		}
		return roleAssessment{Decision: roleIgnore, Reason: "title or metadata indicates a non-software role without coding duties in the JD"}
	}

	if softwareTitle {
		if softwareDescription {
			return roleAssessment{Decision: roleMatch, Reason: "software title; JD: " + jdEvidence}
		}
		return roleAssessment{Decision: roleReview, Reason: "software title, but the JD does not confirm coding duties"}
	}

	if softwareMetadata && softwareDescription {
		return roleAssessment{Decision: roleMatch, Reason: "engineering metadata; JD: " + jdEvidence}
	}

	if softwareMetadata {
		return roleAssessment{Decision: roleReview, Reason: "engineering metadata, but the description is inconclusive"}
	}

	if softwareDescription {
		return roleAssessment{Decision: roleReview, Reason: "software work appears only in the JD: " + jdEvidence}
	}

	// Review needs a positive signal; an internship label alone would send
	// unrelated roles such as brand design and trading into the alert queue.
	return roleAssessment{Decision: roleIgnore, Reason: "no software evidence in title, metadata, or JD"}
}

var htmlTagPattern = regexp.MustCompile(`(?s)<[^>]*>`)

// plainText normalizes Greenhouse's HTML description into the same plain-text
// shape returned directly by Ashby and Lever.
func plainText(value string) string {
	withoutTags := htmlTagPattern.ReplaceAllString(html.UnescapeString(value), " ")
	return strings.Join(strings.Fields(withoutTags), " ")
}
