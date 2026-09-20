package handlers

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/arashrasoulzadeh/appgent/internal/auth"
	"github.com/arashrasoulzadeh/appgent/internal/middleware"
	"github.com/arashrasoulzadeh/appgent/internal/services"
	"golang.org/x/crypto/bcrypt"
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

	user, err := h.appService.GetUserByEmail(r.Context(), req.Email)
	if err != nil {
		if !errors.Is(err, services.ErrUserNotFound) {
			http.Error(w, `{"error": "internal error"}`, http.StatusInternalServerError)
			return
		}
		http.Error(w, `{"error": "invalid credentials"}`, http.StatusUnauthorized)
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		http.Error(w, `{"error": "invalid credentials"}`, http.StatusUnauthorized)
		return
	}

	token, err := h.tokenService.GenerateToken(user.ID.String(), user.Email)
	if err != nil {
		http.Error(w, `{"error": "failed to generate token"}`, http.StatusInternalServerError)
		return
	}

	h.tokenService.SetCookie(w, token)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"user": map[string]string{
			"id":    user.ID.String(),
			"email": user.Email,
		},
	})
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