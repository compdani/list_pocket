package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gofrs/uuid/v5"
	"github.com/labstack/echo/v4"
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

type aiBuilderGenerateReq struct {
	Context        aiBuilderContext `json:"context"`
	Current        aiBuilderCurrent `json:"current"`
	Instructions   string           `json:"instructions"`
	SystemPrompt   string           `json:"systemPrompt,omitempty"`
	Model          string           `json:"model,omitempty"`
	TimeoutSeconds int              `json:"timeoutSeconds,omitempty"`
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
	if err := validateAIBuilderReq(req); err != nil {
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
		return cloneAIBuilderJob(job), nil
	case aiBuilderStatusRunning:
		if cancel, ok := s.cancels[id]; ok && cancel != nil {
			cancel()
		}
		job.Status = aiBuilderStatusCanceled
		job.Progress = 100
		job.Error = "generation cancelled"
		job.UpdatedAt = time.Now().UTC()
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
		return
	}
	job.Status = aiBuilderStatusSuccess
	job.Progress = 100
	job.Result = res
	job.Error = ""
	job.UpdatedAt = time.Now().UTC()
}

func (s *aiBuilderService) failJob(jobID string, err error) {
	if s.log != nil {
		s.log.Printf("ai builder job failed id=%s err=%v", jobID, err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	job, ok := s.jobs[jobID]
	if !ok {
		return
	}
	if job.Status == aiBuilderStatusCanceled {
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

func validateAIBuilderReq(req aiBuilderGenerateReq) error {
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
	return nil
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
	user := map[string]any{
		"context":      req.Context,
		"current":      req.Current,
		"instructions": req.Instructions,
		"rules": []string{
			"contentType must match the editor context unless there is a critical reason.",
			"For visual or grapes_mjml contentType, put source JSON/MJML in body.",
			"Return concise subject and preheader.",
		},
	}

	payload := map[string]any{
		"model": model,
		"messages": []map[string]string{
			{"role": "system", "content": system},
			{"role": "user", "content": toJSONString(user)},
		},
		"response_format": map[string]any{
			"type": "json_schema",
			"json_schema": map[string]any{
				"name":   "campaign_builder_result",
				"strict": true,
				"schema": map[string]any{
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
				},
			},
		},
	}

	b, _ := json.Marshal(payload)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/chat/completions", bytes.NewReader(b))
	if err != nil {
		return aiBuilderResult{}, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return aiBuilderResult{}, err
	}
	defer resp.Body.Close()

	out, _ := io.ReadAll(io.LimitReader(resp.Body, 1_000_000))
	if resp.StatusCode >= 300 {
		return aiBuilderResult{}, fmt.Errorf("provider returned %d: %s", resp.StatusCode, string(out))
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

	return aiBuilderResult{
		Subject:     subject,
		Preheader:   preheader,
		ContentType: ct,
		Body:        body,
		Notes:       "Generated with fallback provider because LISTPOCKET_AI_API_KEY is not configured.",
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

func (a *App) CreateAICampaignBuilderJob(c echo.Context) error {
	if a.aiBuilder == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "AI builder is unavailable")
	}

	var req aiBuilderGenerateReq
	if err := c.Bind(&req); err != nil {
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

	job, err := a.aiBuilder.Submit(req)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	return c.JSON(http.StatusOK, okResp{job})
}

func (a *App) GetAICampaignBuilderJob(c echo.Context) error {
	if a.aiBuilder == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "AI builder is unavailable")
	}
	jobID := strings.TrimSpace(c.Param("id"))
	if jobID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid ID")
	}

	job, ok := a.aiBuilder.Get(jobID)
	if !ok {
		return echo.NewHTTPError(http.StatusNotFound, "job not found")
	}
	return c.JSON(http.StatusOK, okResp{job})
}

func (a *App) CancelAICampaignBuilderJob(c echo.Context) error {
	if a.aiBuilder == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "AI builder is unavailable")
	}
	jobID := strings.TrimSpace(c.Param("id"))
	if jobID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid ID")
	}

	job, err := a.aiBuilder.Cancel(jobID)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "job not found")
	}
	return c.JSON(http.StatusOK, okResp{job})
}
