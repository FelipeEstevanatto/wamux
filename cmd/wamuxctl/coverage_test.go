package main

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"
)

type manifestOperation struct {
	Operation string `json:"operation"`
	Method    string `json:"method"`
	Endpoint  string `json:"endpoint"`
	Status    string `json:"status"`
	Tests     string `json:"tests"`
}

func TestCoverageManifestIsComplete(t *testing.T) {
	data, err := os.ReadFile("../../docs/archive/plans/cloud-api-coverage.json")
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Operations []manifestOperation `json:"operations"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	valid := map[string]bool{"implemented": true, "deferred": true, "separate": true}
	for _, op := range manifest.Operations {
		if op.Operation == "" || op.Method == "" || op.Endpoint == "" {
			t.Fatalf("incomplete operation: %+v", op)
		}
		if !valid[op.Status] {
			t.Fatalf("invalid status for %s: %s", op.Operation, op.Status)
		}
		if seen[op.Operation] {
			t.Fatalf("duplicate operation: %s", op.Operation)
		}
		seen[op.Operation] = true
	}
	if !seen["send text"] || !seen["groups"] {
		t.Fatalf("manifest is missing core or separate operations: %+v", seen)
	}

	var out, errOut bytes.Buffer
	if code := run([]string{"cloud", "coverage", "--json"}, &out, &errOut); code != 0 {
		t.Fatalf("code=%d err=%s", code, errOut.String())
	}
	var cli []struct {
		Operation string `json:"operation"`
	}
	if err := json.Unmarshal(out.Bytes(), &cli); err != nil {
		t.Fatal(err)
	}
	for _, op := range cli {
		if !seen[op.Operation] {
			t.Fatalf("CLI exposes operation %q absent from the manifest", op.Operation)
		}
	}
}
