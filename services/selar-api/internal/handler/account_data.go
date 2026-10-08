package handler

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
	"sort"
	"time"

	"github.com/selar-dev/selar-api/internal/middleware"
	"github.com/selar-dev/selar-api/internal/storage"
)

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

const exportReadme = `SELAR data export

This archive contains the data SELAR stores about your account, as JSON:

  account.json             email, display name, study group, settings
  documents.json           metadata of the readings you added (not their text)
  annotations.json         your highlights and notes
  link_decisions.json      passage suggestions and your confirm/reject decisions
  mental_model_links.json  concept connections and your review history
  quiz_attempts.json       quiz attempts and answers, if any
  reading_sessions.json    reading sessions (pages viewed, scroll depth)
  chat_threads.json        grounded chat conversations
  learning_events.json     the activity log used by the learner model
  analytics_events.json    usage analytics, only if you opted in

The uploaded PDF files themselves are not included; you already have them.
Passwords are never exported.
`

// ExportMyData streams a zip of the caller's data. Only rows owned by the
// authenticated user are read (every query is filtered by user_id).
func (h *Handler) ExportMyData(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	files, err := h.store.ExportUserData(r.Context(), userID)
	if err != nil {
		log.Printf("export for %s failed: %v", userID, err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to export data"})
		return
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	now := time.Now().UTC()
	add := func(name string, body []byte) error {
		f, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate, Modified: now})
		if err != nil {
			return err
		}
		_, err = f.Write(body)
		return err
	}
	if err := add("README.txt", []byte(exportReadme)); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to export data"})
		return
	}
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		var pretty bytes.Buffer
		if json.Indent(&pretty, files[name], "", "  ") != nil {
			pretty.Reset()
			pretty.Write(files[name])
		}
		if err := add(name, pretty.Bytes()); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to export data"})
			return
		}
	}
	if err := zw.Close(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to export data"})
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="selar-export-`+now.Format("2006-01-02")+`.zip"`)
	w.Header().Set("Cache-Control", "private, no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(buf.Bytes())
}

// DeleteAccountConfirmation must be typed by the user.
const DeleteAccountConfirmation = "delete my account"

// DeleteAccount permanently deletes the account after a password re-check
// and a typed confirmation. Order (as in DeleteDocument): resolve storage
// locators, delete all rows in one transaction, then remove stored objects.
// A storage failure after the rows are gone is logged, not reported as a
// failed deletion, because the account no longer exists either way.
func (h *Handler) DeleteAccount(w http.ResponseWriter, r *http.Request) {
	var req struct {
		CurrentPassword string `json:"current_password"`
		Confirm         string `json:"confirm"`
	}
	if err := decodeStrict(r, &req); err != nil || req.CurrentPassword == "" {
		badRequest(w, "current_password is required")
		return
	}
	if req.Confirm != DeleteAccountConfirmation {
		badRequest(w, `type "`+DeleteAccountConfirmation+`" to confirm`)
		return
	}
	if _, ok := h.verifyCurrentPassword(w, r, req.CurrentPassword); !ok {
		return
	}
	userID := middleware.GetUserID(r.Context())
	docs, err := h.store.ListStoredDocuments(r.Context(), userID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to delete account"})
		return
	}
	if err := h.store.DeleteUser(r.Context(), userID); err != nil {
		log.Printf("delete account %s failed: %v", userID, err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to delete account"})
		return
	}
	cleanupFailed := false
	for _, d := range docs {
		if err := h.storage.DeleteDocument(r.Context(), d.ID, d.PDFLocator); err != nil {
			cleanupFailed = true
			log.Printf("storage cleanup for deleted account %s, document %s failed: %v", userID, d.ID, err)
		}
	}
	// Uploads that never became documents (abandoned direct uploads).
	if err := h.storage.DeletePrefix(r.Context(), storage.UserUploadPrefix(userID)); err != nil {
		cleanupFailed = true
		log.Printf("storage cleanup of uploads for deleted account %s failed: %v", userID, err)
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "deleted", "storage_cleanup_complete": !cleanupFailed})
}
