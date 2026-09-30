package minio_storage

import "testing"

func TestNormalizeEndpoint(t *testing.T) {
	cases := []struct {
		in   string
		host string
		ssl  bool
	}{
		{"https://s3.amazonaws.com", "s3.amazonaws.com", true},
		{"http://minio:9000", "minio:9000", false},
		{"minio:9000", "minio:9000", false},
		{"https://s3.example.com/", "s3.example.com", true},
		{"  http://host:9000/  ", "host:9000", false},
		{"", "", false},
	}
	for _, tc := range cases {
		host, ssl := NormalizeEndpoint(tc.in)
		if host != tc.host || ssl != tc.ssl {
			t.Errorf("NormalizeEndpoint(%q) = (%q, %v), want (%q, %v)", tc.in, host, ssl, tc.host, tc.ssl)
		}
	}
}
