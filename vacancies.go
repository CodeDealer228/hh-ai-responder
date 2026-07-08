package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const letterTemplatePath = "letter_template.txt"

// buildReadableTestSolutions converts test tasks and AI answers to human-readable question/answer pairs
func buildReadableTestSolutions(tasks []Task, answers map[int]SolutionFields) []QAPair {
	var result []QAPair
	for _, task := range tasks {
		ans, ok := answers[task.ID]
		if !ok {
			continue
		}

		var answerText string
		if ans.HasChoice {
			for _, sol := range task.CandidateSolutions {
				if id, err := strconv.Atoi(sol.ID); err == nil && id == ans.SolutionID {
					answerText = sol.Text
					break
				}
			}
		} else {
			answerText = ans.TextSolution
		}

		result = append(result, QAPair{
			Question: task.Description,
			Answer:   answerText,
		})
	}
	return result
}

func (r *HHAIResponder) GetVacancyTests(responseURL string) (map[string]VacancyTest, error) {
	if err := r.ctx.Err(); err != nil {
		return nil, err
	}

	req, err := r.buildRequest(http.MethodGet, responseURL, nil, nil)
	if err != nil {
		return nil, err
	}

	resp, err := r.requester.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.Status != http.StatusOK {
		return nil, unexpectedHTTPStatus(resp.Status)
	}

	var tests map[string]VacancyTest
	if err := decodeEmbeddedJSON(resp.Body, `,"vacancyTests":`, &tests); err != nil {
		return nil, err
	}

	return tests, nil
}

func (r *HHAIResponder) SendResponse(payload url.Values, refererURL string) (map[string]any, error) {
	if err := r.ctx.Err(); err != nil {
		return nil, err
	}
	token := r.XSRFToken()
	if token == "" {
		return nil, errors.New("xsrf token not found")
	}

	headers := map[string]string{
		"Content-Type":     "application/x-www-form-urlencoded",
		"X-Hhtmfrom":       "vacancy",
		"X-Hhtmsource":     "vacancy_response",
		"X-Requested-With": "XMLHttpRequest",
		"X-Xsrftoken":      token,
		"Referer":          refererURL,
	}

	req, err := r.buildRequest(http.MethodPost, "/applicant/vacancy_response/popup", strings.NewReader(payload.Encode()), headers)
	if err != nil {
		return nil, err
	}

	resp, err := r.requester.Do(req)
	if err != nil {
		return nil, err
	}

	if err := r.ctx.Err(); err != nil {
		return nil, err
	}

	var result map[string]any
	if err := json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("non JSON response: %w", err)
	}
	return result, nil
}

func (r *HHAIResponder) ApplyVacancy(vacancyID int, refererURL, letter string) (map[string]any, error) {
	if err := r.ctx.Err(); err != nil {
		return nil, err
	}
	token := r.XSRFToken()
	if token == "" {
		return nil, errors.New("xsrf token not found")
	}

	payload := url.Values{
		"_xsrf":            {token},
		"vacancy_id":       {strconv.Itoa(vacancyID)},
		"resume_hash":      {r.resumeHash},
		"letter":           {letter},
		"ignore_postponed": {"true"},
	}

	return r.SendResponse(payload, refererURL)
}

// RenderLetterTemplate fills a static cover-letter template with known values.
// No AI call involved: this is the default letter path, used for every application.
func RenderLetterTemplate(path, fullName, resumeTitle, vacancyName, companyName string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	replacer := strings.NewReplacer(
		"{{Name}}", fullName,
		"{{Title}}", resumeTitle,
		"{{Vacancy}}", vacancyName,
		"{{Company}}", companyName,
	)

	return strings.TrimSpace(replacer.Replace(string(data))), nil
}

