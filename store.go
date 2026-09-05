package main

import (
	"bufio"
	"os"
	"strings"
)

// loadSent reads path and returns the set of job IDs already sent to Discord.
// A missing file is not an error, it means nothing has been sent yet.
func loadSent(path string) (map[string]bool, error) {
	sent := make(map[string]bool)

	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return sent, nil
		}
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		id := strings.TrimSpace(scanner.Text())
		if id == "" {
			continue
		}
		sent[id] = true
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return sent, nil
}
