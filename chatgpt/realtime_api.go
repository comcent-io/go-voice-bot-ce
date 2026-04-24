package chatgpt

import (
	"comcent-io/voice-bot/audio"
	"comcent-io/voice-bot/audioMessage"
	"comcent-io/voice-bot/comcentApi"
	mcpclient "comcent-io/voice-bot/mcpClient"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	openairt "github.com/WqyJh/go-openai-realtime/v2"
	"github.com/rs/zerolog/log"
)

type RealTimeApi struct {
	InChannel              chan audioMessage.AudioMessage
	OutChannel             chan audioMessage.AudioMessage
	VoiceBot               *comcentApi.VoiceBot
	FormattedInstructions  string
	Conn                   *openairt.Conn
	Ctx                    context.Context
	ShouldHangUp           bool
	ShouldEnqueue          bool
	WaitingForHangupAudio  bool
	WaitingForEnqueueAudio bool
	QueueName              string
	PlayingInProgressAudio bool
	inProgressCtx          context.Context
	inProgressCancel       context.CancelFunc
	delayedStartCtx        context.Context
	delayedStartCancel     context.CancelFunc
	McpClientManager       *mcpclient.McpClientManager
}

func (r *RealTimeApi) run(ctx context.Context) {
	var err error
	r.Conn, err = r.connectRealtimeApi(ctx)
	if err != nil {
		log.Error().Err(err).Msg("Fail to connect to OpenAI Realtime API")
		return
	}
	defer r.Conn.Close()

	datas := make(chan []byte, 100)
	responseHandler := r.newResponseHandler(datas)

	handlers := []openairt.ServerEventHandler{
		responseHandler,
	}
	connHandler := openairt.NewConnHandler(ctx, r.Conn, handlers...)
	connHandler.Start()

	r.consumeInputs(ctx, r.Conn)
	time.Sleep(100 * time.Millisecond)
	close(r.OutChannel)
	log.Info().Msg("RealTimeApi: OutChannel closed")
}

func (r *RealTimeApi) connectRealtimeApi(ctx context.Context) (*openairt.Conn, error) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	client := openairt.NewClient(apiKey)

	conn, err := client.Connect(ctx)
	if err != nil {
		return nil, err
	}

	if err := r.configureRealtimeSession(ctx, conn); err != nil {
		conn.Close()
		return nil, fmt.Errorf("configure realtime session: %w", err)
	}

	return conn, nil
}

func (r *RealTimeApi) configureRealtimeSession(ctx context.Context, conn *openairt.Conn) error {
	tools, err := mcpclient.ListTools(r.Ctx, r.McpClientManager)
	if err != nil {
		log.Error().Err(err).Msg("Failed to list MCP tools, continuing without tools")
		tools = []map[string]any{} // Use empty slice on error
	}

	return conn.SendMessage(ctx, openairt.SessionUpdateEvent{
		Session: openairt.SessionUnion{
			Realtime: &openairt.RealtimeSession{
				OutputModalities: []openairt.Modality{openairt.ModalityAudio},
				Instructions:     r.FormattedInstructions,
				Audio: &openairt.RealtimeSessionAudio{
					Input: &openairt.SessionAudioInput{
						Format: &openairt.AudioFormatUnion{
							PCMU: &openairt.AudioFormatPCMU{},
						},
						TurnDetection: &openairt.TurnDetectionUnion{
							SemanticVad: &openairt.RealtimeSessionSemanticVad{
								CreateResponse:    true,
								InterruptResponse: false,
							},
						},
					},
					Output: &openairt.SessionAudioOutput{
						Voice: openairt.VoiceMarin,
						Format: &openairt.AudioFormatUnion{
							PCMU: &openairt.AudioFormatPCMU{},
						},
					},
				},
				Tools: FormatRealtimeTools(tools, r.VoiceBot),
				ToolChoice: &openairt.ToolChoiceUnion{
					Mode: openairt.ToolChoiceModeAuto,
				},
			},
		},
	})
}