func (r *HHAIResponder) GetVacancyDescription(vacancyId int) (string, error) {

	if err := r.ctx.Err(); err != nil {
		return "", err
	}

	req, err := r.buildRequest(http.MethodGet, fmt.Sprintf("/vacancy/%d?hhtmFrom=negotiation_list", vacancyId), nil, nil)
	if err != nil {
		return "", err
	}

	resp, err := r.requester.Do(req)
	if err != nil {
		return "", err
	}

	if resp.Status != http.StatusOK {
		return "", unexpectedHTTPStatus(resp.Status)
	}

	bodyText := string(resp.Body)

	target := `{"redirectConfig":`
	idx := strings.Index(bodyText, target)
	if idx == -1 {
		return "", errors.New("redirect config not found on page")
	}

	jsonStart := bodyText[idx:]

	var vacancyData struct {
		VacancyView struct {
			Description string `json:"description"`
		} `json:"vacancyView"`
	}

	decoder := json.NewDecoder(strings.NewReader(jsonStart))
	if err := decoder.Decode(&vacancyData); err != nil {
		return "", fmt.Errorf("failed to parse vacancy: %w", err)
	}

	return html.UnescapeString(vacancyData.VacancyView.Description), nil
}

func (r *HHAIResponder) ApplyVacancyWithTest(vacancyId int, letter string) (map[string]any, []QAPair, error) {
	if err := r.ctx.Err(); err != nil {
		return nil, nil, err
	}
	token := r.XSRFToken()
	if token == "" {
		return nil, nil, errors.New("xsrf token not found")
	}

	responseURL := r.ResolveURL(fmt.Sprintf("/applicant/vacancy_response?vacancyId=%d&startedWithQuestion=false&hhtmFrom=vacancy", vacancyId))
	tests, err := r.GetVacancyTests(responseURL)
	if err != nil {
		return nil, nil, err
	}

	test, ok := tests[strconv.Itoa(vacancyId)]
	if !ok {
		return nil, nil, fmt.Errorf("vacancy marked with test but no test data found for vacancy %d", vacancyId)
	}

	if len(test.Tasks) == 0 {
		return nil, nil, fmt.Errorf("vacancy marked with test but no tasks returned for vacancy %d", vacancyId)
	}

	payload := url.Values{
		"_xsrf":            {token},
		"uidPk":            {test.UIDPk},
		"guid":             {test.GUID},
		"startTime":        {test.StartTime},
		"testRequired":     {test.Required},
		"vacancy_id":       {strconv.Itoa(vacancyId)},
		"resume_hash":      {r.resumeHash},
		"ignore_postponed": {"true"},
		"incomplete":       {"false"},
		"lux":              {"true"},
		"withoutTest":      {"no"},
		"letter":           {letter},
	}
	payload.Set("mark_applicant_visible_in_vacancy_country", "false")
	payload.Set("country_ids", "[]")

	extraPrompt := strings.TrimSpace(r.extraTestSolutionPrompt)
	if r.testWishes != "" {
		if extraPrompt != "" {
			extraPrompt += "\n\n"
		}
		extraPrompt += r.testWishes
	}

	solutions, err := r.ai.SolveTests(test.Tasks, r.contacts, extraPrompt)
	if err != nil {
		return nil, nil, fmt.Errorf("ai failed to answer test: %w", err)
	}

	if len(solutions) != len(test.Tasks) {
		return nil, nil, fmt.Errorf("incomplete test answers: got %d, expected %d", len(solutions), len(test.Tasks))
	}
	if err := r.ctx.Err(); err != nil {
		return nil, nil, err
	}

	// logger.Debug("AI answers: %v", answers)

	for _, task := range test.Tasks {
		taskID := task.ID
		fieldName := "task_" + strconv.Itoa(taskID)

		answer, ok := solutions[taskID]
		if !ok {
			return nil, nil, fmt.Errorf("ai returned no answer for task %d", taskID)
		}
		if answer.HasChoice {
			payload.Set(fieldName, strconv.Itoa(answer.SolutionID))
			continue
		}

		payload.Set(fieldName+"_text", answer.TextSolution)
	}

	respJSON, err := r.SendResponse(payload, responseURL)
	if err != nil {
		return nil, nil, err
	}

	testSolutions := buildReadableTestSolutions(test.Tasks, solutions)
	return respJSON, testSolutions, nil
}

