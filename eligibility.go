package main

import (
	"regexp"
	"strings"
)

type eligibilityDecision string

const (
	eligibilityReview eligibilityDecision = "review"
	eligibilityIgnore eligibilityDecision = "ignore"
)

type eligibilityAssessment struct {
	Decision eligibilityDecision
	Reason   string
	Evidence string
}

// These are Alex's hard restrictions, not a claim that every other applicant
// is ineligible. Silence about restrictions never proves eligibility.
type eligibilityRule struct {
	requirement *regexp.Regexp
	denial      *regexp.Regexp
	reason      string
}

const usEligibility = `(?:U\.?S\.?|United States)`
const candidateRequirement = `(?:^|\b(?:you|candidates|applicants|interns)\s+)must\s+`
const citizenshipTerm = usEligibility + `\s+citizenship`
const personTerm = usEligibility + `\s+person\s+status`
const clearanceTerm = `(?:(?:active|current)\s+)?(?:(?:secret|top[ -]secret|TS/SCI)\s+(?:security\s+)?clearance|security\s+clearance)`
const notRequiredTo = `(?:not\s+required\s+to|do(?:es)?\s+not\s+(?:have|need)\s+to|don['’]t\s+(?:have|need)\s+to)`

var eligibilityRules = []eligibilityRule{
	{
		requirement: regexp.MustCompile(`(?i)(?:` + candidateRequirement + `(?:be\s+(?:an?\s+)?` + usEligibility + `\s+citizens?|have\s+` + citizenshipTerm + `)\b|\b` + citizenshipTerm + `\s+(?:is\s+)?required\b|\b(?:this|the)\s+(?:role|position|internship)\s+requires\s+` + citizenshipTerm + `\b|^` + usEligibility + `\s+citizens?\s+only\b)`),
		denial:      regexp.MustCompile(`(?i)(?:\b` + citizenshipTerm + `\s+(?:is\s+)?(?:not\s+required|no\s+longer\s+required|optional)|\b` + notRequiredTo + `\s+(?:be\s+(?:an?\s+)?` + usEligibility + `\s+citizen|have\s+` + citizenshipTerm + `))`),
		reason:      "JD explicitly requires U.S. citizenship",
	},
	{
		requirement: regexp.MustCompile(`(?i)(?:` + candidateRequirement + `be\s+(?:an?\s+)?` + usEligibility + `\s+persons?\b|\b` + personTerm + `\s+(?:is\s+)?required\b|\b(?:this|the)\s+(?:role|position|internship)\s+requires\s+` + personTerm + `\b|^` + usEligibility + `\s+persons?\s+only\b)`),
		denial:      regexp.MustCompile(`(?i)(?:\b` + personTerm + `\s+(?:is\s+)?(?:not\s+required|no\s+longer\s+required|optional)|\b` + notRequiredTo + `\s+be\s+(?:an?\s+)?` + usEligibility + `\s+person)`),
		reason:      "JD explicitly requires U.S. Person status",
	},
	{
		requirement: regexp.MustCompile(`(?i)(?:` + candidateRequirement + `(?:(?:be\s+able\s+to|be\s+eligible\s+to)\s+)?(?:obtain|maintain|hold|possess|have)\s+(?:(?:an?|the)\s+)?` + clearanceTerm + `\b|\b` + clearanceTerm + `\s+(?:is\s+)?required\b|\b(?:this|the)\s+(?:role|position|internship)\s+requires\s+(?:an?\s+)?` + clearanceTerm + `\b)`),
		denial:      regexp.MustCompile(`(?i)(?:\b` + clearanceTerm + `\s+(?:is\s+)?(?:not\s+required|no\s+longer\s+required|optional)|\bno\s+` + clearanceTerm + `\s+(?:is\s+)?required|\b` + notRequiredTo + `\s+(?:obtain|maintain|hold|possess|have)\s+(?:an?\s+)?` + clearanceTerm + `)`),
		reason:      "JD explicitly requires security clearance",
	},
}

