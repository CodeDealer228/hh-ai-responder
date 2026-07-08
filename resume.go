package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"mime/multipart"
	"net/http"
	"regexp"
	"strconv"
	"strings"
)

var (
	latesteResumeHashRegexp = regexp.MustCompile(`"latestResumeHash":"([a-f0-9]{30,})"`)
	userIdRegexp            = regexp.MustCompile(`"userId":(\d+)`)
)

func (r *HHAIResponder) LoadProfileData() error {
	if err := r.ctx.Err(); err != nil {
		return err
	}

	req, err := r.buildRequest(http.MethodGet, "/applicant/resumes", nil, nil)
	if err != nil {
		return err
	}

	resp, err := r.requester.Do(req)
	if err != nil {
		return err
	}

	if resp.Status != http.StatusOK {
		return unexpectedHTTPStatus(resp.Status)
	}

	bodyText := string(resp.Body)

	target := `{"redirectConfig":`
	idx := strings.Index(bodyText, target)
	if idx == -1 {
		return errors.New("redirect config not found on page")
	}

	// jsonStart := bodyText[idx:]
	//logger.Debug("%.255s", jsonStart)

	var resumesData struct {
		LatestResumeHash string `json:"latestResumeHash"`
		ApplicantResumes []struct {
			Attributes struct {
				Id   string `json:"id"`
				Hash string `json:"hash"`
				//UserId string `json:"user"`
			} `json:"_attributes"`
			Title []struct {
				String string `json:"string"`
			} `json:"title"`
			Salary []struct {
				Amount   int    `json:"amount"`
				Currency string `json:"currency"`
			} `json:"salary"`
			Area []struct {
				Title string `json:"title"`
			} `json:"area"`
			KeySkills []struct {
				String string `json:"string"`
			} `json:"keySkills"`
		} `json:"applicantResumes"`
		Account struct {
			FirstName  string `json:"firstName"`
			MiddleName string `json:"middleName"`
			LastName   string `json:"lastName"`
			Email      string `json:"email"`
		} `json:"account"`
		UserNotifications []struct {
			UserId int64 `json:"userId"`
		} `json:"userNotifications"`
		// Chatik struct {
		// 	ChatikOrigin string `json:"chatikOrigin"`
		// } `json:"chatik"`
		Config struct {
			StaticHost                 string `json:"staticHost"`
			ApiXhhHost                 string `json:"apiXhhHost"`
			HhcdnHost                  string `json:"hhcdnHost"`
			ImageResizingCdnHost       string `json:"imageResizingCdnHost"`
			DevBuildNotifyEnabled      bool   `json:"devBuildNotifyEnabled"`
			ExternalMicroFrontendHosts struct {
				ApplicantServicesFront string `json:"applicant-services-front"`
				EmployerReviewsFront   string `json:"employer-reviews-front"`
				Chatik                 string `json:"chatik"`
				SkillsFront            string `json:"skills-front"`
				SupportFront           string `json:"support-front"`
				ResumeProfileFront     string `json:"resume-profile-front"`
				BrandingFront          string `json:"branding-front"`
				WebcallFront           string `json:"webcall-front"`
				MentorsFront           string `json:"mentors-front"`
				CareerPlatformFront    string `json:"career-platform-front"`
			} `json:"externalMicroFrontendHosts"`
		} `json:"config"`
	}

	// if err := json.Unmarshal([]byte(jsonStart), &resumesData); err != nil {
	// 	return fmt.Errorf("failed to parse resumes: %w", err)
	// }

	decoder := json.NewDecoder(strings.NewReader(bodyText[idx:]))
	if err := decoder.Decode(&resumesData); err != nil {
		return fmt.Errorf("failed to parse resumes: %w", err)
	}

	r.latestResumeHash = resumesData.LatestResumeHash
	r.firstName = resumesData.Account.FirstName
	r.middleName = resumesData.Account.MiddleName
	r.lastName = resumesData.Account.LastName
	r.email = resumesData.Account.Email
	r.userId = resumesData.UserNotifications[0].UserId
	r.chatURL = resumesData.Config.ExternalMicroFrontendHosts.Chatik
	r.resumeProfileFrontURL = resumesData.Config.ExternalMicroFrontendHosts.ResumeProfileFront

	r.resumes = make([]ResumeItem, 0, len(resumesData.ApplicantResumes))
	for _, resume := range resumesData.ApplicantResumes {
		id, _ := strconv.ParseInt(resume.Attributes.Id, 10, 64)

		var title string
		if len(resume.Title) > 0 {
			title = resume.Title[0].String
		}

		var area string
		if len(resume.Area) > 0 {
			area = resume.Area[0].Title
		}

		var skills []string
		for _, skill := range resume.KeySkills {
			skills = append(skills, skill.String)
		}

		var salaryAmount int
		var salaryCurrency string
		if len(resume.Salary) > 0 {
			salaryAmount = resume.Salary[0].Amount
			salaryCurrency = resume.Salary[0].Currency
		}

		r.resumes = append(r.resumes, ResumeItem{
			Id:     id,
			Hash:   resume.Attributes.Hash,
			Title:  title,
			Area:   area,
			Skills: strings.Join(skills, ", "),
			Salary: strings.Replace(fmt.Sprintf("%d %s", salaryAmount, salaryCurrency), "RUR", "руб", 1),
		})
	}

	return nil
}

