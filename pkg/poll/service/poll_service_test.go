package poll_service

import (
	"testing"
	"time"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
)

// The caller (whatsmeow handlePollVote) resolves the voter's LID to a phone
// number and strips the device suffix before calling this. The builder must
// then store the PN form, matching how the webhook reports Sender everywhere
// else, instead of the raw "269182931329179:39@lid".
func TestBuildPollVoteFromEventUsesResolvedPN(t *testing.T) {
	pollInfo := &types.MessageInfo{
		ID:            "POLLMSG",
		MessageSource: types.MessageSource{Chat: types.NewJID("120363000000000000", types.GroupServer)},
	}
	voteInfo := &types.MessageInfo{
		ID:            "VOTEMSG",
		MessageSource: types.MessageSource{Sender: types.NewJID("5514981170846", types.DefaultUserServer)},
		PushName:      "Felipe",
		Timestamp:     time.Unix(1790139245, 0),
	}

	vote := BuildPollVoteFromEvent(pollInfo, voteInfo, &waE2E.PollVoteMessage{
		SelectedOptions: [][]byte{{0xab, 0xcd}},
	}, "", "instance-1")

	if vote.VoterJid != "5514981170846@s.whatsapp.net" {
		t.Errorf("VoterJid = %q, want the PN form", vote.VoterJid)
	}
	if vote.VoterPhone != "5514981170846" {
		t.Errorf("VoterPhone = %q, want the phone number", vote.VoterPhone)
	}
	if vote.VoterName != "Felipe" {
		t.Errorf("VoterName = %q", vote.VoterName)
	}
	if len(vote.SelectedOptions) != 1 || vote.SelectedOptions[0] != "abcd" {
		t.Errorf("SelectedOptions = %v, want hex of the option bytes", vote.SelectedOptions)
	}
	if vote.PollMessageID != "POLLMSG" || vote.VoteMessageID != "VOTEMSG" {
		t.Errorf("ids not carried over: %+v", vote)
	}
}

// A device suffix must never leak into the stored identifiers.
func TestBuildPollVoteFromEventStripsDeviceSuffix(t *testing.T) {
	voteInfo := &types.MessageInfo{
		ID:            "V",
		MessageSource: types.MessageSource{Sender: types.NewJID("5514981170846", types.DefaultUserServer)},
	}
	voteInfo.Sender.Device = 39

	vote := BuildPollVoteFromEvent(
		&types.MessageInfo{ID: "P"},
		voteInfo,
		&waE2E.PollVoteMessage{SelectedOptions: [][]byte{[]byte("x")}},
		"", "i",
	)

	if vote.VoterJid != "5514981170846@s.whatsapp.net" {
		t.Errorf("VoterJid = %q, want no device suffix", vote.VoterJid)
	}
}