// Uncertain context downgrades an apparent requirement to review. In particular,
// an alternative qualification must not be silently dropped by a partial match.
var uncertainEligibilityPattern = regexp.MustCompile(`(?i)\b(?:not|no|non|never|without|isn't|isn’t|don't|don’t|doesn't|doesn’t|cannot|can't|can’t|if|unless|or|may|might|could|preferred|preferably|desired|optional|waiv\w*|exception\w*)\b`)
var otherEligibilityContext = regexp.MustCompile(`(?i)\b(?:(?:some|certain|other|senior)\s+(?:roles?|positions?|projects?|employees?)|customers?|clients?|partners?|previous|prior|for\s+example)\b`)

// Bare "citizenship" is common in equal-opportunity boilerplate, not evidence
// about this role's eligibility. Require a qualification or an actual signal.
var eligibilitySignalPattern = regexp.MustCompile(`(?i)\b(?:` + usEligibility + `\s+(?:citizens?|citizenship|persons?)|citizenship\s+(?:is\s+)?(?:not\s+)?(?:required|preferred|optional)|ITAR|export\s+control\w*|(?:security|secret|TS/SCI)\s+clearance|sponsor\w*|work\s+authori[sz]\w*|authori[sz]ed\s+to\s+work)\b`)
var usAbbreviationPattern = regexp.MustCompile(`(?i)\bu\.s\.`)

// Keep the original text for evidence, including U.S. punctuation. Splitting on
// every period would turn "U.S. citizenship is required" into separate pieces.
func eligibilityStatements(description string) []string {
	protected := make(map[int]bool)
	for _, span := range usAbbreviationPattern.FindAllStringIndex(description, -1) {
		for i := span[0]; i < span[1]; i++ {
			protected[i] = true
		}
	}
	var statements []string
	start := 0
	for i := 0; i < len(description); i++ {
		if strings.ContainsRune(".!?;\n", rune(description[i])) && !protected[i] {
			if statement := strings.TrimSpace(description[start : i+1]); statement != "" {
				statements = append(statements, statement)
			}
			start = i + 1
		}
	}
	if statement := strings.TrimSpace(description[start:]); statement != "" {
		statements = append(statements, statement)
	}
	return statements
}

func assessEligibility(description string) eligibilityAssessment {
	statements := eligibilityStatements(description)
	review := eligibilityAssessment{Decision: eligibilityReview, Reason: "eligibility is unconfirmed; no clear restriction found"}
	for _, statement := range statements {
		if eligibilitySignalPattern.MatchString(statement) {
			review.Reason = "check the JD's eligibility and sponsorship wording"
			review.Evidence = statement
			break
		}
	}
	for _, rule := range eligibilityRules {
		var required, denied string
		for _, statement := range statements {
			if rule.denial.MatchString(statement) {
				denied = statement
			}
			if rule.requirement.MatchString(statement) && !uncertainEligibilityPattern.MatchString(statement) && !otherEligibilityContext.MatchString(statement) {
				required = statement
			}
		}
		if required == "" {
			continue
		}
		if denied != "" {
			review.Reason = "JD contains conflicting eligibility requirements"
			review.Evidence = required + "\n" + denied
			continue
		}
		return eligibilityAssessment{eligibilityIgnore, rule.reason, required}
	}
	return review
}

func formatEligibility(assessment eligibilityAssessment) string {
	label := "REVIEW"
	if assessment.Decision == eligibilityIgnore {
		label = "SKIP"
	}
	result := "🔎 **Eligibility:** " + label + ": " + assessment.Reason
	if assessment.Evidence != "" {
		// Vendor prose can contain huge paragraphs. Bound the excerpt so this
		// extra field does not consume Discord's entire message allowance.
		evidence := []rune(assessment.Evidence)
		if len(evidence) > 400 {
			evidence = append(evidence[:400], '…')
		}
		result += "\n**JD excerpt:** " + string(evidence)
	}
	return result
}
