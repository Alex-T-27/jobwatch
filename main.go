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

func main() {
	// Loads .env content
	err := godotenv.Load()
	if err != nil {
		log.Fatal(err)
	}

	channelID := os.Getenv("CHANNEL_ID")
	token := os.Getenv("DISCORD_TOKEN")

	// Creates a new client object
	client := &http.Client{}

	// Todo: Fetch one company's job posting and turn it into readable file

	job_list := JobList{}
	company := "Deepgram"
	job_url := fmt.Sprintf("https://api.ashbyhq.com/posting-api/job-board/%s", company)
	req, err := http.NewRequest(
		"GET",
		job_url,
		nil,
	)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	resp, err := client.Do(req)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}
	defer resp.Body.Close()

	fmt.Println(resp.Status)
	fmt.Println(resp.Header.Get("Content-Type"))
	fmt.Println(resp.Header.Get("Content-Length"))

	if resp.StatusCode == http.StatusOK {
		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			fmt.Println("Error:", err)
			return
		}

		err = json.Unmarshal(bodyBytes, &job_list)
		if err != nil {
			fmt.Println("Error:", err)
			return
		}

	}

	//Todo: one Deepgram job appears in discord as readable message.
	//Fetch -> Decode -> Pick jobs -> Format -> Send
	//Checkpoint: Be able to print job[0] title to the terminal

	job := job_list.Jobs[62]
	//Message content
	message := fmt.Sprintf("***New Job Found*** \n**%s - %s**\n%s\n%s", job.Title, company, job.Location, job.JobUrl)

	// The content
	body := Data{Content: message}

	jsonData, err := json.Marshal(body) // Return a []byte @ jsonData
	if err != nil {
		fmt.Println("Error:", err)
		return
	}
	final_content := bytes.NewBuffer(jsonData)

	// The URL to my discord server
	discord_url := fmt.Sprintf("https://discord.com/api/v10/channels/%s/messages", channelID)

	// Creates a request containing METHOD, URL, CONTENT
	req, err = http.NewRequest(
		"POST",
		discord_url,
		final_content,
	)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	// Modify the request's Headers
	req.Header.Add("Authorization", "Bot "+token)
	req.Header.Add("Content-Type", "application/json")

	// Tell the Client to response to that request
	resp, err = client.Do(req)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}
	defer resp.Body.Close()

	fmt.Println(resp.Status)

	if resp.StatusCode != http.StatusOK {
		bodyError, err := io.ReadAll(resp.Body)
		if err != nil {
			fmt.Println("Error:", err)
			return
		}

		fmt.Println(string(bodyError))

	}
}
