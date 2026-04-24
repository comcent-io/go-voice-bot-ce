package chatgpt

import (
	"comcent-io/voice-bot/audio"
	"comcent-io/voice-bot/audioMessage"
	"comcent-io/voice-bot/comcentApi"
	mcpclient "comcent-io/voice-bot/mcpClient"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/packages/ssestream"
	"github.com/openai/openai-go/responses"
)

// ChatGPT represents the main struct holding all data
type ChatGPT struct {
	client           *openai.Client
	shouldHangUp     bool
	shouldEnqueue    bool
	queueName        string
	InChannel        chan audioMessage.AudioMessage
	Messages         []responses.ResponseInputItemUnionParam
	OutChannel       chan audioMessage.AudioMessage
	State            string
	voiceBot         *comcentApi.VoiceBot
	isFirstMessage   bool
	McpClientManager *mcpclient.McpClientManager
}

// NewChatGPT creates a new instance of ChatGPT
func NewChatGPT(voiceBot *comcentApi.VoiceBot, mcpClientManager *mcpclient.McpClientManager) *ChatGPT {
	apiKey := os.Getenv("OPENAI_API_KEY")
	client := openai.NewClient(option.WithAPIKey(apiKey))
	chatGpt := &ChatGPT{
		client:           &client,
		InChannel:        make(chan audioMessage.AudioMessage, 10),
		OutChannel:       make(chan audioMessage.AudioMessage, 10),
		State:            "idle",
		voiceBot:         voiceBot,
		isFirstMessage:   true,
		McpClientManager: mcpClientManager,
	}

	instructions := fmt.Sprintf(`
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
	systemMessage := responses.ResponseInputItemUnionParam{OfMessage: &responses.EasyInputMessageParam{
		Role:    "system",
		Content: responses.EasyInputMessageContentUnionParam{OfString: openai.String(instructions)},
	}}
	chatGpt.Messages = append(chatGpt.Messages, systemMessage)

	go chatGpt.run()
	return chatGpt
}

func (c *ChatGPT) run() {
	for message := range c.InChannel {
		// Check if we should stop processing new messages
		if c.shouldHangUp || c.shouldEnqueue {
			log.Info().Msg("Hangup detected, ignoring new messages")
			continue
		}

		log.Info().Msgf("chatgpt: received message %s, type: %s", message.Text, message.Type)

		switch message.Type {
		case "TRANSCRIPT":
			c.State = "gptInProgress"
			log.Info().Msg("chatgpt: received message " + message.Text)
			if !c.isFirstMessage {
				ackAudioMessage := audioMessage.AudioMessage{
					Type:            "AUDIO",
					Text:            "",
					Audio:           audio.RandomAckAudio(),
					Command:         "",
					SttDuration:     0.0,
					GptInputTokens:  0,
					GptOutputTokens: 0,
					TtsCharacters:   0,
				}
				select {
				case c.OutChannel <- ackAudioMessage:
				default:
					log.Debug().Msg("ChatGPT: OutChannel is full or closed, skipping ack audio")
				}
			}

			userMessage := responses.ResponseInputItemUnionParam{OfMessage: &responses.EasyInputMessageParam{
				Role:    "user",
				Content: responses.EasyInputMessageContentUnionParam{OfString: openai.String(message.Text)},
			}}
			c.Messages = append(c.Messages, userMessage)
			var messagesToSend []responses.ResponseInputItemUnionParam
			if c.isFirstMessage {
				messagesToSend = append(messagesToSend, c.Messages[0]) // System message
				messagesToSend = append(messagesToSend, userMessage)   // Current user message
				c.isFirstMessage = false
			} else {
				messagesToSend = c.Messages
			}

			// Get tools from MCP server
			tools, err := mcpclient.ListTools(context.Background(), c.McpClientManager)
			if err != nil {
				log.Error().Err(err).Msg("Failed to list MCP tools, continuing without tools")
				tools = []map[string]any{} // Use empty slice on error
			}

			stream := c.client.Responses.NewStreaming(context.Background(), responses.ResponseNewParams{
				Input: responses.ResponseNewParamsInputUnion{OfInputItemList: messagesToSend},

				Model: openai.ChatModelGPT4_1Mini,
				Tools: FormatResponseApiTools(tools, c.voiceBot),
			})

			log.Info().Msg("Created run stream")

			if stream.Err() != nil {
				log.Error().Err(stream.Err()).Msg("Error creating stream")
				continue
			} else {
				c.streamHandler(stream)
			}
			c.State = "gptDone"
		default:
			log.Info().Msgf("chatgpt: Sending message directly to OutChannel, type: %s", message.Type)
			select {
			case c.OutChannel <- message:
			default:
				log.Error().Msg("ChatGPT: OutChannel is full or closed, cannot send message")
			}
		}
	}
	// Input channel closed, wait a moment for any ongoing processing to complete
	log.Info().Msg("ChatGPT: Input channel closed, waiting for processing to complete")
	// Close output channels to propagate close signal
	log.Info().Msg("ChatGPT: Closing output channels")
	close(c.OutChannel)
}

func (c *ChatGPT) Pipe(inputChannel chan audioMessage.AudioMessage) {
	go func() {
		for msg := range c.OutChannel {
			select {
			case inputChannel <- msg:
			default:
				log.Error().Msg("ChatGPT: Input channel is full or closed, cannot send message")
			}
		}
		// Output channel closed, close input channel to propagate close signal
		log.Info().Msg("ChatGPT: Output channel closed, closing input channel")
		close(inputChannel)
	}()
}

func (c *ChatGPT) streamHandler(stream *ssestream.Stream[responses.ResponseStreamEventUnion]) {
	sb := strings.Builder{}               // For streaming chunks to TTS
	completeResponse := strings.Builder{} // For storing the complete response

	for stream.Next() {
		currentData := stream.Current()
		currentType := currentData.Type

		if currentType == "response.output_text.delta" {
			sb.WriteString(currentData.Delta.OfString)
			completeResponse.WriteString(currentData.Delta.OfString)
			text := sb.String()

			// Emit if sentence punctuation found
			if strings.HasSuffix(text, ".") || strings.HasSuffix(text, "?") || strings.HasSuffix(text, "!") {
				chunk := strings.TrimSpace(text)
				audioMessage := audioMessage.AudioMessage{
					Type:            "TRANSCRIPT",
					Text:            chunk,
					Audio:           nil,
					Command:         "",
					SttDuration:     0.0,
					GptInputTokens:  0,
					GptOutputTokens: 0,
					TtsCharacters:   0,
				}
				select {
				case c.OutChannel <- audioMessage:
				default:
					log.Debug().Msg("ChatGPT: OutChannel is full or closed, skipping chunk")
				}
				sb.Reset()
			}
		}

		if currentType == "response.output_item.done" {
			fillerSpoken := false
			switch currentData.Item.Type {
			case "function_call":
				functionCall := responses.ResponseInputItemUnionParam{OfMessage: &responses.EasyInputMessageParam{
					Role: "assistant",
					Content: responses.EasyInputMessageContentUnionParam{OfString: openai.String(
						"Calling function: " + currentData.Item.Name + " with args " + currentData.Item.Arguments,
					)},
				}}
				c.Messages = append(c.Messages, functionCall)

				switch currentData.Item.Name {
				case "hangup":
					log.Info().Msg("Hangup detected")
					c.shouldHangUp = true
					audioMessage := audioMessage.AudioMessage{
						Type:            "COMMAND",
						Text:            "",
						Audio:           nil,
						Command:         "HANGUP",
						SttDuration:     0.0,
						GptInputTokens:  0,
						GptOutputTokens: 0,
						TtsCharacters:   0,
					}
					select {
					case c.OutChannel <- audioMessage:
					default:
						log.Debug().Msg("ChatGPT: OutChannel is full or closed, skipping hangup command")
					}

					toolMessage := responses.ResponseInputItemUnionParam{
						OfFunctionCallOutput: &responses.ResponseInputItemFunctionCallOutputParam{
							Type:   "function_call_output",
							Output: `{"success": true}`,
						},
					}
					c.Messages = append(c.Messages, toolMessage)
				case "enqueue":
					log.Info().Msg("Enqueue detected")
					c.shouldEnqueue = true

					var args struct {
						Queue string `json:"queue"`
					}
					c.queueName = args.Queue
					_ = json.Unmarshal([]byte(currentData.Item.Arguments), &args)
					log.Info().Msg("Enqueueing to queue: " + args.Queue)

					audioMessage := audioMessage.AudioMessage{
						Type:            "COMMAND",
						Text:            args.Queue,
						Audio:           nil,
						Command:         "ENQUEUE",
						SttDuration:     0.0,
						GptInputTokens:  0,
						GptOutputTokens: 0,
						TtsCharacters:   0,
					}
					select {
					case c.OutChannel <- audioMessage:
					default:
						log.Debug().Msg("ChatGPT: OutChannel is full or closed, skipping enqueue command")
					}

					toolMessage := responses.ResponseInputItemUnionParam{
						OfFunctionCallOutput: &responses.ResponseInputItemFunctionCallOutputParam{
							Type:   "function_call_output", // include the call ID
							Output: `{"success": true}`,
						},
					}
					c.Messages = append(c.Messages, toolMessage)

				case "get_current_datetime":
					log.Info().Msg("Get current datetime detected")
					currentTime := time.Now().Format(time.RFC3339)
					output := fmt.Sprintf(`{"datetime": "%s"}`, currentTime)
					toolMessage := responses.ResponseInputItemUnionParam{
						OfFunctionCallOutput: &responses.ResponseInputItemFunctionCallOutputParam{
							Type:   "function_call_output",
							Output: output,
						},
					}
					c.Messages = append(c.Messages, toolMessage)

				default:
					if !fillerSpoken {
						// Send random in-progress audio through pipeline
						inProgressAudioMessage := audioMessage.AudioMessage{
							Type:            "AUDIO",
							Text:            "",
							Audio:           audio.RandomInProgressAudio(),
							Command:         "",
							SttDuration:     0.0,
							GptInputTokens:  0,
							GptOutputTokens: 0,
							TtsCharacters:   0,
						}
						select {
						case c.OutChannel <- inProgressAudioMessage:
						default:
							log.Debug().Msg("ChatGPT: OutChannel is full or closed, skipping in-progress audio")
						}
						fillerSpoken = true
					}

					log.Info().Msg("Making call to function: " + currentData.Item.Name + " with arguments: " + currentData.Item.Arguments)

					// Parse JSON string arguments into map
					var args map[string]interface{}
					if err := json.Unmarshal([]byte(currentData.Item.Arguments), &args); err != nil {
						log.Error().Err(err).Msg("Failed to parse function arguments")
						continue
					}

					output, err := mcpclient.CallTool(context.Background(), c.McpClientManager, currentData.Item.Name, args)
					if err != nil {
						log.Error().Err(err).Msg("Failed to call tool: " + err.Error())
						continue
					}

					select {
					case c.OutChannel <- audioMessage.AudioMessage{
						Type:            "TRANSCRIPT",
						Text:            output,
						Audio:           nil,
						Command:         "",
						SttDuration:     0.0,
						GptInputTokens:  0,
						GptOutputTokens: 0,
						TtsCharacters:   0,
					}:
					default:
						log.Debug().Msg("ChatGPT: OutChannel is full or closed, skipping function response")
					}

					functionResponse := responses.ResponseInputItemUnionParam{OfMessage: &responses.EasyInputMessageParam{
						Role: "assistant",
						Content: responses.EasyInputMessageContentUnionParam{OfString: openai.String(
							"Function result (" + currentData.Item.Name + "): " + output,
						)},
					}}
					c.Messages = append(c.Messages, functionResponse)
				}
			}
		}

		if currentType == "response.completed" {
			inputTokens := int(currentData.Response.Usage.InputTokens)
			outputTokens := int(currentData.Response.Usage.OutputTokens)

			// Send GPT_BILLING message
			billingMessage := audioMessage.AudioMessage{
				Type:            "GPT_BILLING",
				Text:            "",
				Audio:           nil,
				Command:         "",
				SttDuration:     0.0,
				GptInputTokens:  inputTokens,
				GptOutputTokens: outputTokens,
				TtsCharacters:   0,
			}
			select {
			case c.OutChannel <- billingMessage:
			default:
				log.Debug().Msg("ChatGPT: OutChannel is full or closed, skipping billing message")
			}

			log.Info().Msg("Last part: " + sb.String())
			log.Info().Msg("Thread run completed")

			finalString := completeResponse.String()
			if len(finalString) > 0 {
				// Append assistant's final message into conversation history
				assistantMessage := responses.ResponseInputItemUnionParam{OfMessage: &responses.EasyInputMessageParam{
					Role:    "assistant",
					Content: responses.EasyInputMessageContentUnionParam{OfString: openai.String(finalString)},
				}}
				c.Messages = append(c.Messages, assistantMessage)
			}
			return
		}
	}
}

type GptCommand struct {
	ShouldHangUp  bool
	ShouldEnqueue bool
	QueueName     string
}
