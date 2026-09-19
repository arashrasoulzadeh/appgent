package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/arashrasoulzadeh/appgent/internal/auth"
	"github.com/arashrasoulzadeh/appgent/internal/middleware"
	"github.com/arashrasoulzadeh/appgent/internal/services"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// MockAppService is a mock implementation of AppService for testing
type MockAppService struct {
	mock.Mock
}

func (m *MockAppService) Create(ctx context.Context, userID uuid.UUID, name, kind, prompt string) (*services.App, *services.Run, error) {
	args := m.Called(ctx, userID, name, kind, prompt)
	if args.Get(0) == nil {
		return nil, nil, args.Error(2)
	}
	return args.Get(0).(*services.App), args.Get(1).(*services.Run), args.Error(2)
}

func (m *MockAppService) GetByID(ctx context.Context, userID, appID uuid.UUID) (*services.App, *services.Run, error) {
	args := m.Called(ctx, userID, appID)
	if args.Get(0) == nil {
		return nil, nil, args.Error(2)
	}
	return args.Get(0).(*services.App), args.Get(1).(*services.Run), args.Error(2)
}

func (m *MockAppService) List(ctx context.Context, userID uuid.UUID) ([]*services.App, error) {
	args := m.Called(ctx, userID)
	return args.Get(0).([]*services.App), args.Error(1)
}

func (m *MockAppService) Delete(ctx context.Context, userID, appID uuid.UUID) error {
	args := m.Called(ctx, userID, appID)
	return args.Error(0)
}

func (m *MockAppService) Regenerate(ctx context.Context, userID, appID uuid.UUID, prompt string) (*services.Run, error) {
	args := m.Called(ctx, userID, appID, prompt)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*services.Run), args.Error(1)
}

func (m *MockAppService) GetRuns(ctx context.Context, userID, appID uuid.UUID) ([]*services.Run, error) {
	args := m.Called(ctx, userID, appID)
	return args.Get(0).([]*services.Run), args.Error(1)
}

func (m *MockAppService) GetRunWithSteps(ctx context.Context, userID, appID, runID uuid.UUID) (*services.Run, []*services.AgentStep, error) {
	args := m.Called(ctx, userID, appID, runID)
	if args.Get(0) == nil {
		return nil, nil, args.Error(2)
	}
	return args.Get(0).(*services.Run), args.Get(1).([]*services.AgentStep), args.Error(2)
}

func (m *MockAppService) CreateDeployment(ctx context.Context, appID, runID uuid.UUID) (*services.Deployment, error) {
	args := m.Called(ctx, appID, runID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*services.Deployment), args.Error(1)
}

func (m *MockAppService) GetDeployments(ctx context.Context, userID, appID uuid.UUID) ([]*services.Deployment, error) {
	args := m.Called(ctx, userID, appID)
	return args.Get(0).([]*services.Deployment), args.Error(1)
}

func (m *MockAppService) GetPreviewURL(ctx context.Context, userID, appID, runID uuid.UUID) (string, time.Time, error) {
	args := m.Called(ctx, userID, appID, runID)
	return args.String(0), args.Get(1).(time.Time), args.Error(2)
}

func TestAuthHandler_Login(t *testing.T) {
	ts := auth.NewTokenService("test-secret", "session", 1, false)
	handler := NewAuthHandler(ts, nil)

	t.Run("valid credentials", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", 
			strings.NewReader(`{"email":"admin","password":"admin"}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		handler.Login(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		
		var resp map[string]interface{}
		err := json.NewDecoder(w.Body).Decode(&resp)
		require.NoError(t, err)
		assert.Contains(t, resp, "user")
	})

	t.Run("invalid credentials", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login",
			strings.NewReader(`{"email":"admin","password":"wrong"}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		handler.Login(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("invalid JSON", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login",
			strings.NewReader(`invalid json`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		handler.Login(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestAuthHandler_Logout(t *testing.T) {
	ts := auth.NewTokenService("test-secret", "session", 1, false)
	handler := NewAuthHandler(ts, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	w := httptest.NewRecorder()

	handler.Logout(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)
	cookies := w.Result().Cookies()
	assert.Len(t, cookies, 1)
	assert.Equal(t, "session", cookies[0].Name)
	assert.Equal(t, "", cookies[0].Value)
	assert.Equal(t, -1, cookies[0].MaxAge)
}

func TestAuthHandler_Me(t *testing.T) {
	ts := auth.NewTokenService("test-secret", "session", 1, false)
	handler := NewAuthHandler(ts, nil)

	t.Run("with valid session", func(t *testing.T) {
		// Manually set context values as AuthMiddleware would
		req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
		ctx := req.Context()
		ctx = context.WithValue(ctx, middleware.UserIDKey, "user-123")
		ctx = context.WithValue(ctx, middleware.EmailKey, "test@example.com")
		req = req.WithContext(ctx)
		w := httptest.NewRecorder()

		handler.Me(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		
		var resp map[string]interface{}
		err := json.NewDecoder(w.Body).Decode(&resp)
		require.NoError(t, err)
		user := resp["user"].(map[string]interface{})
		assert.Equal(t, "user-123", user["id"])
		assert.Equal(t, "test@example.com", user["email"])
	})

	t.Run("without session", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
		w := httptest.NewRecorder()

		handler.Me(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})
}