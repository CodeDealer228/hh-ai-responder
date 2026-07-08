package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	mathrand "math/rand"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"
)

const botRecruiterAnswer = "Спасибо!\nВаши ответы отправлены работодателю. Если ваш отклик его заинтересует, он напишет в этом же чате или позвонит по номеру, который вы указали."

// ===== Chat API Methods =====
// TODO: там есть вебсокеты для получения новых сообщений в реальном времени
func (r *HHAIResponder) GetChats(page int) (*ChatsResponse, error) {
	token := r.XSRFToken()
	if token == "" {
		return nil, errors.New("xsrf token not found")
	}
	headers := map[string]string{
		"Accept":           "application/json",
		"X-Requested-With": "XMLHttpRequest",
		"X-Xsrftoken":      token,
		"Referer":          r.chatURL + "/?platform=xhh&dest=iframe",
	}

	endpoint := r.chatURL + "/chatik/api/chats?filterUnread=false&filterHasTextMessage=false&do_not_track_session_events=true"
	if page > 0 {
		endpoint += "&page=" + strconv.Itoa(page)
	}

	req, err := r.buildRequest(http.MethodGet, endpoint, nil, headers)
	if err != nil {
		return nil, err
	}

	resp, err := r.requester.Do(req)
	if err != nil {
		return nil, err
	}

	var result ChatsResponse
	if err := json.Unmarshal(resp.Body, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (r *HHAIResponder) GetChatData(chatID int64, applicantID int64) (*ChatDataResponse, error) {
	token := r.XSRFToken()
	if token == "" {
		return nil, errors.New("xsrf token not found")
	}
	headers := map[string]string{
		"Accept":           "application/json",
		"X-Requested-With": "XMLHttpRequest",
		"X-Xsrftoken":      token,
		"Referer":          fmt.Sprintf("%s/chat/%d", r.chatURL, chatID),
	}

	endpoint := fmt.Sprintf(
		"%s/chatik/api/chat_data?chatId=%d&applicantId=%d&do_not_track_session_events=true",
		r.chatURL,
		chatID,
		applicantID,
	)

	req, err := r.buildRequest(http.MethodGet, endpoint, nil, headers)
	if err != nil {
		return nil, err
	}

	resp, err := r.requester.Do(req)
	if err != nil {
		return nil, err
	}

	var result ChatDataResponse
	if err := json.Unmarshal(resp.Body, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (r *HHAIResponder) SendChatMessage(chatID int64, text string) (map[string]any, error) {
	token := r.XSRFToken()
	if token == "" {
		return nil, errors.New("xsrf token not found")
	}

	uuid, err := generateUUIDv4()
	if err != nil {
		return nil, err
	}

	payload := map[string]any{
		"chatId":         chatID,
		"text":           text,
		"idempotencyKey": uuid,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	headers := map[string]string{
		"Content-Type":     "application/json",
		"Accept":           "application/json",
		"X-Requested-With": "XMLHttpRequest",
		"X-Xsrftoken":      token,
		"Referer":          r.chatURL + "/?platform=xhh&dest=iframe",
	}

	req, err := r.buildRequest(
		http.MethodPost,
		r.chatURL+"/chatik/api/send",
		bytes.NewReader(body),
		headers,
	)
	if err != nil {
		return nil, err
	}

	resp, err := r.requester.Do(req)
	if err != nil {
		return nil, err
	}

	var result map[string]any
	if err := json.Unmarshal(resp.Body, &result); err != nil {
		return nil, err
	}
	if _, hasErr := result["error"]; hasErr {
		return nil, fmt.Errorf("Send chat message error: %v", result)
	}
	return result, nil
}

func (r *HHAIResponder) LeaveChat(chatId int64) (map[string]any, error) {
	token := r.XSRFToken()
	if token == "" {
		return nil, errors.New("xsrf token not found")
	}

	payload := map[string]any{
		"chatId": chatId,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	headers := map[string]string{
		"Accept":            "application/json",
		"Content-Type":      "application/json",
		"Referer":           fmt.Sprintf("%s/chat/%d", r.chatURL, chatId),
		"X-Requested-With":  "XMLHttpRequest",
		"X-Xsrftoken":       token,
		"X-hhtmFrom":        "resume",
		"X-hhtmFromLabel":   "resume",
		"X-hhtmSource":      "app",
		"X-hhtmSourceLabel": "resume",
	}

	req, err := r.buildRequest(http.MethodPost, r.chatURL+"/chatik/api/leave", bytes.NewReader(body), headers)
	if err != nil {
		return nil, err
	}

	resp, err := r.requester.Do(req)
	if err != nil {
		return nil, err
	}

	var result map[string]any
	if err := json.Unmarshal(resp.Body, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (r *HHAIResponder) getChatsAwaitingReply(maxPages int) ([]ChatToReply, error) {
	resume := r.GetCurrentResume()
	if resume == nil {
		return nil, errors.New("resume not found")
	}

	pages := 1
	var results []ChatToReply

	// ЭТАП 1: Загрузка и первичная фильтрация чатов
	for page := 0; page < pages; page++ {
		chatsResponse, err := r.GetChats(page)
		if err != nil {
			return nil, err
		}

		chats := chatsResponse.Chats

		if len(chats.Items) == 0 {
			logger.Warn("Empty chat list!")
			break
		}

		// var resume ChatResumeResource
		var resumeExists bool
		// resume, exists = chatsResponse.Resources.Resumes[chat.Resources.Resume[0]]
		// if !exists {
		// Фолбечное резюме, если то, с которого был отклик, удалено
		_, resumeExists = chatsResponse.Resources.Resumes[fmt.Sprint(resume.Id)]
		if !resumeExists {
			//return nil, fmt.Errorf("Resume doesn't exists: %s", resumeId)
			continue
		}
		// }

		pages = min(maxPages, chats.Pages)

		for _, chat := range chats.Items {
			if slices.Contains(r.ignoredChats, chat.Id) {
				continue
			}

			// Общение со всем резюме пусть
			// Последнее сообщение свое
			// if len(chat.Resources.Resume) == 0 || !slices.Contains(chat.Resources.Resume, resumeId) {
			// 	continue
			// }

			last := chat.LastMessage

			if last == nil {
				continue
			}

			// На чаты старше 3-х дней не отвечаем
			if time.Since(last.CreationTime) > 72*time.Hour {
				return results, nil
			}

			// Пропускаем чаты, где соискатель писал последним
			participantId, _ := strconv.ParseInt(last.ParticipantID, 10, 64)
			if r.userId == participantId {
				logger.Debug("Skip chat #%d without response", chat.Id)
				continue
			}

			if last.Text == botRecruiterAnswer {
				continue
			}

			if len(chat.Resources.Vacancy) == 0 || len(chat.Resources.Resume) == 0 {
				continue
			}

			if !slices.Contains(chat.Resources.Resume, strconv.FormatInt(resume.Id, 10)) {
				continue
			}

			vacancy, vacancyExists := chatsResponse.Resources.Vacancies[chat.Resources.Vacancy[0]]
			if !vacancyExists {
				continue
			}

			var options []string
			if chat.LastMessage.Actions != nil {
				for _, button := range chat.LastMessage.Actions.TextButtons {
					options = append(options, button.Text)
				}
			}

			// В принципе можно сделать общение во всех чатах, но сейчас под резюме
			// сделано
			chatInfo := ChatToReply{
				ChatId:              chat.Id,
				ContactName:         last.ParticipantDisplay.Name,
				ReplyToMessage:      last.Text,
				ReplyOptions:        options,
				VacancyName:         vacancy.Name,
				VacancyURL:          vacancy.Links.Desktop,
				CompanyName:         vacancy.Company.Name,
				VacancyCompensation: strings.Replace(FormatCompensation(vacancy.Compensation), "RUR", "руб", 1),
				ApplicantId:         r.userId,
				FirstName:           r.firstName,
				LastName:            r.lastName,
				ResumeExperience:    r.resumeExperience,
				ResumeID:            resume.Id,
				ResumeHash:          resume.Hash,
				ResumeTitle:         resume.Title,
				Skills:              resume.Skills,
				Salary:              resume.Salary,
			}

			if last.WorkflowTransition != nil && last.WorkflowTransition.ApplicantState == "DISCARD" {
				chatInfo.IsDiscard = true
			}

			//logger.Debug("append chat #%d", chat.ID)
			results = append(results, chatInfo)
		}
	}

	return results, nil
}

// ===== Auto Chat Responder =====
func (r *HHAIResponder) AutoRespondChats() error {
	chatsToReply, err := r.getChatsAwaitingReply(10)
	if err != nil {
		return fmt.Errorf("load chats error: %v", err)
	}

	logger.Debug("total chats to reply: %d", len(chatsToReply))

	// ЭТАП 2: Обработка собранных чатов
	for _, chatToReply := range chatsToReply {

		if chatToReply.IsDiscard {
			logger.Debug("Skip and leave chat with discard: %d", chatToReply.ChatId)
			r.LeaveChat(chatToReply.ChatId)
			continue
		}

		// No AI involved here by design: always a random filler line from
		// questions.txt, never freeform generation. Keeps this feature (raises
		// account activity by keeping chats alive) on without any AI cost/risk.
		chatDataResponse, err := r.GetChatData(chatToReply.ChatId, chatToReply.ApplicantId)
		if err != nil {
			logger.Warn("Can't load messages from chat #%d: %v", chatToReply.ChatId, err)
			continue
		}
		// Свинья запретила ей писать
		if !chatDataResponse.ChatStates.WriteMessageState.Allowed || len(chatDataResponse.Chat.Messages.Items) >= 20 {
			logger.Debug("Ignore chat #%d", chatDataResponse.Chat.ID)
			r.ignoredChats = append(r.ignoredChats, chatToReply.ChatId)
			continue
		}

		reply := r.randomQuestion()
		if reply == "" {
			logger.Warn("No filler messages available in questions.txt, skipping chat #%d", chatToReply.ChatId)
			continue
		}

		logger.Debug("Reply to chat #%d:\n%s\n%s", chatToReply.ChatId, chatToReply.ReplyToMessage, reply)

		if _, err := r.SendChatMessage(chatToReply.ChatId, reply); err != nil {
			logger.Error("Failed reply to chat #%d: %v", chatToReply.ChatId, err)

			r.writeEvent(ErrorResult{
				Type: "chat_reply_error",
				Context: map[string]any{
					"chat_id":      chatToReply.ChatId,
					"resume":       chatToReply.ResumeHash,
					"resume_title": chatToReply.ResumeTitle,
				},
				Error: err.Error(),
				Time:  time.Now(),
			})

			logger.Debug("Ignore chat: %d", chatToReply.ChatId)
			r.ignoredChats = append(r.ignoredChats, chatToReply.ChatId)
			continue
		}

		logger.Info("Auto-replied in chat %d", chatToReply.ChatId)

		r.writeEvent(ChatResult{
			Type:        "chat_reply",
			Resume:      chatToReply.ResumeHash,
			ResumeTitle: chatToReply.ResumeTitle,
			ChatId:      chatToReply.ChatId,
			EmployerMsg: chatToReply.ReplyToMessage,
			Reply:       reply,
			SentAt:      time.Now(),
		})
	}

	return nil
}

// randomQuestion returns a random filler message, or "" if none are loaded.
func (r *HHAIResponder) randomQuestion() string {
	if len(r.questions) == 0 {
		return ""
	}
	return r.questions[mathrand.Intn(len(r.questions))]
}
