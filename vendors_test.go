package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestFetchAshby(t *testing.T) {
	body, err := os.ReadFile("testdata/ashby.json")
	if err != nil {
		t.Fatal(err)
	}

	fakeAshby := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(body)
	}

	handler := http.HandlerFunc(fakeAshby)
	server := httptest.NewServer(handler)
	defer server.Close()

	postings, err := fetchAshbyFromURL("Deepgram", server.URL)
	if err != nil {
		t.Fatal(err)
	}

	if len(postings) != 1 {
		t.Fatalf("got %d postings, want 1", len(postings))
	}

	want := Posting{
		Vendor:   "ashby",
		Company:  "Deepgram",
		Id:       "job-123",
		Title:    "Software Engineer Intern",
		Location: "Remote",
		Url:      "https://example.com/jobs/job-123",
	}

	if postings[0] != want {
		t.Errorf("got %+v, want %+v", postings[0], want)
	}
}
