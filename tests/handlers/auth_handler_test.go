package handlers_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/arashrasoulzadeh/appgent/internal/auth"
	"github.com/arashrasoulzadeh/appgent/internal/handlers"
	"github.com/arashrasoulzadeh/appgent/internal/middleware"
	"github.com/arashrasoulzadeh/appgent/internal/services"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// MockAppService is a mock implementation of AppService for testing. It
// satisfies whatever unexported service interfaces the handlers package
// requires (e.g. the user-lookup interface used by AuthHandler) purely by
// having the matching exported methods - the interfaces themselves don't
// need to be named here.
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

func (m *MockAppService) CreateDeployment(ctx context.Context, userID, appID, runID uuid.UUID) (*services.Deployment, error) {
	args := m.Called(ctx, userID, appID, runID)
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

func (m *MockAppService) GetUserByEmail(ctx context.Context, email string) (*services.User, error) {
	args := m.Called(ctx, email)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*services.User), args.Error(1)
}

func TestAuthHandler_Login(t *testing.T) {
	ts := auth.NewTokenService("test-secret", "session", 1, false)

	// bcrypt hash of "admin", generated once at package build time via
	// bcrypt.GenerateFromPassword([]byte("admin"), bcrypt.DefaultCost).
	const adminHash = "$2a$10$ahq6GKS4oPrK/R7mIc6zaes1tjjy1h5hwGI6N5Am6fd9KEPjsnjeK"

	t.Run("valid credentials", func(t *testing.T) {
		mockSvc := new(MockAppService)
		mockSvc.On("GetUserByEmail", mock.Anything, "admin").
			Return(&services.User{ID: uuid.New(), Email: "admin", PasswordHash: adminHash}, nil)
		handler := handlers.NewAuthHandler(ts, mockSvc)

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

		// A session cookie must be set on successful login.
		cookies := w.Result().Cookies()
		require.Len(t, cookies, 1)
		assert.Equal(t, "session", cookies[0].Name)
		assert.NotEmpty(t, cookies[0].Value)

		mockSvc.AssertExpectations(t)
	})

	t.Run("wrong password", func(t *testing.T) {
		mockSvc := new(MockAppService)
		mockSvc.On("GetUserByEmail", mock.Anything, "admin").
			Return(&services.User{ID: uuid.New(), Email: "admin", PasswordHash: adminHash}, nil)
		handler := handlers.NewAuthHandler(ts, mockSvc)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login",
			strings.NewReader(`{"email":"admin","password":"wrong"}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		handler.Login(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("unknown user", func(t *testing.T) {
		mockSvc := new(MockAppService)
		mockSvc.On("GetUserByEmail", mock.Anything, "nobody@example.com").
			Return(nil, services.ErrUserNotFound)
		handler := handlers.NewAuthHandler(ts, mockSvc)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login",
			strings.NewReader(`{"email":"nobody@example.com","password":"whatever"}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		handler.Login(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("invalid JSON", func(t *testing.T) {
		handler := handlers.NewAuthHandler(ts, new(MockAppService))
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
	handler := handlers.NewAuthHandler(ts, nil)

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
	handler := handlers.NewAuthHandler(ts, nil)

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
