package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

const (
	acceptHeader         = "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7"
	acceptLanguageHeader = "ru-RU,ru;q=0.9,en-US;q=0.8,en;q=0.7"
	defaultHost          = "hh.ru"
	secCHUAHeader        = `"Chromium";v="149", "Google Chrome";v="149", "Not-A.Brand";v="99"`
	userAgent            = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/149.0.0.0 Safari/537.36"
	testWishesPath       = "content/hh_test_candidate_preferences.txt"
)

type HHAIResponder struct {
	ctx                     context.Context
	baseURL                 *url.URL
	searchParams            url.Values
	cookiesPath             string
	maxResponses            int
	client                  *http.Client
	jar                     *MemoryPersistentJar
	requester               *HHRequester
	resumeHash              string
	resumeExperience        string
	latestResumeHash        string
	resumes                 []ResumeItem
	userId                  int64
	firstName               string
	middleName              string
	lastName                string
	email                   string
	ai                      *AIClient
	extraLetterPrompt       string
	extraTestSolutionPrompt string
	contacts                string
	outputPath              string
	forceLetter             bool
	extraChatReplyPrompt    string
	chatURL                 string
	resumeProfileFrontURL   string
	ignoredChats            []int64
	questions               []string
	testWishes              string

	eventWriter io.Writer
	eventMu     sync.Mutex
}

func (r *HHAIResponder) getBaseHost() string {
	for domain, list := range r.jar.cookies {
		if domain == ".hh.ru" || strings.HasSuffix(domain, ".hh.ru") {
			for _, c := range list {
				if c.Name == "redirect_host" && c.Value != "" {
					return c.Value
				}
			}
		}
	}

	return defaultHost
}

