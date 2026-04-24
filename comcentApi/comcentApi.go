package comcentApi

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/rs/zerolog/log"
)

// Create a struct to hold the billing details
type BillingDetails struct {
	ComcentContextId               string  `json:"comcentContextId"`
	CallDuration                   float64 `json:"callDuration"`
	SttDuration                    float64 `json:"sttDuration"`
	InputTokens                    int     `json:"inputTokens"`
	OutputTokens                   int     `json:"outputTokens"`
	Characters                     int     `json:"characters"`
	OrgID                          string  `json:"orgId"`
	RealtimeInputTextTokens        int     `json:"realtimeInputTextTokens"`
	RealtimeInputAudioTokens       int     `json:"realtimeInputAudioTokens"`
	RealtimeCachedInputTextTokens  int     `json:"realtimeCachedInputTextTokens"`
	RealtimeCachedInputAudioTokens int     `json:"realtimeCachedInputAudioTokens"`
	RealtimeOutputTextTokens       int     `json:"realtimeOutputTextTokens"`
	RealtimeOutputAudioTokens      int     `json:"realtimeOutputAudioTokens"`
}

// McpServer represents an MCP server configuration with URL and token
type McpServer struct {
	URL   string `json:"url"`
	Token string `json:"token"`
}

// VoiceBot structure to match the expected response
type VoiceBot struct {
	ID                   string      `json:"id"`
	OrgID                string      `json:"orgId"`
	Name                 string      `json:"name"`
	AssistantID          string      `json:"assistantId"`
	Instructions         string      `json:"instructions"`
	NotToDoInstructions  string      `json:"notToDoInstructions"`
	GreetingInstructions string      `json:"greetingInstructions"`
	APIKey               string      `json:"apiKey"`
	IsHangup             bool        `json:"isHangup"`
	IsEnqueue            bool        `json:"isEnqueue"`
	Queues               []string    `json:"queues"`
	Pipeline             string      `json:"pipeline"`
	McpServers           []McpServer `json:"mcpServers"`
	Org                  struct {
		Subdomain string `json:"subdomain"`
	} `json:"org"`
}

func GetBotDetails(id string) (*VoiceBot, error) {
	// Get environment variables
	url := os.Getenv("INTERNAL_API_BASE_URL")
	username := os.Getenv("INTERNAL_API_USERNAME")
	password := os.Getenv("INTERNAL_API_PASSWORD")

	if url == "" || username == "" || password == "" {
		log.Error().Msg("Environment variables not set")
		return nil, errors.New("environment variables not set")
	}

	// Construct the request URL
	requestURL := fmt.Sprintf("%s/voice-bot/%s", url, id)

	// Create a new request
	req, err := http.NewRequest("GET", requestURL, nil)
	if err != nil {
		log.Error().Err(err).Msg("Error creating request:")
		return nil, err
	}

	// Set Basic Auth
	req.SetBasicAuth(username, password)

	// Create a client and send the request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Error().Err(err).Msg("Error making request:")
		return nil, err
	}
	defer resp.Body.Close()

	// Check if the request was successful
	if resp.StatusCode != http.StatusOK {
		errorMessage := fmt.Sprintf("Request failed with status: %d", resp.StatusCode)
		log.Error().Err(err).Msg(errorMessage)
		return nil, errors.New(errorMessage)
	}

	// Read the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Error().Err(err).Msg("Error reading response body:")
		return nil, err
	}

	// Parse the response
	var voiceBot VoiceBot
	if err := json.Unmarshal(body, &voiceBot); err != nil {
		log.Error().Err(err).Msg("Error unmarshalling response:")
		return nil, err
	}

	return &voiceBot, nil
}

func PostBillingDetails(comcentContextId string, callDuration float64, sttDuration float64, inputTokens int, outputTokens int, characters int, orgId string, realtimeInputTextTokens int, realtimeInputAudioTokens int, realtimeCachedInputTextTokens int, realtimeCachedInputAudioTokens int, realtimeOutputTextTokens int, realtimeOutputAudioTokens int) {
	// Get environment variables
	baseUrl := os.Getenv("INTERNAL_API_BASE_URL")
	username := os.Getenv("INTERNAL_API_USERNAME")
	password := os.Getenv("INTERNAL_API_PASSWORD")

	if baseUrl == "" || username == "" || password == "" {
		log.Error().Msg("Environment variables not set")
		return
	}

	// Create an instance of the struct and populate it with the parameters
	billingDetails := BillingDetails{
		ComcentContextId:               comcentContextId,
		SttDuration:                    sttDuration,
		CallDuration:                   callDuration,
		InputTokens:                    inputTokens,
		OutputTokens:                   outputTokens,
		Characters:                     characters,
		OrgID:                          orgId,
		RealtimeInputTextTokens:        realtimeInputTextTokens,
		RealtimeInputAudioTokens:       realtimeInputAudioTokens,
		RealtimeCachedInputTextTokens:  realtimeCachedInputTextTokens,
		RealtimeCachedInputAudioTokens: realtimeCachedInputAudioTokens,
		RealtimeOutputTextTokens:       realtimeOutputTextTokens,
		RealtimeOutputAudioTokens:      realtimeOutputAudioTokens,
	}

	// Convert the struct to JSON
	jsonData, err := json.Marshal(billingDetails)
	if err != nil {
		log.Error().Err(err).Msg("Error in JSON marshalling data")
		return
	}
	jsonStr := string(jsonData)

	// Construct the request URL
	req, err := http.NewRequest("POST", baseUrl+"/billing/voice-bot", bytes.NewBuffer([]byte(jsonStr)))
	if err != nil {
		log.Error().Err(err).Msg("Error creating request:")
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(username, password)

	client := &http.Client{}

	// Perform the request
	resp, err := client.Do(req)
	if err != nil {
		log.Error().Err(err).Msg("Error making request")
		return
	}
	if resp.StatusCode != http.StatusOK {
		log.Error().Msgf("Request failed with status: %d", resp.StatusCode)
		return
	} else {
		log.Info().Msg("Billing details sent successfully")
	}
}
