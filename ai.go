package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

const (
	aiRetryDelay        = 3 * time.Second
	chatCompletionsPath = "/v1/chat/completions"
	defaultGithubURL    = "https://github.com/s3rgeym"

	generateLetterPromptPath  = "content/ai_cover_letter_generation_prompt.txt"
	testCommonRulesPromptPath = "content/hh_test_common_rules_prompt.txt"
	testChoiceTaskPromptPath  = "content/hh_test_choice_question_prompt.txt"
	testOpenTaskPromptPath    = "content/hh_test_open_question_prompt.txt"
)

// letterArtifactRegexp catches leaked reasoning/planning text from "thinking" models
// that sometimes write out their scratch work instead of just the final letter.
var letterArtifactRegexp = regexp.MustCompile(`(?i)\bwe need to\b|\blet'?s craft\b|\bsentence \d+\s*:|\blet me\b|\bi need to\b`)

type AIClient struct {
	ctx      context.Context
	baseURL  string
	model    string
	apiKey   string
	attempts int
	client   *http.Client
}

type AIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatCompletionRequest struct {
	Model           string      `json:"model"`
	Messages        []AIMessage `json:"messages"`
	Stream          bool        `json:"stream"`
	MaxTokens       int         `json:"max_tokens,omitempty"`
	Temperature     float64     `json:"temperature,omitempty"`
	ReasoningEffort string      `json:"reasoning_effort,omitempty"`
}

type ChatCompletionResponse struct {
	Choices []ChatCompletionChoice `json:"choices"`
}

type ChatCompletionChoice struct {
	Message AIMessage `json:"message"`
}

func NewAIClient(ctx context.Context, baseURL, model, apiKey string, timeout time.Duration, attempts int) *AIClient {
	if !strings.Contains(baseURL, "://") {
		baseURL = "http://" + baseURL
	}
	return &AIClient{
		ctx:      ctx,
		baseURL:  strings.TrimRight(baseURL, "/"),
		model:    model,
		apiKey:   apiKey,
		attempts: attempts,
		client: &http.Client{
			Timeout: timeout,
		},
	}
}

