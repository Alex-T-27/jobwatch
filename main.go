package main 

import (
	"fmt"
	"net/http"
	"encoding/json"
	"bytes"
	"os"
	"github.com/joho/godotenv"
	"log"
)

type Data struct {
	Content string `json:"content"`
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
	body := Data{Content: "Hello World!!!"}

	// Creates a new client object
	client := &http.Client{}

	jsonData, err := json.Marshal(body) // Return a []byte @ jsonData
	if err != nil {
		fmt.Println("Error:", err)
		return
	}
	final_content := bytes.NewBuffer(jsonData) 

	// The URL to my discord server
	url := fmt.Sprintf("https://discord.com/api/v10/channels/%s/messages", channelID)

	// Creates a request containing METHOD, URL, CONTENT
	req, err := http.NewRequest(
		"POST",
		url,
		final_content,
	)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}
	
	// Modify the request's Headers
	req.Header.Add("Authorization", "Bot " + token)
	req.Header.Add("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}
	defer resp.Body.Close()

	fmt.Println(resp.Status)
}
