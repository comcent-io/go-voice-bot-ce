package deepgramVoiceAgent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"
)

// ServerEvents represent the various event types sent from the Deepgram server
type ServerEventType string

const (
	EventWelcome              ServerEventType = "Welcome"
	EventSettingsApplied      ServerEventType = "SettingsApplied"
	EventConversationText     ServerEventType = "ConversationText"
	EventUserStartedSpeaking  ServerEventType = "UserStartedSpeaking"
	EventAgentThinking        ServerEventType = "AgentThinking"
	EventAgentStartedSpeaking ServerEventType = "AgentStartedSpeaking"
	EventAgentAudioDone       ServerEventType = "AgentAudioDone"
	EventFunctionCallRequest  ServerEventType = "FunctionCallRequest"
)

// ServerEvent is the base structure for all events from the server
type ServerEvent struct {
	Type ServerEventType `json:"type"`
}

// ConversationTextEvent contains text from the conversation
type ConversationTextEvent struct {
	ServerEvent
	Role    string `json:"role"`
	Content string `json:"content"`
}

// DeepgramVoiceAgent manages a websocket connection to Deepgram's Voice Agent API
type DeepgramVoiceAgent struct {
	// Input and output channels for audio data
	InputChannel  chan *[]byte
	OutputChannel chan *[]byte

	// WebSocket connection
	conn       *websocket.Conn
	connMutex  sync.Mutex
	ctx        context.Context
	cancelFunc context.CancelFunc

	// Connection state
	isConnected bool
	apiKey      string
}

// NewDeepgramVoiceAgent creates a new voice agent that connects to Deepgram
func NewDeepgramVoiceAgent(ctx context.Context) (*DeepgramVoiceAgent, error) {
	agentCtx, cancelFunc := context.WithCancel(ctx)

	agent := &DeepgramVoiceAgent{
		InputChannel:  make(chan *[]byte, 100),
		OutputChannel: make(chan *[]byte, 100),
		ctx:           agentCtx,
		cancelFunc:    cancelFunc,
		apiKey:        os.Getenv("DEEPGRAM_API_KEY"),
		isConnected:   false,
	}

	// Connect to Deepgram
	err := agent.connect()
	if err != nil {
		cancelFunc()
		return nil, fmt.Errorf("failed to connect to Deepgram: %w", err)
	}

	// Start processing routines
	go agent.processIncomingMessages()
	go agent.sendAudioLoop()

	return agent, nil
}

// connect establishes a websocket connection to the Deepgram Voice Agent API
func (dg *DeepgramVoiceAgent) connect() error {
	dg.connMutex.Lock()
	defer dg.connMutex.Unlock()

	if dg.isConnected {
		return nil
	}

	// Prepare headers with authorization
	header := http.Header{}
	header.Add("Authorization", "Token "+dg.apiKey)

	// Connect to the Deepgram Voice Agent API
	log.Info().Msg("Connecting to Deepgram Voice Agent API")
	conn, _, err := websocket.DefaultDialer.Dial("wss://agent.deepgram.com/agent", header)
	if err != nil {
		return fmt.Errorf("websocket dial error: %w", err)
	}

	dg.conn = conn
	dg.isConnected = true
	log.Info().Msg("Connected to Deepgram Voice Agent API")

	// Send initial configuration
	err = dg.sendSettingsConfiguration()
	if err != nil {
		dg.conn.Close()
		dg.isConnected = false
		return fmt.Errorf("failed to send initial configuration: %w", err)
	}

	return nil
}

// sendSettingsConfiguration sends the voice agent configuration
func (dg *DeepgramVoiceAgent) sendSettingsConfiguration() error {
	log.Info().Msg("Sending voice agent configuration")

	// Create the configuration JSON
	configJSON := `{
		"type": "SettingsConfiguration",
		"audio": {
			"input": {
				"encoding": "mulaw",
				"sample_rate": 8000
			},
			"output": {
				"encoding": "mulaw",
				"sample_rate": 8000,
				"container": "none"
			}
		},
		"agent": {
			"listen": {
				"model": "nova-3"
			},
			"speak": {
				"provider": "cartesia",
				"voice_id": "726d5ae5-055f-4c3d-8355-d9677de68937"
			},
			"think": {
				"model": "gpt-4o-mini",
				"provider": {
					"type": "open_ai"
				},
				"instructions": "You are helpful assistant who will have conversation like human and talk about various topics"
			}
		},
		"context": {
			"messages": [
				{
					"content": "Hello, how can I help you?",
					"role": "assistant"
				}
			],
			"replay": true
		}
	}`

	return dg.conn.WriteMessage(websocket.TextMessage, []byte(configJSON))
}

