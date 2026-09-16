package main

import (
	"context"
	"io"
	"net/http"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestAssessLocation(t *testing.T) {
	tests := []struct {
		name    string
		posting Posting
		want    locationDecision
	}{
		{"US city", Posting{Location: "San Francisco"}, locationMatch},
		{"US cities", Posting{Location: "San Francisco, Seattle, New York City"}, locationMatch},
		{"US remote", Posting{Location: "Remote - United States"}, locationMatch},
		{"bare remote", Posting{Location: "Remote"}, locationReview},
		{"remote excludes US", Posting{Location: "Remote - Non-US"}, locationIgnore},
		{"US work hours are not a country", Posting{Location: "Remote - US time zones"}, locationReview},
		{"missing", Posting{}, locationReview},
		{"ambiguous city", Posting{Location: "Cambridge"}, locationReview},
		{"London", Posting{Location: "London"}, locationIgnore},
		{"Singapore", Posting{Location: "Singapore"}, locationIgnore},
		{"Bengaluru", Posting{Location: "Bengaluru"}, locationIgnore},
		{"Bucharest", Posting{Location: "Bucharest"}, locationIgnore},
		{"Dublin", Posting{Location: "Dublin"}, locationIgnore},
		{"Toronto", Posting{Location: "Toronto"}, locationIgnore},
		{"mixed cities", Posting{Location: "Toronto, New York, San Francisco"}, locationMatch},
		{"foreign with unknown option", Posting{Location: "London / undisclosed"}, locationReview},
		{"explicit foreign country beats city", Posting{Location: "Austin, Canada"}, locationIgnore},
		{"state context", Posting{Location: "Dublin, CA"}, locationMatch},
		{"bare CA", Posting{Location: "CA"}, locationReview},
		{"structured Canada", Posting{Locations: []postingLocation{{Name: "Remote", Country: "CA"}}}, locationIgnore},
		{"secondary US", Posting{Locations: []postingLocation{{Name: "Toronto", Country: "CA"}, {Name: "Remote", Country: "USA"}}}, locationMatch},
		{"remote primary plus foreign secondary", Posting{Locations: []postingLocation{{Name: "Remote"}, {Name: "London", Country: "GB"}}}, locationReview},
		{"US JD location", Posting{Location: "Remote", Description: "Candidates must reside within the United States."}, locationMatch},
		{"US JD remote", Posting{Location: "Remote", Description: "Remote within the United States."}, locationMatch},
		{"foreign JD location", Posting{Location: "Remote", Description: "This role is based in Canada."}, locationIgnore},
		{"JD boilerplate", Posting{Location: "Remote", Description: "Our headquarters are in the United States. We serve US customers."}, locationReview},
		{"JD citizenship", Posting{Location: "Remote", Description: "You must be a US citizen."}, locationReview},
		{"negated JD location", Posting{Location: "Remote", Description: "This role is not based in the United States."}, locationReview},
		{"conflicting JD", Posting{Location: "Remote", Description: "This role is based in Canada. Candidates must reside in the United States."}, locationReview},
		{"foreign location wins over JD", Posting{Location: "London", Description: "Candidates must reside in the United States."}, locationIgnore},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := assessLocation(tt.posting); got.Decision != tt.want {
				t.Fatalf("got %+v, want %s", got, tt.want)
			}
		})
	}
}

func TestVendorLocations(t *testing.T) {
	tests := []struct {
		vendor string
		body   string
		want   []postingLocation
	}{
		{"ashby", `{"jobs":[{"location":"Toronto","address":{"postalAddress":{"addressCountry":"Canada"}},"secondaryLocations":[{"location":"Remote","address":{"addressCountry":"USA"}}]}]}`, []postingLocation{{"Toronto", "Canada"}, {"Remote", "USA"}}},
		{"greenhouse", `{"jobs":[{"location":{"name":"London"},"offices":[{"location":"Seattle, WA"},{"location":null}]}]}`, []postingLocation{{Name: "London"}, {Name: "Seattle, WA"}}},
		{"lever", `[{"country":"CA","categories":{"location":"Toronto","allLocations":["Toronto","New York"]}}]`, []postingLocation{{"Toronto", "CA"}, {Name: "New York"}}},
	}
	for _, tt := range tests {
		t.Run(tt.vendor, func(t *testing.T) {
			oldClient, oldCache := client, boardsByURL
			t.Cleanup(func() { client, boardsByURL = oldClient, oldCache })
			boardsByURL = make(map[string]cachedBoard)
			client = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(tt.body))}, nil
			})}
			postings, err := fetchJobs(tt.vendor, "fixture")
			if err != nil {
				t.Fatal(err)
			}
			if len(postings) != 1 || !reflect.DeepEqual(postings[0].Locations, tt.want) {
				t.Fatalf("locations lost in adapter: %+v", postings)
			}
			if got := assessLocation(postings[0]); got.Decision != locationMatch {
				t.Fatalf("US secondary lost: %+v", got)
			}
		})
	}
}

func TestLocationFilterBeforeDelivery(t *testing.T) {
	oldClient, oldCache := client, boardsByURL
	t.Cleanup(func() { client, boardsByURL = oldClient, oldCache })
	boardsByURL = make(map[string]cachedBoard)
	posts := 0
	client = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		body := `[{"id":"foreign","text":"SWE Intern","country":"CA","categories":{"location":"Toronto"},"descriptionPlain":"Write software."},{"id":"unclear","text":"SWE Intern","categories":{"location":"Remote"},"descriptionPlain":"Write software."}]`
		if r.Method == "POST" {
			posts++
			payload, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(payload), "REVIEW: US availability unclear") {
				t.Errorf("missing location review label: %s", payload)
			}
			body = `{}`
		}
		return &http.Response{StatusCode: 200, Status: "200 OK", Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	path := filepath.Join(t.TempDir(), "sent.txt")
	log, err := openSentLog(path)
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()
	sent := make(map[string]bool)
	scanner := &scanner{state: make(scanState), path: filepath.Join(t.TempDir(), "scan-state.json")}
	if err := runOnce(context.Background(), scanner, []target{{Vendor: "lever", Company: "fixture"}}, sent, log, false); err != nil {
		t.Fatal(err)
	}
	if posts != 1 || len(sent) != 1 || !sent["lever:fixture:unclear"] {
		t.Fatalf("wrong delivery or state: posts=%d sent=%v", posts, sent)
	}
	persisted, err := loadSent(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(persisted, sent) {
		t.Fatalf("wrong persisted keys: %v", persisted)
	}
}
