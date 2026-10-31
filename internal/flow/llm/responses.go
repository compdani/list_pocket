package llm

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// KindResponses marks an endpoint that speaks the OpenAI Responses API
// instead of Chat Completions — the ChatGPT-plan route (Sign in with
// ChatGPT). Agents, copilots, and workflows don't change: requests are
// translated here and responses come back in the usual shape.
//
// Route requirements (OpenAI "Preview limitations"): stream=true,
// store=false, full history in input, system content as instructions, and
// no temperature, top_p, or max_output_tokens.
const KindResponses = "responses"

func responsesURL(base string) string {
	base = strings.TrimRight(base, "/")
	if strings.HasSuffix(base, "/responses") {
		return base
	}
	return base + "/responses"
}

// buildResponsesBody translates a chat-style request to a Responses request.
func buildResponsesBody(req Request) ([]byte, error) {
	s := req.Settings
	model := req.Model
	if model == "" {
		model = s.Model
	}

	var instructions []string
	input := make([]any, 0, len(req.Messages))
	for _, m := range req.Messages {
		switch m.Role {
		case "system", "developer":
			if strings.TrimSpace(m.Content) != "" {
				instructions = append(instructions, m.Content)
			}
		case "user":
			content := []any{map[string]any{"type": "input_text", "text": m.Content}}
			for _, a := range m.Attachments {
				if a.IsImage() {
					content = append(content, map[string]any{"type": "input_image", "image_url": a.DataURL()})
					continue
				}
				content = append(content, map[string]any{"type": "input_file", "filename": a.Filename, "file_data": a.DataURL()})
			}
			input = append(input, map[string]any{"type": "message", "role": "user", "content": content})
		case "assistant":
			if m.Content != "" {
				input = append(input, map[string]any{"type": "message", "role": "assistant", "content": []any{
					map[string]any{"type": "output_text", "text": m.Content},
				}})
			}
			for _, tc := range m.ToolCalls {
				input = append(input, map[string]any{
					"type": "function_call", "call_id": tc.ID, "name": tc.Function.Name, "arguments": tc.Function.Arguments,
				})
			}
		case "tool":
			input = append(input, map[string]any{"type": "function_call_output", "call_id": m.ToolCallID, "output": m.Content})
		}
	}

	body := map[string]any{
		"model":  model,
		"input":  input,
		"store":  false,
		"stream": true,
	}
	if len(instructions) > 0 {
		body["instructions"] = strings.Join(instructions, "\n\n")
	}

	var tools []any
	for _, t := range req.Tools {
		switch t["type"] {
		case "function":
			fn, _ := t["function"].(map[string]any)
			tools = append(tools, map[string]any{
				"type": "function", "name": fn["name"], "description": fn["description"], "parameters": fn["parameters"], "strict": false,
			})
		case "web_search":
			tools = append(tools, t)
		}
		// Other providers' server tools (e.g. openrouter:web_search) don't
		// exist on this route and are dropped.
	}
	if len(tools) > 0 {
		body["tools"] = tools
		if s.ToolChoice != "" {
			body["tool_choice"] = s.ToolChoice
		}
		if s.ParallelToolCalls != nil {
			body["parallel_tool_calls"] = *s.ParallelToolCalls
		}
	}
	if s.ReasoningEffort != "" {
		body["reasoning"] = map[string]any{"effort": s.ReasoningEffort}
	}
	for k, v := range s.ExtraBody {
		body[k] = v
	}
	for k, v := range req.Extra {
		switch k {
		case "tool_choice":
			// Chat form {"type":"function","function":{"name":X}} → {"type":"function","name":X}.
			if m, ok := v.(map[string]any); ok {
				if fn, ok := m["function"].(map[string]any); ok {
					v = map[string]any{"type": "function", "name": fn["name"]}
				}
			}
			body[k] = v
		case "response_format":
			if m, ok := v.(map[string]any); ok && m["type"] == "json_schema" {
				js, _ := m["json_schema"].(map[string]any)
				body["text"] = map[string]any{"format": map[string]any{
					"type": "json_schema", "name": js["name"], "schema": js["schema"], "strict": false,
				}}
			}
		case "web_search_options", "usage", "cache_control", "stream_options", "max_completion_tokens":
			// Chat Completions / OpenRouter only.
		default:
			body[k] = v
		}
	}
	return json.Marshal(body)
}

type respEvent struct {
	Type     string          `json:"type"`
	Delta    string          `json:"delta"`
	Response *respObject     `json:"response"`
	Item     json.RawMessage `json:"item"`
	Error    *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
	Message string `json:"message"`
	Code    string `json:"code"`
}

