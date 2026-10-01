package message_model

import "testing"

// Every persisted message gets a fresh id; a caller-supplied id is overwritten
// so the primary key can never collide or be spoofed by the request payload.
func TestMessageBeforeCreateAlwaysSetsID(t *testing.T) {
	m := &Message{}
	if err := m.BeforeCreate(nil); err != nil {
		t.Fatal(err)
	}
	if m.Id == "" {
		t.Fatal("expected a generated id")
	}

	m2 := &Message{Id: "caller-supplied"}
	if err := m2.BeforeCreate(nil); err != nil {
		t.Fatal(err)
	}
	if m2.Id == "" || m2.Id == "caller-supplied" {
		t.Fatalf("id = %q, want a freshly generated id", m2.Id)
	}
}
