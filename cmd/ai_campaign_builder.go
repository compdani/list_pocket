package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/compdani/list_pocket/internal/apperr"
	pbcore "github.com/pocketbase/pocketbase/core"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gofrs/uuid/v5"
	"github.com/pocketbase/dbx"
)

const (
	aiBuilderStatusQueued   = "queued"
	aiBuilderStatusRunning  = "running"
	aiBuilderStatusSuccess  = "success"
	aiBuilderStatusFailed   = "failed"
	aiBuilderStatusCanceled = "canceled"
)

type aiBuilderContext struct {
	Origin       string `json:"origin"`
	CampaignType string `json:"campaignType"`
	TemplateType string `json:"templateType"`
	ContentType  string `json:"contentType"`
	EditorMode   string `json:"editorMode"`
}

type aiBuilderCurrent struct {
	Subject    string `json:"subject"`
	Preheader  string `json:"preheader"`
	Body       string `json:"body"`
	TemplateID string `json:"templateId"`
}

// aiBuilderStatusAttachment is optional context for the model (briefs, screenshots, PDFs, etc.).
// Binary payloads are base64 (no data: URL prefix). The server forwards them to OpenAI via the
// Responses API (input_file / input_image) without parsing document contents.
type aiBuilderStatusAttachment struct {
	Filename string `json:"filename"`
	MimeType string `json:"mimeType"`
	Data     string `json:"data"`
}

type aiBuilderGenerateReq struct {
	Context           aiBuilderContext            `json:"context"`
	Current           aiBuilderCurrent            `json:"current"`
	Instructions      string                      `json:"instructions"`
	StatusAttachments []aiBuilderStatusAttachment `json:"statusAttachments,omitempty"`
	SystemPrompt      string                      `json:"systemPrompt,omitempty"`
	Model             string                      `json:"model,omitempty"`
	TimeoutSeconds    int                         `json:"timeoutSeconds,omitempty"`
}

type aiBuilderResult struct {
	Subject     string `json:"subject"`
	Preheader   string `json:"preheader"`
	ContentType string `json:"contentType"`
	Body        string `json:"body"`
	Notes       string `json:"notes"`
}

type aiBuilderJob struct {
	ID         string           `json:"id"`
	Status     string           `json:"status"`
	Progress   int              `json:"progress"`
	Error      string           `json:"error,omitempty"`
	Context    aiBuilderContext `json:"context"`
	CreatedAt  time.Time        `json:"createdAt"`
	UpdatedAt  time.Time        `json:"updatedAt"`
	Result     aiBuilderResult  `json:"result"`
	RequestRaw json.RawMessage  `json:"-"`
}

type aiBuilderProvider interface {
	Generate(context.Context, aiBuilderGenerateReq) (aiBuilderResult, error)
}

type aiBuilderService struct {
	log      loggerLike
	provider aiBuilderProvider

	jobs    map[string]*aiBuilderJob
	cancels map[string]context.CancelFunc
	queue   chan string
	mu      sync.RWMutex
}

type loggerLike interface {
	Printf(format string, v ...any)
}

func (s *aiBuilderService) logf(format string, v ...any) {
	if s != nil && s.log != nil {
		s.log.Printf(format, v...)
	}
}

func newAIBuilderService(provider aiBuilderProvider, log loggerLike) *aiBuilderService {
	s := &aiBuilderService{
		log:      log,
		provider: provider,
		jobs:     map[string]*aiBuilderJob{},
		cancels:  map[string]context.CancelFunc{},
		queue:    make(chan string, 128),
	}
	go s.worker()
	return s
}

func (s *aiBuilderService) Submit(req aiBuilderGenerateReq) (*aiBuilderJob, error) {
	if err := validateAIBuilderReq(&req); err != nil {
		return nil, err
	}

	uu, err := uuid.NewV4()
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	raw, _ := json.Marshal(req)
	job := &aiBuilderJob{
		ID:         uu.String(),
		Status:     aiBuilderStatusQueued,
		Progress:   5,
		Context:    req.Context,
		CreatedAt:  now,
		UpdatedAt:  now,
		RequestRaw: raw,
	}

	s.mu.Lock()
	s.jobs[job.ID] = job
	s.mu.Unlock()

	s.logf("ai builder job queued id=%s origin=%s editor_mode=%s content_type=%s model=%s timeout_s=%d instructions_len=%d body_len=%d status_attachments=%d",
		job.ID,
		strings.TrimSpace(req.Context.Origin),
		strings.TrimSpace(req.Context.EditorMode),
		strings.TrimSpace(req.Context.ContentType),
		strings.TrimSpace(req.Model),
		req.TimeoutSeconds,
		len(req.Instructions),
		len(req.Current.Body),
		len(req.StatusAttachments),
	)

	select {
	case s.queue <- job.ID:
		return cloneAIBuilderJob(job), nil
	default:
		return nil, errors.New("AI generation queue is full, please try again")
	}
}

