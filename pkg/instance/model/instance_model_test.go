package instance_model

import "testing"

func TestBeforeCreateGeneratesIDWhenEmpty(t *testing.T) {
	m := &Instance{}
	if err := m.BeforeCreate(nil); err != nil {
		t.Fatal(err)
	}
	if m.Id == "" {
		t.Fatal("expected a generated id")
	}
}

// An existing id must survive insert (e.g. an imported/preselected id), unlike
// the label/message hooks which always overwrite.
func TestBeforeCreatePreservesExistingID(t *testing.T) {
	m := &Instance{Id: "keep-me"}
	if err := m.BeforeCreate(nil); err != nil {
		t.Fatal(err)
	}
	if m.Id != "keep-me" {
		t.Fatalf("id = %q, want keep-me", m.Id)
	}
}

func TestBoolPtr(t *testing.T) {
	p := BoolPtr(true)
	if p == nil || !*p {
		t.Fatalf("BoolPtr(true) = %v", p)
	}
	if q := BoolPtr(false); q == nil || *q {
		t.Fatalf("BoolPtr(false) = %v", q)
	}
}

func TestIsPaired(t *testing.T) {
	tests := []struct {
		name     string
		instance *Instance
		want     bool
	}{
		{name: "nil receiver", instance: nil, want: false},
		{name: "no jid", instance: &Instance{}, want: false},
		{name: "logged out clears jid", instance: &Instance{Jid: ""}, want: false},
		{name: "paired", instance: &Instance{Jid: "5511999999999@s.whatsapp.net"}, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.instance.IsPaired(); got != tt.want {
				t.Fatalf("IsPaired() = %v, want %v", got, tt.want)
			}
		})
	}
}
