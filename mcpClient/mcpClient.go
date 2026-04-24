package mcpclient

import (
	"context"
	"errors"
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/rs/zerolog/log"
)

// McpServer represents an MCP server configuration with URL and token
type McpServer struct {
	URL   string
	Token string
}

// McpClientManager manages multiple MCP client sessions
type McpClientManager struct {
	Sessions []*mcp.ClientSession
}

// authTransport wraps an http.RoundTripper to add authentication headers
type authTransport struct {
	base  http.RoundTripper
	token string
}

func NewMcpClient(ctx context.Context, mcpServers []McpServer) *McpClientManager {
	if len(mcpServers) == 0 {
		log.Warn().Msg("No MCP servers provided")
		return &McpClientManager{Sessions: []*mcp.ClientSession{}}
	}

	manager := &McpClientManager{
		Sessions: make([]*mcp.ClientSession, 0, len(mcpServers)),
	}

	for _, server := range mcpServers {
		if server.URL == "" {
			continue
		}
		client := mcp.NewClient(&mcp.Implementation{
			Name:    "voicebot-mcp-client",
			Version: "1.0.0",
		}, nil)

		// Create HTTP client with authorization
		httpClient := &http.Client{
			Transport: &authTransport{
				base:  http.DefaultTransport,
				token: server.Token,
			},
		}

		transport := &mcp.StreamableClientTransport{
			Endpoint:   server.URL,
			HTTPClient: httpClient,
		}
		session, err := client.Connect(ctx, transport, nil)
		if err != nil {
			log.Error().Err(err).Str("url", server.URL).Msg("Failed to connect to MCP server")
			continue
		}
		manager.Sessions = append(manager.Sessions, session)
		log.Info().Str("url", server.URL).Msg("Successfully connected to MCP server")
	}

	if len(manager.Sessions) == 0 {
		log.Warn().Msg("No MCP server connections established")
	}

	return manager
}

// CallTool calls a tool on all client sessions until one succeeds
func CallTool(ctx context.Context, manager *McpClientManager, toolName string, toolArgs map[string]any) (string, error) {
	if manager == nil || len(manager.Sessions) == 0 {
		log.Error().Msg("No MCP client sessions available")
		return "", errors.New("no MCP client sessions available")
	}

	var lastErr error
	for i, session := range manager.Sessions {
		output, err := session.CallTool(ctx, &mcp.CallToolParams{
			Name:      toolName,
			Arguments: toolArgs,
		})
		if err != nil {
			log.Debug().Err(err).Int("session", i).Str("tool", toolName).Msg("Failed to call tool on session, trying next")
			lastErr = err
			continue
		}

		outputString := ""
		for _, content := range output.Content {
			if textContent, ok := content.(*mcp.TextContent); ok {
				outputString += textContent.Text
			}
		}
		log.Info().Int("session", i).Str("tool", toolName).Msg("Successfully called tool")
		return outputString, nil
	}

	log.Error().Err(lastErr).Str("tool", toolName).Msg("Failed to call tool on all MCP servers")
	return "", lastErr
}

// ListTools combines tools from all client sessions
func ListTools(ctx context.Context, manager *McpClientManager) ([]map[string]any, error) {
	if manager == nil || len(manager.Sessions) == 0 {
		log.Warn().Msg("No MCP client sessions available, returning empty tools list")
		return []map[string]any{}, nil
	}

	allTools := []map[string]any{}
	toolNames := make(map[string]bool) // To track unique tool names

	// Collect tools from all sessions
	for i, session := range manager.Sessions {
		toolResponse, err := session.ListTools(ctx, nil)
		if err != nil {
			log.Error().Err(err).Int("session", i).Msg("Failed to list tools from session")
			continue
		}

		for _, tool := range toolResponse.Tools {
			// Skip duplicate tool names (first occurrence wins)
			if toolNames[tool.Name] {
				log.Debug().Str("tool", tool.Name).Int("session", i).Msg("Skipping duplicate tool name")
				continue
			}

			toolMap := map[string]any{
				"name":        tool.Name,
				"inputSchema": tool.InputSchema,
				"description": tool.Description,
			}
			allTools = append(allTools, toolMap)
			toolNames[tool.Name] = true
		}
		log.Info().Int("session", i).Int("tools", len(toolResponse.Tools)).Msg("Listed tools from MCP server")
	}

	log.Info().Int("total_tools", len(allTools)).Msg("Combined tools from all MCP servers")
	return allTools, nil
}
func (t *authTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Set("Authorization", "Bearer "+t.token)
	return t.base.RoundTrip(req)
}
