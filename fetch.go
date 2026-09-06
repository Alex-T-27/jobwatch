package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// fetchJobs picks the adapter for a vendor and hands back postings that are
// already normalized. Adding a vendor means one case here plus one pair of
// struct and function in vendors.go. Nothing else in the program changes.
func fetchJobs(vendor, company string) ([]Posting, error) {
	switch vendor {
	case "ashby":
		return fetchAshby(company)
	case "greenhouse":
		return fetchGreenhouse(company)
	case "lever":
		return fetchLever(company)
	}
	return nil, fmt.Errorf("unknown vendor %q", vendor)
}

// getJSON does the GET, checks the status, and decodes the body into target,
// which must be a pointer. Every adapter shares this so none of them repeat the
// HTTP boilerplate. The body is read before the status check so a failure can
// report what the server actually said.
func getJSON(url string, target any) error {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s returned %s: %s", url, resp.Status, body)
	}

	return json.Unmarshal(body, target)
}
