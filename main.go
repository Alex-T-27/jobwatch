package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"

	"github.com/joho/godotenv"
)

type Data struct {
	Content string `json:"content"`
}

type Job struct {
	Id       string
	Title    string
	Company  string
	Location string
	JobUrl   string
}

type JobList struct {
	Jobs []Job `json:"jobs"`
}

// Creates a new client object
var client = &http.Client{}

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

func formatJob(job Job, company string) string {
	//Message content
	message := fmt.Sprintf("***New Job Found*** \n**%s - %s**\n%s\n%s",
		job.Title,
		company,
		job.Location,
		job.JobUrl,
	)
	return message
}

func sendToDiscord(message string) error {
	channelID := os.Getenv("CHANNEL_ID")
	token := os.Getenv("DISCORD_TOKEN")

	// The content
	body := Data{Content: message}

	jsonData, err := json.Marshal(body) // Return a []byte @ jsonData
	if err != nil {
		fmt.Println("Error:", err)
		return err
	}
	final_content := bytes.NewBuffer(jsonData)

	// The URL to my discord server
	discord_url := fmt.Sprintf(
		"https://discord.com/api/v10/channels/%s/messages",
		channelID,
	)

	// Creates a request containing METHOD, URL, CONTENT
	req, err := http.NewRequest(
		"POST",
		discord_url,
		final_content,
	)
	if err != nil {
		fmt.Println("Error:", err)
		return err
	}

	// Modify the request's Headers
	req.Header.Add("Authorization", "Bot "+token)
	req.Header.Add("Content-Type", "application/json")

	// Tell the Client to response to that request
	resp, err := client.Do(req)
	if err != nil {
		fmt.Println("Error:", err)
		return err
	}
	defer resp.Body.Close()

	fmt.Println(resp.Status)

	if resp.StatusCode != http.StatusOK {
		bodyError, err := io.ReadAll(resp.Body)
		if err != nil {
			fmt.Println("Error:", err)
			return err
		}

		return fmt.Errorf("ashby returned %s: %s", resp.Status, bodyError)

	}

	return nil
}

func main() {
	// Loads .env content
	err := godotenv.Load()
	if err != nil {
		log.Fatal(err)
	}

	// Todo: Fetch one company's job posting and turn it into readable file

	company := "Deepgram"

	//Todo: one Deepgram job appears in discord as readable message.
	//Fetch -> Decode -> Pick jobs -> Format -> Send
	//Checkpoint: Be able to print job[0] title to the terminal

	jobs, err := fetchJobs(company)
	if err != nil {
		log.Fatal(err)
	}

	job := jobs[60]
	message := formatJob(job, company)

	sendToDiscord(message)
	// Todo: Break main() into three functions:
	// fetchJobs(company string)  -> []Job, error
	// formatJob(job Job)         -> string
	// sendToDiscord(msg string)  -> error

	// Todo: prints all job titles in terminal, filter jobs then send to Discord desired jobs
}
