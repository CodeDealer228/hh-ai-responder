package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
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
