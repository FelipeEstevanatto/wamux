package group_service

import (
	"strings"
	"testing"

	"github.com/evolution-foundation/evolution-go/pkg/utils"
	"go.mau.fi/whatsmeow/types"
)

// Realistic identifiers taken from a live LID-addressed account:
//
//	Store.ID  = 5514991421911:5@s.whatsapp.net
//	Store.LID = 102839351287983:5@lid
var (
	selfPN  = types.JID{User: "5514991421911", Device: 5, Server: types.DefaultUserServer}
	selfLID = types.JID{User: "102839351287983", Device: 5, Server: types.HiddenUserServer}
)

func groupWith(ownerJID, ownerPN types.JID) *types.GroupInfo {
	return &types.GroupInfo{
		JID:      types.NewJID("120363428571571388", types.GroupServer),
		OwnerJID: ownerJID,
		OwnerPN:  ownerPN,
	}
}

// A group owned by this account on a LID-addressed server: OwnerJID is the
// account's LID and OwnerPN is its phone number.
func TestFilterGroupsOwnedByLIDAddressed(t *testing.T) {
	owned := groupWith(selfLID, selfPN)
	other := groupWith(
		types.JID{User: "269182931329179", Server: types.HiddenUserServer},
		types.NewJID("5514981170846", types.DefaultUserServer),
	)

	got := filterGroupsOwnedBy([]*types.GroupInfo{owned, other}, selfPN, selfLID)
	if len(got) != 1 {
		t.Fatalf("expected 1 owned group, got %d", len(got))
	}
	if got[0].JID != owned.JID {
		t.Errorf("expected group %s, got %s", owned.JID, got[0].JID)
	}
}

// OwnerPN alone is enough to identify the owner, even when OwnerJID is an
// unrelated LID (e.g. the account was addressed by LID only).
func TestFilterGroupsOwnedByOwnerPNOnly(t *testing.T) {
	owned := groupWith(types.JID{User: "999", Server: types.HiddenUserServer}, selfPN)
	if got := filterGroupsOwnedBy([]*types.GroupInfo{owned}, selfPN, selfLID); len(got) != 1 {
		t.Fatalf("expected OwnerPN to match, got %d", len(got))
	}
}

// Classic PN-addressed group: OwnerJID is the phone number, OwnerPN is empty.
func TestFilterGroupsOwnedByPNAndressed(t *testing.T) {
	owned := groupWith(selfPN, types.JID{})
	if got := filterGroupsOwnedBy([]*types.GroupInfo{owned}, selfPN, selfLID); len(got) != 1 {
		t.Fatalf("expected PN owner to match, got %d", len(got))
	}
}

func TestFilterGroupsOwnedByEmptyIsNonNil(t *testing.T) {
	// The handler serializes this slice directly; nil would become JSON null
	// instead of [].
	got := filterGroupsOwnedBy(nil, selfPN, selfLID)
	if got == nil {
		t.Fatal("expected an empty non-nil slice")
	}
	if len(got) != 0 {
		t.Fatalf("expected no groups, got %d", len(got))
	}
}

// TestFilterGroupsOwnedByRegressionPlusPrefix pins the root cause of the
// "not working" /group/myall endpoint: the old code parsed the client JID with
// utils.ParseJID, which prefixes phone numbers with "+", so the strict struct
// comparison against the server-provided owner JID never matched and the
// endpoint always returned an empty list.
func TestFilterGroupsOwnedByRegressionPlusPrefix(t *testing.T) {
	owner := types.NewJID("5514991421911", types.DefaultUserServer)

	// Reproduce the old logic exactly.
	parsed, ok := utils.ParseJID(strings.Split(selfPN.String(), ".")[0])
	if !ok {
		t.Fatal("ParseJID unexpectedly failed")
	}
	if parsed.User == owner.User {
		t.Fatalf("regression premise changed: ParseJID no longer prefixes '+' (%q)", parsed.User)
	}
	if parsed == owner {
		t.Fatal("old comparison unexpectedly matched")
	}

	// The new logic must match.
	groups := []*types.GroupInfo{groupWith(owner, types.JID{})}
	if got := filterGroupsOwnedBy(groups, selfPN, selfLID); len(got) != 1 {
		t.Fatalf("expected the owner filter to match, got %d", len(got))
	}
}

// TestNormalizeParticipantJIDs pins the CreateGroup/UpdateParticipant bug: a
// phone-number participant was passed to WhatsApp as "+5514...@s.whatsapp.net"
// (because utils.ParseJID -> CreateJID prefixes "+"), which the server cannot
// resolve, so the request hung and failed with "info query timed out". The
// helper must emit the canonical digits-only JID while leaving LID/group JIDs
// untouched.
func TestNormalizeParticipantJIDs(t *testing.T) {
	got, err := normalizeParticipantJIDs([]string{
		"5514981170846",           // phone number -> must lose the "+" prefix
		"269182931329179@lid",     // LID -> unchanged
		"120363000000000000@g.us", // group JID -> unchanged
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 JIDs, got %d", len(got))
	}
	if got[0].String() != "5514981170846@s.whatsapp.net" {
		t.Errorf("phone not canonicalized: %s", got[0].String())
	}
	if strings.HasPrefix(got[0].User, "+") {
		t.Errorf("phone still has '+': %q", got[0].User)
	}
	if got[1].String() != "269182931329179@lid" {
		t.Errorf("LID changed: %s", got[1].String())
	}
	if got[2].String() != "120363000000000000@g.us" {
		t.Errorf("group JID changed: %s", got[2].String())
	}

	if _, err := normalizeParticipantJIDs([]string{"+-()@#$%"}); err == nil {
		t.Error("expected an error for an invalid participant")
	}
}
