package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/arashrasoulzadeh/appgent/internal/middleware"
	"github.com/arashrasoulzadeh/appgent/internal/services"
	"github.com/google/uuid"
)

type AppHandler struct {
	appService *services.AppService
}

func NewAppHandler(appService *services.AppService) *AppHandler {
	return &AppHandler{appService: appService}
}

func (h *AppHandler) ListApps(w http.ResponseWriter, r *http.Request) {
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

	apps, err := h.appService.List(r.Context(), userID)
	if err != nil {
		http.Error(w, `{"error": "failed to list apps"}`, http.StatusInternalServerError)
		return
	}
	if apps == nil {
		apps = []*services.App{}
	}
	for _, a := range apps {
		a.DeploymentURL.String = absoluteURL(r, a.DeploymentURL.String)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"apps": apps})
}

func (h *AppHandler) CreateApp(w http.ResponseWriter, r *http.Request) {
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

	var req struct {
		Name   string `json:"name"`
		Kind   string `json:"kind"`
		Prompt string `json:"prompt"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error": "invalid request"}`, http.StatusBadRequest)
		return
	}

	if req.Name == "" || req.Kind == "" || req.Prompt == "" {
		http.Error(w, `{"error": "name, kind, and prompt are required"}`, http.StatusBadRequest)
		return
	}

	app, run, err := h.appService.Create(r.Context(), userID, req.Name, req.Kind, req.Prompt)
	if err != nil {
		if err == services.ErrAppLimitReached {
			http.Error(w, fmt.Sprintf(`{"error": "you can have at most %d apps"}`, services.MaxAppsPerUser), http.StatusBadRequest)
			return
		}
		http.Error(w, `{"error": "failed to create app"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"app": app,
		"run": run,
	})
}

func (h *AppHandler) GetApp(w http.ResponseWriter, r *http.Request) {
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

	app, run, err := h.appService.GetByID(r.Context(), userID, appID)
	if err != nil {
		if err == services.ErrAppNotFound {
			http.Error(w, `{"error": "app not found"}`, http.StatusNotFound)
			return
		}
		http.Error(w, `{"error": "failed to get app"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"app": map[string]interface{}{
			"id":         app.ID,
			"name":       app.Name,
			"slug":       app.Slug,
			"kind":       app.Kind,
			"status":     app.Status,
			"created_at": app.CreatedAt,
		},
		"latest_run": run,
	})
}

func (h *AppHandler) DeleteApp(w http.ResponseWriter, r *http.Request) {
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

	err = h.appService.Delete(r.Context(), userID, appID)
	if err != nil {
		if err == services.ErrAppNotFound {
			http.Error(w, `{"error": "app not found"}`, http.StatusNotFound)
			return
		}
		http.Error(w, `{"error": "failed to delete app"}`, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *AppHandler) RegenerateApp(w http.ResponseWriter, r *http.Request) {
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

	var req struct {
		Prompt string `json:"prompt"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
		http.Error(w, `{"error": "invalid request"}`, http.StatusBadRequest)
		return
	}

	run, err := h.appService.Regenerate(r.Context(), userID, appID, req.Prompt)
	if err != nil {
		if err == services.ErrAppNotFound {
			http.Error(w, `{"error": "app not found"}`, http.StatusNotFound)
			return
		}
		http.Error(w, `{"error": "failed to regenerate"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"run": run,
	})
}

func (h *AppHandler) ListRuns(w http.ResponseWriter, r *http.Request) {
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

	runs, err := h.appService.GetRuns(r.Context(), userID, appID)
	if err != nil {
		http.Error(w, `{"error": "failed to get runs"}`, http.StatusInternalServerError)
		return
	}
	if runs == nil {
		runs = []*services.Run{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"runs": runs})
}

func (h *AppHandler) GetRun(w http.ResponseWriter, r *http.Request) {
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

	run, steps, err := h.appService.GetRunWithSteps(r.Context(), userID, appID, runID)
	if err != nil {
		if err == services.ErrRunNotFound {
			http.Error(w, `{"error": "run not found"}`, http.StatusNotFound)
			return
		}
		http.Error(w, `{"error": "failed to get run"}`, http.StatusInternalServerError)
		return
	}
	if steps == nil {
		steps = []*services.AgentStep{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"run":   run,
		"steps": steps,
	})
}
