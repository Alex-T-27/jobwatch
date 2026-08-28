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
