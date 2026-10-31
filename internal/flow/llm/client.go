package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

// Endpoint is a resolved provider: where to send requests and how to auth.
type Endpoint struct {
	Name    string
	BaseURL string // e.g. https://openrouter.ai/api/v1
	APIKey  string
	Headers map[string]string
	Prices  map[string]Price // per model, USD per 1M tokens
	Kind    string           // "" = Chat Completions; KindResponses = Responses API (ChatGPT plan)
}

type Price struct {
	Prompt     float64 `json:"prompt"`
	Completion float64 `json:"completion"`
}

// IsOpenRouter reports whether the endpoint is OpenRouter (enables its
// usage accounting and server tools).
func (e Endpoint) IsOpenRouter() bool {
	return strings.Contains(e.BaseURL, "openrouter.ai")
}

// Cost returns the USD cost of usage: provider-reported when available,
// else computed from the endpoint's price table, else 0.
func (e Endpoint) Cost(model string, u Usage) float64 {
	if u.Cost != nil {
		return *u.Cost
	}
	p, ok := e.Prices[model]
	if !ok {
		return 0
	}
	return (float64(u.PromptTokens)*p.Prompt + float64(u.CompletionTokens)*p.Completion) / 1_000_000
}

// StatusError is a non-2xx response from the provider.
type StatusError struct {
	Status int
	Body   string
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("provider returned %d: %s", e.Status, truncate(e.Body, 500))
}

func (e *StatusError) retryable() bool {
	return e.Status == http.StatusTooManyRequests || e.Status >= 500
}

type Client struct {
	HTTP *http.Client
	// Backoff between retries; defaults to 1s, 2s, 4s…
	Backoff func(attempt int) time.Duration
}

func NewClient() *Client {
	return &Client{HTTP: &http.Client{}}
}

// Complete sends one chat completion. onDelta, if set, receives streamed text
// as it arrives (streaming requests only). Retries 429/5xx/network errors
// that happen before any text was streamed.
func (c *Client) Complete(ctx context.Context, ep Endpoint, req Request, onDelta func(string)) (*Response, error) {
	var body []byte
	var err error
	if ep.Kind == KindResponses {
		body, err = buildResponsesBody(req)
	} else {
		body, err = buildBody(ep, req)
	}
	if err != nil {
		return nil, err
	}

	var lastErr error
	for attempt := 0; attempt <= req.Settings.Retries; attempt++ {
		if attempt > 0 {
			if err := sleep(ctx, c.backoff(attempt)); err != nil {
				return nil, err
			}
		}
		streamed := false
		wrapped := onDelta
		if onDelta != nil {
			wrapped = func(s string) { streamed = true; onDelta(s) }
		}
		resp, err := c.once(ctx, ep, req, body, wrapped)
		if err == nil {
			return resp, nil
		}
		lastErr = err
		if streamed || ctx.Err() != nil || !isRetryable(err) {
			return nil, err
		}
	}
	return nil, lastErr
}

func (c *Client) backoff(attempt int) time.Duration {
	if c.Backoff != nil {
		return c.Backoff(attempt)
	}
	return time.Duration(1<<(attempt-1)) * time.Second
}

func isRetryable(err error) bool {
	var se *StatusError
	if errors.As(err, &se) {
		return se.retryable()
	}
	var pe *providerError
	if errors.As(err, &pe) {
		return false
	}
	return true // network / decode errors
}

func sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func buildBody(ep Endpoint, req Request) ([]byte, error) {
	s := req.Settings
	body := map[string]any{
		"model":    req.Model,
		"messages": req.Messages,
	}
	if body["model"] == "" {
		body["model"] = s.Model
	}
	if len(req.Tools) > 0 {
		body["tools"] = req.Tools
		if s.ToolChoice != "" {
			body["tool_choice"] = s.ToolChoice
		}
		if s.ParallelToolCalls != nil {
			body["parallel_tool_calls"] = *s.ParallelToolCalls
		}
	}
	if s.Temperature != nil {
		body["temperature"] = *s.Temperature
	}
	if s.TopP != nil {
		body["top_p"] = *s.TopP
	}
	if s.MaxTokens > 0 {
		body["max_tokens"] = s.MaxTokens
	}
	if s.ReasoningEffort != "" {
		body["reasoning_effort"] = s.ReasoningEffort
	}
	if req.Stream {
		body["stream"] = true
		body["stream_options"] = map[string]any{"include_usage": true}
	}
	if ep.IsOpenRouter() {
		body["usage"] = map[string]any{"include": true}
		// Automatic caching: OpenRouter puts the breakpoint on the last
		// cacheable block and moves it as the conversation grows, so each
		// agent turn re-reads the previous turns at the cache-read price.
		// Providers that cache implicitly (OpenAI, Gemini, DeepSeek) ignore it.
		switch s.PromptCache {
		case "5m":
			body["cache_control"] = map[string]any{"type": "ephemeral"}
		case "1h":
			body["cache_control"] = map[string]any{"type": "ephemeral", "ttl": "1h"}
		}
	}
	for k, v := range s.ExtraBody {
		body[k] = v
	}
	for k, v := range req.Extra {
		body[k] = v
	}
	return json.Marshal(body)
}

