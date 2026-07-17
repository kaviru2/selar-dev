package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/selar-dev/selar-api/internal/middleware"
	"github.com/selar-dev/selar-api/internal/model"
)

func (h *Handler) ListChatThreads(w http.ResponseWriter, r *http.Request) {
	threads, err := h.store.ListChatThreads(r.Context(), middleware.GetUserID(r.Context()))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list conversations"})
		return
	}
	writeJSON(w, http.StatusOK, threads)
}

func (h *Handler) CreateChatThread(w http.ResponseWriter, r *http.Request) {
	thread, err := h.store.CreateChatThread(r.Context(), middleware.GetUserID(r.Context()), "New conversation")
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create conversation"})
		return
	}
	writeJSON(w, http.StatusCreated, thread)
}

func (h *Handler) ListChatMessages(w http.ResponseWriter, r *http.Request) {
	messages, err := h.store.ListChatMessages(r.Context(), middleware.GetUserID(r.Context()), chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list messages"})
		return
	}
	writeJSON(w, http.StatusOK, messages)
}

func (h *Handler) CreateChatMessage(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	threadID := chi.URLParam(r, "id")
	var request model.ChatMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	request.Content = strings.TrimSpace(request.Content)
	if request.Content == "" || len(request.Content) > 4000 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "message must contain 1 to 4000 characters"})
		return
	}

	userMessage, err := h.store.CreateChatMessage(r.Context(), userID, threadID, "user", request.Content, "complete", "")
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "conversation not found"})
		return
	}
	history, err := h.store.ListChatMessages(r.Context(), userID, threadID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load conversation context"})
		return
	}
	if len(history) > 8 {
		history = history[len(history)-8:]
	}

	answer, err := requestChatAnswer(userID, threadID, request.Content, history)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	assistantMessage, err := h.store.SaveChatAnswer(r.Context(), userID, threadID, userMessage.ID, request.Content, *answer)
	if err != nil {
		log.Printf("failed to save grounded chat answer: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to save grounded answer"})
		return
	}
	writeJSON(w, http.StatusCreated, assistantMessage)
}

func requestChatAnswer(userID, threadID, question string, history []model.ChatMessage) (*model.ChatAnswer, error) {
	workerURL := os.Getenv("WORKER_URL")
	if workerURL == "" {
		workerURL = "http://localhost:8000"
	}
	payload, err := json.Marshal(map[string]any{
		"user_id": userID, "thread_id": threadID, "question": question, "history": history,
	})
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequest(http.MethodPost, workerURL+"/chat", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 50 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("chat worker unavailable")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("chat worker could not produce an answer")
	}
	var answer model.ChatAnswer
	if err := json.NewDecoder(response.Body).Decode(&answer); err != nil {
		return nil, fmt.Errorf("invalid chat worker response")
	}
	return &answer, nil
}
