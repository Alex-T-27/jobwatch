package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
)

type Data struct {
	Content string `json:"content"`
}

// Posting is the one shape the rest of the program works in. Vendor JSON is
// converted into this inside vendors.go and nowhere else.
type Posting struct {
	Vendor         string
	Company        string
	Id             string
	Title          string
	Location       string
	Url            string
	Description    string
	Department     string
	Team           string
	EmploymentType string
}

// Key namespaces an id by where it came from. Two Greenhouse companies can hand
// back the same numeric id, and keying on the bare id would treat the second one
// as already sent.
func (p Posting) Key() string {
	return fmt.Sprintf("%s:%s:%s", p.Vendor, p.Company, p.Id)
}

// A stalled board must not hold up every later board indefinitely.
var client = &http.Client{Timeout: 30 * time.Second}

// Where the keys of postings already sent to Discord are recorded
const sentPath = "sent.txt"

// Where the list of company boards to poll is configured.
const targetsPath = "companies.json"

// How many postings one run is allowed to send. Keeps a first run, which sees
// every posting as new, from firing hundreds of messages at Discord at once.
const maxPerRun = 5

// How often a running JobWatch process checks every configured board.
const pollInterval = time.Minute

func formatPosting(p Posting, role roleAssessment) string {
	assessment := assessSponsorship(p.Description)
	sponsorship := "❓ **Sponsorship:** unknown"
	if assessment.LikelyBlocked {
		sponsorship = fmt.Sprintf(
			"⚠️ **Sponsorship risk:** likely blocked\n**Sponsorship reason:** %s",
			assessment.Reason,
		)
	}

	roleLabel := "✅ **Role match:** software"
	if role.Decision == roleReview {
		roleLabel = "🔎 **Role match:** review"
	}

	message := fmt.Sprintf("***New Job Found*** \n**%s - %s**\n%s\n%s\n**Role evidence:** %s\n%s\n%s",
		p.Title,
		p.Company,
		p.Location,
		roleLabel,
		role.Reason,
		sponsorship,
		p.Url,
	)
	return message
}

func main() {
	once := flag.Bool("once", false, "check boards once and exit")
	dryRun := flag.Bool("dry-run", false, "preview one cycle without sending or recording jobs")
	flag.Parse()
	if !*dryRun {
		if err := godotenv.Load(); err != nil {
			log.Fatal(err)
		}
	}

	targets, err := loadTargets(targetsPath)
	if err != nil {
		log.Fatal(err)
	}

	sent, err := loadSent(sentPath)
	if err != nil {
		log.Fatal(err)
	}

	var sentLog *os.File
	if !*dryRun {
		sentLog, err = openSentLog(sentPath)
		if err != nil {
			log.Fatal(err)
		}
		defer sentLog.Close()
	}

	ctx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()

	// Check immediately on startup, then keep checking until the process is
	// asked to stop. A single loop prevents two polling runs from overlapping.
	for {
		runOnce(targets, sent, sentLog, *dryRun)
		if *once || *dryRun || ctx.Err() != nil {
			return
		}

		log.Printf("next check in %s", pollInterval)
		timer := time.NewTimer(pollInterval)
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			log.Print("shutdown requested, exiting")
			return
		}
	}
}

func runOnce(targets []target, sent map[string]bool, sentLog *os.File, dryRun bool) {
	// One board failing should not cost us the other two.
	var postings []Posting
	for _, t := range targets {
		got, err := fetchJobs(t.Vendor, t.Company)
		if err != nil {
			log.Printf("%s/%s failed: %v", t.Vendor, t.Company, err)
			continue
		}
		fmt.Printf("%-11s %-10s %4d postings\n", t.Vendor, t.Company, len(got))
		postings = append(postings, got...)
	}

	newFound := 0
	internshipMatches := 0
	softwareMatches := 0
	reviewMatches := 0
	sentThisRun := 0
	stopped := false

	for _, p := range postings {
		if !isInternship(p) {
			continue
		}
		internshipMatches++

		role := assessRole(p)
		if dryRun {
			fmt.Printf("[%s] %s / %s | %s | %s\n", role.Decision, p.Company, p.Title, p.Location, role.Reason)
		}
		if role.Decision == roleIgnore {
			continue
		}
		if role.Decision == roleMatch {
			softwareMatches++
		} else {
			reviewMatches++
		}

		key := p.Key()
		if sent[key] {
			continue
		}
		newFound++
		if dryRun {
			continue
		}

		// Keep counting the rest so the summary is honest, but send no more.
		if stopped || sentThisRun >= maxPerRun {
			continue
		}

		// Placeholder, not real rate limiting. Discord allows roughly five
		// messages per five seconds per channel. Backoff comes later.
		if sentThisRun > 0 {
			time.Sleep(time.Second)
		}

		if err := sendToDiscord(formatPosting(p, role)); err != nil {
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

	fmt.Printf("fetched %d, internship matches %d, software matches %d, review %d, new %d, sent this run %d, left %d\n",
		len(postings), internshipMatches, softwareMatches, reviewMatches, newFound, sentThisRun, newFound-sentThisRun)
}
