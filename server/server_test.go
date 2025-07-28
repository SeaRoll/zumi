package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// await calls the method until it returns nil or timeout occurs.
func await(t *testing.T, fn func() error) {
	t.Helper()
	timeout := time.After(5 * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			t.Fatal("Timeout waiting for condition")
		case <-ticker.C:
			if err := fn(); err == nil {
				return
			}
		}
	}
}

// waitUntilServerStarted waits until the server is reachable at the given address.
func waitUntilServerStarted(t *testing.T, addr string) {
	t.Helper()
	await(t, func() error {
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return err
		}
		conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, port), 5*time.Second)
		if err != nil {
			return err
		}
		defer func() { _ = conn.Close() }()
		return nil
	})
}

// waitUntilServerStopped waits until the server is stopped by checking if it can connect to the address.
func waitUntilServerStopped(t *testing.T, addr string) {
	t.Helper()
	await(t, func() error {
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return err
		}
		conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, port), 5*time.Second)
		if err == nil {
			defer func() { _ = conn.Close() }()
			return fmt.Errorf("server still running at %s", addr)
		}
		return nil
	})
}

func TestServer(t *testing.T) {
	addr := "localhost:8080"
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() {
		cancel() // Ensure the context is cancelled after the test
		ClearServer()
		waitUntilServerStopped(t, addr)
	})

	middlewareCalledTimes := 0
	AddMiddleware(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			middlewareCalledTimes++
			next.ServeHTTP(w, r)
		})
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
	AddHandler("POST /{name}/hello", func(w http.ResponseWriter, r *http.Request) {
		type res struct {
			HelloMessage string `json:"helloMessage"`
		}
		var req struct {
			Name string `path:"name" validate:"required"`
			Body struct {
				Message string `json:"message" validate:"required"`
			} `body:"json"`
		}
		if err := ParseRequest(r, &req); err != nil {
			WriteError(w, http.StatusBadRequest, "Invalid request format")
			return
		}

		WriteJSON(w, http.StatusOK, res{
			HelloMessage: "Hello, " + req.Name + " - " + req.Body.Message,
		})
	})

	// perform http request to the server
	go func() {
		if err := StartServer(ctx, addr); err != nil {
			t.Errorf("Failed to start server: %v", err)
		}
	}()

	// Wait for the server to start
	waitUntilServerStarted(t, addr)

	t.Run("Health Check", func(t *testing.T) {
		resp, err := http.Get("http://" + addr + "/health?message=Yohan")
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

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
		assert.Equal(t, 1, middlewareCalledTimes, "Middleware should be called once")
	})

	t.Run("Health Check - no query", func(t *testing.T) {
		resp, err := http.Get("http://" + addr + "/health")
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

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

	t.Run("Post hello with path name", func(t *testing.T) {
		type requestBody struct {
			Message string `json:"message" validate:"required"`
		}
		body := requestBody{Message: "hello!"}
		bodyBytes, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("Failed to marshal request body: %v", err)
		}
		resp, err := http.Post("http://"+addr+"/Yohan/hello", "application/json", bytes.NewBuffer(bodyBytes))
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status OK, got %s", resp.Status)
		}

		var res struct {
			HelloMessage string `json:"helloMessage"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}
		assert.Equal(t, "Hello, Yohan - hello!", res.HelloMessage)
	})

	t.Run("Post hello with path name - Error no body", func(t *testing.T) {
		type requestBody struct {
			Message string `json:"message" validate:"required"`
		}
		body := requestBody{Message: ""}
		bodyBytes, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("Failed to marshal request body: %v", err)
		}
		resp, err := http.Post("http://"+addr+"/Yohan/hello", "application/json", bytes.NewBuffer(bodyBytes))
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("Expected status BadRequest, got %s", resp.Status)
		}
	})
}