func (r *HHAIResponder) fetchVacancyPage(page int) ([]Vacancy, error) {
	if err := r.ctx.Err(); err != nil {
		return nil, err
	}
	params := cloneValues(r.searchParams)
	params.Set("page", strconv.Itoa(page))
	req, err := r.buildRequest(http.MethodGet, "/search/vacancy?"+params.Encode(), nil, nil)
	if err != nil {
		return nil, err
	}

	resp, err := r.requester.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.Status != http.StatusOK {
		return nil, unexpectedHTTPStatus(resp.Status)
	}

	var vacancies []Vacancy
	if err := decodeEmbeddedJSON(resp.Body, `,"vacancies":`, &vacancies); err != nil {
		return nil, err
	}

	return vacancies, nil
}

func (r *HHAIResponder) ApplyVacancies() error {
	resume := r.GetCurrentResume()
	if resume == nil {
		return errors.New("resume not found")
	}

	for page := 0; ; page++ {
		if r.ctx.Err() != nil {
			return r.ctx.Err()
		}

		vacancies, err := r.fetchVacancyPage(page)
		if err != nil {
			logger.Error("Failed to fetch vacancies: %v", err)
			return err
		}

		if len(vacancies) == 0 {
			break
		}

		for _, vacancy := range vacancies {
			if r.ctx.Err() != nil {
				return r.ctx.Err()
			}
			if len(vacancy.UserLabels) > 0 || vacancy.Archived || vacancy.ResponseURL != "" {
				continue
			}
			if r.maxResponses > 0 && vacancy.TotalResponsesCount > r.maxResponses {
				continue
			}

			vacancyURL, ok := vacancy.Links["desktop"]
			if !ok || vacancyURL == "" {
				logger.Warn("Vacancy %d has no desktop link", vacancy.ID)
				continue
			}

			// if responder.dryRun {
			// 	logger.Debug("Application skipped (dry-run): %s", vacancyURL)
			// 	continue
			// }

			var letter string
			if vacancy.ResponseLetterRequired || r.forceLetter {
				letter, err = RenderLetterTemplate(letterTemplatePath, r.GetFullName(), resume.Title, vacancy.Name, vacancy.Company.Name)
				if err != nil || strings.TrimSpace(letter) == "" {
					logger.Error("Failed to render letter template for %s: %v", vacancyURL, err)
					continue
				}
				logger.Debug("Coverage letter:\n\n%s", letter)
			}

			var responseResult map[string]any
			var solutions []QAPair
			if vacancy.UserTestPresent {
				responseResult, solutions, err = r.ApplyVacancyWithTest(vacancy.ID, letter)
			} else {
				responseResult, err = r.ApplyVacancy(vacancy.ID, vacancyURL, letter)
			}

			if errVal, hasErr := responseResult["error"].(string); hasErr {
				if errVal == "negotiations-limit-exceeded" {
					logger.Warn("Negotiations limit exceeded!")
					return nil
				}

				err = fmt.Errorf("Send response error: %s", errVal)
			}

			if err != nil {
				logger.Error("Failed to send application %d: %v", vacancy.ID, err)
				r.writeEvent(ErrorResult{
					Type: "application_error",
					Context: map[string]any{
						"vacancy_id":   vacancy.ID,
						"vacancy_name": vacancy.Name,
						"url":          vacancyURL,
						"resume":       r.resumeHash,
						"resume_title": resume.Title,
					},
					Error: err.Error(),
					Time:  time.Now(),
				})
				continue
			}

			if len(solutions) > 0 {
				logger.Debug("test answers: %v", solutions)
			}

			if successStr, ok := responseResult["success"].(string); ok && successStr == "true" {
				newCount := vacancy.TotalResponsesCount + 1
				logger.Info("Application successfully sent (responses: %d): %s", newCount, vacancyURL)
				r.writeEvent(ApplyResult{
					Type:           "application",
					Resume:         r.resumeHash,
					ResumeTitle:    resume.Title,
					VacancyID:      vacancy.ID,
					URL:            vacancyURL,
					Name:           vacancy.Name,
					Letter:         letter,
					AppliedAt:      time.Now(),
					ResponsesCount: newCount,
					TestSolutions:  solutions,
				})
			} else {
				logger.Warn("Application sent but response wrong: %s", vacancyURL)
			}
		}
	}

	logger.Info("Finished processing!")
	return nil
}
