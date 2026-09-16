package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type cachedBoard struct {
	etag string
	body []byte
}

// A 304 saves a download, but cached jobs still need processing: Discord may
// have failed or the previous cycle may have reached its send cap.
// The poller is sequential; concurrent fetching would need synchronization.
var boardsByURL = make(map[string]cachedBoard)

// fetchJobs picks the adapter for a vendor and hands back postings that are
// already normalized. Adding a vendor means one case here plus one pair of
// struct and function in vendors.go. Nothing else in the program changes.
func fetchJobs(ctx context.Context, vendor, company string) ([]Posting, error) {
	switch vendor {
	case "ashby":
		return fetchAshby(ctx, company)
	case "greenhouse":
		return fetchGreenhouse(ctx, company)
	case "lever":
		return fetchLever(ctx, company)
	}
	return nil, fmt.Errorf("unknown vendor %q", vendor)
}

// getJSON does the GET, checks the status, and decodes the body into target,
// which must be a pointer. Every adapter shares this so none of them repeat the
// HTTP boilerplate. The body is read before the status check so a failure can
// report what the server actually said.
func getJSON(ctx context.Context, url string, target any) error {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "JobWatch/1.0 (+https://github.com/Alex-T-27/jobwatch)")
	if etag := boardsByURL[url].etag; etag != "" {
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
		cached, ok := boardsByURL[url]
		if !ok {
			return fmt.Errorf("%s returned 304 without a cached board", url)
		}
		return json.Unmarshal(cached.body, target)
	}

	if resp.StatusCode != http.StatusOK {
		return &httpStatusError{Code: resp.StatusCode, Message: fmt.Sprintf("%s returned %s: %.500s", url, resp.Status, body)}
	}

	if err := json.Unmarshal(body, target); err != nil {
		return err
	}

	// Store the new version only after its body decoded successfully. Otherwise,
	// a bad response could be cached and skipped on every later poll.
	boardsByURL[url] = cachedBoard{etag: resp.Header.Get("ETag"), body: body}

	return nil
}

// Callers can stop on throttling without guessing from an error message.
type httpStatusError struct {
	Code    int
	Message string
}

func (e *httpStatusError) Error() string { return e.Message }