func NewHHAIResponder(ctx context.Context, cfg Config) (*HHAIResponder, error) {
	var baseURL *url.URL
	var searchParams url.Values

	if strings.TrimSpace(cfg.SearchURL) != "" {
		parsed, err := url.Parse(cfg.SearchURL)
		if err != nil {
			return nil, err
		}
		if parsed.Scheme == "" || parsed.Host == "" {
			return nil, fmt.Errorf("invalid search URL: %s", cfg.SearchURL)
		}
		baseURL = &url.URL{Scheme: parsed.Scheme, Host: parsed.Host}
		q := parsed.Query()
		q.Del("page")
		searchParams = q
	}
	jar, err := NewMemoryPersistentJar(cfg.CookiesPath)
	if err != nil {
		return nil, err
	}

	client := &http.Client{
		Jar:     jar,
		Timeout: 30 * time.Second,
	}

	aiBaseURL, aiModel, aiAPIKey := cfg.ActiveAI()
	logger.Debug("AI provider: %s (base_url=%s, model=%s)", cfg.AIProvider, aiBaseURL, aiModel)

	responder := &HHAIResponder{
		ctx:                     ctx,
		baseURL:                 baseURL,
		cookiesPath:             cfg.CookiesPath,
		maxResponses:            cfg.MaxResponses,
		client:                  client,
		jar:                     jar,
		resumeHash:              cfg.Resume,
		ai:                      NewAIClient(ctx, aiBaseURL, aiModel, aiAPIKey, cfg.AITimeout, cfg.AIAttempts),
		extraLetterPrompt:       cfg.ExtraLetterPrompt,
		extraTestSolutionPrompt: cfg.ExtraTestSolutionPrompt,
		contacts:                cfg.Contacts,
		outputPath:              cfg.OutputPath,
		forceLetter:             cfg.ForceLetter,
		extraChatReplyPrompt:    cfg.ExtraChatReplyPrompt,
	}

	responder.requester = NewHHRequester(ctx, client, cfg.RequestInterval)

	// initialize event writer once
	var out io.Writer = os.Stdout
	if cfg.OutputPath != "" {
		f, err := os.OpenFile(cfg.OutputPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
		if err != nil {
			return nil, err
		}
		out = f
	}

	responder.eventWriter = out
	responder.searchParams = searchParams
	responder.questions = loadQuestions("content/chat_filler_messages.txt")
	responder.testWishes = loadTextFile(testWishesPath)

	if err := responder.LoadProfileData(); err != nil {
		return nil, err
	}

	logger.Debug("You are logged as: %s #%d", responder.GetFullName(), responder.userId)

	if responder.resumeHash == "" {
		responder.resumeHash = responder.latestResumeHash
	}

	resume := responder.GetCurrentResume()

	if resume == nil && responder.resumeHash != "" {
		if fallback, err := responder.FetchResumeSummary(responder.resumeHash); err == nil {
			responder.resumes = append(responder.resumes, *fallback)
			resume = responder.GetCurrentResume()
		} else {
			logger.Warn("Fallback resume fetch failed: %v", err)
		}
	}

	if resume == nil {
		return nil, errors.New("resume not found")
	}

	logger.Debug("Current resume hash=%s (%s)", responder.resumeHash, resume.Title)

	// If baseURL not provided via -u, resolve from redirect_host cookie for .hh.ru
	if responder.baseURL == nil {
		host := responder.getBaseHost()
		responder.baseURL = &url.URL{Scheme: "https", Host: host}
	}

	resumeExperience, err := responder.GetResumeExperience()
	if err != nil {
		return nil, errors.New("can't load resume experience")
	}
	responder.resumeExperience = resumeExperience

	// If no search params provided, add resume parameter
	if len(responder.searchParams) == 0 {
		responder.searchParams = make(url.Values)
		responder.searchParams.Set("resume", responder.resumeHash)
	}

	return responder, nil
}

// RefreshResumeData reloads title/skills/salary/experience for the active resume from
// hh.ru. Without this, resume edits made on the site are never picked up: this data is
// otherwise only fetched once in NewHHAIResponder and then cached in memory for the
// entire lifetime of the process (which can run for days under docker-compose).
func (r *HHAIResponder) RefreshResumeData() error {
	if err := r.LoadProfileData(); err != nil {
		return err
	}

	if r.resumeHash == "" {
		r.resumeHash = r.latestResumeHash
	}

	resume := r.GetCurrentResume()

	if resume == nil && r.resumeHash != "" {
		if fallback, err := r.FetchResumeSummary(r.resumeHash); err == nil {
			r.resumes = append(r.resumes, *fallback)
			resume = r.GetCurrentResume()
		} else {
			logger.Warn("Fallback resume fetch failed: %v", err)
		}
	}

	if resume == nil {
		return errors.New("resume not found")
	}

	resumeExperience, err := r.GetResumeExperience()
	if err != nil {
		return errors.New("can't load resume experience")
	}
	r.resumeExperience = resumeExperience

	return nil
}

func (r *HHAIResponder) writeEvent(v any) {
	r.eventMu.Lock()
	defer r.eventMu.Unlock()
	_ = json.NewEncoder(r.eventWriter).Encode(v)
}

func (r *HHAIResponder) ResolveURL(endpoint string) string {
	ref, err := url.Parse(endpoint)
	if err != nil {
		return endpoint
	}
	return r.baseURL.ResolveReference(ref).String()
}

// buildRequest creates an HTTP request with standard headers
func (r *HHAIResponder) buildRequest(method, endpoint string, body io.Reader, headers map[string]string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(r.ctx, method, r.ResolveURL(endpoint), body)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	// Standard headers
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept-Language", acceptLanguageHeader)
	req.Header.Set("Accept", acceptHeader)
	req.Header.Set("Sec-CH-UA", secCHUAHeader)
	req.Header.Set("Sec-CH-UA-Mobile", "?0")
	req.Header.Set("Sec-CH-UA-Platform", `"Windows"`)
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	req.Header.Set("Sec-Fetch-Mode", "cors")
	req.Header.Set("Sec-Fetch-Dest", "empty")

	// Additional headers
	for key, value := range headers {
		if value != "" {
			req.Header.Set(key, value)
		}
	}

	return req, nil
}

func (r *HHAIResponder) GetCurrentResume() *ResumeItem {
	for _, res := range r.resumes {
		if res.Hash == r.resumeHash {
			return &res
		}
	}
	return nil
}

func (r *HHAIResponder) GetFullName() string {
	return fmt.Sprintf("%s %s", r.firstName, r.lastName)
}

func (r *HHAIResponder) XSRFToken() string {
	for _, cookie := range r.jar.Cookies(r.baseURL) {
		if cookie.Name == "_xsrf" {
			return cookie.Value
		}
	}
	return ""
}

func (r *HHAIResponder) SaveCookies() error {
	return r.jar.Save(r.cookiesPath)
}

func (r *HHAIResponder) Run() {
	logger.Info("Starting tasks...")

	// Touch resume loop (every 4h after completion)
	go func() {
		for {
			select {
			case <-r.ctx.Done():
				return
			default:
			}

			updated, err := r.TouchResume()
			if err != nil {
				logger.Error("Touch resume error: %v", err)
			} else if updated {
				logger.Info("Resume updated")
			} else {
				logger.Warn("Resume not updated")
			}

			select {
			case <-r.ctx.Done():
				return
			case <-time.After(4 * time.Hour):
			}
		}
	}()

	go func() {
		for {
			select {
			case <-r.ctx.Done():
				return
			default:
			}

			success, _ := r.SetActiveJobSearchStatus()
			if success {
				logger.Info("Job search status is active")
			} else {
				logger.Warn("Can't change job search status")
			}

			select {
			case <-r.ctx.Done():
				return
			case <-time.After(24 * time.Hour):
			}
		}
	}()

	// Apply vacancies loop (every 24h after completion)
	go func() {
		for {
			select {
			case <-r.ctx.Done():
				return
			default:
			}

			if err := r.RefreshResumeData(); err != nil {
				logger.Error("Refresh resume data error: %v", err)
			}

			if err := r.ApplyVacancies(); err != nil {
				logger.Error("Apply error: %v", err)
			}

			select {
			case <-r.ctx.Done():
				return
			case <-time.After(12 * time.Hour):
			}
		}
	}()

	// Auto chat loop (every 15m after completion)
	go func() {
		for {
			select {
			case <-r.ctx.Done():
				return
			default:
			}

			if err := r.AutoRespondChats(); err != nil {
				logger.Error("Auto chat error: %v", err)
			}

			select {
			case <-r.ctx.Done():
				return
			case <-time.After(15 * time.Minute):
			}
		}
	}()

	// Block main until shutdown
	<-r.ctx.Done()
	logger.Info("Shutting down...")
}
