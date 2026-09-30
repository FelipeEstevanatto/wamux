package group_handler

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	group_service "github.com/evolution-foundation/evolution-go/pkg/group/service"
	instance_model "github.com/evolution-foundation/evolution-go/pkg/instance/model"
	"go.mau.fi/whatsmeow/types"
)

type stubGroupService struct {
	group_service.GroupService
	listCalls   int
	updateCalls int
	lastUpdate  *group_service.UpdateGroupRequestParticipantsStruct
}

func (s *stubGroupService) GetGroupRequestParticipants(_ *group_service.GetGroupRequestParticipantsStruct, _ *instance_model.Instance) ([]group_service.EnrichedGroupParticipantRequest, error) {
	s.listCalls++
	return []group_service.EnrichedGroupParticipantRequest{}, nil
}

func (s *stubGroupService) UpdateGroupRequestParticipants(d *group_service.UpdateGroupRequestParticipantsStruct, _ *instance_model.Instance) ([]types.GroupParticipant, error) {
	s.updateCalls++
	s.lastUpdate = d
	return nil, nil
}

func newGroupRouter(svc group_service.GroupService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("instance", &instance_model.Instance{Id: "i"}) })
	h := NewGroupHandler(svc)
	r.POST("/group/requestparticipants", h.GetGroupRequestParticipants)
	r.POST("/group/updaterequestparticipants", h.UpdateGroupRequestParticipants)
	return r
}

func postJSON(r *gin.Engine, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestRequestParticipantsRequiresGroupJid(t *testing.T) {
	svc := &stubGroupService{}
	r := newGroupRouter(svc)

	if w := postJSON(r, "/group/requestparticipants", `{}`); w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	if svc.listCalls != 0 {
		t.Fatal("service must not be called without a groupJid")
	}
}

func TestRequestParticipantsLists(t *testing.T) {
	svc := &stubGroupService{}
	r := newGroupRouter(svc)

	w := postJSON(r, "/group/requestparticipants", `{"groupJid":"120363000000000000@g.us"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", w.Code, w.Body.String())
	}
	if svc.listCalls != 1 {
		t.Fatalf("list calls = %d, want 1", svc.listCalls)
	}
}

func TestUpdateRequestParticipantsValidatesInput(t *testing.T) {
	svc := &stubGroupService{}
	r := newGroupRouter(svc)

	// missing action
	if w := postJSON(r, "/group/updaterequestparticipants", `{"groupJid":"1@g.us","participants":["5511999999999"]}`); w.Code != http.StatusBadRequest {
		t.Fatalf("missing action: status = %d, want 400", w.Code)
	}
	// missing participants
	if w := postJSON(r, "/group/updaterequestparticipants", `{"groupJid":"1@g.us","action":"approve"}`); w.Code != http.StatusBadRequest {
		t.Fatalf("missing participants: status = %d, want 400", w.Code)
	}
	if svc.updateCalls != 0 {
		t.Fatal("service must not be called with invalid input")
	}

	// valid
	w := postJSON(r, "/group/updaterequestparticipants", `{"groupJid":"1@g.us","action":"approve","participants":["5511999999999"]}`)
	if w.Code != http.StatusOK {
		t.Fatalf("valid: status = %d, body %s", w.Code, w.Body.String())
	}
	if svc.updateCalls != 1 || svc.lastUpdate.Action != "approve" {
		t.Fatalf("update calls = %d, action = %q", svc.updateCalls, svc.lastUpdate.Action)
	}
}
