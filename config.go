package main

import (
	"bufio"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	defaultAIAttempts      = 2
	defaultAITimeout       = 45 * time.Second
	defaultRequestInterval = 1200 * time.Millisecond
	defaultWorkers         = 2

	defaultOllamaBaseURL     = "http://localhost:11434"
	defaultOllamaModel       = "llama3:8b"
	defaultOpenRouterBaseURL = "https://openrouter.ai/api"
	defaultOpenRouterModel   = "openrouter/free"
	// Polza.ai docs (quickstart) give the full endpoint as
	// https://polza.ai/api/v1/chat/completions — chatCompletionsPath (ai.go) already
	// appends "/v1/chat/completions", so the base URL here stops at "/api".
	defaultPolzaBaseURL = "https://polza.ai/api"
	defaultPolzaModel   = "deepseek/deepseek-v4-flash"
)

type Config struct {
	SearchURL    string
	CookiesPath  string
	LogLevel     string
	Resume       string
	MaxResponses int

	// AIProvider selects which of the three backends below is actually used: "ollama"
	// (default, matches the project's original local-first behavior), "openrouter" or
	// "polza".
	AIProvider        string
	OllamaBaseURL     string
	OllamaModel       string
	OllamaAPIKey      string
	OpenRouterBaseURL string
	OpenRouterModel   string
	OpenRouterAPIKey  string
	PolzaBaseURL         string
	PolzaModel           string
	PolzaAPIKey          string
	PolzaReasoningEffort string

	AITimeout               time.Duration
	AIAttempts              int
	ExtraLetterPrompt       string
	ExtraTestSolutionPrompt string
	RequestInterval         time.Duration
	OutputPath              string
	Contacts                string
	ListResumes             bool
	ForceLetter             bool
	ExtraChatReplyPrompt    string
	DebugSolveTests         bool
	DebugScrapeVacancies    bool
	DebugScrapeLimit        int
	DebugScrapeOutput       string
	DebugEvalQA             bool
	DebugEvalQAInput        string
}

// ActiveAI resolves which provider name/base URL/model/API key/reasoning effort to use
// based on AIProvider. Unknown or empty AIProvider falls back to Ollama, matching the
// pre-routing default. Ollama and OpenRouter always get reasoning explicitly disabled
// ("none" — see AIClient.chat for why); Polza's reasoning effort is configurable since
// its models don't share Ollama's "defaults to thinking" problem.
func (cfg Config) ActiveAI() (provider, baseURL, model, apiKey, reasoningEffort string) {
	switch strings.ToLower(strings.TrimSpace(cfg.AIProvider)) {
	case "openrouter":
		return "openrouter", cfg.OpenRouterBaseURL, cfg.OpenRouterModel, cfg.OpenRouterAPIKey, "none"
	case "polza":
		effort := cfg.PolzaReasoningEffort
		if effort == "" {
			effort = "none"
		}
		return "polza", cfg.PolzaBaseURL, cfg.PolzaModel, cfg.PolzaAPIKey, effort
	default:
		return "ollama", cfg.OllamaBaseURL, cfg.OllamaModel, cfg.OllamaAPIKey, "none"
	}
}