func (s *aiBuilderService) Get(id string) (*aiBuilderJob, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	job, ok := s.jobs[id]
	if !ok {
		return nil, false
	}
	return cloneAIBuilderJob(job), true
}

func (s *aiBuilderService) Cancel(id string) (*aiBuilderJob, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	job, ok := s.jobs[id]
	if !ok {
		return nil, errors.New("job not found")
	}

	switch job.Status {
	case aiBuilderStatusSuccess, aiBuilderStatusFailed, aiBuilderStatusCanceled:
		return cloneAIBuilderJob(job), nil
	case aiBuilderStatusQueued:
		job.Status = aiBuilderStatusCanceled
		job.Progress = 100
		job.Error = "generation cancelled"
		job.UpdatedAt = time.Now().UTC()
		s.logf("ai builder job canceled while queued id=%s", id)
		return cloneAIBuilderJob(job), nil
	case aiBuilderStatusRunning:
		if cancel, ok := s.cancels[id]; ok && cancel != nil {
			cancel()
		}
		job.Status = aiBuilderStatusCanceled
		job.Progress = 100
		job.Error = "generation cancelled"
		job.UpdatedAt = time.Now().UTC()
		s.logf("ai builder job cancel signal sent id=%s", id)
		return cloneAIBuilderJob(job), nil
	default:
		return cloneAIBuilderJob(job), nil
	}
}

func (s *aiBuilderService) worker() {
	for jobID := range s.queue {
		s.runJob(jobID)
	}
}

func (s *aiBuilderService) runJob(jobID string) {
	s.mu.Lock()
	job, ok := s.jobs[jobID]
	if !ok {
		s.mu.Unlock()
		return
	}
	if job.Status == aiBuilderStatusCanceled {
		s.mu.Unlock()
		return
	}
	job.Status = aiBuilderStatusRunning
	job.Progress = 30
	job.UpdatedAt = time.Now().UTC()
	rawReq := make([]byte, len(job.RequestRaw))
	copy(rawReq, job.RequestRaw)
	s.mu.Unlock()

	var req aiBuilderGenerateReq
	if err := json.Unmarshal(rawReq, &req); err != nil {
		s.failJob(jobID, fmt.Errorf("invalid job payload: %w", err))
		return
	}

	timeoutSeconds := req.TimeoutSeconds
	if timeoutSeconds <= 0 {
		timeoutSeconds = defaultAIBuilderTimeoutSeconds
	}
	s.logf("ai builder job running id=%s model=%s timeout_s=%d", jobID, strings.TrimSpace(req.Model), timeoutSeconds)
	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSeconds)*time.Second)
	s.mu.Lock()
	s.cancels[jobID] = cancel
	s.mu.Unlock()
	defer func() {
		cancel()
		s.mu.Lock()
		delete(s.cancels, jobID)
		s.mu.Unlock()
	}()

	res, err := s.provider.Generate(ctx, req)
	if err != nil {
		s.logf("ai builder provider error id=%s elapsed_ms=%d err=%v", jobID, time.Since(start).Milliseconds(), err)
		s.failJob(jobID, err)
		return
	}

	normalizeAIBuilderResult(&res, req)

	s.mu.Lock()
	defer s.mu.Unlock()
	job, ok = s.jobs[jobID]
	if !ok {
		return
	}
	if job.Status == aiBuilderStatusCanceled {
		s.logf("ai builder job ignored success after cancel id=%s elapsed_ms=%d", jobID, time.Since(start).Milliseconds())
		return
	}
	job.Status = aiBuilderStatusSuccess
	job.Progress = 100
	job.Result = res
	job.Error = ""
	job.UpdatedAt = time.Now().UTC()
	s.logf("ai builder job succeeded id=%s elapsed_ms=%d subject_len=%d preheader_len=%d body_len=%d",
		jobID,
		time.Since(start).Milliseconds(),
		len(job.Result.Subject),
		len(job.Result.Preheader),
		len(job.Result.Body),
	)
}

