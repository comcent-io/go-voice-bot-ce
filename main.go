package main

import (
	"comcent-io/voice-bot/audio"
	"comcent-io/voice-bot/audioMessage"
	"comcent-io/voice-bot/chatgpt"
	"comcent-io/voice-bot/comcentApi"
	mcpclient "comcent-io/voice-bot/mcpClient"
	"comcent-io/voice-bot/serviceRegistration"
	"comcent-io/voice-bot/speechToText"
	"comcent-io/voice-bot/textToSpeech"

	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"sync"
	"time"

	dgClient "github.com/deepgram/deepgram-go-sdk/pkg/client/listen"
	"github.com/emiago/diago"
	"github.com/emiago/diago/media"
	"github.com/emiago/sipgo"
	"github.com/emiago/sipgo/sip"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// computeBillingDetails calculates and logs billing information for all services
func computeBillingDetails(sttDuration float64, inputTokens int, outputTokens int, ttsCharacters int, startTime time.Time, comcentContextId string, orgID string) {
	endTime := time.Now()
	callDuration := endTime.Sub(startTime).Seconds()
	log.Info().Msgf("Total time taken to convert Speech to Text by Deepgram: %.2f minutes", sttDuration/60.0)
	log.Info().Msgf("Total number of input tokens given to OpenAI Assistant: %d", inputTokens)
	log.Info().Msgf("Total number of output tokens processed by OpenAI Assistant: %d", outputTokens)
	log.Info().Msgf("Total number of characters converted from Text to Speech: %d", ttsCharacters)
	log.Info().Msgf("Total call duration: %.2f minutes", callDuration/60.0)

	// Post billing details to API
	comcentApi.PostBillingDetails(comcentContextId, callDuration, sttDuration, inputTokens, outputTokens, ttsCharacters, orgID, 0, 0, 0, 0, 0, 0)
}

func computeRealtimeBillingDetails(realtimeInputTextTokens int, realtimeInputAudioTokens int, realtimeCachedInputTextTokens int, realtimeCachedInputAudioTokens int, realtimeOutputTextTokens int, realtimeOutputAudioTokens int, startTime time.Time, comcentContextId string, orgID string) {
	endTime := time.Now()
	callDuration := endTime.Sub(startTime).Seconds()
	log.Info().Msgf("Total number of input text tokens given to OpenAI Realtime API: %d", realtimeInputTextTokens)
	log.Info().Msgf("Total number of input audio tokens given to OpenAI Realtime API: %d", realtimeInputAudioTokens)
	log.Info().Msgf("Total number of cached input text tokens given to OpenAI Realtime API: %d", realtimeCachedInputTextTokens)
	log.Info().Msgf("Total number of cached input audio tokens given to OpenAI Realtime API: %d", realtimeCachedInputAudioTokens)
	log.Info().Msgf("Total number of output text tokens processed by OpenAI Realtime API: %d", realtimeOutputTextTokens)
	log.Info().Msgf("Total number of output audio tokens processed by OpenAI Realtime API: %d", realtimeOutputAudioTokens)
	log.Info().Msgf("Total call duration: %.2f minutes", callDuration/60.0)

	// Post billing details to API
	comcentApi.PostBillingDetails(comcentContextId, callDuration, 0.0, 0, 0, 0, orgID, realtimeInputTextTokens, realtimeInputAudioTokens, realtimeCachedInputTextTokens, realtimeCachedInputAudioTokens, realtimeOutputTextTokens, realtimeOutputAudioTokens)
}

func main() {
	setUpLogger()
	go serviceRegistration.StartServiceDiscoverySignal()

	// Get local IP for default values
	localIp, err := serviceRegistration.GetLocalIP()
	if err != nil {
		log.Error().Err(err).Msg("Failed to get local IP")
		return
	}
	// Get advertised IP from environment or use local IP
	advertisedIp := os.Getenv("ADVERTISE_IP")
	if advertisedIp == "" {
		advertisedIp = localIp
	}

	port := flag.Int("p", getEnvInt("PORT", 5080), "My listen port")
	username := flag.String("u", getEnvString("USERNAME", "bot"), "SIP Username")
	flag.Parse()

	// Setup media ports
	media.RTPPortStart = getEnvInt("RTP_START_PORT", 16384)
	media.RTPPortEnd = getEnvInt("RTP_END_PORT", 32768)

	log.Info().Msgf("Server Started at: %s:%d", localIp, *port)
	log.Info().Msgf("Username: %s", *username)
	log.Info().Msgf("RTP port range: %d-%d", media.RTPPortStart, media.RTPPortEnd)
	log.Info().Msgf("External IP for media: %s", advertisedIp)

	// deepgram client
	dgClient.InitWithDefault()

	// Setup Diago Server
	ua, _ := sipgo.NewUA(sipgo.WithUserAgent(*username))
	dg := diago.NewDiago(ua, diago.WithTransport(diago.Transport{
		Transport:       "udp",
		BindHost:        "0.0.0.0",
		BindPort:        *port,
		ExternalHost:    advertisedIp,
		MediaExternalIP: net.ParseIP(advertisedIp),
	}))

	ctx := context.Background()
	// Serve incoming SIP INVITE requests
	dg.Serve(ctx, func(inDialog *diago.DialogServerSession) {
		startTime := time.Now()
		log.Info().Msg("INVITE received")
		// extract a custom context ID from SIP header
		comcentHeader := inDialog.InviteRequest.GetHeader("X-Comcent-Context-Id")
		comcentContextId := ""
		if comcentHeader != nil {
			comcentContextId = comcentHeader.Value()
		}
		if comcentContextId == "" {
			comcentContextId = uuid.NewString()
		}
		// Respond with SIP 'Ringing' status
		err := inDialog.Ringing()
		if err != nil {
			log.Error().Err(err).Msg("Fail to respond with status: Ringing")
		}

		// Bot config
		toUser := inDialog.InviteRequest.To().Address.User
		voiceBot, err := comcentApi.GetBotDetails(toUser)
		if err != nil {
			log.Error().Err(err).Msg("Fail to get voice bot details")
			// Note: diago doesn't have a direct equivalent for StatusBadRequest
			// We'll need to keep using sipgo for this specific case
			err := inDialog.Respond(sip.StatusBadRequest, "Couldn't find the voice bot", nil)
			if err != nil {
				log.Error().Err(err).Msg("Fail to respond with error: Fail to get voice bot details")
			}
			return
		}

		// Answer the call - this will automatically create and configure the media session
		err = inDialog.Answer()
		if err != nil {
			log.Error().Err(err).Msg("Fail to answer the call")
			return
		}
		log.Info().Msg("Call answered and media session created")

		sampleSize := 160
		// Convert comcentApi.McpServer to mcpclient.McpServer
		mcpServers := make([]mcpclient.McpServer, len(voiceBot.McpServers))
		for i, server := range voiceBot.McpServers {
			mcpServers[i] = mcpclient.McpServer{
				URL:   server.URL,
				Token: server.Token,
			}
		}
		mcpClientManager := mcpclient.NewMcpClient(ctx, mcpServers)
		pipelineName := voiceBot.Pipeline
		inChannel, outChannel := getPipeLine(pipelineName, voiceBot, mcpClientManager)

		firstResponseReceived := false
		// play ringing audio
		ringPlayer := audio.NewAudioPlayer(&audio.PhoneRingingAudio, sampleSize)

		// Send initial greeting message through STT pipeline
		greetingMessage := audioMessage.AudioMessage{
			Type:            "TRANSCRIPT",
			Text:            "Hello",
			Audio:           nil,
			Command:         "",
			SttDuration:     0.0,
			GptInputTokens:  0,
			GptOutputTokens: 0,
			TtsCharacters:   0,
		}
		inChannel <- greetingMessage

		botOutputAudio := make([]byte, 0)
		botOutputAudioLock := sync.Mutex{}
		context := inDialog.Context()
		isChannelsClosed := false

		// Billing details accumulator
		var sttDuration float64
		var inputTokens, outputTokens, ttsCharacters int

		// Realtime billing details accumulator
		var realtimeInputTextTokens, realtimeInputAudioTokens, realtimeCachedInputTextTokens, realtimeCachedInputAudioTokens, realtimeOutputTextTokens, realtimeOutputAudioTokens int

		// Separate goroutine to handle TTS output and initiate close loop
		go func() {
			for message := range outChannel {
				switch message.Type {
				case "REALTIME_BILLING":
					realtimeInputTextTokens += message.RealtimeBillingDetails.InputTextTokens
					realtimeInputAudioTokens += message.RealtimeBillingDetails.InputAudioTokens
					realtimeCachedInputTextTokens += message.RealtimeBillingDetails.CachedInputTextTokens
					realtimeCachedInputAudioTokens += message.RealtimeBillingDetails.CachedInputAudioTokens
					realtimeOutputTextTokens += message.RealtimeBillingDetails.OutputTextTokens
					realtimeOutputAudioTokens += message.RealtimeBillingDetails.OutputAudioTokens
				case "STT_BILLING":
					sttDuration += message.SttDuration
				case "GPT_BILLING":
					inputTokens += message.GptInputTokens
					outputTokens += message.GptOutputTokens
				case "TTS_BILLING":
					ttsCharacters += message.TtsCharacters
				case "COMMAND":
					switch message.Command {
					case "HANGUP":
						log.Info().Msg("Hangup command received")
						err := inDialog.Respond(sip.StatusTemporarilyUnavailable, "Temporarly unavailable", nil)

						if err != nil {
							log.Error().Err(err).Msg("Fail to hangup")
						} else {
							log.Info().Msg("Call hung up successfully")
						}
						if !isChannelsClosed {
							close(inChannel)
							isChannelsClosed = true
						}
					case "ENQUEUE":
						log.Info().Msg("Enqueue command received")
						queueName := message.Text
						subdomain := voiceBot.Org.Subdomain
						sipDomain := getEnvString("SIP_DOMAIN", "")
						if sipDomain == "" {
							log.Error().Msg("SIP_DOMAIN env var not set")
							return
						}
						referTo := fmt.Sprintf("sip:%s@%s.%s", queueName, subdomain, sipDomain)
						fmt.Println("referTo: ", referTo)
						var referUri sip.Uri
						err := sip.ParseUri(referTo, &referUri)
						if err != nil {
							log.Error().Err(err).Msg("Error parsing refer-to URI")
							return
						}
						err = inDialog.Refer(context, referUri)
						if err != nil {
							log.Error().Err(err).Msg("Error in sending refer request")
							return
						}
						log.Info().Msg("Successfully referred")
					}
				default:
					firstResponseReceived = true
					log.Info().Msgf("Received audio from TTS, type: %s", message.Type)
					botOutputAudioLock.Lock()
					botOutputAudio = append(botOutputAudio, message.Audio...)
					botOutputAudioLock.Unlock()
				}
			}
			log.Info().Msg("output channel closed")
			switch pipelineName {
			case "DEEPGRAM_AND_OPENAI":
				computeBillingDetails(sttDuration, inputTokens, outputTokens, ttsCharacters, startTime, comcentContextId, voiceBot.OrgID)
			case "REALTIME_API":
				computeRealtimeBillingDetails(realtimeInputTextTokens, realtimeInputAudioTokens, realtimeCachedInputTextTokens, realtimeCachedInputAudioTokens, realtimeOutputTextTokens, realtimeOutputAudioTokens, startTime, comcentContextId, voiceBot.OrgID)
			default:
				log.Error().Msgf("Invalid pipeline name: %s", pipelineName)
			}
		}()

		// Start reading RTP packets from media session
		buf := make([]byte, media.RTPBufSize)
		for {
			select {
			case <-context.Done():
				log.Info().Msg("Main context canceled")
				if !isChannelsClosed {
					close(inChannel)
					isChannelsClosed = true
				}
				return
			default:
				// Read audio data from RTP packet reader
				n, err := inDialog.RTPPacketReader.Read(buf)
				if err != nil {
					if errors.Is(err, io.ErrClosedPipe) {
						return
					}
					log.Error().Err(err).Msg("Fail to read RTP Probably Hangup")
					inDialog.Respond(sip.StatusTemporarilyUnavailable, "Temporarly unavailable", nil)
					if !isChannelsClosed {
						close(inChannel)
						isChannelsClosed = true
					}
					return
				}

				// Only process if we actually read data
				if n == 0 {
					continue
				}

				// Extract payload from the read data
				payload := buf[:n]

				var audioToSend []byte
				if !firstResponseReceived {
					// Play ringing tone until assistant responds
					audioToSend = ringPlayer.GetNextBytes()
				} else {
					botOutputAudioLock.Lock()
					botAudioLen := len(botOutputAudio)
					if botAudioLen > 0 {
						endIndex := min(botAudioLen, sampleSize)
						audioToSend = botOutputAudio[:endIndex]
						botOutputAudio = botOutputAudio[endIndex:]
					}
					botOutputAudioLock.Unlock()
				}

				if len(audioToSend) > 0 {
					if len(audioToSend) < sampleSize {
						// Use proper silence values for μ-law encoding (127 or 255)
						// This reduces noise when padding with silence
						silenceValue := byte(255) // μ-law silence value
						padding := make([]byte, sampleSize-len(audioToSend))
						for i := range padding {
							padding[i] = silenceValue
						}
						audioToSend = append(audioToSend, padding...)
					}
					_, err := inDialog.RTPPacketWriter.Write(audioToSend)
					if err != nil {
						log.Error().Err(err).Msg("Failed to send the media")
					}
				}

				if firstResponseReceived && !isChannelsClosed {
					// Create AudioMessage for incoming audio
					audioMessage := audioMessage.AudioMessage{
						Type:            "AUDIO",
						Text:            "",
						Audio:           payload,
						Command:         "",
						SttDuration:     0.0,
						GptInputTokens:  0,
						GptOutputTokens: 0,
						TtsCharacters:   0,
					}
					select {
					case inChannel <- audioMessage:
					default:
						log.Debug().Msg("STT channel is full or closed, skipping audio message")
					}
				}
			}
		}
	})
}

// Environment helper functions
func getEnvString(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}

func getEnvInt(key string, defaultValue int) int {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	intValue, err := strconv.Atoi(value)
	if err != nil {
		return defaultValue
	}
	return intValue
}

func getPipeLine(name string, voiceBot *comcentApi.VoiceBot, mcpClientManager *mcpclient.McpClientManager) (chan audioMessage.AudioMessage, chan audioMessage.AudioMessage) {
	switch name {
	case "DEEPGRAM_AND_OPENAI":
		stt := speechToText.NewSpeechToText()
		chatAssistant := chatgpt.NewChatGPT(voiceBot, mcpClientManager)
		tts := textToSpeech.NewTextToSpeech()
		stt.Pipe(chatAssistant.InChannel)
		chatAssistant.Pipe(tts.InChannel)
		return stt.InChannel, tts.OutChannel

	case "REALTIME_API":
		realTimeApi := chatgpt.NewRealTimeApi(voiceBot, mcpClientManager)
		return realTimeApi.InChannel, realTimeApi.OutChannel

	default:
		log.Error().Msgf("Invalid pipeline name: %s", name)
		return nil, nil
	}
}

// UASRequestBuild function removed - replaced with diago's Refer() method

func setUpLogger() {
	lev, err := zerolog.ParseLevel(os.Getenv("LOG_LEVEL"))
	if err != nil || lev == zerolog.NoLevel {
		lev = zerolog.InfoLevel
	}

	zerolog.TimeFieldFormat = zerolog.TimeFormatUnixMicro

	env := os.Getenv("ENV")
	if env == "" {
		env = "dev"
	}

	if env == "dev" {
		log.Logger = zerolog.New(zerolog.ConsoleWriter{
			Out:        os.Stdout,
			TimeFormat: time.StampMicro,
		}).With().Timestamp().Str("env", env).Logger().Level(lev)
	} else {
		log.Logger = zerolog.New(os.Stdout).
			With().Timestamp().Str("env", env).Logger().Level(lev)
	}
}
