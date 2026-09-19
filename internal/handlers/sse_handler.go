package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/arashrasoulzadeh/appgent/internal/middleware"
	"github.com/arashrasoulzadeh/appgent/internal/services"
	"github.com/google/uuid"
)

type SSEHandler struct {
	appService *services.AppService
}

func NewSSEHandler(appService *services.AppService) *SSEHandler {
	return &SSEHandler{appService: appService}
}

func (h *SSEHandler) RunEvents(w http.ResponseWriter, r *http.Request) {
	userIDStr, ok := middleware.GetUserID(r)
	if !ok {
		http.Error(w, `{"error": "unauthorized"}`, http.StatusUnauthorized)
		return
	}
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		http.Error(w, `{"error": "invalid user id"}`, http.StatusBadRequest)
		return
	}

	appIDStr := r.PathValue("app_id")
	appID, err := uuid.Parse(appIDStr)
	if err != nil {
		http.Error(w, `{"error": "invalid app id"}`, http.StatusBadRequest)
		return
	}

	runIDStr := r.PathValue("run_id")
	runID, err := uuid.Parse(runIDStr)
	if err != nil {
		http.Error(w, `{"error": "invalid run id"}`, http.StatusBadRequest)
		return
	}

	// Verify run belongs to user
	run, _, err := h.appService.GetRunWithSteps(r.Context(), userID, appID, runID)
	if err != nil {
		if err == services.ErrRunNotFound {
			http.Error(w, `{"error": "run not found"}`, http.StatusNotFound)
			return
		}
		http.Error(w, `{"error": "failed to get run"}`, http.StatusInternalServerError)
		return
	}

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	// Send initial event
	fmt.Fprintf(w, "event: run\ndata: {\"status\": \"%s\"}\n\n", run.Status)
	flusher.Flush()

	// For MVP, poll the run status every second
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	ctx := r.Context()
	lastStatus := run.Status

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			currentRun, _, err := h.appService.GetRunWithSteps(ctx, userID, appID, runID)
			if err != nil {
				continue
			}

			if currentRun.Status != lastStatus {
				fmt.Fprintf(w, "event: run\ndata: {\"status\": \"%s\"}\n\n", currentRun.Status)
				flusher.Flush()
				lastStatus = currentRun.Status
			}

			if currentRun.Status == "succeeded" || currentRun.Status == "needs_review" || currentRun.Status == "failed" {
				// Send final step events
				// In production, this would come from Temporal queries or Postgres LISTEN/NOTIFY
				return
			}
		}
	}
}

func writeSSEEvent(w http.ResponseWriter, event, data string) {
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data)
}

func writeSSEJSONEvent(w http.ResponseWriter, event string, v interface{}) {
	data, _ := json.Marshal(v)
	writeSSEEvent(w, event, string(data))
}