func (r *RealTimeApi) newResponseHandler(datas chan []byte) func(ctx context.Context, event openairt.ServerEvent) {
	return func(ctx context.Context, event openairt.ServerEvent) {
		switch event.ServerEventType() {
		case openairt.ServerEventTypeResponseOutputAudioTranscriptDone:
			fmt.Printf("[response] %s\n", event.(openairt.ResponseOutputAudioTranscriptDoneEvent).Transcript)
		case openairt.ServerEventTypeResponseDone:
			response := event.(openairt.ResponseDoneEvent).Response
			if len(response.Output) > 0 && response.Output[0].FunctionCall != nil {
				// Cancel any pending delayed start
				if r.delayedStartCancel != nil {
					r.delayedStartCancel()
				}
				// Create new context for delayed start
				r.delayedStartCtx, r.delayedStartCancel = context.WithCancel(r.Ctx)
				// Start streaming in_progress audio after 2 seconds if actual audio hasn't arrived
				r.startInProgressAudioDelayed()
				calledFunction := response.Output[0].FunctionCall
				r.handleFunctionCallResponse(calledFunction)
			}

			billingMessage := audioMessage.AudioMessage{
				Type: "REALTIME_BILLING",
				RealtimeBillingDetails: audioMessage.RealtimeBillingDetails{
					InputTextTokens:        response.Usage.InputTokenDetails.TextTokens,
					InputAudioTokens:       response.Usage.InputTokenDetails.AudioTokens,
					CachedInputTextTokens:  response.Usage.InputTokenDetails.CachedTokensDetails.TextTokens,
					CachedInputAudioTokens: response.Usage.InputTokenDetails.CachedTokensDetails.AudioTokens,
					OutputTextTokens:       response.Usage.OutputTokenDetails.TextTokens,
					OutputAudioTokens:      response.Usage.OutputTokenDetails.AudioTokens,
				},
			}
			select {
			case r.OutChannel <- billingMessage:
			default:
				log.Debug().Msg("RealTimeApi: OutChannel is full or closed, skipping billing message")
			}

		case openairt.ServerEventTypeResponseOutputAudioDelta:
			msg := event.(openairt.ResponseOutputAudioDeltaEvent)
			// Cancel delayed start if it's still waiting
			if r.delayedStartCancel != nil {
				r.delayedStartCancel()
			}
			// Stop in_progress audio if it's playing
			r.stopInProgressAudio()
			r.handleAudioDelta(datas, msg)
		case openairt.ServerEventTypeResponseOutputAudioDone:
			datas = r.handleAudioDone(datas)
		}
	}
}

func (r *RealTimeApi) handleFunctionCallResponse(calledFunction *openairt.MessageItemFunctionCall) {
	switch calledFunction.Name {
	case "hangup":
		r.handleHangup(calledFunction)
	case "enqueue":
		r.handleEnqueue(calledFunction)
	case "get_current_datetime":
		r.handleGetCurrentDatetime(calledFunction)
	default:
		r.handleCustomFunction(calledFunction)
	}
}

func (r *RealTimeApi) handleHangup(calledFunction *openairt.MessageItemFunctionCall) {
	log.Info().Msg("Hangup detected")
	r.ShouldHangUp = true
	r.WaitingForHangupAudio = true
	output := `{"success": true}`
	r.sendFunctionCallOutput(calledFunction, output)
}

func (r *RealTimeApi) handleEnqueue(calledFunction *openairt.MessageItemFunctionCall) {
	log.Info().Msg("Enqueue detected")
	r.ShouldEnqueue = true
	r.WaitingForEnqueueAudio = true
	arguments := calledFunction.Arguments

	var args map[string]interface{}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		log.Error().Err(err).Msg("Failed to parse enqueue arguments")
		return
	}

	queue, ok := args["queue"].(string)
	if !ok {
		log.Error().Msg("Queue field not found or not a string in arguments")
		return
	}
	r.QueueName = queue

	output := `{"success": true}`
	r.sendFunctionCallOutput(calledFunction, output)
}

func (r *RealTimeApi) handleGetCurrentDatetime(calledFunction *openairt.MessageItemFunctionCall) {
	log.Info().Msg("Get current datetime detected")
	currentTime := time.Now().Format(time.RFC3339)
	output := fmt.Sprintf(`{"datetime": "%s"}`, currentTime)
	r.sendFunctionCallOutput(calledFunction, output)
}

func (r *RealTimeApi) handleCustomFunction(calledFunction *openairt.MessageItemFunctionCall) {
	log.Info().Msg("Making call to function: " + calledFunction.Name + " with arguments: " + calledFunction.Arguments)

	// Parse JSON string arguments into map
	var args map[string]interface{}
	if err := json.Unmarshal([]byte(calledFunction.Arguments), &args); err != nil {
		log.Error().Err(err).Msg("Failed to parse function arguments")
		return
	}

	output, err := mcpclient.CallTool(r.Ctx, r.McpClientManager, calledFunction.Name, args)
	if err != nil {
		log.Error().Err(err).Msg("Failed to call tool: " + err.Error())
		return
	}
	r.sendFunctionCallOutput(calledFunction, output)
}

