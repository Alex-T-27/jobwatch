package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

var errNotModified = errors.New("job board not modified")

// etagByURL remembers the version of each board returned by its server. It is
// intentionally in memory: after a restart, one full fetch safely rebuilds it.
var etagByURL = make(map[string]string)

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
	if etag := etagByURL[url]; etag != "" {
		req.Header.Set("If-None-Match", etag)
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

	if resp.StatusCode == http.StatusNotModified {
		return errNotModified
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s returned %s: %s", url, resp.Status, body)
	}

	if err := json.Unmarshal(body, target); err != nil {
		return err
	}

	// Store the new version only after its body decoded successfully. Otherwise,
	// a bad response could be cached and skipped on every later poll.
	if etag := resp.Header.Get("ETag"); etag != "" {
		etagByURL[url] = etag
	}

	return nil
}
