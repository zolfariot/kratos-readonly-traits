package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
)

// Identity structure matching Kratos API response
type Identity struct {
	ID        string `json:"id"`
	SchemaID  string `json:"schema_id"`	
	Traits 	  map[string]interface{} `json:"traits"`
}

// Webhook request body
type WebhookRequest struct {
	SchemaID  string         `json:"schema_id"`
	OldTraits map[string]any `json:"old_traits"`
	NewTraits map[string]any `json:"new_traits"`
}

type WebhookResponse struct {
	Messages []WebhookResponseTopMessage `json:"messages"`
}

type WebhookResponseTopMessage struct {
	InstancePtr string `json:"instance_ptr"`
	Messages []WebhookResponseNestedMessage `json:"messages"`
}

type WebhookResponseNestedMessage struct {
	ID   int    `json:"id"`
	Text string `json:"text"`
	Type string `json:"type"`
}

// KratosSchema represents the Kratos schema (simplified for this example)
type KratosSchema struct {
	Type       string `json:"type"`
	Properties struct {
		Traits struct {
			Properties map[string]map[string]interface{}
		} `json:"traits"`
	} `json:"properties"`
}

// FetchSchema fetches the schema for identity traits from Kratos Admin API
func fetchSchemaImmutableTraits(schemaID string) (map[string]struct{}, error) {
	kratosURL := os.Getenv("KRATOS_PUBLIC_URL")
	if kratosURL == "" {
		return nil, errors.New("Kratos URL is not set")
	}

	log.Printf("Fetching schema (%s/schemas/%s)", kratosURL, schemaID)
	// Fetch identity schema from Kratos
	log.Printf("Sending request to %s/schemas/%s", kratosURL, schemaID)
	resp, err := http.Get(fmt.Sprintf("%s/schemas/%s", kratosURL, schemaID))
	if err != nil || resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch schema: %v", err)
	}
	defer resp.Body.Close()

	var schema KratosSchema
	err = json.NewDecoder(resp.Body).Decode(&schema)
	if err != nil {
		return nil, fmt.Errorf("failed to decode schema: %v", err)
	}

	// Process the schema to identify immutable traits
	immutableTraits := make(map[string]struct{})
	for trait, props := range schema.Properties.Traits.Properties {
		if immutable, ok := props["zolfa.dev/kratos-readonly"]; ok && immutable.(bool) {
			immutableTraits[trait] = struct{}{}
		}
	}

	log.Printf("Schema fetched")
	return immutableTraits, nil
}

// webhookHandler processes the webhook request
func webhookHandler(w http.ResponseWriter, r *http.Request) {
	var payload WebhookRequest

	// Decode JSON request
	err := json.NewDecoder(r.Body).Decode(&payload)
	if err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	immutableTraits, err := fetchSchemaImmutableTraits(payload.SchemaID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to obtain schema immutable traits: %v", err), http.StatusInternalServerError)
		return
	}

	response := WebhookResponse{
		Messages: make([]WebhookResponseTopMessage, 0, len(immutableTraits)),
	}
	// Check for immutable traits and deny modification if changed
	for trait := range immutableTraits {
		if payload.NewTraits[trait] != nil && payload.NewTraits[trait] != payload.OldTraits[trait] {
			response.Messages = append(response.Messages, WebhookResponseTopMessage{
				InstancePtr: "#/traits/" + trait,
				Messages: []WebhookResponseNestedMessage{
					{
						ID: 1377,
						Text: "Element is read-only.",
						Type: "conflict",

					},
				},
			})
		}
	}

	if len(response.Messages) > 0 {
		log.Printf("Update request denied")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(response)
	}
	w.WriteHeader(http.StatusOK)
}


func main() {
	port := os.Getenv("PORT")
	if port == "" {	
		port = "3000" // Default port
	}

	http.HandleFunc("/hooks/check-readonly-traits", webhookHandler)

	log.Println("Webhook running on port", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

