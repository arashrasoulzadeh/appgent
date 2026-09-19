package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/arashrasoulzadeh/appgent/internal/auth"
	"github.com/arashrasoulzadeh/appgent/internal/middleware"
	"github.com/arashrasoulzadeh/appgent/internal/services"
	"github.com/google/uuid"
)

type AuthHandler struct {
	tokenService *auth.TokenService
	appService   *services.AppService
}

func NewAuthHandler(tokenService *auth.TokenService, appService *services.AppService) *AuthHandler {
	return &AuthHandler{
		tokenService: tokenService,
		appService:   appService,
	}
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error": "invalid request"}`, http.StatusBadRequest)
		return
	}

	// For MVP, just check if user exists and password matches
	// Real implementation would verify bcrypt hash
	if req.Email == "admin" && req.Password == "admin" {
		// Get user from DB
		// For now, use a fixed UUID for the seeded admin
		userID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
		
		token, err := h.tokenService.GenerateToken(userID.String(), req.Email)
		if err != nil {
			http.Error(w, `{"error": "failed to generate token"}`, http.StatusInternalServerError)
			return
		}

		h.tokenService.SetCookie(w, token)
		
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"user": map[string]string{
				"id":    userID.String(),
				"email": req.Email,
			},
		})
		return
	}

	http.Error(w, `{"error": "invalid credentials"}`, http.StatusUnauthorized)
}

func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	h.tokenService.ClearCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r)
	if !ok {
		http.Error(w, `{"error": "unauthorized"}`, http.StatusUnauthorized)
		return
	}

	email, _ := middleware.GetEmail(r)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"user": map[string]string{
			"id":    userID,
			"email": email,
		},
	})
}