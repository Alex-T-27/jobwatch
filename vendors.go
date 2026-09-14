package main

import (
	"fmt"
	"strconv"
	"strings"
)

// Every vendor gets a private struct matching its JSON exactly, plus a function
// that converts it into []Posting. These structs never leave this file, so a
// vendor changing its schema can only break one adapter.

// ---------- Ashby ----------

type ashbyBoard struct {
	Jobs []ashbyJob `json:"jobs"`
}

type ashbyJob struct {
	Id               string `json:"id"`
	Title            string `json:"title"`
	Location         string `json:"location"`
	JobUrl           string `json:"jobUrl"`
	DescriptionPlain string `json:"descriptionPlain"`
	Department       string `json:"department"`
	Team             string `json:"team"`
	EmploymentType   string `json:"employmentType"`
}

func fetchAshby(company string) ([]Posting, error) {
	url := fmt.Sprintf(
		"https://api.ashbyhq.com/posting-api/job-board/%s",
		company,
	)

	return fetchAshbyFromURL(company, url)
}

func fetchAshbyFromURL(company, url string) ([]Posting, error) {
	var board ashbyBoard
	if err := getJSON(url, &board); err != nil {
		return nil, err
	}

	postings := make([]Posting, 0, len(board.Jobs))
	for _, j := range board.Jobs {
		postings = append(postings, Posting{
			Vendor:         "ashby",
			Company:        company,
			Id:             j.Id,
			Title:          j.Title,
			Location:       j.Location,
			Url:            j.JobUrl,
			Description:    j.DescriptionPlain,
			Department:     j.Department,
			Team:           j.Team,
			EmploymentType: j.EmploymentType,
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
	Id          int64  `json:"id"`
	Title       string `json:"title"`
	Content     string `json:"content"`
	Departments []struct {
		Name string `json:"name"`
	} `json:"departments"`
	Location struct {
		Name string `json:"name"`
	} `json:"location"`
	AbsoluteUrl string `json:"absolute_url"`
}

func fetchGreenhouse(company string) ([]Posting, error) {
	url := fmt.Sprintf("https://boards-api.greenhouse.io/v1/boards/%s/jobs?content=true", company)

	var board greenhouseBoard
	if err := getJSON(url, &board); err != nil {
		return nil, err
	}

	postings := make([]Posting, 0, len(board.Jobs))
	for _, j := range board.Jobs {
		departments := make([]string, 0, len(j.Departments))
		for _, department := range j.Departments {
			departments = append(departments, department.Name)
		}

		postings = append(postings, Posting{
			Vendor:      "greenhouse",
			Company:     company,
			Id:          strconv.FormatInt(j.Id, 10),
			Title:       j.Title,
			Location:    j.Location.Name,
			Url:         j.AbsoluteUrl,
			Description: plainText(j.Content),
			Department:  strings.Join(departments, ", "),
		})
	}
	return postings, nil
}

// ---------- Lever ----------

// Lever has no wrapper object at all, the response is a bare array, so this
// decodes into a slice directly. Title lives under "text" and location is
// nested inside "categories".
type leverJob struct {
	Id               string `json:"id"`
	Text             string `json:"text"`
	HostedUrl        string `json:"hostedUrl"`
	DescriptionPlain string `json:"descriptionPlain"`
	Categories       struct {
		Location   string `json:"location"`
		Team       string `json:"team"`
		Department string `json:"department"`
		Commitment string `json:"commitment"`
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
			Vendor:         "lever",
			Company:        company,
			Id:             j.Id,
			Title:          j.Text,
			Location:       j.Categories.Location,
			Url:            j.HostedUrl,
			Description:    j.DescriptionPlain,
			Department:     j.Categories.Department,
			Team:           j.Categories.Team,
			EmploymentType: j.Categories.Commitment,
		})
	}
	return postings, nil
}
