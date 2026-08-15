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
	Id               string
	Title            string
	Location         string
	DescriptionPlain string
	JobUrl           string
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

	// The content
	body := Data{Content: "I'm fetching TikTok's Job Posting LMAO JAJAJA"}

	// Creates a new client object
	client := &http.Client{}

	jsonData, err := json.Marshal(body) // Return a []byte @ jsonData
	if err != nil {
		fmt.Println("Error:", err)
		return
	}
	final_content := bytes.NewBuffer(jsonData)

	// The URL to my discord server
	discord_url := fmt.Sprintf("https://discord.com/api/v10/channels/%s/messages", channelID)

	// Creates a request containing METHOD, URL, CONTENT
	req, err := http.NewRequest(
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
	resp, err := client.Do(req)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}
	defer resp.Body.Close()

	fmt.Println(resp.Status)

	// Todo: Fetch one company's job posting and turn it into readable file

	TheJob := JobList{}
	job_url := "https://api.ashbyhq.com/posting-api/job-board/Deepgram"
	req, err = http.NewRequest(
		"GET",
		job_url,
		nil,
	)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	resp, err = client.Do(req)
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

		err = json.Unmarshal(bodyBytes, &TheJob)
		if err != nil {
			fmt.Println("Error:", err)
			return
		}

	}

	fmt.Println(TheJob)

	//Todo: one Deepgram job appears in discord as readable message.
	//Fetch -> Decode -> Pick jobs -> Format -> Send
	//Checkpoint: Be able to print job[0] title to the terminal

}
