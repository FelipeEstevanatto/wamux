package minio_storage

import "testing"

func TestPublicPolicyRequiresExplicitOptIn(t *testing.T) {
	for _, tc := range []struct {
		name string
		o    Options
		want bool
	}{
		{"default is private/presigned", Options{}, false},
		{"public URL alone does not grant access", Options{PublicURL: "https://cdn.example.com"}, false},
		{"explicit public read", Options{PublicRead: true}, true},
		{"skip policy wins", Options{PublicRead: true, SkipBucketPolicy: true}, false},
	} {
		if got := appliesPublicPolicy(tc.o); got != tc.want {
			t.Errorf("%s: appliesPublicPolicy = %v, want %v", tc.name, got, tc.want)
		}
	}
}