func (r *RealTimeApi) sendFunctionCallOutput(calledFunction *openairt.MessageItemFunctionCall, output string) {
	if r.Conn != nil {
		err := r.Conn.SendMessage(r.Ctx, &openairt.ConversationItemCreateEvent{
			Item: openairt.MessageItemUnion{
				FunctionCallOutput: &openairt.MessageItemFunctionCallOutput{
					CallID: calledFunction.CallID,
					Output: output,
				},
			},
		})
		if err != nil {
			log.Error().Err(err).Msg("Failed to send function call output")
			return
		}
	}
	r.requestAudioResponse(r.Ctx, r.Conn)
}

func (r *RealTimeApi) newAudioMessage(msgType string, text string, audioData []byte, command string) audioMessage.AudioMessage {
	return audioMessage.AudioMessage{
		Type:            msgType,
		Text:            text,
		Audio:           audioData,
		Command:         command,
		SttDuration:     0.0,
		GptInputTokens:  0,
		GptOutputTokens: 0,
		TtsCharacters:   0,
	}
}

func (r *RealTimeApi) handleAudioDelta(datas chan []byte, msg openairt.ResponseOutputAudioDeltaEvent) {
	data, err := base64.StdEncoding.DecodeString(msg.Delta)
	if err != nil {
		log.Error().Err(err).Msg("failed to decode base64 audio delta")
		return
	}
	datas <- data
}

func (r *RealTimeApi) handleAudioDone(datas chan []byte) chan []byte {
	audioData := []byte{}
	close(datas)
	for d := range datas {
		audioData = append(audioData, d...)
	}

	audioMsg := r.newAudioMessage("AUDIO", "", audioData, "")

	select {
	case r.OutChannel <- audioMsg:
	default:
		log.Error().Msg("Output channel is full or closed, cannot send audio message")
	}

	// If we're waiting for hangup audio, send HANGUP command after audio is sent
	if r.WaitingForHangupAudio {
		r.WaitingForHangupAudio = false
		hangupMessage := r.newAudioMessage("COMMAND", "", nil, "HANGUP")
		select {
		case r.OutChannel <- hangupMessage:
			log.Info().Msg("HANGUP command sent after audio completion")
		default:
			log.Debug().Msg("RealTimeApi: OutChannel is full or closed, skipping hangup command")
		}
	}

	// If we're waiting for enqueue audio, send ENQUEUE command after audio is sent
	if r.WaitingForEnqueueAudio {
		r.WaitingForEnqueueAudio = false
		enqueueMessage := r.newAudioMessage("COMMAND", r.QueueName, nil, "ENQUEUE")
		select {
		case r.OutChannel <- enqueueMessage:
			log.Info().Msg("ENQUEUE command sent after audio completion")
		default:
			log.Debug().Msg("RealTimeApi: OutChannel is full or closed, skipping enqueue command")
		}
	}

	return make(chan []byte, 100)
}

func (r *RealTimeApi) consumeInputs(ctx context.Context, conn *openairt.Conn) {
	for input := range r.InChannel {
		if r.ShouldHangUp {
			continue
		}
		if err := r.dispatchInput(ctx, conn, input); err != nil {
			log.Error().Err(err).Msg("failed to process audio message")
		}
	}

}

func (r *RealTimeApi) dispatchInput(ctx context.Context, conn *openairt.Conn, input audioMessage.AudioMessage) error {
	switch input.Type {
	case "TRANSCRIPT":
		return r.sendTranscript(ctx, conn, input.Text)
	case "AUDIO":
		return r.sendAudio(ctx, conn, input.Audio)
	default:
		log.Error().Msgf("Unknown AudioMessage type: %s", input.Type)
		return fmt.Errorf("unknown audio message type: %s", input.Type)
	}
}