func parseConfig() (Config, error) {
	wd, err := os.Getwd()
	if err != nil {
		wd = "."
	}

	cfg := Config{}

	flag.StringVar(&cfg.SearchURL, "u", "", "URL для поиска вакансий")
	flag.StringVar(&cfg.CookiesPath, "c", filepath.Join(wd, "cookies.txt"), "Путь к файлу cookies")
	flag.StringVar(&cfg.LogLevel, "l", "info", "Уровень логирования: debug, info, warn, error")
	flag.StringVar(&cfg.Resume, "r", "", "ID резюме (если не указан — используется последнее)")
	flag.StringVar(&cfg.OutputPath, "o", "", "Файл для вывода результатов (по умолчанию — в STDOUT)")
	flag.IntVar(&cfg.MaxResponses, "mr", 0, "Пропускать вакансии с количеством откликов больше N")
	flag.BoolVar(&cfg.ListResumes, "R", false, "Показать список резюме и выйти")
	flag.BoolVar(&cfg.ForceLetter, "force-letter", false, "Всегда генерировать сопроводительное письмо")
	flag.DurationVar(&cfg.AITimeout, "ai-timeout", defaultAITimeout, "Таймаут AI-запросов")
	flag.DurationVar(&cfg.RequestInterval, "request-interval", defaultRequestInterval, "Минимальный интервал между запросами к hh.ru")
	flag.IntVar(&cfg.AIAttempts, "ai-attempts", defaultAIAttempts, "Количество попыток отправить запрос к ИИ")
	flag.StringVar(&cfg.AIProvider, "ai-provider", "ollama", "Провайдер ИИ: ollama, openrouter или polza")
	flag.StringVar(&cfg.OllamaBaseURL, "ollama-base-url", defaultOllamaBaseURL, "Базовый URL локальной Ollama")
	flag.StringVar(&cfg.OllamaModel, "ollama-model", defaultOllamaModel, "Модель Ollama")
	flag.StringVar(&cfg.OllamaAPIKey, "ollama-api-key", "", "API-ключ Ollama (обычно не требуется)")
	flag.StringVar(&cfg.OpenRouterBaseURL, "openrouter-base-url", defaultOpenRouterBaseURL, "Базовый URL OpenRouter")
	flag.StringVar(&cfg.OpenRouterModel, "openrouter-model", defaultOpenRouterModel, "Модель OpenRouter (например, openrouter/free)")
	flag.StringVar(&cfg.OpenRouterAPIKey, "openrouter-api-key", "", "API-ключ OpenRouter")
	flag.StringVar(&cfg.PolzaBaseURL, "polza-base-url", defaultPolzaBaseURL, "Базовый URL Polza.ai")
	flag.StringVar(&cfg.PolzaModel, "polza-model", defaultPolzaModel, "Модель Polza.ai (формат provider/model, например deepseek/deepseek-v4-flash)")
	flag.StringVar(&cfg.PolzaAPIKey, "polza-api-key", "", "API-ключ Polza.ai")
	flag.StringVar(&cfg.PolzaReasoningEffort, "polza-reasoning-effort", "", "reasoning_effort для Polza.ai (например, minimal/low/medium/high); пусто = none")
	flag.StringVar(&cfg.Contacts, "contacts", "", "Контакты для передачи работодателю")
	flag.StringVar(&cfg.ExtraTestSolutionPrompt, "solution-prompt", "", "Дополнительный промпт для решения тестов при отклике")
	flag.StringVar(&cfg.ExtraChatReplyPrompt, "chat-reply-prompt", "", "Дополнительный промпт для сообщений в чатах с работодателями")
	flag.StringVar(&cfg.ExtraLetterPrompt, "letter-prompt", "", "Дополнительный промпт для сопроводительного письма")
	flag.BoolVar(&cfg.DebugSolveTests, "debug-solve-tests", false, "Прогнать SolveTests на синтетическом наборе задач и выйти, не трогая hh.ru")
	flag.BoolVar(&cfg.DebugScrapeVacancies, "debug-scrape-vacancies", false, "Собрать описания вакансий по поисковому запросу (-u) в JSON-файл и выйти, не откликаясь")
	flag.IntVar(&cfg.DebugScrapeLimit, "debug-scrape-limit", 80, "Максимум вакансий для сбора в режиме -debug-scrape-vacancies")
	flag.StringVar(&cfg.DebugScrapeOutput, "debug-scrape-output", "vacancies.json", "Файл для сохранения собранных вакансий в режиме -debug-scrape-vacancies")
	flag.BoolVar(&cfg.DebugEvalQA, "debug-eval-qa", false, "Прогнать вопросы из JSON-файла (-debug-eval-qa-input) через промпт открытых вопросов дважды (с reasoning и без), сохранить ответы, не трогая hh.ru")
	flag.StringVar(&cfg.DebugEvalQAInput, "debug-eval-qa-input", "tests_qa.json", "Входной JSON-файл для режима -debug-eval-qa")
	flag.Parse()

	_ = loadDotEnv(".env")

	flags := map[string]bool{}
	flag.Visit(func(f *flag.Flag) {
		flags[f.Name] = true
	})

	if !flags["u"] {
		cfg.SearchURL = getEnv("HH_SEARCH_URL", cfg.SearchURL)
	}
	if !flags["r"] {
		cfg.Resume = getEnv("HH_RESUME", cfg.Resume)
	}
	if !flags["ai-provider"] {
		cfg.AIProvider = getEnv("HH_AI_PROVIDER", cfg.AIProvider)
	}
	if !flags["ollama-base-url"] {
		cfg.OllamaBaseURL = getEnv("HH_OLLAMA_BASE_URL", cfg.OllamaBaseURL)
	}
	if !flags["ollama-model"] {
		cfg.OllamaModel = getEnv("HH_OLLAMA_MODEL", cfg.OllamaModel)
	}
	if !flags["ollama-api-key"] {
		cfg.OllamaAPIKey = getEnv("HH_OLLAMA_API_KEY", cfg.OllamaAPIKey)
	}
	if !flags["openrouter-base-url"] {
		cfg.OpenRouterBaseURL = getEnv("HH_OPENROUTER_BASE_URL", cfg.OpenRouterBaseURL)
	}
	if !flags["openrouter-model"] {
		cfg.OpenRouterModel = getEnv("HH_OPENROUTER_MODEL", cfg.OpenRouterModel)
	}
	if !flags["openrouter-api-key"] {
		cfg.OpenRouterAPIKey = getEnv("HH_OPENROUTER_API_KEY", cfg.OpenRouterAPIKey)
	}
	if !flags["polza-base-url"] {
		cfg.PolzaBaseURL = getEnv("HH_POLZA_BASE_URL", cfg.PolzaBaseURL)
	}
	if !flags["polza-model"] {
		cfg.PolzaModel = getEnv("HH_POLZA_MODEL", cfg.PolzaModel)
	}
	if !flags["polza-api-key"] {
		cfg.PolzaAPIKey = getEnv("HH_POLZA_API_KEY", cfg.PolzaAPIKey)
	}
	if !flags["polza-reasoning-effort"] {
		cfg.PolzaReasoningEffort = getEnv("HH_POLZA_REASONING_EFFORT", cfg.PolzaReasoningEffort)
	}
	// HH_COOKIE_FILENAME overrides just the filename (resolved against the working
	// directory), so switching accounts doesn't require passing the full -c path.
	if !flags["c"] {
		if filename := getEnv("HH_COOKIE_FILENAME", ""); filename != "" {
			cfg.CookiesPath = filepath.Join(wd, filename)
		}
	}
	if !flags["letter-prompt"] {
		cfg.ExtraLetterPrompt = getEnv("HH_LETTER_PROMPT", cfg.ExtraLetterPrompt)
	}
	if !flags["solution-prompt"] {
		cfg.ExtraTestSolutionPrompt = getEnv("HH_SOLUTION_PROMPT", cfg.ExtraTestSolutionPrompt)
	}
	if !flags["chat-reply-prompt"] {
		cfg.ExtraChatReplyPrompt = getEnv("HH_CHAT_REPLY_PROMPT", cfg.ExtraChatReplyPrompt)
	}
	if !flags["contacts"] {
		cfg.Contacts = getEnv("HH_CONTACTS", cfg.Contacts)
	}

	if cfg.AIAttempts < 1 {
		return Config{}, errors.New("ai-attempts must be greater than 0")
	}
	if cfg.RequestInterval <= 0 {
		return Config{}, errors.New("request-interval must be greater than 0")
	}

	return cfg, nil
}

func getEnv(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func loadDotEnv(path string) error {
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		if strings.HasPrefix(line, "export ") {
			line = strings.TrimSpace(line[len("export "):])
		}

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}

		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)

		if key == "" {
			continue
		}

		// Удаляем комментарий только вне кавычек.
		if len(value) > 0 && value[0] != '"' && value[0] != '\'' {
			if idx := strings.Index(value, " #"); idx >= 0 {
				value = strings.TrimSpace(value[:idx])
			}
		}

		if len(value) >= 2 {
			switch value[0] {
			case '"':
				if value[len(value)-1] == '"' {
					if unquoted, err := strconv.Unquote(value); err == nil {
						value = unquoted
					}
				}

			case '\'':
				if value[len(value)-1] == '\'' {
					// strconv.Unquote не умеет одинарные кавычки для строк.
					value = value[1 : len(value)-1]
				}
			}
		}

		if err := os.Setenv(key, value); err != nil {
			return err
		}
	}

	return scanner.Err()
}
