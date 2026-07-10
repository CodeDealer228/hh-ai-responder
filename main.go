package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := parseConfig()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	logger = NewLogger(os.Stderr, parseLogLevel(cfg.LogLevel))

	if cfg.DebugSolveTests {
		runDebugSolveTests(ctx, cfg)
		return
	}

	if cfg.DebugEvalQA {
		aiBaseURL, aiModel, aiAPIKey := cfg.ActiveAI()
		ai := NewAIClient(ctx, aiBaseURL, aiModel, aiAPIKey, cfg.AITimeout, cfg.AIAttempts)
		if err := runDebugEvalQA(ai, cfg); err != nil {
			logger.Error("%v", err)
			os.Exit(1)
		}
		return
	}

	responder, err := NewHHAIResponder(ctx, cfg)
	if err != nil {
		logger.Error("%v", err)
		os.Exit(1)
	}

	if cfg.ListResumes {
		for _, res := range responder.resumes {
			fmt.Printf("%s\t%s\n", res.Hash, res.Title)
		}
		return
	}

	if cfg.DebugScrapeVacancies {
		if err := runDebugScrapeVacancies(responder, cfg.DebugScrapeLimit, cfg.DebugScrapeOutput); err != nil {
			logger.Error("%v", err)
			os.Exit(1)
		}
		return
	}

	responder.Run()
}

// runDebugSolveTests exercises the real SolveTests code path against a synthetic set of
// test questions, talking only to the configured AI backend — no hh.ru requests at all.
// Temporary manual-testing aid for the local-LLM migration; not wired into normal runs.
func runDebugSolveTests(ctx context.Context, cfg Config) {
	aiBaseURL, aiModel, aiAPIKey := cfg.ActiveAI()
	ai := NewAIClient(ctx, aiBaseURL, aiModel, aiAPIKey, cfg.AITimeout, cfg.AIAttempts)

	tasks := []Task{
		{ID: 1, Description: "Какой у вас опыт работы с большими языковыми моделями (LLM)?", Open: "true", CandidateSolutions: []Solution{}},
		{ID: 2, Description: "Какой фреймворк вы использовали для построения RAG-систем?", Open: "false", CandidateSolutions: []Solution{
			{ID: "101", Text: "LangChain"},
			{ID: "102", Text: "Ни один из перечисленных"},
			{ID: "103", Text: "LlamaIndex"},
			{ID: "104", Text: "Не работал с RAG"},
		}},
		{ID: 3, Description: "Готовы ли вы работать в офисе 5 дней в неделю?", Open: "false", CandidateSolutions: []Solution{
			{ID: "201", Text: "Да"},
			{ID: "202", Text: "Нет, только удаленно"},
			{ID: "203", Text: "Готов на гибридный формат"},
		}},
		{ID: 4, Description: "Опишите ваш опыт с векторными базами данных (Qdrant, Pinecone, Weaviate и т.п.)", Open: "true", CandidateSolutions: []Solution{}},
		{ID: 5, Description: "Какой у вас минимальный ожидаемый уровень заработной платы в месяц на руки?", Open: "true", CandidateSolutions: []Solution{}},
		{ID: 6, Description: "Какие языки программирования вы использовали в продакшене?", Open: "false", CandidateSolutions: []Solution{
			{ID: "301", Text: "Python"},
			{ID: "302", Text: "Go"},
			{ID: "303", Text: "Java"},
			{ID: "304", Text: "C++"},
		}},
		{ID: 7, Description: "Через какое время вы готовы приступить к работе?", Open: "true", CandidateSolutions: []Solution{}},
	}

	start := time.Now()
	solutions, err := ai.SolveTests(tasks, cfg.Contacts, cfg.ExtraTestSolutionPrompt)
	elapsed := time.Since(start)
	if err != nil {
		fmt.Fprintf(os.Stderr, "SolveTests failed after %s: %v\n", elapsed, err)
		os.Exit(1)
	}

	for _, task := range tasks {
		answer := solutions[task.ID]
		if answer.HasChoice {
			fmt.Printf("task %d [CHOICE]: solution_id=%d\n", task.ID, answer.SolutionID)
		} else {
			fmt.Printf("task %d [OPEN]: %s\n", task.ID, answer.TextSolution)
		}
	}
	fmt.Printf("\nOK: %d/%d answered in %s\n", len(solutions), len(tasks), elapsed)
}

// ScrapedVacancy is one entry in the -debug-scrape-vacancies output file.
type ScrapedVacancy struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Company     string `json:"company"`
	Area        string `json:"area"`
	Salary      string `json:"salary"`
	URL         string `json:"url"`
	Description string `json:"description"`
}

