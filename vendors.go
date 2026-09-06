package main

import (
	"fmt"
	"strconv"
)

// Every vendor gets a private struct matching its JSON exactly, plus a function
// that converts it into []Posting. These structs never leave this file, so a
// vendor changing its schema can only break one adapter.

// ---------- Ashby ----------

type ashbyBoard struct {
	Jobs []ashbyJob `json:"jobs"`
}

type ashbyJob struct {
	Id       string `json:"id"`
	Title    string `json:"title"`
	Location string `json:"location"`
	JobUrl   string `json:"jobUrl"`
}

func fetchAshby(company string) ([]Posting, error) {
	url := fmt.Sprintf("https://api.ashbyhq.com/posting-api/job-board/%s", company)

	var board ashbyBoard
	if err := getJSON(url, &board); err != nil {
		return nil, err
	}

	postings := make([]Posting, 0, len(board.Jobs))
	for _, j := range board.Jobs {
		postings = append(postings, Posting{
			Vendor:   "ashby",
			Company:  company,
			Id:       j.Id,
			Title:    j.Title,
			Location: j.Location,
			Url:      j.JobUrl,
		})
	}
	return postings, nil
}

// ---------- Greenhouse ----------

type greenhouseBoard struct {
	Jobs []greenhouseJob `json:"jobs"`
}

// Id is a number here, not a string, and location is an object rather than a
// plain field. Both are the reason a shared struct with json tags cannot work.
type greenhouseJob struct {
	Id       int64  `json:"id"`
	Title    string `json:"title"`
	Location struct {
		Name string `json:"name"`
	} `json:"location"`
	AbsoluteUrl string `json:"absolute_url"`
}

func fetchGreenhouse(company string) ([]Posting, error) {
	url := fmt.Sprintf("https://boards-api.greenhouse.io/v1/boards/%s/jobs", company)

	var board greenhouseBoard
	if err := getJSON(url, &board); err != nil {
		return nil, err
	}

	postings := make([]Posting, 0, len(board.Jobs))
	for _, j := range board.Jobs {
		postings = append(postings, Posting{
			Vendor:   "greenhouse",
			Company:  company,
			Id:       strconv.FormatInt(j.Id, 10),
			Title:    j.Title,
			Location: j.Location.Name,
			Url:      j.AbsoluteUrl,
		})
	}
	return postings, nil
}

// ---------- Lever ----------

// Lever has no wrapper object at all, the response is a bare array, so this
// decodes into a slice directly. Title lives under "text" and location is
// nested inside "categories".
type leverJob struct {
	Id         string `json:"id"`
	Text       string `json:"text"`
	HostedUrl  string `json:"hostedUrl"`
	Categories struct {
		Location string `json:"location"`
	} `json:"categories"`
}

func fetchLever(company string) ([]Posting, error) {
	url := fmt.Sprintf("https://api.lever.co/v0/postings/%s?mode=json", company)

	var jobs []leverJob
	if err := getJSON(url, &jobs); err != nil {
		return nil, err
	}

	postings := make([]Posting, 0, len(jobs))
	for _, j := range jobs {
		postings = append(postings, Posting{
			Vendor:   "lever",
			Company:  company,
			Id:       j.Id,
			Title:    j.Text,
			Location: j.Categories.Location,
			Url:      j.HostedUrl,
		})
	}
	return postings, nil
}
