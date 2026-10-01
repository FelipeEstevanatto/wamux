package label_model

import "testing"

func TestLabelBeforeCreateAlwaysSetsID(t *testing.T) {
	m := &Label{}
	if err := m.BeforeCreate(nil); err != nil {
		t.Fatal(err)
	}
	if m.Id == "" {
		t.Fatal("expected a generated id")
	}

	m2 := &Label{Id: "caller-supplied"}
	if err := m2.BeforeCreate(nil); err != nil {
		t.Fatal(err)
	}
	if m2.Id == "" || m2.Id == "caller-supplied" {
		t.Fatalf("id = %q, want a freshly generated id", m2.Id)
	}
}
