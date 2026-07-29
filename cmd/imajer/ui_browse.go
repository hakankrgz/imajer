package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
)

type browsePathFunc func(context.Context, string, string) (path string, canceled bool, err error)

type uiBrowseRequest struct {
	Kind        string `json:"kind"`
	CurrentPath string `json:"current_path"`
}

func (s *uiServer) handleBrowse(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.mu.Lock()
	running := s.status.Running
	s.mu.Unlock()
	if running {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "Çalışan işlem sırasında dosya seçilemez"})
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4<<10)
	var request uiBrowseRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Geçersiz seçim isteği"})
		return
	}
	request.Kind = strings.ToLower(strings.TrimSpace(request.Kind))
	if request.Kind != "file" && request.Kind != "directory" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Seçim türü file veya directory olmalıdır"})
		return
	}
	request.CurrentPath = strings.TrimSpace(request.CurrentPath)
	if s.browsePath == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "Bu sistemde yerel dosya seçici kullanılamıyor"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 9*time.Minute)
	defer cancel()
	path, canceled, err := s.browsePath(ctx, request.Kind, request.CurrentPath)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			writeJSON(w, http.StatusRequestTimeout, map[string]string{"error": "Dosya seçimi zaman aşımına uğradı"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Dosya seçici açılamadı: " + err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"path": path, "canceled": canceled})
}