func (c *Client) once(ctx context.Context, ep Endpoint, req Request, body []byte, onDelta func(string)) (*Response, error) {
	timeout := time.Duration(req.Settings.TimeoutS) * time.Second
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	url := completionsURL(ep.BaseURL)
	if ep.Kind == KindResponses {
		url = responsesURL(ep.BaseURL)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if ep.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+ep.APIKey)
	}
	if ep.IsOpenRouter() {
		httpReq.Header.Set("X-Title", "List Pocket")
	}
	for k, v := range ep.Headers {
		httpReq.Header.Set(k, v)
	}

	httpResp, err := c.HTTP.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode < 200 || httpResp.StatusCode > 299 {
		raw, _ := io.ReadAll(io.LimitReader(httpResp.Body, 64<<10))
		return nil, &StatusError{Status: httpResp.StatusCode, Body: string(raw)}
	}

	if ep.Kind == KindResponses {
		return readResponsesStream(httpResp.Body, onDelta) // this route always streams
	}
	if req.Stream {
		return readStream(httpResp.Body, onDelta)
	}
	return readJSON(httpResp.Body)
}

func completionsURL(base string) string {
	base = strings.TrimRight(base, "/")
	if strings.HasSuffix(base, "/chat/completions") {
		return base
	}
	return base + "/chat/completions"
}

// providerError is an error object returned inside a 200 response.
type providerError struct{ msg string }

func (e *providerError) Error() string { return "provider error: " + e.msg }

func readJSON(r io.Reader) (*Response, error) {
	var wr wireResponse
	if err := json.NewDecoder(r).Decode(&wr); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	if wr.Error != nil {
		return nil, &providerError{msg: wr.Error.Message}
	}
	out := &Response{Model: wr.Model, Usage: wr.Usage.toUsage()}
	if len(wr.Choices) == 0 || wr.Choices[0].Message == nil {
		return nil, errors.New("response has no choices")
	}
	ch := wr.Choices[0]
	out.Message.Role = "assistant"
	if ch.Message.Content != nil {
		out.Message.Content = *ch.Message.Content
	}
	out.Message.ToolCalls = ch.Message.ToolCalls
	out.Citations = appendCitations(nil, ch.Message.Annotations)
	if ch.FinishReason != nil {
		out.FinishReason = *ch.FinishReason
	}
	return out, nil
}

func readStream(r io.Reader, onDelta func(string)) (*Response, error) {
	out := &Response{Message: Message{Role: "assistant"}}
	var text strings.Builder
	calls := map[int]*ToolCall{}

	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64<<10), 8<<20)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, ":") { // SSE comments (e.g. OpenRouter keep-alives)
			continue
		}
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			break
		}
		var chunk wireResponse
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			return nil, fmt.Errorf("decoding stream chunk: %w", err)
		}
		if chunk.Error != nil {
			return nil, &providerError{msg: chunk.Error.Message}
		}
		if chunk.Model != "" {
			out.Model = chunk.Model
		}
		if chunk.Usage != nil {
			out.Usage = chunk.Usage.toUsage()
		}
		for _, ch := range chunk.Choices {
			if ch.FinishReason != nil && *ch.FinishReason != "" {
				out.FinishReason = *ch.FinishReason
			}
			d := ch.Delta
			if d == nil {
				continue
			}
			if d.Content != nil && *d.Content != "" {
				text.WriteString(*d.Content)
				if onDelta != nil {
					onDelta(*d.Content)
				}
			}
			out.Citations = appendCitations(out.Citations, d.Annotations)
			for _, tc := range d.ToolCalls {
				acc, ok := calls[tc.Index]
				if !ok {
					acc = &ToolCall{Type: "function"}
					calls[tc.Index] = acc
				}
				if tc.ID != "" {
					acc.ID = tc.ID
				}
				if tc.Function.Name != "" {
					acc.Function.Name += tc.Function.Name
				}
				acc.Function.Arguments += tc.Function.Arguments
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading stream: %w", err)
	}

	out.Message.Content = text.String()
	indexes := make([]int, 0, len(calls))
	for i := range calls {
		indexes = append(indexes, i)
	}
	sort.Ints(indexes)
	for _, i := range indexes {
		tc := calls[i]
		if tc.ID == "" {
			tc.ID = fmt.Sprintf("call_%d", i)
		}
		out.Message.ToolCalls = append(out.Message.ToolCalls, *tc)
	}
	return out, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
