package chatgpt

import (
	"comcent-io/voice-bot/comcentApi"
	"fmt"

	openairt "github.com/WqyJh/go-openai-realtime/v2"
	"github.com/openai/openai-go/packages/param"
	"github.com/openai/openai-go/responses"
)

// FormatFunctions formats tools for the OpenAI API from MCP tools
func FormatResponseApiTools(tools []map[string]any, voiceBot *comcentApi.VoiceBot) []responses.ToolUnionParam {
	var functionTools []responses.ToolUnionParam

	// Process MCP tools
	for _, tool := range tools {
		// inputSchema is a JSON Schema object with type, properties, and required fields
		inputSchema, ok := tool["inputSchema"].(map[string]any)
		if !ok {
			continue
		}

		// Extract properties from the JSON Schema
		properties := make(map[string]any)
		required := []string{}

		// Get properties from the schema
		if schemaProperties, ok := inputSchema["properties"].(map[string]any); ok {
			for propName, propDef := range schemaProperties {
				if propDefMap, ok := propDef.(map[string]any); ok {
					properties[propName] = propDefMap
				}
			}
		}

		// Get required fields from the schema
		if schemaRequired, ok := inputSchema["required"].([]any); ok {
			for _, req := range schemaRequired {
				if reqStr, ok := req.(string); ok {
					required = append(required, reqStr)
				}
			}
		}

		// If no properties found, try to parse as direct property map (fallback for different schema formats)
		if len(properties) == 0 {
			// Check if inputSchema itself is a properties map
			for name, param := range inputSchema {
				if name == "type" || name == "required" || name == "properties" {
					continue
				}
				if paramMap, ok := param.(map[string]any); ok {
					properties[name] = paramMap
					// Check if this field is required
					if requiredVal, hasRequired := paramMap["required"]; hasRequired {
						if isRequired, ok := requiredVal.(bool); ok && isRequired {
							required = append(required, name)
						}
					}
				}
			}
		}

		// Extract name and description with type assertions
		name, ok := tool["name"].(string)
		if !ok {
			continue
		}
		description, ok := tool["description"].(string)
		if !ok {
			description = ""
		}

		functionTools = append(functionTools, responses.ToolUnionParam{
			OfFunction: &responses.FunctionToolParam{
				Name:        name,
				Description: param.Opt[string]{Value: description},
				Parameters: map[string]any{
					"type":       "object",
					"properties": properties,
					"required":   required,
				},
			},
		})
	}

	// Add hangup function if enabled
	if voiceBot != nil && voiceBot.IsHangup {
		functionTools = append(functionTools, responses.ToolUnionParam{
			OfFunction: &responses.FunctionToolParam{
				Name:        "hangup",
				Description: param.Opt[string]{Value: "used to hang up the call"},
				Parameters: map[string]any{
					"type":       "object",
					"properties": map[string]any{},
					"required":   []string{},
				},
			},
		})
	}

	// Add enqueue function if enabled
	if voiceBot != nil && voiceBot.IsEnqueue {
		queuesDescription := fmt.Sprintf("queue name from one of the following values only: %s", voiceBot.Queues)
		functionTools = append(functionTools, responses.ToolUnionParam{
			OfFunction: &responses.FunctionToolParam{
				Name:        "enqueue",
				Description: param.Opt[string]{Value: fmt.Sprintf("Enqueue will take %s as parameters when customer ask us to transfer %s respectively", voiceBot.Queues, voiceBot.Queues)},
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"queue": map[string]any{
							"type":        "string",
							"description": queuesDescription,
						},
					},
					"required": []string{"queue"},
				},
			},
		})
	}

	// Add get_current_datetime function
	functionTools = append(functionTools, responses.ToolUnionParam{
		OfFunction: &responses.FunctionToolParam{
			Name:        "get_current_datetime",
			Description: param.Opt[string]{Value: "Returns today's date and time"},
			Parameters: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
				"required":   []string{},
			},
		},
	})

	return functionTools
}

// FormatRealtimeTools formats tools for the Realtime API directly from MCP tools
func FormatRealtimeTools(tools []map[string]any, voiceBot *comcentApi.VoiceBot) []openairt.ToolUnion {
	var realtimeTools []openairt.ToolUnion

	// Process MCP tools
	for _, tool := range tools {
		// inputSchema is a JSON Schema object with type, properties, and required fields
		inputSchema, ok := tool["inputSchema"].(map[string]any)
		if !ok {
			continue
		}

		// Extract properties from the JSON Schema
		properties := make(map[string]any)
		required := []string{}

		// Get properties from the schema
		if schemaProperties, ok := inputSchema["properties"].(map[string]any); ok {
			for propName, propDef := range schemaProperties {
				if propDefMap, ok := propDef.(map[string]any); ok {
					properties[propName] = propDefMap
				}
			}
		}

		// Get required fields from the schema
		if schemaRequired, ok := inputSchema["required"].([]any); ok {
			for _, req := range schemaRequired {
				if reqStr, ok := req.(string); ok {
					required = append(required, reqStr)
				}
			}
		}

		// If no properties found, try to parse as direct property map (fallback for different schema formats)
		if len(properties) == 0 {
			// Check if inputSchema itself is a properties map
			for name, param := range inputSchema {
				if name == "type" || name == "required" || name == "properties" {
					continue
				}
				if paramMap, ok := param.(map[string]any); ok {
					properties[name] = paramMap
					// Check if this field is required
					if requiredVal, hasRequired := paramMap["required"]; hasRequired {
						if isRequired, ok := requiredVal.(bool); ok && isRequired {
							required = append(required, name)
						}
					}
				}
			}
		}

		// Extract name and description with type assertions
		name, ok := tool["name"].(string)
		if !ok {
			continue
		}
		description, ok := tool["description"].(string)
		if !ok {
			description = ""
		}

		realtimeTools = append(realtimeTools, openairt.ToolUnion{
			Function: &openairt.ToolFunction{
				Name:        name,
				Description: description,
				Parameters: map[string]any{
					"type":       "object",
					"properties": properties,
					"required":   required,
				},
			},
		})
	}

	// Add hangup function if enabled
	if voiceBot != nil && voiceBot.IsHangup {
		realtimeTools = append(realtimeTools, openairt.ToolUnion{
			Function: &openairt.ToolFunction{
				Name:        "hangup",
				Description: "used to hang up the call",
				Parameters: map[string]any{
					"type":       "object",
					"properties": map[string]any{},
					"required":   []string{},
				},
			},
		})
	}

	// Add enqueue function if enabled
	if voiceBot != nil && voiceBot.IsEnqueue {
		queuesDescription := fmt.Sprintf("queue name from one of the following values only: %s", voiceBot.Queues)
		realtimeTools = append(realtimeTools, openairt.ToolUnion{
			Function: &openairt.ToolFunction{
				Name:        "enqueue",
				Description: fmt.Sprintf("Enqueue will take %s as parameters when customer ask us to transfer %s respectively", voiceBot.Queues, voiceBot.Queues),
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"queue": map[string]any{
							"type":        "string",
							"description": queuesDescription,
						},
					},
					"required": []string{"queue"},
				},
			},
		})
	}

	// Add get_current_datetime function
	realtimeTools = append(realtimeTools, openairt.ToolUnion{
		Function: &openairt.ToolFunction{
			Name:        "get_current_datetime",
			Description: "Returns today's date and time",
			Parameters: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
				"required":   []string{},
			},
		},
	})

	return realtimeTools
}
