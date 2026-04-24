package speechToText

import (
	"context"
	"os"
	"strings"
	"time"

	"comcent-io/voice-bot/audioMessage"

	interfacesv1 "github.com/deepgram/deepgram-go-sdk/pkg/api/listen/v1/websocket/interfaces"
	"github.com/deepgram/deepgram-go-sdk/pkg/client/interfaces"
	dgClient "github.com/deepgram/deepgram-go-sdk/pkg/client/listen"
	"github.com/rs/zerolog/log"
)

type SpeechToText struct {
	InChannel  chan audioMessage.AudioMessage
	OutChannel chan audioMessage.AudioMessage
}

func (s *SpeechToText) run() {
	clientOptions := interfaces.ClientOptions{
		EnableKeepAlive: true, // Enable KeepAlive option
	}
	transcriptOptions := interfaces.LiveTranscriptionOptions{
		Language:    "en-US",
		Model:       "nova-2",
		SmartFormat: true,
		Channels:    1,
		SampleRate:  8000,
		Encoding:    "mulaw",
		VadEvents:   true,
		// To get UtteranceEnd, the following must be set:
		InterimResults: true,
		UtteranceEndMs: "1000",
		// End of UtteranceEnd settings
	}
	callBack := NewVoiceBotDgCallback(s)
	deepgramApiKey := os.Getenv("DEEPGRAM_API_KEY")
	dgWebsocket, err := dgClient.NewWebSocketUsingCallback(context.Background(), deepgramApiKey, &clientOptions, &transcriptOptions, callBack)
	if err != nil {
		log.Error().Msg(err.Error())
		log.Error().Err(err).Msg("Fail to create Deepgram client")
		return
	}
	dgConnected := dgWebsocket.Connect()
	if !dgConnected {
		log.Error().Msg("Fail to connect to Deepgram")
		return
	}
	defer dgWebsocket.Stop()

	for input := range s.InChannel {
		switch input.Type {
		case "TRANSCRIPT":
			select {
			case s.OutChannel <- input:
			default:
				log.Error().Msg("No reader available for the transcript message")
			}
		case "AUDIO":
			dgWebsocket.WriteBinary(input.Audio)
		default:
			log.Error().Msgf("Unknown AudioMessage type: %s", input.Type)
		}
	}
	// Input channel closed, wait a moment for any ongoing processing to complete
	log.Info().Msg("STT: Input channel closed, waiting for processing to complete")
	time.Sleep(100 * time.Millisecond)
	// Close output channel to propagate close signal
	log.Info().Msg("STT: Closing output channel")
	close(s.OutChannel)
	log.Info().Msg("STT: Output channel closed")
}

func (s *SpeechToText) Pipe(inputChannel chan audioMessage.AudioMessage) {
	go func() {
		defer close(inputChannel)
		for message := range s.OutChannel {
			select {
			case inputChannel <- message:
			default:
				log.Info().Msg("No reader available for the speech to text result pipe")
			}
		}
	}()
}

func NewSpeechToText() *SpeechToText {
	stt := &SpeechToText{
		InChannel:  make(chan audioMessage.AudioMessage, 10),
		OutChannel: make(chan audioMessage.AudioMessage, 10),
	}
	go stt.run()
	return stt
}

type VoiceBotDgCallback struct {
	StringBuilder strings.Builder
	stt           *SpeechToText
}

func NewVoiceBotDgCallback(stt *SpeechToText) *VoiceBotDgCallback {
	return &VoiceBotDgCallback{
		StringBuilder: strings.Builder{},
		stt:           stt,
	}
}

func (c *VoiceBotDgCallback) Open(or *interfacesv1.OpenResponse) error {
	//log.Info().Msg("Open event received")
	return nil
}

