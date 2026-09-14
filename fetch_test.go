package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestConditionalFetchRetainsUnsentJobs(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if requests > 1 {
			if got := r.Header.Get("If-None-Match"); got != `"v1"` {
				t.Errorf("validator = %q, want v1", got)
			}
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", `"v1"`)
		fmt.Fprint(w, `{"jobs":[{"id":"first"},{"id":"still-unsent"}]}`)
	}))
	defer server.Close()
	defer delete(boardsByURL, server.URL)

	first, err := fetchAshbyFromURL("example", server.URL)
	if err != nil {
		t.Fatal(err)
	}
	sent := map[string]bool{first[0].Key(): true}
	second, err := fetchAshbyFromURL("example", server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if len(second) != 2 || second[1].Id != "still-unsent" || sent[second[1].Key()] {
		t.Fatalf("304 lost pending work: %+v", second)
	}
}

func TestConditionalFetchClearsOldValidator(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if requests == 1 {
			w.Header().Set("ETag", `"v1"`)
		}
		if requests == 3 && r.Header.Get("If-None-Match") != "" {
			t.Error("kept stale validator after 200 without ETag")
		}
		fmt.Fprint(w, `{"jobs":[]}`)
	}))
	defer server.Close()
	defer delete(boardsByURL, server.URL)
	for range 3 {
		if _, err := fetchAshbyFromURL("example", server.URL); err != nil {
			t.Fatal(err)
		}
	}
}
