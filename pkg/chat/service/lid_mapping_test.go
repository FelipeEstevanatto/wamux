package chat_service

import (
	"context"
	"testing"

	"go.mau.fi/whatsmeow/types"
)

type fakeLIDStore struct {
	pnByLID map[types.JID]types.JID
	lidByPN map[types.JID]types.JID
}

func (f fakeLIDStore) GetPNForLID(_ context.Context, lid types.JID) (types.JID, error) {
	return f.pnByLID[lid], nil
}

func (f fakeLIDStore) GetManyLIDsForPNs(_ context.Context, pns []types.JID) (map[types.JID]types.JID, error) {
	out := map[types.JID]types.JID{}
	for _, pn := range pns {
		if lid, ok := f.lidByPN[pn]; ok {
			out[pn] = lid
		}
	}
	return out, nil
}

func TestNormalizeMappingJID(t *testing.T) {
	if jid, ok := normalizeMappingJID("5511999999999", "pn"); !ok || jid.Server != types.DefaultUserServer {
		t.Fatalf("bare pn = %v %v", jid, ok)
	}
	if jid, ok := normalizeMappingJID("1234567890", "lid"); !ok || jid.Server != types.HiddenUserServer {
		t.Fatalf("bare lid = %v %v", jid, ok)
	}
	if jid, ok := normalizeMappingJID("123:5@lid", "lid"); !ok || jid.Device != 0 {
		t.Fatalf("device lid = %v %v", jid, ok)
	}
	if _, ok := normalizeMappingJID("123@s.whatsapp.net", "lid"); ok {
		t.Fatal("pn accepted as lid")
	}
	if _, ok := normalizeMappingJID("", "pn"); ok {
		t.Fatal("empty accepted")
	}
}

func TestResolveLidMappingBothDirections(t *testing.T) {
	lid := types.NewJID("1234567890", types.HiddenUserServer)
	pn := types.NewJID("5511999999999", types.DefaultUserServer)
	store := fakeLIDStore{
		pnByLID: map[types.JID]types.JID{lid: pn},
		lidByPN: map[types.JID]types.JID{pn: lid},
	}

	got, err := resolveLidMapping(context.Background(), store, &LidMappingStruct{
		Lids: []string{"1234567890@lid", "unknown@lid"},
		Pns:  []string{"5511999999999", "5511888888888"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if v := got.Lids["1234567890@lid"]; v == nil || *v != pn.String() {
		t.Fatalf("lid mapping = %v", v)
	}
	if got.Lids["unknown@lid"] != nil {
		t.Fatal("unknown lid was not null")
	}
	if v := got.Pns["5511999999999"]; v == nil || *v != lid.String() {
		t.Fatalf("pn mapping = %v", v)
	}
	if got.Pns["5511888888888"] != nil {
		t.Fatal("unknown pn was not null")
	}
}
