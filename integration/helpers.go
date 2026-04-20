//go:build integration

package integration

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	// Default timeout for service readiness checks.
	defaultWaitTimeout = 120 * time.Second
	// Polling interval for readiness checks.
	defaultPollInterval = 2 * time.Second
	// Default timeout for message response.
	defaultResponseTimeout = 60 * time.Second
)

// waitForService polls url until it returns HTTP 200 or timeout elapses.
func waitForService(url string, timeout time.Duration) error {
	if timeout == 0 {
		timeout = defaultWaitTimeout
	}
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 5 * time.Second}

	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(defaultPollInterval)
	}
	return fmt.Errorf("service at %s not ready after %v", url, timeout)
}

// waitForOllamaModel polls Ollama until the specified model is available.
func waitForOllamaModel(ollamaURL, model string, timeout time.Duration) error {
	if timeout == 0 {
		timeout = defaultWaitTimeout
	}
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 10 * time.Second}

	for time.Now().Before(deadline) {
		resp, err := client.Get(ollamaURL + "/api/tags")
		if err != nil {
			time.Sleep(defaultPollInterval)
			continue
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			time.Sleep(defaultPollInterval)
			continue
		}

		var tags struct {
			Models []struct {
				Name string `json:"name"`
			} `json:"models"`
		}
		if err := json.Unmarshal(body, &tags); err != nil {
			time.Sleep(defaultPollInterval)
			continue
		}

		for _, m := range tags.Models {
			if strings.HasPrefix(m.Name, model) {
				return nil
			}
		}
		time.Sleep(defaultPollInterval)
	}
	return fmt.Errorf("model %s not available at %s after %v", model, ollamaURL, timeout)
}

// simplexAPIGet performs a GET request to the SimpleX Chat HTTP API.
func simplexAPIGet(simplexURL string, path string) (map[string]interface{}, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(simplexURL + path)
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", path, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse response: %w (body: %s)", err, string(body))
	}
	return result, nil
}

// simplexAPIPost performs a POST request to the SimpleX Chat HTTP API.
func simplexAPIPost(simplexURL string, path string, payload interface{}) (map[string]interface{}, error) {
	var bodyReader io.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("marshal payload: %w", err)
		}
		bodyReader = strings.NewReader(string(data))
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Post(simplexURL+path, "application/json", bodyReader)
	if err != nil {
		return nil, fmt.Errorf("POST %s: %w", path, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse response: %w (body: %s)", err, string(body))
	}
	return result, nil
}

// getBotAddress retrieves the SimpleX address for a bot via its HTTP API.
// The bot must have an address created (via /_address command).
func getBotAddress(simplexURL string) (string, error) {
	result, err := simplexAPIGet(simplexURL, "/show-address")
	if err != nil {
		return "", fmt.Errorf("get bot address: %w", err)
	}

	// Extract the connection link from the response.
	link, ok := result["connLinkContact"]
	if !ok {
		link, ok = result["contactLink"]
	}
	if !ok {
		// Try nested contactLink structure.
		if cl, ok := result["contactLink"].(map[string]interface{}); ok {
			if connStr, ok := cl["connLinkContact"].(string); ok {
				return connStr, nil
			}
		}
		return "", fmt.Errorf("no address found in response: %v", result)
	}

	addr, ok := link.(string)
	if !ok {
		return "", fmt.Errorf("address is not a string: %v", link)
	}
	return addr, nil
}

// connectToBot connects the test client to the bot using the bot's SimpleX address.
func connectToBot(clientURL, botAddress string) error {
	payload := map[string]interface{}{
		"address": botAddress,
	}
	_, err := simplexAPIPost(clientURL, "/connect", payload)
	if err != nil {
		return fmt.Errorf("connect to bot: %w", err)
	}
	return nil
}

// sendMessage sends a text message from the test client to a contact.
func sendMessage(clientURL, contactID, text string) (map[string]interface{}, error) {
	payload := map[string]interface{}{
		"contactId": contactID,
		"text":      text,
	}
	result, err := simplexAPIPost(clientURL, "/send", payload)
	if err != nil {
		return nil, fmt.Errorf("send message: %w", err)
	}
	return result, nil
}

// waitForResponse polls the client's messages until a response from the bot is received.
func waitForResponse(clientURL, contactID string, timeout time.Duration) (string, error) {
	if timeout == 0 {
		timeout = defaultResponseTimeout
	}
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		result, err := simplexAPIGet(clientURL, "/messages?contact="+contactID+"&limit=10")
		if err != nil {
			time.Sleep(2 * time.Second)
			continue
		}

		messages, ok := result["messages"].([]interface{})
		if !ok || len(messages) == 0 {
			time.Sleep(2 * time.Second)
			continue
		}

		// Look for a response message from the bot (not from us).
		for _, msg := range messages {
			m, ok := msg.(map[string]interface{})
			if !ok {
				continue
			}
			// Check if this is an incoming message from the bot.
			if direction, ok := m["direction"].(string); ok && direction == "incoming" {
				if content, ok := m["content"].(map[string]interface{}); ok {
					if text, ok := content["text"].(string); ok && text != "" {
						return text, nil
					}
				}
			}
			// Fallback: check for text in message body.
			if text, ok := m["text"].(string); ok && text != "" {
				return text, nil
			}
		}
		time.Sleep(2 * time.Second)
	}
	return "", fmt.Errorf("no response received within %v", timeout)
}

// waitForContactConnected waits until the contact connection is established.
func waitForContactConnected(clientURL, botAddress string, timeout time.Duration) (string, error) {
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		result, err := simplexAPIGet(clientURL, "/contacts")
		if err != nil {
			time.Sleep(2 * time.Second)
			continue
		}

		contacts, ok := result["contacts"].([]interface{})
		if !ok {
			time.Sleep(2 * time.Second)
			continue
		}

		for _, c := range contacts {
			contact, ok := c.(map[string]interface{})
			if !ok {
				continue
			}
			status, _ := contact["status"].(string)
			if status == "ready" {
				if id, ok := contact["contactId"].(string); ok {
					return id, nil
				}
				if id, ok := contact["contactId"].(float64); ok {
					return fmt.Sprintf("%d", int64(id)), nil
				}
			}
		}
		time.Sleep(2 * time.Second)
	}
	return "", fmt.Errorf("contact not connected within %v", timeout)
}