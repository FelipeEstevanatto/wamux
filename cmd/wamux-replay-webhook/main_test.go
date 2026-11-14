package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/felipeestevanatto/wamux/pkg/webhooksign"
)

func TestParseEnvelopeObjectAndStringPayloads(t *testing.T) {
	env, payload, err := parseEnvelope([]byte(`{"url":"https://example/hook","userID":"i","payload":{"a":1}}`))
	if err != nil || env.URL != "https://example/hook" || string(payload) != `{"a":1}` {
		t.Fatalf("object: %+v %q %v", env, payload, err)
	}
	_, payload, err = parseEnvelope([]byte(`{"url":"https://example/hook","payload":"plain text"}`))
	if err != nil || string(payload) != "plain text" {
		t.Fatalf("string: %q %v", payload, err)
	}
}

func TestParseEnvelopeRejectsBadURLAndEmptyPayload(t *testing.T) {
	for _, body := range []string{
		`{"url":"ftp://example/hook","payload":{"a":1}}`,
		`{"url":"https://example/hook"}`,
		`not json`,
	} {
		if _, _, err := parseEnvelope([]byte(body)); err == nil {
			t.Errorf("accepted %q", body)
		}
	}
}

func TestPlanHashIsStableAcrossOrder(t *testing.T) {
	a := amqp.Delivery{Body: []byte(`{"a":1}`)}
	b := amqp.Delivery{Body: []byte(`{"b":2}`)}
	if planHash([]amqp.Delivery{a, b}) != planHash([]amqp.Delivery{b, a}) {
		t.Fatal("plan hash depends on queue order")
	}
	if planHash([]amqp.Delivery{a, b}) == planHash([]amqp.Delivery{a}) {
		t.Fatal("different plans have the same hash")
	}
}

func TestDeliverPostsPayloadAndSigns(t *testing.T) {
	var body string
	var signature string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		body = string(raw)
		signature = r.Header.Get(webhooksign.SignatureHeader)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	if err := deliver(context.Background(), &http.Client{Timeout: time.Second}, server.URL, []byte(`{"x":1}`), "01234567890123456789012345678901"); err != nil {
		t.Fatalf("deliver: %v", err)
	}
	if body != `{"x":1}` || !webhooksign.Verify([]byte("01234567890123456789012345678901"), []byte(body), signature) {
		t.Fatalf("body=%q signature=%q", body, signature)
	}
}

func TestHMACSourceReadsAndDecryptsInstanceKey(t *testing.T) {
	encKey, err := webhooksign.DeriveWebhookHMACEncryptionKey("dedicated-secret", "")
	if err != nil {
		t.Fatal(err)
	}
	plain := "01234567890123456789012345678901"
	encrypted, err := webhooksign.Encrypt(encKey, plain)
	if err != nil {
		t.Fatal(err)
	}

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectQuery(`SELECT COALESCE\(hmac_key, ''\) FROM instances WHERE id = \$1`).
		WithArgs("inst-1").
		WillReturnRows(sqlmock.NewRows([]string{"hmac_key"}).AddRow(encrypted))

	s := &hmacSource{db: db, encKey: encKey, cache: map[string]string{}}
	got, err := s.key("inst-1")
	if err != nil || got != plain {
		t.Fatalf("key = %q, err %v", got, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestHMACSourceFailsClosedForEncryptedKeyWithoutSecret(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectQuery(`SELECT COALESCE\(hmac_key, ''\) FROM instances WHERE id = \$1`).
		WithArgs("inst-1").
		WillReturnRows(sqlmock.NewRows([]string{"hmac_key"}).AddRow("ciphertext"))

	s := &hmacSource{db: db, cache: map[string]string{}}
	if _, err := s.key("inst-1"); err == nil {
		t.Fatal("encrypted key without an encryption secret was accepted")
	}
}

func TestDeliverRejectsNon2xx(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer server.Close()
	if err := deliver(context.Background(), &http.Client{Timeout: time.Second}, server.URL, []byte(`{}`), ""); err == nil {
		t.Fatal("502 accepted")
	}
}
