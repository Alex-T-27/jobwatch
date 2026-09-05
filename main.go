package main

import (
	"fmt"
	"log"
	"net/http"

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

// Where the ids of jobs already sent to Discord are recorded
const sentPath = "sent.txt"

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

func main() {
	// Loads .env content
	err := godotenv.Load()
	if err != nil {
		log.Fatal(err)
	}

	company := "Deepgram"

	jobs, err := fetchJobs(company)
	if err != nil {
		log.Fatal(err)
	}

	sent, err := loadSent(sentPath)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("loaded %d already-sent ids\n", len(sent))

	for i, job := range jobs {
		fmt.Printf("%d: %s\n", i, job.Title)
	}

	// Todo: skip jobs already sent, then send the new ones to Discord
}
