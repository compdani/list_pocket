// Package llm is the harness's only LLM client: OpenAI-compatible
// /chat/completions with streaming and tool calls.
package llm

import (
	"encoding/json"
	"strings"
)

type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	// Attachments go with a user message as extra content parts.
	Attachments []Attachment `json:"-"`
}

// Attachment is an image or PDF sent to the model. Data is base64.
type Attachment struct {
	Filename string
	Mime     string
	Data     string
}

// DataURL is the attachment as a data: URL.
func (a Attachment) DataURL() string {
	return "data:" + a.Mime + ";base64," + a.Data
}

// IsImage reports whether the model should see this as a picture rather than a file.
func (a Attachment) IsImage() bool {
	return strings.HasPrefix(a.Mime, "image/")
}

// MarshalJSON writes Chat Completions content parts when there are attachments.
func (m Message) MarshalJSON() ([]byte, error) {
	type plain Message
	if len(m.Attachments) == 0 {
		return json.Marshal(plain(m))
	}
	parts := []any{map[string]any{"type": "text", "text": m.Content}}
	for _, a := range m.Attachments {
		if a.IsImage() {
			parts = append(parts, map[string]any{"type": "image_url", "image_url": map[string]string{"url": a.DataURL()}})
			continue
		}
		parts = append(parts, map[string]any{"type": "file", "file": map[string]string{"filename": a.Filename, "file_data": a.DataURL()}})
	}
	return json.Marshal(struct {
		Role    string `json:"role"`
		Content []any  `json:"content"`
	}{m.Role, parts})
}

type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function FunctionCall `json:"function"`
}

type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// FunctionTool builds a standard function tool definition.
func FunctionTool(name, description string, parameters map[string]any) map[string]any {
	if parameters == nil {
		parameters = map[string]any{"type": "object", "properties": map[string]any{}}
	}
	return map[string]any{
		"type": "function",
		"function": map[string]any{
			"name":        name,
			"description": description,
			"parameters":  parameters,
		},
	}
}

// Request is one chat completion call. Tools are raw maps so provider server
// tools (e.g. {"type":"openrouter:web_search"}) pass through untouched.
type Request struct {
	Model    string
	Messages []Message
	Tools    []map[string]any
	Settings Settings
	Stream   bool
	// Extra is merged into the request body last (after Settings.ExtraBody).
	Extra map[string]any
}

type Citation struct {
	URL     string `json:"url"`
	Title   string `json:"title,omitempty"`
	Content string `json:"content,omitempty"`
}

type Usage struct {
	PromptTokens      int      `json:"prompt_tokens"`
	CompletionTokens  int      `json:"completion_tokens"`
	TotalTokens       int      `json:"total_tokens"`
	CachedTokens      int      `json:"cached_tokens,omitempty"`      // prompt tokens read from cache
	CacheWriteTokens  int      `json:"cache_write_tokens,omitempty"` // prompt tokens written to cache
	Cost              *float64 `json:"cost,omitempty"`               // OpenRouter reports USD cost directly
	WebSearchRequests int      `json:"web_search_requests,omitempty"`
}

type Response struct {
	Message      Message    `json:"message"`
	FinishReason string     `json:"finish_reason"`
	Citations    []Citation `json:"citations,omitempty"`
	Usage        Usage      `json:"usage"`
	Model        string     `json:"model,omitempty"`
}

// --- wire formats ---

type wireAnnotation struct {
	Type        string `json:"type"`
	URLCitation *struct {
		URL     string `json:"url"`
		Title   string `json:"title"`
		Content string `json:"content"`
	} `json:"url_citation"`
}

type wireUsage struct {
	PromptTokens     int      `json:"prompt_tokens"`
	CompletionTokens int      `json:"completion_tokens"`
	TotalTokens      int      `json:"total_tokens"`
	Cost             *float64 `json:"cost"`
	PromptDetails    *struct {
		CachedTokens     int `json:"cached_tokens"`
		CacheWriteTokens int `json:"cache_write_tokens"`
	} `json:"prompt_tokens_details"`
	// OpenRouter's docs say server_tool_use; responses send server_tool_use_details.
	ServerToolUse        *serverToolUse `json:"server_tool_use"`
	ServerToolUseDetails *serverToolUse `json:"server_tool_use_details"`
}

type serverToolUse struct {
	WebSearchRequests int `json:"web_search_requests"`
}

type wireError struct {
	Message string          `json:"message"`
	Code    json.RawMessage `json:"code"`
}

type wireToolCallDelta struct {
	Index    int    `json:"index"`
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type wireChoice struct {
	Message *struct {
		Content     *string          `json:"content"`
		ToolCalls   []ToolCall       `json:"tool_calls"`
		Annotations []wireAnnotation `json:"annotations"`
	} `json:"message"`
	Delta *struct {
		Content     *string             `json:"content"`
		ToolCalls   []wireToolCallDelta `json:"tool_calls"`
		Annotations []wireAnnotation    `json:"annotations"`
	} `json:"delta"`
	FinishReason *string `json:"finish_reason"`
}

type wireResponse struct {
	Model   string       `json:"model"`
	Choices []wireChoice `json:"choices"`
	Usage   *wireUsage   `json:"usage"`
	Error   *wireError   `json:"error"`
}

func (u *wireUsage) toUsage() Usage {
	if u == nil {
		return Usage{}
	}
	out := Usage{
		PromptTokens:     u.PromptTokens,
		CompletionTokens: u.CompletionTokens,
		TotalTokens:      u.TotalTokens,
		Cost:             u.Cost,
	}
	if u.PromptDetails != nil {
		out.CachedTokens = u.PromptDetails.CachedTokens
		out.CacheWriteTokens = u.PromptDetails.CacheWriteTokens
	}
	for _, stu := range []*serverToolUse{u.ServerToolUseDetails, u.ServerToolUse} {
		if stu != nil && stu.WebSearchRequests > out.WebSearchRequests {
			out.WebSearchRequests = stu.WebSearchRequests
		}
	}
	return out
}

func appendCitations(dst []Citation, anns []wireAnnotation) []Citation {
	for _, a := range anns {
		if a.Type != "url_citation" || a.URLCitation == nil || a.URLCitation.URL == "" {
			continue
		}
		dst = append(dst, Citation{URL: a.URLCitation.URL, Title: a.URLCitation.Title, Content: a.URLCitation.Content})
	}
	return dst
}