func (s *aiBuilderService) failJob(jobID string, err error) {
	s.logf("ai builder job failed id=%s err=%v", jobID, err)
	s.mu.Lock()
	defer s.mu.Unlock()
	job, ok := s.jobs[jobID]
	if !ok {
		return
	}
	if job.Status == aiBuilderStatusCanceled {
		s.logf("ai builder job already canceled id=%s", jobID)
		return
	}
	job.Status = aiBuilderStatusFailed
	job.Progress = 100
	job.Error = err.Error()
	job.UpdatedAt = time.Now().UTC()
}

func cloneAIBuilderJob(in *aiBuilderJob) *aiBuilderJob {
	if in == nil {
		return nil
	}
	out := *in
	out.RequestRaw = nil
	return &out
}

func validateAIBuilderReq(req *aiBuilderGenerateReq) error {
	req.Context.Origin = strings.TrimSpace(req.Context.Origin)
	req.Context.ContentType = strings.TrimSpace(req.Context.ContentType)
	req.Context.EditorMode = strings.TrimSpace(req.Context.EditorMode)
	req.Instructions = strings.TrimSpace(req.Instructions)
	if req.Context.Origin == "" {
		return errors.New("context.origin is required")
	}
	if req.Context.ContentType == "" {
		return errors.New("context.contentType is required")
	}
	if req.Context.EditorMode == "" {
		return errors.New("context.editorMode is required")
	}
	if req.Instructions == "" {
		return errors.New("instructions is required")
	}
	if len(req.Instructions) > 20000 {
		return errors.New("instructions too long")
	}
	if len(req.Current.Body) > 250000 {
		return errors.New("current content is too large")
	}
	return validateAIBuilderStatusAttachments(req)
}

const (
	aiBuilderMaxStatusAttachments      = 12
	aiBuilderMaxStatusAttachmentBytes  = 6 << 20  // per file
	aiBuilderMaxStatusAttachmentsTotal = 20 << 20 // decoded bytes across all files
)

func validateAIBuilderStatusAttachments(req *aiBuilderGenerateReq) error {
	if len(req.StatusAttachments) == 0 {
		return nil
	}
	if len(req.StatusAttachments) > aiBuilderMaxStatusAttachments {
		return fmt.Errorf("too many status attachments (max %d)", aiBuilderMaxStatusAttachments)
	}
	total := 0
	for i := range req.StatusAttachments {
		a := &req.StatusAttachments[i]
		a.Filename = filepath.Base(strings.TrimSpace(a.Filename))
		if a.Filename == "" || a.Filename == "." {
			return fmt.Errorf("status attachment %d: filename is required", i+1)
		}
		if len(a.Filename) > 240 {
			return fmt.Errorf("status attachment %d: filename too long", i+1)
		}
		raw := strings.TrimSpace(a.Data)
		raw = strings.ReplaceAll(raw, "\n", "")
		raw = strings.ReplaceAll(raw, "\r", "")
		a.Data = raw
		if raw == "" {
			return fmt.Errorf("status attachment %d: empty data", i+1)
		}
		decoded, err := base64.StdEncoding.DecodeString(raw)
		if err != nil {
			return fmt.Errorf("status attachment %d: invalid base64", i+1)
		}
		n := len(decoded)
		if n > aiBuilderMaxStatusAttachmentBytes {
			return fmt.Errorf("status attachment %d exceeds max size (%d MiB)", i+1, aiBuilderMaxStatusAttachmentBytes>>20)
		}
		total += n
		if total > aiBuilderMaxStatusAttachmentsTotal {
			return fmt.Errorf("status attachments exceed combined max size (%d MiB)", aiBuilderMaxStatusAttachmentsTotal>>20)
		}
		mime := normalizeAIBuilderStatusMIME(a.Filename, strings.TrimSpace(a.MimeType))
		if mime == "" {
			return fmt.Errorf("status attachment %q: unsupported type (use images, PDF, txt, rtf, or Word docx)", a.Filename)
		}
		a.MimeType = mime
	}
	return nil
}

