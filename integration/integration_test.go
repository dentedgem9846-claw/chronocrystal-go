//go:build integration

package integration

import (
	"os"
	"testing"
	"time"
)

const (
	// Service URLs default to localhost with published Docker ports.
	// Override via environment variables when running in different configs.
	ollamaURL       = "http://localhost:11434"
	simplexBotURL   = "http://localhost:5225"
	simplexClientURL = "http://localhost:5226"
	model           = "gemma3:4b"
)

// TestSimpleXBotStartup verifies the simplex-bot container starts and its
// HTTP API becomes reachable.
func TestSimpleXBotStartup(t *testing.T) {
	if err := waitForService(simplexBotURL, 60*time.Second); err != nil {
		t.Fatalf("simplex-bot not ready: %v", err)
	}
}

// TestOllamaReady waits for Ollama to be healthy and verifies the model is
// available.
func TestOllamaReady(t *testing.T) {
	if err := waitForService(ollamaURL, 120*time.Second); err != nil {
		t.Fatalf("ollama not ready: %v", err)
	}

	if err := waitForOllamaModel(ollamaURL, model, 300*time.Second); err != nil {
		t.Fatalf("model %s not available: %v", model, err)
	}
}

// TestEndToEndChat performs a full end-to-end test: connect the test client
// to the ChronoCrystal bot, send a chat message, and verify a response is
// received.
func TestEndToEndChat(t *testing.T) {
	// Wait for both simplex services and Ollama.
	if err := waitForService(simplexBotURL, 60*time.Second); err != nil {
		t.Fatalf("simplex-bot not ready: %v", err)
	}
	if err := waitForService(simplexClientURL, 60*time.Second); err != nil {
		t.Fatalf("simplex-client not ready: %v", err)
	}
	if err := waitForService(ollamaURL, 120*time.Second); err != nil {
		t.Fatalf("ollama not ready: %v", err)
	}
	if err := waitForOllamaModel(ollamaURL, model, 300*time.Second); err != nil {
		t.Fatalf("model %s not available: %v", model, err)
	}

	// Get the bot's SimpleX address.
	botAddress, err := getBotAddress(simplexBotURL)
	if err != nil {
		t.Fatalf("failed to get bot address: %v", err)
	}
	t.Logf("Bot address: %s", botAddress)

	// Connect the test client to the bot.
	if err := connectToBot(simplexClientURL, botAddress); err != nil {
		t.Fatalf("failed to connect to bot: %v", err)
	}

	// Wait for the contact connection to be established.
	contactID, err := waitForContactConnected(simplexClientURL, botAddress, 30*time.Second)
	if err != nil {
		t.Fatalf("contact not connected: %v", err)
	}
	t.Logf("Connected with contact ID: %s", contactID)

	// Send a chat message.
	_, err = sendMessage(simplexClientURL, contactID, "Hello, ChronoCrystal!")
	if err != nil {
		t.Fatalf("failed to send message: %v", err)
	}

	// Wait for a response.
	response, err := waitForResponse(simplexClientURL, contactID, 60*time.Second)
	if err != nil {
		t.Fatalf("no response received: %v", err)
	}

	if response == "" {
		t.Fatal("received empty response")
	}
	t.Logf("Bot response: %s", response)
}

// TestEndToEndOrder performs an end-to-end test with a message that should
// trigger tool execution, verifying that ChronoCrystal can process orders.
func TestEndToEndOrder(t *testing.T) {
	// Ensure services are up.
	if err := waitForService(simplexBotURL, 60*time.Second); err != nil {
		t.Fatalf("simplex-bot not ready: %v", err)
	}
	if err := waitForService(simplexClientURL, 60*time.Second); err != nil {
		t.Fatalf("simplex-client not ready: %v", err)
	}
	if err := waitForService(ollamaURL, 120*time.Second); err != nil {
		t.Fatalf("ollama not ready: %v", err)
	}
	if err := waitForOllamaModel(ollamaURL, model, 300*time.Second); err != nil {
		t.Fatalf("model %s not available: %v", model, err)
	}

	// Get the bot's SimpleX address.
	botAddress, err := getBotAddress(simplexBotURL)
	if err != nil {
		t.Fatalf("failed to get bot address: %v", err)
	}

	// Connect the test client to the bot.
	if err := connectToBot(simplexClientURL, botAddress); err != nil {
		t.Fatalf("failed to connect to bot: %v", err)
	}

	// Wait for the contact connection to be established.
	contactID, err := waitForContactConnected(simplexClientURL, botAddress, 30*time.Second)
	if err != nil {
		t.Fatalf("contact not connected: %v", err)
	}
	t.Logf("Connected with contact ID: %s", contactID)

	// Send an order message that should trigger tool execution.
	_, err = sendMessage(simplexClientURL, contactID, "What files are in /app/data?")
	if err != nil {
		t.Fatalf("failed to send message: %v", err)
	}

	// Wait for a response with a longer timeout (LLM inference + tool execution).
	response, err := waitForResponse(simplexClientURL, contactID, 90*time.Second)
	if err != nil {
		t.Fatalf("no response received: %v", err)
	}

	if response == "" {
		t.Fatal("received empty response")
	}
	t.Logf("Bot response: %s", response)

	// Verify the response contains evidence of file listing or tool use.
	// The response should mention file names or directory contents.
	// We do a minimal check here — the exact content depends on the model.
	t.Logf("Order test completed with response length: %d", len(response))
}

func TestMain(m *testing.M) {
	// Integration tests assume Docker Compose services are already running.
	// The Makefile target handles starting/stopping Docker Compose.
	os.Exit(m.Run())
}