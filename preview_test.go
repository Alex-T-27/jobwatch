package main

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestLeverDescriptionAndDryRun(t *testing.T) {
	previousClient, previousCache := client, boardsByURL
	t.Cleanup(func() { client, boardsByURL = previousClient, previousCache })
	boardsByURL = make(map[string]cachedBoard)
	client = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method != "GET" || r.URL.Host != "api.lever.co" {
			t.Fatalf("dry run attempted external send: %s %s", r.Method, r.URL.Host)
		}
		return &http.Response{
			StatusCode: 200,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`[{"id":"fixture", "text":"Technology Analyst", "descriptionPlain":"This role is an internship.", "categories":{"department":"Engineering"}, "lists":[{"text":"Responsibilities", "content":"<ul><li>Build backend services.</li></ul>"}], "additionalPlain":"We are unable to provide visa sponsorship."}]`)),
		}, nil
	})}
	postings, err := fetchLever("fixture")
	if err != nil {
		t.Fatal(err)
	}
	if len(postings) != 1 {
		t.Fatalf("got %d postings", len(postings))
	}
	p := postings[0]
	if role := assessRole(p); role.Decision != roleMatch {
		t.Fatalf("requirements missing from classifier: %+v", role)
	}
	if !assessSponsorship(p.Description).LikelyBlocked {
		t.Fatal("closing section missing from sponsorship assessment")
	}
	sent := make(map[string]bool)
	// A nil log also ensures the preview cannot record a send.
	scanner := &scanner{state: make(scanState), path: "must-not-be-written"}
	if err := runOnce(context.Background(), scanner, []target{{Vendor: "lever", Company: "fixture"}}, sent, nil, true); err != nil {
		t.Fatal(err)
	}
	if len(scanner.state) != 0 {
		t.Fatal("preview changed scan history")
	}
	if len(sent) != 0 {
		t.Fatal("preview changed sent state")
	}
}