// runDebugScrapeVacancies walks the search results for responder's configured search URL
// (set via -u / HH_SEARCH_URL), fetches the full description for each vacancy, and writes
// everything to a JSON file. Read-only: it never responds to or applies for a vacancy, so
// it's unaffected by hh.ru's daily negotiations/response limit. Reuses the same
// authenticated session (cookies, rate-limited requester) as the real apply flow.
func runDebugScrapeVacancies(responder *HHAIResponder, limit int, outputPath string) error {
	var results []ScrapedVacancy
	seen := make(map[int]bool)

	for page := 0; len(results) < limit; page++ {
		if err := responder.ctx.Err(); err != nil {
			return err
		}

		vacancies, err := responder.fetchVacancyPage(page)
		if err != nil {
			return fmt.Errorf("fetch page %d: %w", page, err)
		}
		if len(vacancies) == 0 {
			break
		}

		for _, v := range vacancies {
			if len(results) >= limit {
				break
			}
			if seen[v.ID] {
				continue
			}
			seen[v.ID] = true

			description, err := responder.GetVacancyDescription(v.ID)
			if err != nil {
				logger.Warn("Failed to fetch description for vacancy %d: %v", v.ID, err)
				continue
			}

			results = append(results, ScrapedVacancy{
				ID:          v.ID,
				Name:        v.Name,
				Company:     v.Company.Name,
				Area:        v.Area.Name,
				Salary:      FormatCompensation(&v.Compensation),
				URL:         v.Links["desktop"],
				Description: description,
			})
			logger.Info("Scraped %d/%d: %s", len(results), limit, v.Name)
		}
	}

	data, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		return err
	}

	if err := os.WriteFile(outputPath, data, 0o644); err != nil {
		return err
	}

	logger.Info("Saved %d vacancies to %s", len(results), outputPath)
	return nil
}

// QATestEntry is one entry in a -debug-eval-qa input file (tests_qa.json), matching the
// shape produced from real ApplyResult.TestSolutions history.
type QATestEntry struct {
	VacancyID   int      `json:"vacancy_id"`
	VacancyName string   `json:"vacancy_name"`
	QA          []QAPair `json:"qa"`
}

// QAEvalResult is one answered question in a -debug-eval-qa output file.
type QAEvalResult struct {
	VacancyID   int    `json:"vacancy_id"`
	VacancyName string `json:"vacancy_name"`
	Question    string `json:"question"`
	Answer      string `json:"answer"`
	ElapsedMS   int64  `json:"elapsed_ms"`
	Error       string `json:"error,omitempty"`
}

// runDebugEvalQA replays every question from a QATestEntry JSON file (see QATestEntry)
// through the real open-question prompt (content/hh_test_open_question_prompt.txt) and
// the same contacts/preferences merging ApplyVacancyWithTest uses, so it reflects
// whatever that prompt currently says. Runs the whole set twice — once with reasoning
// left on (Ollama's default for reasoning-capable models when unset) and once with it
// explicitly disabled ("none", the production default) — writing each pass to its own
// JSON file for comparison. Talks only to the AI backend, never to hh.ru.
func runDebugEvalQA(ai *AIClient, cfg Config) error {
	data, err := os.ReadFile(cfg.DebugEvalQAInput)
	if err != nil {
		return err
	}

	var entries []QATestEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return fmt.Errorf("parse %s: %w", cfg.DebugEvalQAInput, err)
	}

	type job struct {
		vacancyID   int
		vacancyName string
		question    string
	}

	var jobs []job
	for _, e := range entries {
		for _, qa := range e.QA {
			if strings.TrimSpace(qa.Question) == "" {
				continue
			}
			jobs = append(jobs, job{e.VacancyID, e.VacancyName, qa.Question})
		}
	}

	logger.Info("Loaded %d questions from %s", len(jobs), cfg.DebugEvalQAInput)

	extraPrompt := strings.TrimSpace(cfg.ExtraTestSolutionPrompt)
	if wishes := loadTextFile(testWishesPath); wishes != "" {
		if extraPrompt != "" {
			extraPrompt += "\n\n"
		}
		extraPrompt += wishes
	}
	rules := commonTaskRules(cfg.Contacts, extraPrompt)
	systemPrompt := strings.Replace(loadTextFile(testOpenTaskPromptPath), "{{RULES}}", rules, 1)

	runPass := func(passName, reasoningEffort, outputPath string) error {
		results := make([]QAEvalResult, 0, len(jobs))
		for i, j := range jobs {
			start := time.Now()
			answer, err := ai.chat(systemPrompt, j.question, 200, 0.2, reasoningEffort)
			elapsed := time.Since(start)

			res := QAEvalResult{
				VacancyID:   j.vacancyID,
				VacancyName: j.vacancyName,
				Question:    j.question,
				Answer:      answer,
				ElapsedMS:   elapsed.Milliseconds(),
			}
			if err != nil {
				res.Error = err.Error()
				logger.Warn("[%s %d/%d] failed after %s: %v", passName, i+1, len(jobs), elapsed, err)
			} else {
				logger.Info("[%s %d/%d] answered in %s", passName, i+1, len(jobs), elapsed)
			}
			results = append(results, res)
		}

		out, err := json.MarshalIndent(results, "", "  ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(outputPath, out, 0o644); err != nil {
			return err
		}
		logger.Info("Pass %q: saved %d answers to %s", passName, len(results), outputPath)
		return nil
	}

	logger.Info("Pass 1/2: with reasoning")
	if err := runPass("with-reasoning", "", "qa_answers_with_reasoning.json"); err != nil {
		return fmt.Errorf("reasoning pass: %w", err)
	}

	logger.Info("Pass 2/2: without reasoning")
	if err := runPass("no-reasoning", "none", "qa_answers_no_reasoning.json"); err != nil {
		return fmt.Errorf("no-reasoning pass: %w", err)
	}

	return nil
}
