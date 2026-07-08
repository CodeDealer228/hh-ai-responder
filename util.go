package main

import (
	"os"
	"strings"
)

// loadTextFile reads a whole file and trims it, returning "" if it doesn't exist.
func loadTextFile(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// loadQuestions reads newline-delimited filler messages sent to employer chats by
// AutoRespondChats. Missing file just means the chat filler feature has nothing to send.
func loadQuestions(path string) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	var questions []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		questions = append(questions, line)
	}
	return questions
}
