package main

import (
	"fmt"
	"log"
	"net/http"
	"time"

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

// How many jobs one run is allowed to send. Keeps the first run, which sees
// every posting as new, from firing ninety-odd messages at Discord at once.
const maxPerRun = 5

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

	sentLog, err := openSentLog(sentPath)
	if err != nil {
		log.Fatal(err)
	}
	defer sentLog.Close()

	newFound := 0
	sentThisRun := 0
	stopped := false

	for _, job := range jobs {
		if sent[job.Id] {
			continue
		}
		newFound++

		// Keep counting the rest so the summary is honest, but send no more.
		if stopped || sentThisRun >= maxPerRun {
			continue
		}

		// Placeholder, not real rate limiting. Discord allows roughly five
		// messages per five seconds per channel. Backoff comes later.
		if sentThisRun > 0 {
			time.Sleep(time.Second)
		}

		if err := sendToDiscord(formatJob(job, company)); err != nil {
			log.Printf("send failed on %s, stopping this run: %v", job.Id, err)
			stopped = true
			continue
		}

		// Record the id only after the send succeeded. Crashing between the
		// two means this job sends twice next run, which beats losing it.
		if err := markSent(sentLog, job.Id); err != nil {
			log.Fatalf("sent %s but could not record it: %v", job.Id, err)
		}
		sent[job.Id] = true
		sentThisRun++
	}

	fmt.Printf("fetched %d, new %d, sent this run %d, left %d\n",
		len(jobs), newFound, sentThisRun, newFound-sentThisRun)
}
