package poll_service

import "testing"

func TestStringArrayToPostgresArray(t *testing.T) {
	if got := stringArrayToPostgresArray(nil); got != "{}" {
		t.Fatalf("empty = %q, want {}", got)
	}
	if got := stringArrayToPostgresArray([]string{"a", "b"}); got != "{a,b}" {
		t.Fatalf("got %q, want {a,b}", got)
	}
}

func TestPostgresArrayToStringSlice(t *testing.T) {
	if got := postgresArrayToStringSlice("{}"); len(got) != 0 {
		t.Fatalf("empty = %v, want no elements", got)
	}
	got := postgresArrayToStringSlice("{a,b}")
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("got %v, want [a b]", got)
	}
}

func TestPollOptionArrayRoundTrip(t *testing.T) {
	in := []string{"deadbeef", "cafebabe"}
	if got := postgresArrayToStringSlice(stringArrayToPostgresArray(in)); len(got) != 2 || got[0] != in[0] || got[1] != in[1] {
		t.Fatalf("round trip = %v, want %v", got, in)
	}
}
