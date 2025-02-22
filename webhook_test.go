package main

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// Helper functions to mock the public and admin APIs
func mockKratosPublicAPI(schemaID string, statusCode int, body string) *httptest.Server {
	handler := http.NewServeMux()
	handler.HandleFunc("/schemas/"+schemaID, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(statusCode)
		w.Write([]byte(body))
	})
	return httptest.NewServer(handler)
}

func mockKratosAdminAPI(identityID string, statusCode int, body string) *httptest.Server {
	handler := http.NewServeMux()
	handler.HandleFunc("/identities/"+identityID, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(statusCode)
		w.Write([]byte(body))
	})
	return httptest.NewServer(handler)
}

func TestWebhookHandler(t *testing.T) {
	// Define mock responses for the Kratos API
	publicAPIResponse := `{
		"type": "object",
		"properties": {
			"traits": {
				"properties": {
					"email": {
						"zolfa.dev/kratos-readonly": true
					},
					"username": {
						"zolfa.dev/kratos-readonly": false
					}
				}
			}
		}
	}`

	adminAPIResponse := `{
		"id": "identity123",
		"schema_id": "schema123",
		"traits": {
			"email": "test@example.com",
			"username": "testuser"
		}
	}`

	// Mock Kratos Public and Admin APIs
	publicAPIServer := mockKratosPublicAPI("schema123", http.StatusOK, publicAPIResponse)
	defer publicAPIServer.Close()

	adminAPIServer := mockKratosAdminAPI("identity123", http.StatusOK, adminAPIResponse)
	defer adminAPIServer.Close()

	// Set environment variables for the test
	os.Setenv("KRATOS_PUBLIC_URL", publicAPIServer.URL)
	os.Setenv("KRATOS_ADMIN_URL", adminAPIServer.URL)

	// Test case 1: Modifying a mutable trait (username)
	t.Run("Modifying mutable trait", func(t *testing.T) {
		webhookRequest := WebhookRequest{
			Identity: Identity{
				ID:       "identity123",
				SchemaID: "schema123",
				Traits: map[string]interface{}{
					"username": "newusername",
				},
			},
		}

		requestBody, err := json.Marshal(webhookRequest)
		if err != nil {
			t.Fatalf("Failed to marshal request body: %v", err)
		}

		req := httptest.NewRequest(http.MethodPost, "/hooks/check-readonly-traits", nil)
		req.Body = ioutil.NopCloser(bytes.NewReader(requestBody))

		rr := httptest.NewRecorder()
		handler := http.HandlerFunc(webhookHandler)
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("Expected status code %d, got %d", http.StatusOK, rr.Code)
		}
	})

	// Test case 2: Modifying an immutable trait (email)
	t.Run("Modifying immutable trait", func(t *testing.T) {
		webhookRequest := WebhookRequest{
			Identity: Identity{
				ID:       "identity123",
				SchemaID: "schema123",
				Traits: map[string]interface{}{
					"email": "newemail@example.com",
				},
			},
		}

		requestBody, err := json.Marshal(webhookRequest)
		if err != nil {
			t.Fatalf("Failed to marshal request body: %v", err)
		}

		req := httptest.NewRequest(http.MethodPost, "/hooks/check-readonly-traits", nil)
		req.Body = ioutil.NopCloser(bytes.NewReader(requestBody))

		rr := httptest.NewRecorder()
		handler := http.HandlerFunc(webhookHandler)
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusConflict {
			t.Errorf("Expected status code %d, got %d", http.StatusBadRequest, rr.Code)
		}

	})

	// Test case 3: Sending wrong JSON format (missing traits)
	t.Run("Sending invalid JSON", func(t *testing.T) {
		invalidJSON := `{"identity: {"id": "identity123", "schema_id": "schema123"}}`

		req := httptest.NewRequest(http.MethodPost, "/hooks/check-readonly-traits", nil)
		req.Body = ioutil.NopCloser(bytes.NewReader([]byte(invalidJSON)))

		rr := httptest.NewRecorder()
		handler := http.HandlerFunc(webhookHandler)
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Errorf("Expected status code %d, got %d", http.StatusBadRequest, rr.Code)
		}

		expectedMessage := "Invalid JSON"
		if !strings.Contains(rr.Body.String(), expectedMessage) {
			t.Errorf("Expected message to contain %q, got %q", expectedMessage, rr.Body.String())
		}
	})

	// Test case 4: API response failure (public Kratos API down)
	t.Run("Public API failure", func(t *testing.T) {
		// Mock Kratos Public API failure
		publicAPIFailureServer := mockKratosPublicAPI("schema123", http.StatusInternalServerError, "")
		defer publicAPIFailureServer.Close()

		// Update environment variable
		os.Setenv("KRATOS_PUBLIC_URL", publicAPIFailureServer.URL)

		webhookRequest := WebhookRequest{
			Identity: Identity{
				ID:       "identity123",
				SchemaID: "schema123",
				Traits: map[string]interface{}{
					"username": "newusername",
				},
			},
		}

		requestBody, err := json.Marshal(webhookRequest)
		if err != nil {
			t.Fatalf("Failed to marshal request body: %v", err)
		}

		req := httptest.NewRequest(http.MethodPost, "/hooks/check-readonly-traits", nil)
		req.Body = ioutil.NopCloser(bytes.NewReader(requestBody))

		rr := httptest.NewRecorder()
		handler := http.HandlerFunc(webhookHandler)
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusInternalServerError {
			t.Errorf("Expected status code %d, got %d", http.StatusInternalServerError, rr.Code)
		}
	})

	// Test case 5: API response failure (admin Kratos API down)
	t.Run("Admin API failure", func(t *testing.T) {
		// Mock Kratos Admin API failure
		adminAPIFailureServer := mockKratosAdminAPI("identity123", http.StatusInternalServerError, "")
		defer adminAPIFailureServer.Close()

		// Update environment variable
		os.Setenv("KRATOS_ADMIN_URL", adminAPIFailureServer.URL)

		webhookRequest := WebhookRequest{
			Identity: Identity{
				ID:       "identity123",
				SchemaID: "schema123",
				Traits: map[string]interface{}{
					"username": "newusername",
				},
			},
		}

		requestBody, err := json.Marshal(webhookRequest)
		if err != nil {
			t.Fatalf("Failed to marshal request body: %v", err)
		}

		req := httptest.NewRequest(http.MethodPost, "/hooks/check-readonly-traits", nil)
		req.Body = ioutil.NopCloser(bytes.NewReader(requestBody))

		rr := httptest.NewRecorder()
		handler := http.HandlerFunc(webhookHandler)
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusInternalServerError {
			t.Errorf("Expected status code %d, got %d", http.StatusInternalServerError, rr.Code)
		}
	})
}