func (c *VoiceBotDgCallback) Message(mr *interfacesv1.MessageResponse) error {
	// Early return if no alternatives
	if len(mr.Channel.Alternatives) == 0 {
		if mr.IsFinal {
			// Send STT_BILLING message
			billingMessage := audioMessage.AudioMessage{
				Type:            "STT_BILLING",
				Text:            "",
				Audio:           nil,
				Command:         "",
				SttDuration:     mr.Duration,
				GptInputTokens:  0,
				GptOutputTokens: 0,
				TtsCharacters:   0,
			}
			select {
			case c.stt.OutChannel <- billingMessage:
			default:
				log.Debug().Msg("STT: OutChannel is full or closed, skipping billing message")
			}
		}
		return nil
	}

	// Get and validate transcript
	sentence := strings.TrimSpace(mr.Channel.Alternatives[0].Transcript)
	if len(sentence) == 0 {
		if mr.IsFinal {
			// Send STT_BILLING message
			billingMessage := audioMessage.AudioMessage{
				Type:            "STT_BILLING",
				Text:            "",
				Audio:           nil,
				Command:         "",
				SttDuration:     mr.Duration,
				GptInputTokens:  0,
				GptOutputTokens: 0,
				TtsCharacters:   0,
			}
			select {
			case c.stt.OutChannel <- billingMessage:
			default:
				log.Debug().Msg("STT: OutChannel is full or closed, skipping billing message")
			}
		}
		return nil
	}

	// Handle final messages
	if mr.IsFinal {
		// Send STT_BILLING message
		billingMessage := audioMessage.AudioMessage{
			Type:            "STT_BILLING",
			Text:            "",
			Audio:           nil,
			Command:         "",
			SttDuration:     mr.Duration,
			GptInputTokens:  0,
			GptOutputTokens: 0,
			TtsCharacters:   0,
		}
		select {
		case c.stt.OutChannel <- billingMessage:
		default:
			log.Debug().Msg("STT: OutChannel is full or closed, skipping billing message")
		}
		c.StringBuilder.WriteString(sentence + " ")

		if mr.SpeechFinal {
			log.Info().Msgf("[------- Is Final]: %s", c.StringBuilder.String())
		}
	}

	return nil
}

func (c *VoiceBotDgCallback) Metadata(md *interfacesv1.MetadataResponse) error {
	//log.Info().Msgf("[Metadata] Received")
	//log.Info().Msgf("Metadata.RequestID: %s", strings.TrimSpace(md.RequestID))
	//log.Info().Msgf("Metadata.Channels: %d", md.Channels)
	//log.Info().Msgf("Metadata.Created: %s", strings.TrimSpace(md.Created))
	return nil
}

func (c *VoiceBotDgCallback) SpeechStarted(ssr *interfacesv1.SpeechStartedResponse) error {
	//log.Info().Msgf("[SpeechStarted] Received")
	return nil
}

func (c *VoiceBotDgCallback) UtteranceEnd(ur *interfacesv1.UtteranceEndResponse) error {
	utterance := strings.TrimSpace(c.StringBuilder.String())
	if len(utterance) > 0 {
		log.Info().Msgf("[------- UtteranceEnd]: %s", utterance)
		// Create AudioMessage with transcript in text field
		message := audioMessage.AudioMessage{
			Type:            "TRANSCRIPT",
			Text:            c.StringBuilder.String(),
			Audio:           nil,
			Command:         "",
			SttDuration:     0.0,
			GptInputTokens:  0,
			GptOutputTokens: 0,
			TtsCharacters:   0,
		}
		select {
		case c.stt.OutChannel <- message:
		default:
			log.Error().Msg("No reader available for the deepgram result")
		}
		c.StringBuilder.Reset()
	} else {
		log.Info().Msgf("[UtteranceEnd] Received")
	}

	return nil
}

func (c *VoiceBotDgCallback) Close(cr *interfacesv1.CloseResponse) error {
	//log.Info().Msgf("[Close] Received")
	return nil
}

func (c *VoiceBotDgCallback) Error(er *interfacesv1.ErrorResponse) error {
	log.Error().Msgf("[Error] Received")
	log.Error().Msgf("Error.Type: %s", er.Type)
	log.Error().Msgf("Error.ErrCode: %s", er.ErrCode)
	log.Error().Msgf("Error.Description: %s", er.Description)
	return nil
}

func (c *VoiceBotDgCallback) UnhandledEvent(byData []byte) error {
	// handle the unhandled event
	log.Info().Msgf("[UnhandledEvent] Received")
	log.Info().Msgf("UnhandledEvent: %s", string(byData))
	return nil
}