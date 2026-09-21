package handlers

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/arashrasoulzadeh/appgent/internal/middleware"
	"github.com/arashrasoulzadeh/appgent/internal/services"
	"github.com/arashrasoulzadeh/appgent/internal/storage"
	"github.com/google/uuid"
)

// absoluteURL turns a relative API path (e.g. "/api/v1/apps/.../preview/
// index.html") into an absolute one using the incoming request's own
// scheme/host, so it works as a plain href regardless of the domain this
// API is reached at (localhost during dev, a VPS IP or a real domain in
// prod) without needing a separate "public base URL" env var.
func absoluteURL(r *http.Request, path string) string {
	if path == "" {
		return ""
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
		scheme = proto
	}
	return scheme + "://" + r.Host + path
}

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

	deployment, err := h.appService.Deploy(r.Context(), userID, appID)
	if err != nil {
		switch {
		case errors.Is(err, services.ErrAppNotFound):
			http.Error(w, `{"error": "app not found"}`, http.StatusNotFound)
		case errors.Is(err, services.ErrNothingToDeploy):
			http.Error(w, `{"error": "no published run to deploy — generate an app first"}`, http.StatusBadRequest)
		default:
			http.Error(w, `{"error": "failed to deploy"}`, http.StatusInternalServerError)
		}
		return
	}
	deployment.URL.String = absoluteURL(r, deployment.URL.String)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{"deployment": deployment})
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
	for _, d := range deployments {
		d.URL.String = absoluteURL(r, d.URL.String)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"deployments": deployments})
}

type PreviewHandler struct {
	appService    *services.AppService
	storageClient *storage.Client
	bucket        string
}

func NewPreviewHandler(appService *services.AppService, storageClient *storage.Client, bucket string) *PreviewHandler {
	return &PreviewHandler{appService: appService, storageClient: storageClient, bucket: bucket}
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
		if errors.Is(err, services.ErrRunNotFound) {
			http.Error(w, `{"error": "run not found"}`, http.StatusNotFound)
			return
		}
		if errors.Is(err, services.ErrNoPreview) {
			http.Error(w, `{"error": "preview not available"}`, http.StatusNotFound)
			return
		}
		http.Error(w, `{"error": "failed to get preview"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"preview_url": absoluteURL(r, previewURL),
		"expires_at":  expiresAt.Format(time.RFC3339),
	})
}

// ServeRunFile streams one file from a run's published bundle. Requires
// the same session auth as the rest of the API (a run's preview is for
// its owner reviewing generation output, not a public share link — that's
// what a deployment's live URL is for, via ServeLiveFile below).
func (h *PreviewHandler) ServeRunFile(w http.ResponseWriter, r *http.Request) {
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
	appID, err := uuid.Parse(r.PathValue("app_id"))
	if err != nil {
		http.Error(w, `{"error": "invalid app id"}`, http.StatusBadRequest)
		return
	}
	runID, err := uuid.Parse(r.PathValue("run_id"))
	if err != nil {
		http.Error(w, `{"error": "invalid run id"}`, http.StatusBadRequest)
		return
	}

	// Confirms ownership and that this run actually has a published
	// bundle; the relative URL it returns isn't used here, only the error.
	if _, _, err := h.appService.GetPreviewURL(r.Context(), userID, appID, runID); err != nil {
		if errors.Is(err, services.ErrRunNotFound) || errors.Is(err, services.ErrNoPreview) {
			http.Error(w, `{"error": "preview not available"}`, http.StatusNotFound)
			return
		}
		http.Error(w, `{"error": "failed to get preview"}`, http.StatusInternalServerError)
		return
	}

	path := r.PathValue("path")
	if path == "" {
		path = "index.html"
	}
	key := "runs/" + runID.String() + "/" + path
	h.serveObject(w, r, key)
}

// ServeLiveFile streams one file from an app's current live deployment.
// Deliberately public/unauthenticated — a deployment exists specifically
// to be a shareable public URL, unlike a run preview.
func (h *PreviewHandler) ServeLiveFile(w http.ResponseWriter, r *http.Request) {
	appID, err := uuid.Parse(r.PathValue("app_id"))
	if err != nil {
		http.Error(w, `{"error": "invalid app id"}`, http.StatusBadRequest)
		return
	}
	path := r.PathValue("path")
	if path == "" {
		path = "index.html"
	}
	key := "apps/" + appID.String() + "/" + path
	h.serveObject(w, r, key)
}

func (h *PreviewHandler) serveObject(w http.ResponseWriter, r *http.Request, key string) {
	// Defense in depth against path traversal in the {path...} wildcard —
	// net/http's ServeMux already cleans "." / ".." segments out of the
	// URL before routing, but don't rely solely on that upstream behavior
	// for something that reaches straight into object storage.
	if strings.Contains(key, "..") {
		http.Error(w, `{"error": "invalid path"}`, http.StatusBadRequest)
		return
	}

	obj, err := h.storageClient.Download(r.Context(), h.bucket, key)
	if err != nil {
		http.Error(w, `{"error": "file not found"}`, http.StatusNotFound)
		return
	}
	defer obj.Close()

	contentType := mime.TypeByExtension(filepath.Ext(key))
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	w.Header().Set("Content-Type", contentType)
	if _, err := io.Copy(w, obj); err != nil {
		// Response headers/status are already sent at this point — best
		// effort is all that's possible here.
		_ = err
	}
}
