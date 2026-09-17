package main

import (
	"fmt"
	"regexp"
	"strings"
)

type seasonDecision string

const (
	seasonMatch  seasonDecision = "match"
	seasonReview seasonDecision = "review"
	seasonIgnore seasonDecision = "ignore"
)

type seasonAssessment struct {
	Decision seasonDecision
	Reason   string
}

const (
	fullTime = 1 << iota
	partTime
)

var seasonNames = []string{"summer", "fall", "winter", "spring"}
var seasonWordPattern = regexp.MustCompile(`(?i)\b(summer|fall|autumn|winter|spring)\b`)
var springFrameworkPattern = regexp.MustCompile(`(?i)\bspring\s+(?:boot|framework)\b`)
var workloadPattern = regexp.MustCompile(`(?i)\b(full|part)[\s-]*time\b`)
var scheduleSentencePattern = regexp.MustCompile(`[.!?;\n]+`)
var scheduleAlternativePattern = regexp.MustCompile(`(?i)\s+(?:and|or|but)\s+|,`)
var scheduleContextPattern = regexp.MustCompile(`(?i)\b(?:intern(?:ship)?s?|co[- ]?ops?|this\s+(?:role|position)|(?:work|working)\s+(?:hours|schedule)|(?:you|candidates|applicants)\s+(?:must|will)|schedule\s*:|employment\s+type\s*:)`)
var unrelatedSchedulePattern = regexp.MustCompile(`(?i)\b(?:graduat\w*|previous|prior|last\s+(?:summer|fall|winter|spring)|full[ -]*time\s+(?:employees?|staff|students?|enrollment|engineers?|offers?|employment|opportunit\w*|careers?|jobs?)|(?:return|returning)\s+(?:full[ -]*time|offers?)|convert\w*|conversion|(?:after|following)\s+(?:(?:the|your|this|an)\s+)?internship|upon\s+completion|leads?\s+to|future\s+employment)\b`)
var negatedSchedulePattern = regexp.MustCompile(`(?i)\b(?:not|no|never|isn't|isn’t|cannot|can't|can’t)\b`)

type scheduleEvidence struct {
	seasons   map[string]bool
	bySeason  map[string]int
	workloads int
	ambiguous bool
}

func newScheduleEvidence() scheduleEvidence {
	return scheduleEvidence{seasons: make(map[string]bool), bySeason: make(map[string]int)}
}

func scheduleSeasons(text string) map[string]bool {
	found := make(map[string]bool)
	text = springFrameworkPattern.ReplaceAllString(text, "")
	for _, season := range seasonWordPattern.FindAllString(strings.ToLower(text), -1) {
		if season == "autumn" {
			season = "fall"
		}
		found[season] = true
	}
	return found
}

func scheduleWorkloads(text string) int {
	result := 0
	for _, match := range workloadPattern.FindAllStringSubmatch(text, -1) {
		if strings.EqualFold(match[1], "full") {
			result |= fullTime
		} else {
			result |= partTime
		}
	}
	return result
}

func (e *scheduleEvidence) add(text string) {
	seasons := scheduleSeasons(text)
	workloads := scheduleWorkloads(text)
	for season := range seasons {
		e.seasons[season] = true
	}
	if len(seasons) == 0 {
		e.workloads |= workloads
		return
	}
	if workloads != fullTime|partTime {
		for season := range seasons {
			e.bySeason[season] |= workloads
		}
		return
	}
	// Keep options paired: part-time summer plus full-time winter does not
	// imply that full-time summer is offered.
	paired := make(map[string]bool)
	for _, option := range scheduleAlternativePattern.Split(text, -1) {
		optionWorkload := scheduleWorkloads(option)
		optionSeasons := scheduleSeasons(option)
		if optionWorkload != 0 && len(optionSeasons) == 0 {
			e.ambiguous = true
		}
		if optionWorkload != fullTime && optionWorkload != partTime {
			continue
		}
		for season := range optionSeasons {
			e.bySeason[season] |= optionWorkload
			paired[season] = true
		}
	}
	if len(paired) != len(seasons) {
		e.ambiguous = true
	}
}

func assessSeason(p Posting) seasonAssessment {
	// Normalize typography, but do not infer seasons from graduation years or
	// workloads from "Intern", remote work, or an unspecified number of hours.
	normalize := strings.NewReplacer("\u00a0", " ", "‑", "-", "–", "-", "—", "-")
	title := normalize.Replace(p.Title)
	if negatedSchedulePattern.MatchString(title) {
		return seasonAssessment{seasonReview, "title contains a negation; check the offered schedule"}
	}
	evidence := newScheduleEvidence()
	evidence.add(title)
	titleSeasons := scheduleSeasons(title)
	evidence.workloads |= scheduleWorkloads(p.EmploymentType)
	for _, sentence := range scheduleSentencePattern.Split(normalize.Replace(p.Description), -1) {
		if !scheduleContextPattern.MatchString(sentence) || unrelatedSchedulePattern.MatchString(sentence) || negatedSchedulePattern.MatchString(sentence) {
			continue
		}
		evidence.add(sentence)
	}
	if len(evidence.seasons) == 0 {
		return seasonAssessment{seasonReview, "internship season is unclear"}
	}
	if len(titleSeasons) > 0 {
		for season := range evidence.seasons {
			if !titleSeasons[season] {
				return seasonAssessment{seasonReview, "title and JD name different seasons; check the offered options"}
			}
		}
	}
	uncertain := evidence.ambiguous
	var mismatches []string
	for _, season := range seasonNames {
		if !evidence.seasons[season] {
			continue
		}
		workload := evidence.bySeason[season] | evidence.workloads
		if workload != fullTime && workload != partTime {
			uncertain = true
			continue
		}
		label := "part-time"
		if workload == fullTime {
			label = "full-time"
		}
		// Alex's current rule lives here so future preferences need not rewrite
		// how schedule evidence is extracted from postings.
		if (season == "summer" && workload == fullTime) || (season != "summer" && workload == partTime) {
			return seasonAssessment{seasonMatch, fmt.Sprintf("%s %s fits your seasonal schedule", season, label)}
		}
		mismatches = append(mismatches, season+" "+label)
	}
	if uncertain {
		return seasonAssessment{seasonReview, "workload is missing, conflicting, or not clearly paired with a season"}
	}
	return seasonAssessment{seasonIgnore, strings.Join(mismatches, "; ") + "; looking for full-time summer or part-time fall/winter/spring"}
}