func normalizeAIBuilderStatusMIME(filename, declared string) string {
	d := strings.TrimSpace(strings.ToLower(declared))
	switch d {
	case "image/jpeg", "image/jpg":
		return "image/jpeg"
	case "image/png", "image/gif", "image/webp":
		return d
	case "text/plain":
		return "text/plain"
	case "text/rtf", "application/rtf":
		return "application/rtf"
	case "application/pdf":
		return "application/pdf"
	case "application/msword":
		return "application/msword"
	case "application/vnd.openxmlformats-officedocument.wordprocessingml.document":
		return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	}
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".txt":
		return "text/plain"
	case ".rtf":
		return "application/rtf"
	case ".pdf":
		return "application/pdf"
	case ".doc":
		return "application/msword"
	case ".docx":
		return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	default:
		return ""
	}
}

func normalizeAIBuilderResult(res *aiBuilderResult, req aiBuilderGenerateReq) {
	res.ContentType = strings.TrimSpace(res.ContentType)
	if res.ContentType == "" {
		res.ContentType = req.Context.ContentType
	}
	res.Subject = strings.TrimSpace(res.Subject)
	res.Preheader = strings.TrimSpace(res.Preheader)
	res.Body = strings.TrimSpace(res.Body)
	if res.Body == "" {
		res.Body = strings.TrimSpace(req.Current.Body)
	}
}

type aiBuilderOpenAIProvider struct {
	apiKey  string
	model   string
	baseURL string
}

func newAIBuilderProvider() aiBuilderProvider {
	apiKey := strings.TrimSpace(os.Getenv("LISTPOCKET_AI_API_KEY"))
	if apiKey == "" {
		return aiBuilderFallbackProvider{}
	}
	model := strings.TrimSpace(os.Getenv("LISTPOCKET_AI_MODEL"))
	if model == "" {
		model = "gpt-5.4-mini"
	}

	baseURL := strings.TrimSpace(os.Getenv("LISTPOCKET_AI_BASE_URL"))
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	return &aiBuilderOpenAIProvider{apiKey: apiKey, model: model, baseURL: baseURL}
}

func aiBuilderOpenAIResultSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"subject":     map[string]any{"type": "string"},
			"preheader":   map[string]any{"type": "string"},
			"contentType": map[string]any{"type": "string"},
			"body":        map[string]any{"type": "string"},
			"notes":       map[string]any{"type": "string"},
		},
		"required":             []string{"subject", "preheader", "contentType", "body", "notes"},
		"additionalProperties": false,
	}
}

func aiBuilderOpenAIJSONResponseFormat() map[string]any {
	return map[string]any{
		"type": "json_schema",
		"json_schema": map[string]any{
			"name":   "campaign_builder_result",
			"strict": true,
			"schema": aiBuilderOpenAIResultSchema(),
		},
	}
}

// aiBuilderOpenAIResponsesTextFormat is the text.format object for POST /v1/responses.
// Responses expect name/schema on format itself; chat/completions nests them under json_schema.
func aiBuilderOpenAIResponsesTextFormat() map[string]any {
	return map[string]any{
		"type":   "json_schema",
		"name":   "campaign_builder_result",
		"strict": true,
		"schema": aiBuilderOpenAIResultSchema(),
	}
}

func aiBuilderUserPayload(req aiBuilderGenerateReq) map[string]any {
	return map[string]any{
		"context":      req.Context,
		"current":      req.Current,
		"instructions": req.Instructions,
		"rules": []string{
			"contentType must match the editor context unless there is a critical reason.",
			"For visual or grapes_mjml contentType, put source JSON/MJML in body.",
			"Return concise subject and preheader.",
			"Reference files (if any) are separate from campaign image URLs; use them only as context.",
		},
	}
}

func openAIDataURL(mime, b64 string) string {
	return "data:" + mime + ";base64," + strings.TrimSpace(b64)
}

func (p *aiBuilderOpenAIProvider) Generate(ctx context.Context, req aiBuilderGenerateReq) (aiBuilderResult, error) {
	system := strings.TrimSpace(req.SystemPrompt)
	if system == "" {
		system = defaultAIBuilderSystemPrompt
	}
	model := strings.TrimSpace(req.Model)
	if model == "" {
		model = strings.TrimSpace(p.model)
	}
	if model == "" {
		model = defaultAIBuilderModel
	}
	start := time.Now()
	if len(req.StatusAttachments) > 0 {
		return p.generateOpenAIResponses(ctx, req, system, model, start)
	}
	return p.generateOpenAIChatCompletions(ctx, req, system, model, start)
}