func (c *AIClient) Chat(systemPrompt, userPrompt string, maxTokens int, temperature float64) (string, error) {
	payload := ChatCompletionRequest{
		Model:       c.model,
		Messages:    []AIMessage{{Role: "system", Content: systemPrompt}, {Role: "user", Content: userPrompt}},
		Stream:      false,
		MaxTokens:   maxTokens,
		Temperature: temperature,
		// Ollama's OpenAI-compat layer defaults reasoning-capable models to thinking=true
		// when this is unset, which burns the token budget on hidden reasoning and can
		// leave Content empty. Providers that don't support this field ignore it.
		ReasoningEffort: "none",
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	var lastErr error
	for attempt := 1; attempt <= c.attempts; attempt++ {
		result, err := c.getChatResponse(body)
		if err == nil {
			return result, nil
		}
		lastErr = err

		if attempt == c.attempts || c.ctx.Err() != nil {
			break
		}

		logger.Warn("AI request failed, retrying (%d/%d): %v", attempt, c.attempts, err)
		timer := time.NewTimer(aiRetryDelay)
		select {
		case <-timer.C:
		case <-c.ctx.Done():
			timer.Stop()
			return "", c.ctx.Err()
		}
	}

	return "", lastErr
}

func (c *AIClient) getChatResponse(body []byte) (string, error) {
	endpoint := c.baseURL + chatCompletionsPath
	logger.Debug("%s %s %s", http.MethodPost, endpoint, string(body))

	req, err := http.NewRequestWithContext(c.ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	logger.Debug("%d %s %s", resp.StatusCode, resp.Request.Method, resp.Request.URL.String())

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if err := c.ctx.Err(); err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("ai request failed: %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}

	var result ChatCompletionResponse
	if err := json.Unmarshal(data, &result); err != nil {
		return "", err
	}
	if len(result.Choices) == 0 {
		return "", errors.New("ai response has no choices")
	}

	return strings.TrimSpace(result.Choices[0].Message.Content), nil
}

func (c *AIClient) GenerateLetter(v Vacancy, vacancyDescription, fullName, resumeTitle, salary, experience, skills, contacts, extraPrompt string) (string, error) {
	if err := c.ctx.Err(); err != nil {
		return "", err
	}
	instructions := loadTextFile(generateLetterPromptPath)

	// Built via concatenation, not Sprintf, so that "%" in resume text (percentages,
	// metrics) can never be misparsed as a format verb and shift the arguments.
	systemPrompt := instructions + "\n\nТебя зовут: " + fullName +
		"\nТы ищешь работу в качестве: " + resumeTitle +
		"\nЗарплата: " + salary +
		"\nТвои навыки: " + skills +
		"\nТвой опыт:\n\n" + experience

	if strings.TrimSpace(contacts) != "" {
		systemPrompt += "\nКонтакты для указания в письме: " + contacts
	}

	if strings.TrimSpace(extraPrompt) != "" {
		systemPrompt += "\nДополнительные инструкции:\n" + extraPrompt
	}

	userPrompt := fmt.Sprintf(
		"Название вакансии: %s\nКомпания: %s\nОписание вакансии:\n%s",
		v.Name,
		v.Company.Name,
		vacancyDescription,
	)

	letter, err := c.Chat(systemPrompt, userPrompt, 300, 0.2)
	if err != nil {
		return "", err
	}
	if letterArtifactRegexp.MatchString(letter) {
		return "", fmt.Errorf("ai response looks like a leaked reasoning draft, not a letter: %q", letter)
	}

	return letter, nil
}

func (c *AIClient) SolveTests(tasks []Task, contacts, extraPrompt string) (map[int]SolutionFields, error) {
	if err := c.ctx.Err(); err != nil {
		return nil, err
	}
	if len(tasks) == 0 {
		return nil, nil
	}

	results := make(map[int]SolutionFields, len(tasks))
	for _, task := range tasks {
		if err := c.ctx.Err(); err != nil {
			return nil, err
		}

		solution, err := c.solveTask(task, contacts, extraPrompt)
		if err != nil {
			return nil, fmt.Errorf("task %d: %w", task.ID, err)
		}
		results[task.ID] = solution
	}

	return results, nil
}

// solveTask asks the AI about a single test question, one request per task instead of
// batching the whole test into one response. The caller already knows from
// task.CandidateSolutions whether this is a choice or a free-text question, so that
// branch is decided here in Go rather than left for the model to (re-)infer — small
// local models handle one unambiguous instruction far more reliably than picking the
// right one of two output schemas per task.
func (c *AIClient) solveTask(task Task, contacts, extraPrompt string) (SolutionFields, error) {
	if len(task.CandidateSolutions) > 0 {
		return c.solveChoiceTask(task, contacts, extraPrompt)
	}
	return c.solveOpenTask(task, contacts, extraPrompt)
}

func commonTaskRules(contacts, extraPrompt string) string {
	base := strings.ReplaceAll(loadTextFile(testCommonRulesPromptPath), "{{GITHUB_URL}}", defaultGithubURL)
	rules := "\n" + base
	if strings.TrimSpace(contacts) != "" {
		rules += "\n- Если попросят указать контакты, то используй:" + contacts
	}
	if strings.TrimSpace(extraPrompt) != "" {
		rules += "\n\nДополнительные инструкции:\n" + extraPrompt
	}
	return rules
}

func (c *AIClient) solveChoiceTask(task Task, contacts, extraPrompt string) (SolutionFields, error) {
	optionsJSON, err := json.Marshal(task.CandidateSolutions)
	if err != nil {
		return SolutionFields{}, err
	}

	systemPrompt := strings.Replace(loadTextFile(testChoiceTaskPromptPath), "{{RULES}}", commonTaskRules(contacts, extraPrompt), 1)

	userPrompt := fmt.Sprintf("Вопрос: %s\nВарианты ответа (JSON): %s", task.Description, string(optionsJSON))

	response, err := c.Chat(systemPrompt, userPrompt, 64, 0.2)
	if err != nil {
		return SolutionFields{}, err
	}

	var parsed struct {
		SolutionID *int `json:"solution_id"`
	}
	if err := parseJSON(response, &parsed); err != nil || parsed.SolutionID == nil {
		logger.Warn("AI returned invalid choice JSON for task %d: %s", task.ID, strings.TrimSpace(response))
		if err == nil {
			err = errors.New("missing solution_id")
		}
		return SolutionFields{}, err
	}

	return SolutionFields{SolutionID: *parsed.SolutionID, HasChoice: true}, nil
}

func (c *AIClient) solveOpenTask(task Task, contacts, extraPrompt string) (SolutionFields, error) {
	systemPrompt := strings.Replace(loadTextFile(testOpenTaskPromptPath), "{{RULES}}", commonTaskRules(contacts, extraPrompt), 1)

	response, err := c.Chat(systemPrompt, task.Description, 200, 0.2)
	if err != nil {
		return SolutionFields{}, err
	}

	return SolutionFields{TextSolution: strings.TrimSpace(response)}, nil
}

func parseJSON[T any](answer string, target *T) error {
	start := strings.Index(answer, "{")
	end := strings.LastIndex(answer, "}")

	if start == -1 || end == -1 || end < start {
		return errors.New("ai returned invalid JSON")
	}

	raw := answer[start : end+1]

	if err := json.Unmarshal([]byte(raw), target); err != nil {
		return fmt.Errorf("json unmarshal failed: %w; json=%s", err, raw)
	}

	return nil
}
