package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

func fetchJobs(company string) ([]Job, error) {
	job_list := JobList{}
	job_url := fmt.Sprintf("https://api.ashbyhq.com/posting-api/job-board/%s", company)
	req, err := http.NewRequest(
		"GET",
		job_url,
		nil,
	)
	if err != nil {
		fmt.Println("Error:", err)
		return nil, err
	}

	resp, err := client.Do(req)
	if err != nil {
		fmt.Println("Error:", err)
		return nil, err
	}
	defer resp.Body.Close()

	fmt.Println(resp.Status)

	if resp.StatusCode != http.StatusOK {
		bodyError, err := io.ReadAll(resp.Body)
		if err != nil {
			fmt.Println("Error:", err)
			return nil, err
		}
		return nil, fmt.Errorf("ashby returned %s: %s", resp.Status, bodyError)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Println("Error:", err)
		return nil, err
	}

	err = json.Unmarshal(bodyBytes, &job_list)
	if err != nil {
		fmt.Println("Error:", err)
		return nil, err
	}

	return job_list.Jobs, nil
}