// processIncomingMessages handles messages received from the websocket
func (dg *DeepgramVoiceAgent) processIncomingMessages() {
	defer func() {
		if r := recover(); r != nil {
			log.Error().Interface("recover", r).Msg("Recovered from panic in processIncomingMessages")
			go dg.reconnect()
		}
	}()

	for {
		select {
		case <-dg.ctx.Done():
			return
		default:
			// Check connection
			if !dg.isConnected {
				time.Sleep(1 * time.Second)
				err := dg.reconnect()
				if err != nil {
					log.Error().Err(err).Msg("Failed to reconnect")
				}
				continue
			}

			// Read message
			messageType, message, err := dg.conn.ReadMessage()
			if err != nil {
				log.Error().Err(err).Msg("Error reading from websocket")
				// Don't try to reconnect immediately to avoid connection thrashing
				time.Sleep(1 * time.Second)
				go dg.reconnect()
				break
			}

			// Process based on message type
			switch messageType {
			case websocket.BinaryMessage:
				// Audio data - send to output channel
				audioCopy := make([]byte, len(message))
				copy(audioCopy, message)
				audioData := &audioCopy

				select {
				case dg.OutputChannel <- audioData:
					// Successfully sent the audio data
				default:
					log.Warn().Msg("Output channel full, discarding audio data")
				}

			case websocket.TextMessage:
				// Parse the event type
				var baseEvent ServerEvent
				if err := json.Unmarshal(message, &baseEvent); err != nil {
					log.Error().Err(err).Msg("Failed to parse event")
					continue
				}

				log.Info().Str("event", string(baseEvent.Type)).Msg("Received event " + string(message))

				// Handle based on event type
				switch baseEvent.Type {
				case EventConversationText:
					var event ConversationTextEvent
					if err := json.Unmarshal(message, &event); err != nil {
						log.Error().Err(err).Msg("Failed to parse conversation text")
						continue
					}
					log.Info().Str("role", event.Role).Str("content", event.Content).Msg("Conversation")

				case EventUserStartedSpeaking:
					log.Info().Msg("User started speaking")

				case EventAgentStartedSpeaking:
					log.Info().Msg("Agent started speaking")

				case EventAgentAudioDone:
					log.Info().Msg("Agent audio done")

				case EventWelcome:
					log.Info().Msg("Received welcome message")

				case EventSettingsApplied:
					log.Info().Msg("Settings applied successfully")

				case EventAgentThinking:
					log.Info().Msg("Agent is thinking")

				case EventFunctionCallRequest:
					log.Info().Msg("Function call request received")
				}
			}
		}
	}
}

// sendAudioLoop forwards audio from the input channel to the websocket
func (dg *DeepgramVoiceAgent) sendAudioLoop() {
	for {
		select {
		case <-dg.ctx.Done():
			return
		case audioData := <-dg.InputChannel:
			if !dg.isConnected {
				log.Warn().Msg("Not connected, dropping audio data")
				continue
			}

			if audioData == nil || len(*audioData) == 0 {
				continue
			}

			err := dg.conn.WriteMessage(websocket.BinaryMessage, *audioData)
			if err != nil {
				log.Error().Err(err).Msg("Failed to send audio data")
				dg.reconnect()
			}
		}
	}
}

// reconnect attempts to reestablish the connection to Deepgram
func (dg *DeepgramVoiceAgent) reconnect() error {
	dg.connMutex.Lock()
	if dg.conn != nil {
		dg.conn.Close()
		dg.isConnected = false
	}
	dg.connMutex.Unlock()

	// Try to reconnect a few times
	for attempt := 1; attempt <= 3; attempt++ {
		log.Info().Int("attempt", attempt).Msg("Reconnecting to Deepgram")

		err := dg.connect()
		if err == nil {
			// Add a small delay after successful connection before resuming
			time.Sleep(500 * time.Millisecond)
			return nil
		}

		log.Error().Err(err).Int("attempt", attempt).Msg("Reconnection failed")
		time.Sleep(time.Duration(attempt) * time.Second)
	}

	return fmt.Errorf("failed to reconnect after multiple attempts")
}

// Stop gracefully closes the connection and stops all goroutines
func (dg *DeepgramVoiceAgent) Stop() {
	dg.connMutex.Lock()
	defer dg.connMutex.Unlock()

	// Cancel context to stop goroutines
	dg.cancelFunc()

	// Close connection
	if dg.conn != nil {
		dg.conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
		dg.conn.Close()
		dg.isConnected = false
	}

	log.Info().Msg("DeepgramVoiceAgent stopped")
}
