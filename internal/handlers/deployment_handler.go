package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/arashrasoulzadeh/appgent/internal/middleware"
	"github.com/arashrasoulzadeh/appgent/internal/services"
	"github.com/google/uuid"
)

type DeploymentHandler struct {
	appService *services.AppService
}

func NewDeploymentHandler(appService *services.AppService) *DeploymentHandler {
	return &DeploymentHandler{appService: appService}
}

func (h *DeploymentHandler) DeployApp(w http.ResponseWriter, r *http.Request) {
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

	// Get latest successful run
	runs, err := h.appService.GetRuns(r.Context(), userID, appID)
	if err != nil || len(runs) == 0 {
		http.Error(w, `{"error": "no runs found"}`, http.StatusNotFound)
		return
	}

	var latestRun *services.Run
	for _, run := range runs {
		if run.Status == "succeeded" {
			latestRun = run
			break
		}
	}

	if latestRun == nil {
		http.Error(w, `{"error": "no successful run to deploy"}`, http.StatusBadRequest)
		return
	}

	deployment, err := h.appService.CreateDeployment(r.Context(), userID, appID, latestRun.ID)
	if err != nil {
		if err == services.ErrAppNotFound {
			http.Error(w, `{"error": "app not found"}`, http.StatusNotFound)
			return
		}
		http.Error(w, `{"error": "failed to create deployment"}`, http.StatusInternalServerError)
		return
	}

	// TODO: Actually trigger deployment via Provisioner
	// For now, just return the deployment record

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"deployment": map[string]interface{}{
			"id":     deployment.ID,
			"status": deployment.Status,
		},
	})
}

func (h *DeploymentHandler) ListDeployments(w http.ResponseWriter, r *http.Request) {
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

	deployments, err := h.appService.GetDeployments(r.Context(), userID, appID)
	if err != nil {
		http.Error(w, `{"error": "failed to get deployments"}`, http.StatusInternalServerError)
		return
	}
	if deployments == nil {
		deployments = []*services.Deployment{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"deployments": deployments})
}

type PreviewHandler struct {
	appService *services.AppService
}

func NewPreviewHandler(appService *services.AppService) *PreviewHandler {
	return &PreviewHandler{appService: appService}
}

func (h *PreviewHandler) GetPreviewURL(w http.ResponseWriter, r *http.Request) {
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

	previewURL, expiresAt, err := h.appService.GetPreviewURL(r.Context(), userID, appID, runID)
	if err != nil {
		if err == services.ErrRunNotFound {
			http.Error(w, `{"error": "run not found"}`, http.StatusNotFound)
			return
		}
		if err == services.ErrNoPreview {
			http.Error(w, `{"error": "preview not available"}`, http.StatusNotFound)
			return
		}
		http.Error(w, `{"error": "failed to get preview"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"preview_url": previewURL,
		"expires_at":  expiresAt.Format(time.RFC3339),
	})
}