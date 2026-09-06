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

// Posting is the one shape the rest of the program works in. Vendor JSON is
// converted into this inside vendors.go and nowhere else.
type Posting struct {
	Vendor   string
	Company  string
	Id       string
	Title    string
	Location string
	Url      string
}

// Key namespaces an id by where it came from. Two Greenhouse companies can hand
// back the same numeric id, and keying on the bare id would treat the second one
// as already sent.
func (p Posting) Key() string {
	return fmt.Sprintf("%s:%s:%s", p.Vendor, p.Company, p.Id)
}

// target is one board to poll. Hardcoded for now, moves to a config file later.
type target struct {
	vendor  string
	company string
}

var targets = []target{
	{"ashby", "Deepgram"},
	{"greenhouse", "stripe"},
	{"lever", "spotify"},
}

// Creates a new client object
var client = &http.Client{}

// Where the keys of postings already sent to Discord are recorded
const sentPath = "sent.txt"

// How many postings one run is allowed to send. Keeps a first run, which sees
// every posting as new, from firing hundreds of messages at Discord at once.
const maxPerRun = 5

func formatPosting(p Posting) string {
	//Message content
	message := fmt.Sprintf("***New Job Found*** \n**%s - %s**\n%s\n%s",
		p.Title,
		p.Company,
		p.Location,
		p.Url,
	)
	return message
}

func main() {
	// Loads .env content
	err := godotenv.Load()
	if err != nil {
		log.Fatal(err)
	}

	// One board failing should not cost us the other two.
	var postings []Posting
	for _, t := range targets {
		got, err := fetchJobs(t.vendor, t.company)
		if err != nil {
			log.Printf("%s/%s failed: %v", t.vendor, t.company, err)
			continue
		}
		fmt.Printf("%-11s %-10s %4d postings\n", t.vendor, t.company, len(got))
		postings = append(postings, got...)
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

	for _, p := range postings {
		key := p.Key()
		if sent[key] {
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

		if err := sendToDiscord(formatPosting(p)); err != nil {
			log.Printf("send failed on %s, stopping this run: %v", key, err)
			stopped = true
			continue
		}

		// Record the key only after the send succeeded. Crashing between the
		// two means this posting sends twice next run, which beats losing it.
		if err := markSent(sentLog, key); err != nil {
			log.Fatalf("sent %s but could not record it: %v", key, err)
		}
		sent[key] = true
		sentThisRun++
	}

	fmt.Printf("fetched %d, new %d, sent this run %d, left %d\n",
		len(postings), newFound, sentThisRun, newFound-sentThisRun)
}
