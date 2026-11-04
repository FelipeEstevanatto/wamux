package user_handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	instance_model "github.com/felipeestevanatto/wamux/pkg/instance/model"
	user_service "github.com/felipeestevanatto/wamux/pkg/user/service"
)

type guardStubUserService struct {
	user_service.UserService
	contactsCalls int
}

func (s *guardStubUserService) GetContacts(*instance_model.Instance) ([]user_service.ContactInfo, error) {
	s.contactsCalls++
	return nil, nil
}

func newContactsRouter(svc user_service.UserService, instance *instance_model.Instance) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("instance", instance)
	})
	r.GET("/user/contacts", NewUserHandler(svc).GetContacts)
	return r
}

func TestGetContactsUnpairedReturnsConflictWithoutCallingService(t *testing.T) {
	svc := &guardStubUserService{}
	r := newContactsRouter(svc, &instance_model.Instance{Id: "inst-1"})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/user/contacts", nil))

	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d (body %s)", w.Code, http.StatusConflict, w.Body.String())
	}
	if svc.contactsCalls != 0 {
		t.Fatalf("GetContacts called %d times, want 0", svc.contactsCalls)
	}
}

func TestGetContactsPairedReachesService(t *testing.T) {
	svc := &guardStubUserService{}
	r := newContactsRouter(svc, &instance_model.Instance{Id: "inst-1", Jid: "5511999999999@s.whatsapp.net"})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/user/contacts", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body %s)", w.Code, http.StatusOK, w.Body.String())
	}
	if svc.contactsCalls != 1 {
		t.Fatalf("GetContacts called %d times, want 1", svc.contactsCalls)
	}
}
