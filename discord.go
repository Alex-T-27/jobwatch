package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
)

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