func (r *HHAIResponder) SetActiveJobSearchStatus() (bool, error) {
	if err := r.ctx.Err(); err != nil {
		return false, err
	}

	token := r.XSRFToken()
	if token == "" {
		return false, errors.New("xsrf token not found")
	}

	endpoint := fmt.Sprintf("%s/profile/shards/user_statuses/job_search_status?status=looking_for_offers", r.resumeProfileFrontURL)

	headers := map[string]string{
		"Accept":            "application/json",
		"X-hhtmSource":      "resume_list",
		"X-hhtmFrom":        "",
		"X-hhtmSourceLabel": "",
		"X-hhtmFromLabel":   "",
		"X-Requested-With":  "XMLHttpRequest",
		"X-Xsrftoken":       token,
	}

	req, err := r.buildRequest(http.MethodPost, endpoint, nil, headers)
	if err != nil {
		return false, err
	}

	resp, err := r.requester.Do(req)
	if err != nil {
		return false, err
	}

	if resp.Status != http.StatusOK {
		return false, unexpectedHTTPStatus(resp.Status)
	}

	return true, nil
}

func (r *HHAIResponder) GetResumeExperience() (string, error) {
	if err := r.ctx.Err(); err != nil {
		return "", err
	}

	req, err := r.buildRequest(http.MethodGet, fmt.Sprintf("/resume/%s", r.resumeHash), nil, nil)
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

	var cfg struct {
		ApplicantResume struct {
			Experience []struct {
				StartDate   string  `json:"startDate"`
				EndDate     *string `json:"endDate"`
				CompanyName string  `json:"companyName"`
				Position    string  `json:"position"`
				Description string  `json:"description"`
			} `json:"experience"`
		} `json:"applicantResume"`
	}

	decoder := json.NewDecoder(strings.NewReader(jsonStart))
	if err := decoder.Decode(&cfg); err != nil {
		return "", fmt.Errorf("failed to parse resume: %w", err)
	}

	var sb strings.Builder
	for i, exp := range cfg.ApplicantResume.Experience {
		// Ограничиваем описание опыта тремя последними местами работы
		if i >= 3 {
			break
		}
		if i > 0 {
			sb.WriteString("\n\n")
		}

		end := "по настоящее время"
		if exp.EndDate != nil {
			end = *exp.EndDate
		}

		sb.WriteString(html.UnescapeString(exp.Position))
		sb.WriteString("\n")
		sb.WriteString(html.UnescapeString(exp.CompanyName))
		sb.WriteString("\n")
		sb.WriteString(exp.StartDate)
		sb.WriteString(" - ")
		sb.WriteString(end)
		sb.WriteString("\n\n")
		sb.WriteString(html.UnescapeString(exp.Description))
	}

	return sb.String(), nil
}

