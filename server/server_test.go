package server

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

func TestServer(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() {
		cancel() // Ensure the context is cancelled after the test
		ClearServer()
	})

	AddHandler("GET /health", func(w http.ResponseWriter, r *http.Request) {
		var res struct {
			HelloMessage string `json:"helloMessage"`
		}
		var req struct {
			Message *string `query:"message"`
		}
		if err := ParseRequest(r, &req); err != nil {
			WriteError(w, http.StatusBadRequest, "Invalid request format")
			return
		}

		if req.Message == nil {
			res.HelloMessage = "Hello, World"
		} else {
			res.HelloMessage = "Hello, " + *req.Message
		}
		WriteJSON(w, http.StatusOK, res)
	})

	// perform http request to the server
	addr := "localhost:8080"
	go func() {
		if err := StartServer(ctx, addr); err != nil {
			t.Errorf("Failed to start server: %v", err)
		}
	}()

	// Wait for the server to start
	time.Sleep(1 * time.Second)

	t.Run("Health Check", func(t *testing.T) {
		resp, err := http.Get("http://" + addr + "/health?message=Yohan")
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status OK, got %s", resp.Status)
		}

		var res struct {
			HelloMessage string `json:"helloMessage"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if res.HelloMessage != "Hello, Yohan" {
			t.Errorf("Expected HelloMessage 'Hello, Yohan', got '%s'", res.HelloMessage)
		}
	})

	t.Run("Health Check - no query", func(t *testing.T) {
		resp, err := http.Get("http://" + addr + "/health")
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status OK, got %s", resp.Status)
		}

		var res struct {
			HelloMessage string `json:"helloMessage"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if res.HelloMessage != "Hello, World" {
			t.Errorf("Expected HelloMessage 'Hello, World', got '%s'", res.HelloMessage)
		}
	})
}