func (r *RealTimeApi) sendTranscript(ctx context.Context, conn *openairt.Conn, text string) error {
	err := conn.SendMessage(ctx, &openairt.ConversationItemCreateEvent{
		Item: openairt.MessageItemUnion{
			User: &openairt.MessageItemUser{
				ID:     openairt.GenerateID("msg_", 10),
				Status: openairt.ItemStatusCompleted,
				Content: []openairt.MessageContentInput{
					{
						Type: openairt.MessageContentTypeInputText,
						Text: text,
					},
				},
			},
		},
	})
	if err != nil {
		return err
	}

	return r.requestAudioResponse(ctx, conn)
}

func (r *RealTimeApi) sendAudio(ctx context.Context, conn *openairt.Conn, audio []byte) error {
	base64Audio := base64.StdEncoding.EncodeToString(audio)
	if err := conn.SendMessage(ctx, openairt.InputAudioBufferAppendEvent{
		Audio: base64Audio,
	}); err != nil {
		return err
	}
	return nil
}

func (r *RealTimeApi) requestAudioResponse(ctx context.Context, conn *openairt.Conn) error {
	return conn.SendMessage(ctx, openairt.ResponseCreateEvent{
		Response: openairt.ResponseCreateParams{
			Instructions: r.FormattedInstructions,
		},
	})
}

func (r *RealTimeApi) startInProgressAudioDelayed() {
	go func() {
		ctx := r.delayedStartCtx
		timer := time.NewTimer(2 * time.Second)
		defer timer.Stop()

		select {
		case <-timer.C:
			if ctx.Err() == nil {
				r.startInProgressAudio()
			}
		case <-ctx.Done():
			return
		}
	}()
}

func (r *RealTimeApi) startInProgressAudio() {
	r.stopInProgressAudio()

	r.inProgressCtx, r.inProgressCancel = context.WithCancel(r.Ctx)
	r.PlayingInProgressAudio = true

	inProgressData := audio.GetInProgressAudio()
	sampleSize := 160

	go func() {
		ctx := r.inProgressCtx
		index := 0
		ticker := time.NewTicker(30 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				r.PlayingInProgressAudio = false
				return
			case <-ticker.C:
				var chunk []byte
				if index+sampleSize <= len(inProgressData) {
					chunk = inProgressData[index : index+sampleSize]
					index += sampleSize
				} else {
					chunk = append(chunk, inProgressData[index:]...)
					remaining := sampleSize - len(chunk)
					index = remaining
					if remaining > 0 {
						chunk = append(chunk, inProgressData[:remaining]...)
					}
				}

				audioMsg := r.newAudioMessage("AUDIO", "", chunk, "")
				select {
				case r.OutChannel <- audioMsg:
				default:
					log.Debug().Msg("RealTimeApi: OutChannel is full or closed, skipping in-progress audio chunk")
				}
			}
		}
	}()
}

func (r *RealTimeApi) stopInProgressAudio() {
	if r.PlayingInProgressAudio && r.inProgressCancel != nil {
		r.inProgressCancel()
		r.PlayingInProgressAudio = false
	}
}

func NewRealTimeApi(voiceBot *comcentApi.VoiceBot, mcpClientManager *mcpclient.McpClientManager) *RealTimeApi {
	ctx := context.Background()
	realTimeApi := &RealTimeApi{
		InChannel:              make(chan audioMessage.AudioMessage, 10),
		OutChannel:             make(chan audioMessage.AudioMessage, 10),
		ShouldHangUp:           false,
		WaitingForHangupAudio:  false,
		Conn:                   nil,
		Ctx:                    ctx,
		VoiceBot:               voiceBot,
		FormattedInstructions:  strings.TrimSpace(formatInstructions(voiceBot)),
		PlayingInProgressAudio: false,
		inProgressCtx:          nil,
		inProgressCancel:       nil,
		delayedStartCtx:        nil,
		delayedStartCancel:     nil,
		McpClientManager:       mcpClientManager,
	}
	go realTimeApi.run(ctx)
	return realTimeApi
}

func formatInstructions(voiceBot *comcentApi.VoiceBot) string {
	return fmt.Sprintf(`
	You are a helpful voice bot assistant who will have conversation like human.
	You will greet the user as per the following instructions, you will do this only once.

	<greeting_instructions>
	%s
	</greeting_instructions>

	You will follow the following instructions when user ask you to do something:
	<instructions>
	%s
	</instructions>

	Following instructions specifically says what not to do when user ask you to do something:
	<not_to_do_instructions>
	%s
	</not_to_do_instructions>
	`,
		voiceBot.GreetingInstructions,
		voiceBot.Instructions,
		voiceBot.NotToDoInstructions)
}