func (p *aiBuilderOpenAIProvider) generateOpenAIChatCompletions(ctx context.Context, req aiBuilderGenerateReq, system, model string, start time.Time) (aiBuilderResult, error) {
	user := aiBuilderUserPayload(req)
	payload := map[string]any{
		"model": model,
		"messages": []map[string]string{
			{"role": "system", "content": system},
			{"role": "user", "content": toJSONString(user)},
		},
		"response_format": aiBuilderOpenAIJSONResponseFormat(),
	}
	return p.doOpenAIChatCompletion(ctx, payload, start)
}

func (p *aiBuilderOpenAIProvider) doOpenAIChatCompletion(ctx context.Context, payload map[string]any, start time.Time) (aiBuilderResult, error) {
	b, _ := json.Marshal(payload)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimSuffix(p.baseURL, "/")+"/chat/completions", bytes.NewReader(b))
	if err != nil {
		return aiBuilderResult{}, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || ctx.Err() == context.DeadlineExceeded {
			return aiBuilderResult{}, fmt.Errorf("provider timeout after %dms: %w", time.Since(start).Milliseconds(), err)
		}
		if errors.Is(err, context.Canceled) || ctx.Err() == context.Canceled {
			return aiBuilderResult{}, fmt.Errorf("provider request canceled after %dms: %w", time.Since(start).Milliseconds(), err)
		}
		return aiBuilderResult{}, err
	}
	defer resp.Body.Close()

	out, _ := io.ReadAll(io.LimitReader(resp.Body, 1_000_000))
	if resp.StatusCode >= 300 {
		return aiBuilderResult{}, fmt.Errorf("provider returned %d after %dms: %s", resp.StatusCode, time.Since(start).Milliseconds(), string(out))
	}

	var parsed struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(out, &parsed); err != nil {
		return aiBuilderResult{}, err
	}
	if len(parsed.Choices) == 0 {
		return aiBuilderResult{}, errors.New("provider returned no choices")
	}

	var result aiBuilderResult
	if err := json.Unmarshal([]byte(parsed.Choices[0].Message.Content), &result); err != nil {
		return aiBuilderResult{}, fmt.Errorf("invalid provider JSON: %w", err)
	}
	return result, nil
}

func (p *aiBuilderOpenAIProvider) generateOpenAIResponses(ctx context.Context, req aiBuilderGenerateReq, system, model string, start time.Time) (aiBuilderResult, error) {
	user := aiBuilderUserPayload(req)
	textPayload := toJSONString(user)
	parts := []any{
		map[string]any{"type": "input_text", "text": textPayload},
	}
	for _, att := range req.StatusAttachments {
		filename := att.Filename
		if filename == "" {
			filename = "attachment"
		}
		mime := att.MimeType
		if strings.HasPrefix(mime, "image/") {
			// Responses API expects image_url as a string (URL or data: URL), not { "url": "..." }.
			parts = append(parts, map[string]any{
				"type":      "input_image",
				"image_url": openAIDataURL(mime, att.Data),
			})
			continue
		}
		parts = append(parts, map[string]any{
			"type":      "input_file",
			"filename":  filename,
			"file_data": openAIDataURL(mime, att.Data),
		})
	}

	payload := map[string]any{
		"model":        model,
		"instructions": system,
		"input": []any{
			map[string]any{
				"role":    "user",
				"content": parts,
			},
		},
		"text": map[string]any{
			"format": aiBuilderOpenAIResponsesTextFormat(),
		},
	}

	b, _ := json.Marshal(payload)
	url := strings.TrimSuffix(p.baseURL, "/") + "/responses"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return aiBuilderResult{}, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || ctx.Err() == context.DeadlineExceeded {
			return aiBuilderResult{}, fmt.Errorf("provider timeout after %dms: %w", time.Since(start).Milliseconds(), err)
		}
		if errors.Is(err, context.Canceled) || ctx.Err() == context.Canceled {
			return aiBuilderResult{}, fmt.Errorf("provider request canceled after %dms: %w", time.Since(start).Milliseconds(), err)
		}
		return aiBuilderResult{}, err
	}
	defer resp.Body.Close()

	out, _ := io.ReadAll(io.LimitReader(resp.Body, 2_000_000))
	if resp.StatusCode >= 300 {
		return aiBuilderResult{}, fmt.Errorf("responses API returned %d after %dms: %s (reference files need a provider that supports POST /v1/responses)", resp.StatusCode, time.Since(start).Milliseconds(), string(out))
	}

	content, err := parseOpenAIResponsesOutputText(out)
	if err != nil {
		return aiBuilderResult{}, err
	}
	var result aiBuilderResult
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return aiBuilderResult{}, fmt.Errorf("invalid provider JSON: %w", err)
	}
	return result, nil
}

