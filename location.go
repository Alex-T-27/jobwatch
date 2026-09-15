package main

import (
	"regexp"
	"strings"
)

// Country belongs to one location, not the entire posting: a foreign primary
// office must not hide a secondary US office.
type postingLocation struct {
	Name    string
	Country string
}

type locationDecision string

const (
	locationMatch  locationDecision = "match"
	locationReview locationDecision = "review"
	locationIgnore locationDecision = "ignore"
)

type locationAssessment struct {
	Decision locationDecision
	Reason   string
}

var usCountryPattern = regexp.MustCompile(`(?i)\b(?:United States(?: of America)?|USA|U\.S\.A\.?|U\.S\.?|US)\b`)
var foreignCountryPattern = regexp.MustCompile(`(?i)\b(?:Canada|United Kingdom|UK|England|Scotland|Ireland|India|Singapore|Romania|Germany|France|Australia|Netherlands|Sweden|Spain|Poland|Japan|China|Vietnam|Brazil|Mexico|Israel|Switzerland|Italy|Portugal|Denmark|Norway|Finland|South Korea|New Zealand)\b`)
var usCityPattern = regexp.MustCompile(`(?i)\b(?:San Francisco|New York(?: City)?|NYC|Seattle|Boston|Austin|Chicago|Los Angeles|San Diego|San Jose|Atlanta|Washington,? DC|Washington,? D\.C\.|Palo Alto|Mountain View|Sunnyvale|San Mateo|Redmond|Bellevue|Tampa|Miami|Denver|Pittsburgh|Philadelphia)\b`)
var foreignCityPattern = regexp.MustCompile(`(?i)\b(?:London|Toronto|Bengaluru|Bangalore|Bucharest|Dublin|Berlin|Stockholm|Amsterdam|Montreal|Vancouver|Mumbai|Hyderabad|Tokyo|Paris|Sydney|Melbourne)\b`)

// State abbreviations need an address-like context. Bare CA could mean Canada,
// and ordinary words such as IN or OR are not location evidence.
var usStatePattern = regexp.MustCompile(`,\s*(?:AL|AK|AZ|AR|CA|CO|CT|DE|DC|FL|GA|HI|ID|IL|IN|IA|KS|KY|LA|ME|MD|MA|MI|MN|MS|MO|MT|NE|NV|NH|NJ|NM|NY|NC|ND|OH|OK|OR|PA|RI|SC|SD|TN|TX|UT|VT|VA|WA|WV|WI|WY)(?:\s+\d{5})?(?:$|[;)])`)
var locationSeparator = regexp.MustCompile(`(?i)\s*(?:[;/|]|\s+or\s+|\s+and\s+)\s*`)
var remotePattern = regexp.MustCompile(`(?i)\b(?:remote|worldwide|global|anywhere)\b`)
var excludedUSPattern = regexp.MustCompile(`(?i)\b(?:non[- ]US|outside (?:the )?(?:US|USA|United States)|except (?:the )?(?:US|USA|United States))\b`)
var usHoursPattern = regexp.MustCompile(`(?i)\b(?:US|USA|United States)\s+(?:time\s*zones?|(?:business\s+)?hours)\b`)

// Require a work-location statement. Citizenship, customer locations and office
// lists in company boilerplate do not establish where this role can be done.
var jdLocationPattern = regexp.MustCompile(`(?i)\b(?:(?:this|the)\s+(?:role|position)\s+(?:is|will be)\s+(?:based|located)\s+in|(?:you|candidates|applicants)\s+must\s+(?:be\s+(?:based|located)|reside|live)\s+(?:in|within)|work\s+remotely\s+(?:from|within))\s+(?:the\s+)?(United States(?: of America)?|USA|U\.S\.?|US|Canada|United Kingdom|UK|India|Singapore|Ireland|Romania)\b`)
var jdRemoteUSPattern = regexp.MustCompile(`(?im)(?:^|[.!?]\s+)remote\s+within\s+(?:the\s+)?(?:United States|USA|U\.S\.?|US)\b`)

func assessLocation(p Posting) locationAssessment {
	locations := p.Locations
	if len(locations) == 0 {
		locations = []postingLocation{{Name: p.Location}}
	}
	uncertain := false
	for _, location := range locations {
		country := strings.ToUpper(strings.TrimSpace(location.Country))
		if country != "" {
			if usCountryPattern.MatchString(country) {
				return locationAssessment{locationMatch, "US country on " + location.Name}
			}
			if (len(country) == 2 && country != "XX" && country != "ZZ") || foreignCountryPattern.MatchString(country) {
				continue
			}
		}
		for _, name := range locationSeparator.Split(location.Name, -1) {
			switch classifyLocationName(strings.TrimSpace(name)) {
			case locationMatch:
				return locationAssessment{locationMatch, "US location: " + name}
			case locationReview:
				uncertain = true
			}
		}
	}
	if !uncertain {
		return locationAssessment{locationIgnore, "listed locations are outside the US"}
	}

	usJD, foreignJD := jdRemoteUSPattern.MatchString(p.Description), false
	for _, evidence := range jdLocationPattern.FindAllStringSubmatch(p.Description, -1) {
		if usCountryPattern.MatchString(evidence[1]) {
			usJD = true
		} else {
			foreignJD = true
		}
	}
	if usJD && !foreignJD {
		return locationAssessment{locationMatch, "JD explicitly places this role in the US"}
	}
	if foreignJD && !usJD {
		return locationAssessment{locationIgnore, "JD explicitly places this role outside the US"}
	}
	return locationAssessment{locationReview, "US availability is unclear; check the listed locations and JD"}
}

func classifyLocationName(name string) locationDecision {
	if excludedUSPattern.MatchString(name) {
		return locationIgnore
	}
	if usHoursPattern.MatchString(name) {
		return locationReview
	}
	if usCountryPattern.MatchString(name) {
		return locationMatch
	}
	if foreignCountryPattern.MatchString(name) {
		return locationIgnore
	}
	if usStatePattern.MatchString(name) || usCityPattern.MatchString(name) {
		return locationMatch
	}
	if remotePattern.MatchString(name) {
		return locationReview
	}
	if foreignCityPattern.MatchString(name) {
		return locationIgnore
	}
	return locationReview
}