// FetchResumeSummary loads title/salary/keySkills for a single resume directly from its
// own page. Used as a fallback when /applicant/resumes doesn't expose applicantResumes
// (hh.ru redirects accounts with a single resume to /applicant/profile/me instead, which
// doesn't embed the resume list).
func (r *HHAIResponder) FetchResumeSummary(hash string) (*ResumeItem, error) {
	if err := r.ctx.Err(); err != nil {
		return nil, err
	}

	req, err := r.buildRequest(http.MethodGet, fmt.Sprintf("/resume/%s", hash), nil, nil)
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

	bodyText := string(resp.Body)

	target := `{"redirectConfig":`
	idx := strings.Index(bodyText, target)
	if idx == -1 {
		return nil, errors.New("redirect config not found on resume page")
	}

	var cfg struct {
		ApplicantResume struct {
			Attributes struct {
				Id string `json:"id"`
			} `json:"_attributes"`
			Title []struct {
				String string `json:"string"`
			} `json:"title"`
			// Salary is "[]" when unset and an object when set, hence RawMessage.
			Salary    json.RawMessage `json:"salary"`
			KeySkills []struct {
				String string `json:"string"`
			} `json:"keySkills"`
		} `json:"applicantResume"`
	}

	decoder := json.NewDecoder(strings.NewReader(bodyText[idx:]))
	if err := decoder.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("failed to parse resume: %w", err)
	}

	id, _ := strconv.ParseInt(cfg.ApplicantResume.Attributes.Id, 10, 64)

	var title string
	if len(cfg.ApplicantResume.Title) > 0 {
		title = cfg.ApplicantResume.Title[0].String
	}

	var skills []string
	for _, s := range cfg.ApplicantResume.KeySkills {
		skills = append(skills, s.String)
	}

	var salary string
	if len(cfg.ApplicantResume.Salary) > 0 && cfg.ApplicantResume.Salary[0] == '{' {
		var salaryObj struct {
			Amount   int    `json:"amount"`
			Currency string `json:"currency"`
		}
		if err := json.Unmarshal(cfg.ApplicantResume.Salary, &salaryObj); err == nil && salaryObj.Currency != "" {
			salary = strings.Replace(fmt.Sprintf("%d %s", salaryObj.Amount, salaryObj.Currency), "RUR", "руб", 1)
		}
	}

	return &ResumeItem{
		Id:     id,
		Hash:   hash,
		Title:  html.UnescapeString(title),
		Skills: strings.Join(skills, ", "),
		Salary: salary,
	}, nil
}

// TouchResume raises (updates) resume position in search results
func (r *HHAIResponder) TouchResume() (bool, error) {
	if err := r.ctx.Err(); err != nil {
		return false, err
	}

	token := r.XSRFToken()
	if token == "" {
		return false, errors.New("xsrf token not found")
	}

	if r.resumeHash == "" {
		return false, errors.New("resume hash is empty")
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	if err := writer.WriteField("resume", r.resumeHash); err != nil {
		return false, err
	}
	if err := writer.WriteField("undirectable", "true"); err != nil {
		return false, err
	}
	if err := writer.Close(); err != nil {
		return false, err
	}

	headers := map[string]string{
		"Content-Type":     writer.FormDataContentType(),
		"Accept":           "application/json",
		"X-Requested-With": "XMLHttpRequest",
		"X-Xsrftoken":      token,
		"X-Hhtmfrom":       "negotiation_list",
		"X-Hhtmsource":     "resume_list",
		"Referer":          r.ResolveURL("/applicant/resumes"),
	}

	req, err := r.buildRequest(http.MethodPost, "/applicant/resumes/touch", &body, headers)
	if err != nil {
		return false, err
	}

	resp, err := r.requester.Do(req)
	if err != nil {
		return false, err
	}

	return resp.Status == http.StatusOK, nil
}