type respObject struct {
	Model  string            `json:"model"`
	Status string            `json:"status"`
	Output []json.RawMessage `json:"output"`
	Usage  *struct {
		InputTokens        int `json:"input_tokens"`
		OutputTokens       int `json:"output_tokens"`
		TotalTokens        int `json:"total_tokens"`
		InputTokensDetails *struct {
			CachedTokens int `json:"cached_tokens"`
		} `json:"input_tokens_details"`
	} `json:"usage"`
	Error *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
	IncompleteDetails *struct {
		Reason string `json:"reason"`
	} `json:"incomplete_details"`
}

type respItem struct {
	Type      string `json:"type"`
	CallID    string `json:"call_id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
	Content   []struct {
		Type        string `json:"type"`
		Text        string `json:"text"`
		Annotations []struct {
			Type  string `json:"type"`
			URL   string `json:"url"`
			Title string `json:"title"`
		} `json:"annotations"`
	} `json:"content"`
}

// readResponsesStream parses Responses SSE. Inference counts as successful
// only after response.completed.
func readResponsesStream(r io.Reader, onDelta func(string)) (*Response, error) {
	out := &Response{Message: Message{Role: "assistant"}}
	var streamed strings.Builder
	var items []json.RawMessage // output_item.done; the plan route's completed event omits output
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64<<10), 16<<20)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" || data == "[DONE]" {
			continue
		}
		var ev respEvent
		if err := json.Unmarshal([]byte(data), &ev); err != nil {
			return nil, fmt.Errorf("decoding stream event: %w", err)
		}
		switch ev.Type {
		case "response.output_text.delta":
			streamed.WriteString(ev.Delta)
			if onDelta != nil && ev.Delta != "" {
				onDelta(ev.Delta)
			}
		case "response.output_item.done":
			if len(ev.Item) > 0 {
				items = append(items, ev.Item)
			}
		case "response.completed":
			if ev.Response == nil {
				return nil, fmt.Errorf("response.completed without a response")
			}
			if len(ev.Response.Output) == 0 {
				ev.Response.Output = items
			}
			fillFromResponse(out, ev.Response)
			if out.Message.Content == "" {
				out.Message.Content = streamed.String()
			}
			return out, nil
		case "response.failed":
			msg, code := "response failed", ""
			if ev.Response != nil && ev.Response.Error != nil {
				msg, code = ev.Response.Error.Message, ev.Response.Error.Code
			}
			return nil, &providerError{msg: strings.TrimSpace(code + " " + msg)}
		case "response.incomplete":
			reason := "unknown"
			if ev.Response != nil && ev.Response.IncompleteDetails != nil {
				reason = ev.Response.IncompleteDetails.Reason
			}
			return nil, &providerError{msg: "response incomplete: " + reason}
		case "error":
			msg := ev.Message
			if ev.Error != nil {
				msg = strings.TrimSpace(ev.Error.Code + " " + ev.Error.Message)
			}
			return nil, &providerError{msg: msg}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading stream: %w", err)
	}
	return nil, fmt.Errorf("stream ended before response.completed")
}

func fillFromResponse(out *Response, resp *respObject) {
	out.Model = resp.Model
	out.FinishReason = "stop"
	var text strings.Builder
	for _, raw := range resp.Output {
		var item respItem
		if json.Unmarshal(raw, &item) != nil {
			continue
		}
		switch item.Type {
		case "message":
			for _, c := range item.Content {
				if c.Type == "output_text" {
					text.WriteString(c.Text)
					for _, a := range c.Annotations {
						if a.Type == "url_citation" && a.URL != "" {
							out.Citations = append(out.Citations, Citation{URL: a.URL, Title: a.Title})
						}
					}
				}
			}
		case "function_call":
			out.Message.ToolCalls = append(out.Message.ToolCalls, ToolCall{
				ID: item.CallID, Type: "function", Function: FunctionCall{Name: item.Name, Arguments: item.Arguments},
			})
			out.FinishReason = "tool_calls"
		case "web_search_call":
			out.Usage.WebSearchRequests++
		}
	}
	out.Message.Content = text.String()
	if u := resp.Usage; u != nil {
		out.Usage.PromptTokens = u.InputTokens
		out.Usage.CompletionTokens = u.OutputTokens
		out.Usage.TotalTokens = u.TotalTokens
		if u.InputTokensDetails != nil {
			out.Usage.CachedTokens = u.InputTokensDetails.CachedTokens
		}
	}
	// Plan usage is covered by the ChatGPT subscription: no per-token cost.
	zero := 0.0
	out.Usage.Cost = &zero
}