func parseOpenAIResponsesOutputText(body []byte) (string, error) {
	var envelope struct {
		Output     []map[string]any `json:"output"`
		OutputText string           `json:"output_text"`
		Error      *struct {
			Message string `json:"message"`
			Type    string `json:"type"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return "", err
	}
	if envelope.Error != nil && strings.TrimSpace(envelope.Error.Message) != "" {
		return "", errors.New(envelope.Error.Message)
	}
	if t := strings.TrimSpace(envelope.OutputText); t != "" {
		return t, nil
	}
	for _, item := range envelope.Output {
		if item == nil || item["type"] != "message" {
			continue
		}
		content, _ := item["content"].([]any)
		for _, block := range content {
			cm, ok := block.(map[string]any)
			if !ok {
				continue
			}
			if cm["type"] != "output_text" {
				continue
			}
			if text, ok := cm["text"].(string); ok && strings.TrimSpace(text) != "" {
				return strings.TrimSpace(text), nil
			}
		}
	}
	return "", errors.New("no model output text in Responses API payload")
}

type aiBuilderFallbackProvider struct{}

func (aiBuilderFallbackProvider) Generate(_ context.Context, req aiBuilderGenerateReq) (aiBuilderResult, error) {
	ct := strings.TrimSpace(req.Context.ContentType)
	if ct == "" {
		ct = "html"
	}
	body := strings.TrimSpace(req.Current.Body)
	if body == "" {
		body = "<p>Hello {{ .Subscriber.Email }},</p><p>We're excited to share this update with you.</p><p>{{ template \"content\" . }}</p>"
	}
	subject := strings.TrimSpace(req.Current.Subject)
	if subject == "" {
		subject = "Your next update is ready"
	}
	preheader := strings.TrimSpace(req.Current.Preheader)
	if preheader == "" {
		preheader = "A quick preview of what is inside."
	}

	notes := "Generated with fallback provider because LISTPOCKET_AI_API_KEY is not configured."
	if len(req.StatusAttachments) > 0 {
		notes += " Reference files were not sent to the model."
	}
	return aiBuilderResult{
		Subject:     subject,
		Preheader:   preheader,
		ContentType: ct,
		Body:        body,
		Notes:       notes,
	}, nil
}

func toJSONString(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

const (
	defaultAIBuilderSystemPrompt     = "You generate email campaign drafts. Return strict JSON only with keys: subject, preheader, contentType, body, notes."
	defaultAIBuilderModel            = "gpt-5.4-mini"
	defaultAIBuilderTimeoutSeconds   = 180
	aiBuilderSystemMessageCollection = "ai_builder_system_messages"
)

func (a *App) getAIBuilderSystemPrompt(editorMode string) string {
	mode := strings.TrimSpace(editorMode)
	if mode == "" || a == nil || a.pb == nil {
		return defaultAIBuilderSystemPrompt
	}

	rec, err := a.pb.FindFirstRecordByFilter(
		aiBuilderSystemMessageCollection,
		"editor_mode={:mode}",
		dbx.Params{"mode": mode},
	)
	if err != nil || rec == nil {
		return defaultAIBuilderSystemPrompt
	}

	msg := strings.TrimSpace(rec.GetString("prompt"))
	if msg == "" {
		return defaultAIBuilderSystemPrompt
	}
	return msg
}

func (a *App) CreateAICampaignBuilderJob(re *pbcore.RequestEvent) error {
	if a.aiBuilder == nil {
		return apperr.New(http.StatusServiceUnavailable, "AI builder is unavailable")
	}

	var req aiBuilderGenerateReq
	if err := bindJSON(re, &req); err != nil {
		return err
	}
	req.Context.EditorMode = strings.TrimSpace(req.Context.EditorMode)
	if req.Context.EditorMode == "" {
		req.Context.EditorMode = strings.TrimSpace(req.Context.ContentType)
	}
	req.SystemPrompt = a.getAIBuilderSystemPrompt(req.Context.EditorMode)
	model, timeout, _ := a.getAIBuilderSettingsValues()
	req.Model = model
	req.TimeoutSeconds = timeout
	if a.log != nil {
		a.log.Printf("ai builder create request origin=%s editor_mode=%s content_type=%s model=%s timeout_s=%d",
			req.Context.Origin, req.Context.EditorMode, req.Context.ContentType, req.Model, req.TimeoutSeconds)
	}

	job, err := a.aiBuilder.Submit(req)
	if err != nil {
		return apperr.BadRequest(err.Error())
	}

	return okJSON(re, job)
}

func (a *App) GetAICampaignBuilderJob(re *pbcore.RequestEvent) error {
	if a.aiBuilder == nil {
		return apperr.New(http.StatusServiceUnavailable, "AI builder is unavailable")
	}
	jobID := strings.TrimSpace(pathParam(re, "id"))
	if jobID == "" {
		return apperr.BadRequest("invalid ID")
	}

	job, ok := a.aiBuilder.Get(jobID)
	if !ok {
		return apperr.NotFound("job not found")
	}
	return okJSON(re, job)
}

func (a *App) StreamAICampaignBuilderJob(re *pbcore.RequestEvent) error {
	if a.aiBuilder == nil {
		return apperr.New(http.StatusServiceUnavailable, "AI builder is unavailable")
	}

	jobID := strings.TrimSpace(pathParam(re, "id"))
	if jobID == "" {
		return apperr.BadRequest("invalid ID")
	}

	if _, ok := a.aiBuilder.Get(jobID); !ok {
		return apperr.NotFound("job not found")
	}

	res := re.Response
	req := re.Request
	res.Header().Set("Content-Type", "text/event-stream")
	res.Header().Set("Cache-Control", "no-cache")
	res.Header().Set("Connection", "keep-alive")
	res.WriteHeader(http.StatusOK)
	flushResponse(res)

	lastSig := ""
	lastHeartbeat := time.Now()
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	sendJob := func(job *aiBuilderJob) error {
		payload, err := json.Marshal(job)
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintf(res, "event: job\ndata: %s\n\n", payload); err != nil {
			return err
		}
		flushResponse(res)
		return nil
	}

	for {
		select {
		case <-req.Context().Done():
			return nil
		case <-ticker.C:
			job, ok := a.aiBuilder.Get(jobID)
			if !ok {
				return nil
			}

			sig := fmt.Sprintf("%s|%d|%s", job.Status, job.Progress, strings.TrimSpace(job.Error))
			if job.Status == aiBuilderStatusSuccess {
				sig = sig + fmt.Sprintf("|%d", len(job.Result.Body))
			}
			if sig != lastSig {
				lastSig = sig
				if err := sendJob(job); err != nil {
					return nil
				}
			}

			// Keep connection alive for proxies that close idle streams.
			if time.Since(lastHeartbeat) >= 15*time.Second {
				if _, err := fmt.Fprint(res, ": keep-alive\n\n"); err != nil {
					return nil
				}
				flushResponse(res)
				lastHeartbeat = time.Now()
			}

			if job.Status == aiBuilderStatusSuccess || job.Status == aiBuilderStatusFailed || job.Status == aiBuilderStatusCanceled {
				if _, err := fmt.Fprint(res, "event: done\ndata: {}\n\n"); err != nil {
					return nil
				}
				flushResponse(res)
				return nil
			}
		}
	}
}

func (a *App) CancelAICampaignBuilderJob(re *pbcore.RequestEvent) error {
	if a.aiBuilder == nil {
		return apperr.New(http.StatusServiceUnavailable, "AI builder is unavailable")
	}
	jobID := strings.TrimSpace(pathParam(re, "id"))
	if jobID == "" {
		return apperr.BadRequest("invalid ID")
	}

	job, err := a.aiBuilder.Cancel(jobID)
	if err != nil {
		return apperr.NotFound("job not found")
	}
	return okJSON(re, job)
}
