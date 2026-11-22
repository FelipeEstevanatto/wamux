package instance_service

import (
	"testing"

	instance_model "github.com/felipeestevanatto/wamux/pkg/instance/model"
	"github.com/felipeestevanatto/wamux/pkg/tokencrypt"
)

// Reads hydrate Token from TokenEnc, so the plaintext field alone looks like a
// legacy row. A row that already has TokenEnc must not be rewritten on every
// boot (which also created a per-instance logger and a DB write per row).
func TestBackfillTokenSkipsAlreadyEncryptedRows(t *testing.T) {
	codec, err := tokencrypt.New("test-key")
	if err != nil {
		t.Fatal(err)
	}
	inst := &instance_model.Instance{
		Id:        "already-encrypted",
		Token:     "plaintext-hydrated-from-TokenEnc",
		TokenEnc:  "ciphertext",
		TokenHash: "hash",
	}
	svc := &instances{tokenCodec: codec, instanceRepository: nil, loggerWrapper: nil}

	if err := svc.backfillToken(inst, inst.Token); err != nil {
		t.Fatalf("backfillToken: %v", err)
	}
	if inst.Token != "plaintext-hydrated-from-TokenEnc" || inst.TokenEnc != "ciphertext" || inst.TokenHash != "hash" {
		t.Fatalf("already encrypted row was rewritten: %+v", inst)
	}
}
