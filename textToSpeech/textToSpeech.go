package textToSpeech

import (
	"comcent-io/voice-bot/audioMessage"
	"context"
	"os"
	"time"

	speak "github.com/deepgram/deepgram-go-sdk/pkg/api/speak/v1"
	"github.com/deepgram/deepgram-go-sdk/pkg/client/interfaces"
	dgSpeakClient "github.com/deepgram/deepgram-go-sdk/pkg/client/speak"
	"github.com/rs/zerolog/log"
)

type TextToSpeech struct {
	InChannel  chan audioMessage.AudioMessage
	OutChannel chan audioMessage.AudioMessage
}

func NewTextToSpeech() *TextToSpeech {
	tts := &TextToSpeech{
		InChannel:  make(chan audioMessage.AudioMessage, 100),
		OutChannel: make(chan audioMessage.AudioMessage, 10),
	}
	go tts.run()
	return tts
}

func (s *TextToSpeech) run() {

	options := interfaces.SpeakOptions{
		Model:      "aura-asteria-en",
		Encoding:   "mulaw",
		Container:  "none",
		SampleRate: 8000,
	}
	clientOptions := interfaces.ClientOptions{
		EnableKeepAlive: true, // Enable KeepAlive option
	}

	apiKey := os.Getenv("DEEPGRAM_API_KEY")
	c := dgSpeakClient.NewREST(apiKey, &clientOptions)
	dg := speak.New(c)

	for message := range s.InChannel {
		log.Info().Msgf("TTS: received message %s, type: %s", message.Text, message.Type)

		switch message.Type {
		case "TRANSCRIPT":
			log.Info().Msg("Text to speech: received text " + message.Text)
			resp := interfaces.RawResponse{}
			res, err := dg.ToStream(context.Background(), message.Text, &options, &resp)
			if err != nil {
				log.Error().Msg(err.Error())
				continue
			}
			characters := res.Characters
			log.Info().Msg("Text to speech: received audio")

			// Send TTS_BILLING message
			billingMessage := audioMessage.AudioMessage{
				Type:            "TTS_BILLING",
				Text:            "",
				Audio:           nil,
				Command:         "",
				SttDuration:     0.0,
				GptInputTokens:  0,
				GptOutputTokens: 0,
				TtsCharacters:   characters,
			}
			select {
			case s.OutChannel <- billingMessage:
			default:
				log.Debug().Msg("TTS: OutChannel is full or closed, skipping billing message")
			}

			audioMessage := audioMessage.AudioMessage{
				Type:            "AUDIO",
				Text:            message.Text,
				Audio:           resp.Bytes(),
				Command:         message.Command,
				SttDuration:     0.0,
				GptInputTokens:  0,
				GptOutputTokens: 0,
				TtsCharacters:   0,
			}
			select {
			case s.OutChannel <- audioMessage:
			default:
				log.Error().Msg("TTS: Output channel is full or closed, cannot send audio message")
			}

		default:
			log.Info().Msgf("TTS: Sending message directly, type: %s", message.Type)
			select {
			case s.OutChannel <- message:
			default:
				log.Error().Msg("TTS: Output channel is full or closed, cannot send message")
			}
		}
	}
	// Input channel closed, wait a moment for any ongoing processing to complete
	log.Info().Msg("TTS: Input channel closed, waiting for processing to complete")
	time.Sleep(100 * time.Millisecond)
	// Close output channel to propagate close signal
	log.Info().Msg("TTS: Closing output channel")
	close(s.OutChannel)
}
