package instance_handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/felipeestevanatto/wamux/pkg/config"
	instance_model "github.com/felipeestevanatto/wamux/pkg/instance/model"
	instance_service "github.com/felipeestevanatto/wamux/pkg/instance/service"
)

// stateStubService records the calls used by the create/disconnect/logout
// handlers. The embedded interface satisfies the rest of InstanceService.
type stateStubService struct {
	instance_service.InstanceService
	createCalls   int
	createErr     error
	disconnectErr error
	logoutErr     error
}

func (s *stateStubService) Create(data *instance_service.CreateStruct) (*instance_model.Instance, error) {
	s.createCalls++
	if s.createErr != nil {
		return nil, s.createErr
	}
	return &instance_model.Instance{Id: data.InstanceId, Name: data.Name}, nil
}

func (s *stateStubService) Disconnect(instance *instance_model.Instance) (*instance_model.Instance, error) {
	if s.disconnectErr != nil {
		return instance, s.disconnectErr
	}
	return instance, nil
}

func (s *stateStubService) Logout(instance *instance_model.Instance) (*instance_model.Instance, error) {
	if s.logoutErr != nil {
		return instance, s.logoutErr
	}
	return instance, nil
}

func newStateRouter(svc instance_service.InstanceService, instance *instance_model.Instance) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("instance", instance)
	})
	h := NewInstanceHandler(svc, &config.Config{})
	r.POST("/instance/create", h.Create)
	r.POST("/instance/disconnect", h.Disconnect)
	r.DELETE("/instance/logout", h.Logout)
	return r
}

func doJSON(t *testing.T, r *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func errorField(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	var payload map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal body %q: %v", w.Body.String(), err)
	}
	return payload["error"]
}

func TestCreateRejectsNonUUIDInstanceID(t *testing.T) {
	svc := &stateStubService{}
	r := newStateRouter(svc, &instance_model.Instance{})

	w := doJSON(t, r, http.MethodPost, "/instance/create",
		`{"name":"ops","token":"tok","instanceId":"not-a-uuid"}`)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (body %s)", w.Code, http.StatusBadRequest, w.Body.String())
	}
	if !strings.Contains(errorField(t, w), "UUID") {
		t.Fatalf("error = %q, want a UUID validation message", errorField(t, w))
	}
	if svc.createCalls != 0 {
		t.Fatalf("Create called %d times, want 0", svc.createCalls)
	}
}

func TestCreateAcceptsUUIDInstanceID(t *testing.T) {
	svc := &stateStubService{}
	r := newStateRouter(svc, &instance_model.Instance{})
	id := uuid.NewString()

	w := doJSON(t, r, http.MethodPost, "/instance/create",
		`{"name":"ops","token":"tok","instanceId":"`+id+`"}`)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body %s)", w.Code, http.StatusOK, w.Body.String())
	}
	if svc.createCalls != 1 {
		t.Fatalf("Create called %d times, want 1", svc.createCalls)
	}
}

func TestCreateWithoutInstanceIDLetsServiceGenerateIt(t *testing.T) {
	svc := &stateStubService{}
	r := newStateRouter(svc, &instance_model.Instance{})

	w := doJSON(t, r, http.MethodPost, "/instance/create", `{"name":"ops","token":"tok"}`)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body %s)", w.Code, http.StatusOK, w.Body.String())
	}
	if svc.createCalls != 1 {
		t.Fatalf("Create called %d times, want 1", svc.createCalls)
	}
}

func TestDisconnectWhenNotConnectedReturnsConflict(t *testing.T) {
	svc := &stateStubService{disconnectErr: instance_service.ErrNotConnected}
	r := newStateRouter(svc, &instance_model.Instance{Id: "inst-1"})

	w := doJSON(t, r, http.MethodPost, "/instance/disconnect", "")

	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d (body %s)", w.Code, http.StatusConflict, w.Body.String())
	}
	if errorField(t, w) == "" {
		t.Fatal("expected a non-empty error")
	}
}

func TestLogoutWhenNotConnectedReturnsConflict(t *testing.T) {
	svc := &stateStubService{logoutErr: instance_service.ErrNotConnected}
	r := newStateRouter(svc, &instance_model.Instance{Id: "inst-1"})

	w := doJSON(t, r, http.MethodDelete, "/instance/logout", "")

	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d (body %s)", w.Code, http.StatusConflict, w.Body.String())
	}
	if errorField(t, w) == "" {
		t.Fatal("expected a non-empty error")
	}
}
