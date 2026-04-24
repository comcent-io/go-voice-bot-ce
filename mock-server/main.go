package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
)

type VoiceBot struct {
	ID                   string           `json:"id"`
	OrgID                string           `json:"orgId"`
	Name                 string           `json:"name"`
	Instructions         string           `json:"instructions"`
	NotToDoInstructions  string           `json:"notToDoInstructions"`
	GreetingInstructions string           `json:"greetingInstructions"`
	IsHangup             bool             `json:"isHangup"`
	IsEnqueue            bool             `json:"isEnqueue"`
	Queues               []string         `json:"queues"`
	Org                  struct {
		ID        string `json:"id"`
		Subdomain string `json:"subdomain"`
	} `json:"org"`
}

func main() {
	http.HandleFunc("/internal-api/voice-bot/", func(w http.ResponseWriter, r *http.Request) {
		// Set CORS headers
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		// Create mock data
		mockData := VoiceBot{
			ID:           "35f4493a-8a18-4644-941b-f14e630ceb3d",
			Name:         "Test Voice Bot",
			Instructions: "You are helpful voice bot who will answer the question and your name is Siri",
			Org: struct {
				ID        string `json:"id"`
				Subdomain string `json:"subdomain"`
			}{
				ID:        "a2687db2-a90a-41e7-ab28-1b231eee6d8b",
				Subdomain: "acme",
			},
			OrgID:                "a2687db2-a90a-41e7-ab28-1b231eee6d8b",
			Queues:               []string{"qeee", "eeee"},
			NotToDoInstructions:  "If conversation is related to bus then you don't answer it.",
			GreetingInstructions: "Greet the user with \"Welcome to Ram Raj\" when greeted with hello",
			IsHangup:  true,
			IsEnqueue: true,
		}

		// Convert to JSON
		jsonData, err := json.Marshal(mockData)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Set content type and write response
		w.Header().Set("Content-Type", "application/json")
		w.Write(jsonData)
	})

	http.HandleFunc("/internal-api/billing/voice-bot", func(w http.ResponseWriter, r *http.Request) {
		// Set CORS headers
		log.Println("Billing request received")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		// Return 200 OK
		log.Println("Billing response sent successfully")
		w.WriteHeader(http.StatusOK)
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8123"
	}

	log.Printf("Mock server starting on port %s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
