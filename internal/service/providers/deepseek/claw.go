package deepseek

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"movieagent/internal/service/claw/provider"
	"movieagent/internal/service/claw/schema"
)

var _ provider.Provider = (*Client)(nil)

// Chat implements claw/provider.Provider.
// req.Tools == nil → Thinking (no tools). Non-nil → Action with tool schemas.
func (c *Client) Chat(ctx context.Context, req schema.ChatRequest) (*schema.ChatResponse, error) {
	if c == nil || c.apiKey == "" {
		return nil, ErrNotConfigured
	}
	if len(req.Messages) == 0 {
		return nil, ErrEmptyPrompt
	}

	msgs, err := toAPIMessages(req.Messages)
	if err != nil {
		return nil, err
	}

	body := map[string]any{
		"model":    c.model,
		"messages": msgs,
		"stream":   false,
	}
	// Thinking: omit tools. Action: attach schemas (even if empty slice).
	if req.Tools != nil {
		body["tools"] = toAPITools(req.Tools)
	}

	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/chat/completions", bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

	client := c.httpClient
	if client == nil {
		client = &http.Client{}
	}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("deepseek http %d: %s", resp.StatusCode, truncate(string(respBody), 400))
	}

	var out apiChatResponse
	if err := json.Unmarshal(respBody, &out); err != nil {
		return nil, fmt.Errorf("decode deepseek response: %w", err)
	}
	if out.Error != nil && out.Error.Message != "" {
		return nil, fmt.Errorf("deepseek api: %s", out.Error.Message)
	}
	if len(out.Choices) == 0 {
		return nil, fmt.Errorf("deepseek returned no choices")
	}

	choice := out.Choices[0]
	msg := schema.Message{
		Role:    schema.RoleAssistant,
		Content: choice.Message.Content,
	}
	for _, tc := range choice.Message.ToolCalls {
		name := tc.Function.Name
		if name == "" {
			continue
		}
		msg.ToolCalls = append(msg.ToolCalls, schema.ToolCall{
			ID:        tc.ID,
			Name:      name,
			Arguments: tc.Function.Arguments,
		})
	}
	finish := choice.FinishReason
	if finish == "" {
		if len(msg.ToolCalls) > 0 {
			finish = "tool_calls"
		} else {
			finish = "stop"
		}
	}
	return &schema.ChatResponse{Message: msg, FinishReason: finish}, nil
}

type apiChatResponse struct {
	Choices []struct {
		Message struct {
			Content   string `json:"content"`
			ToolCalls []struct {
				ID       string `json:"id"`
				Type     string `json:"type"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func toAPIMessages(msgs []schema.Message) ([]map[string]any, error) {
	out := make([]map[string]any, 0, len(msgs))
	for _, m := range msgs {
		switch m.Role {
		case schema.RoleSystem, schema.RoleUser:
			out = append(out, map[string]any{
				"role":    string(m.Role),
				"content": m.Content,
			})
		case schema.RoleTool:
			out = append(out, map[string]any{
				"role":         "tool",
				"content":      m.Content,
				"tool_call_id": m.ToolCallID,
				"name":         m.Name,
			})
		case schema.RoleAssistant:
			item := map[string]any{"role": "assistant"}
			if m.Content != "" {
				item["content"] = m.Content
			}
			if len(m.ToolCalls) > 0 {
				calls := make([]map[string]any, 0, len(m.ToolCalls))
				for _, tc := range m.ToolCalls {
					calls = append(calls, map[string]any{
						"id":   tc.ID,
						"type": "function",
						"function": map[string]any{
							"name":      tc.Name,
							"arguments": tc.Arguments,
						},
					})
				}
				item["tool_calls"] = calls
			} else if m.Content == "" {
				item["content"] = ""
			}
			out = append(out, item)
		default:
			return nil, fmt.Errorf("unsupported message role: %s", m.Role)
		}
	}
	return out, nil
}

func toAPITools(defs []schema.ToolDefinition) []map[string]any {
	if defs == nil {
		return nil
	}
	out := make([]map[string]any, 0, len(defs))
	for _, d := range defs {
		params := d.Parameters
		if params == nil {
			params = map[string]any{"type": "object", "properties": map[string]any{}}
		}
		out = append(out, map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        d.Name,
				"description": d.Description,
				"parameters":  params,
			},
		})
	}
	return out
}
