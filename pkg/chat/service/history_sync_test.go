package chat_service

import (
	"testing"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/types"
)

func TestHistorySyncCountClamps(t *testing.T) {
	cases := map[int]int{
		0:    defaultHistorySyncCount,
		-1:   defaultHistorySyncCount,
		5:    5,
		100:  100,
		1000: maxHistorySyncCount,
	}
	for input, want := range cases {
		if got := historySyncCount(input); got != want {
			t.Errorf("historySyncCount(%d) = %d, want %d", input, got, want)
		}
	}
}

func TestHistorySyncMemberMatchesAnyAlias(t *testing.T) {
	own := types.NewJID("5511999990000", types.DefaultUserServer)
	client := whatsmeow.NewClient(&store.Device{ID: &own}, nil)

	info := &types.GroupInfo{Participants: []types.GroupParticipant{
		{JID: types.NewJID("5511888888888", types.DefaultUserServer)},
		{PhoneNumber: own},
	}}
	if !historySyncMember(client, info) {
		t.Fatal("a participant carrying our phone JID must match")
	}

	info.Participants = []types.GroupParticipant{
		{JID: types.NewJID("5511777777777", types.DefaultUserServer)},
	}
	if historySyncMember(client, info) {
		t.Fatal("an unrelated roster must not match")
	}
	if historySyncMember(client, nil) {
		t.Fatal("a nil group must not match")
	}
}

func TestHistorySyncMemberLIDAlias(t *testing.T) {
	ownLID := types.NewJID("269182931329179", types.HiddenUserServer)
	client := whatsmeow.NewClient(&store.Device{ID: &ownLID}, nil)
	info := &types.GroupInfo{Participants: []types.GroupParticipant{
		{LID: ownLID},
	}}
	if !historySyncMember(client, info) {
		t.Fatal("a participant carrying our LID must match")
	}
}
