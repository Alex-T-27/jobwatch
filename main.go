package main

import (
	"context"
	"errors"
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
	Locations      []postingLocation
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

// Pause after a complete sweep, not after each batch.
const pollInterval = time.Minute

func formatPosting(p Posting, role roleAssessment) string {
	season := assessSeason(p)
	seasonLabel := "fits seasonal schedule"
	if season.Decision == seasonReview {
		seasonLabel = "REVIEW: season or workload unclear"
	}
	location := assessLocation(p)
	locationLabel := "US option available"
	if location.Decision == locationReview {
		locationLabel = "REVIEW: US availability unclear"
	}
	eligibility := formatEligibility(assessEligibility(p.Description))

	roleLabel := "✅ **Role match:** software"
	if role.Decision == roleReview {
		roleLabel = "🔎 **Role match:** review"
	}

	message := fmt.Sprintf("***New Job Found*** \n**%s - %s**\n%s\n**Location:** %s\n**Location evidence:** %s\n%s\n**Role evidence:** %s\n**Schedule:** %s\n**Schedule evidence:** %s\n%s\n%s",
		p.Title,
		p.Company,
		p.Location,
		locationLabel,
		location.Reason,
		roleLabel,
		role.Reason,
		seasonLabel,
		season.Reason,
		eligibility,
		p.Url,
	)
	return message
}

func main() {
	once := flag.Bool("once", false, "check boards once and exit")
	dryRun := flag.Bool("dry-run", false, "preview without sending alerts or changing any files")
	discoverOnly := flag.Bool("discover-only", false, "discover company boards and exit without sending alerts")
	noDiscovery := flag.Bool("no-discovery", false, "scan only configured companies without searching for new boards")
	flag.Parse()
	if *discoverOnly && *noDiscovery {
		log.Fatal("-discover-only and -no-discovery cannot be combined")
	}
	if !*dryRun && !*discoverOnly {
		if err := godotenv.Load(); err != nil {
			log.Fatal(err)
		}
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if *discoverOnly {
		_, report, err := discoverCompanies(ctx, targetsPath, *dryRun)
		if err != nil {
			log.Fatal(err)
		}
		printDiscoveryReport(report, *dryRun)
		return
	}

	targets, err := loadTargets(targetsPath)
	if err != nil {
		log.Fatal(err)
	}

	sent, err := loadSent(sentPath)
	if err != nil {
		log.Fatal(err)
	}
	scans, err := loadScanState(scanPath)
	if err != nil {
		log.Fatal(err)
	}
	scanner := &scanner{state: scans, path: scanPath}

	var sentLog *os.File
	if !*dryRun {
		sentLog, err = openSentLog(sentPath)
		if err != nil {
			log.Fatal(err)
		}
		defer sentLog.Close()
	}

	// Check immediately on startup, then keep checking until the process is
	// asked to stop. A single loop prevents two polling runs from overlapping.
	var nextDiscovery time.Time
	for {
		if refreshed, err := loadTargets(targetsPath); err != nil {
			log.Printf("company config reload failed, keeping current boards: %v", err)
		} else {
			targets = refreshed
		}
		if !*noDiscovery && discoveryDue(time.Now(), nextDiscovery) {
			refreshed, report, err := discoverCompanies(ctx, targetsPath, *dryRun)
			nextDiscovery = time.Now().Add(discoveryInterval)
			if err != nil {
				log.Printf("discovery failed, keeping current boards: %v", err)
			} else {
				targets = refreshed
				printDiscoveryReport(report, *dryRun)
			}
		}
		if err := runOnce(ctx, scanner, targets, sent, sentLog, *dryRun); err != nil {
			if errors.Is(err, context.Canceled) {
				log.Print("shutdown requested, exiting")
				return
			}
			log.Fatal(err)
		}
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

func runOnce(ctx context.Context, scanner *scanner, targets []target, sent map[string]bool, sentLog *os.File, dryRun bool) error {
	budget := &deliveryBudget{}
	for start := 0; start < len(targets); start += scanBatchSize {
		end := min(start+scanBatchSize, len(targets))
		postings, reports, err := scanner.scanBatch(ctx, targets[start:end], dryRun)
		if err != nil {
			return err
		}
		fmt.Printf("\nBatch %d/%d: companies %d-%d\n", start/scanBatchSize+1, (len(targets)+scanBatchSize-1)/scanBatchSize, start+1, end)
		for _, report := range reports {
			fmt.Println(formatBoardReport(report))
		}
		if err := deliverPostings(ctx, postings, sent, sentLog, dryRun, budget); err != nil {
			return err
		}
	}
	return nil
}

// The send cap applies to a whole sweep. More batches must not multiply it.
type deliveryBudget struct {
	sent    int
	stopped bool
}

func deliverPostings(ctx context.Context, postings []Posting, sent map[string]bool, sentLog *os.File, dryRun bool, budget *deliveryBudget) error {
	newFound := 0
	internshipMatches := 0
	softwareMatches := 0
	reviewMatches := 0
	locationSkipped := 0
	locationReviews := 0
	seasonSkipped := 0
	seasonReviews := 0
	eligibilitySkipped := 0
	sentThisRun := 0

	for _, p := range postings {
		if err := ctx.Err(); err != nil {
			return err
		}
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

		location := assessLocation(p)
		if dryRun {
			fmt.Printf("  location [%s]: %s\n", location.Decision, location.Reason)
		}
		if location.Decision == locationIgnore {
			locationSkipped++
			continue
		}
		if location.Decision == locationReview {
			locationReviews++
		}

		season := assessSeason(p)
		if dryRun {
			fmt.Printf("  schedule [%s]: %s\n", season.Decision, season.Reason)
		}
		if season.Decision == seasonIgnore {
			seasonSkipped++
			continue
		}
		if season.Decision == seasonReview {
			seasonReviews++
		}

		eligibility := assessEligibility(p.Description)
		if dryRun {
			fmt.Printf("  eligibility [%s]: %s\n", eligibility.Decision, eligibility.Reason)
			if eligibility.Evidence != "" {
				fmt.Printf("  JD evidence: %s\n", eligibility.Evidence)
			}
		}
		if eligibility.Decision == eligibilityIgnore {
			eligibilitySkipped++
			continue
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
		if budget.stopped || budget.sent >= maxPerRun {
			continue
		}

		// Placeholder, not real rate limiting. Discord allows roughly five
		// messages per five seconds per channel. Backoff comes later.
		if budget.sent > 0 {
			timer := time.NewTimer(time.Second)
			select {
			case <-timer.C:
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			}
		}

		if err := sendToDiscord(formatPosting(p, role)); err != nil {
			log.Printf("send failed on %s, stopping this run: %v", key, err)
			budget.stopped = true
			continue
		}

		// Record the key only after the send succeeded. Crashing between the
		// two means this posting sends twice next run, which beats losing it.
		if err := markSent(sentLog, key); err != nil {
			return fmt.Errorf("sent %s but could not record it: %w", key, err)
		}
		sent[key] = true
		sentThisRun++
		budget.sent++
	}

	fmt.Printf("fetched %d, internship matches %d, software matches %d, role review %d, outside US %d, location review %d, schedule skipped %d, schedule review %d, eligibility skipped %d, pending alerts %d, sent this batch %d, left %d\n",
		len(postings), internshipMatches, softwareMatches, reviewMatches, locationSkipped, locationReviews, seasonSkipped, seasonReviews, eligibilitySkipped, newFound, sentThisRun, newFound-sentThisRun)
	return nil
}
