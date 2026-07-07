package evolutionapi_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kituomenyu/lango/internal/infrastructure/evolutionapi"
)

// TestCreateInstance_AlreadyExists_TreatedAsIdempotentSuccess locks in a bug
// found empirically against evoapicloud/evolution-api:v2.3.6: it reports an
// already-existing instance as HTTP 403 Forbidden with a
// `{"message":["... already in use."]}` body, not the 409 Conflict one might
// expect — CreateInstance must treat both as success, since the connect flow
// calls it on every poll (idempotent by design).
func TestCreateInstance_AlreadyExists_TreatedAsIdempotentSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"status":403,"error":"Forbidden","response":{"message":["This name \"abc\" is already in use."]}}`))
	}))
	defer srv.Close()

	client := evolutionapi.NewAdminClient(srv.URL, "test-key")
	if err := client.CreateInstance(context.Background(), "abc"); err != nil {
		t.Fatalf("expected 403 'already in use' to be treated as success, got error: %v", err)
	}
}

func TestCreateInstance_Conflict409_TreatedAsIdempotentSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
	}))
	defer srv.Close()

	client := evolutionapi.NewAdminClient(srv.URL, "test-key")
	if err := client.CreateInstance(context.Background(), "abc"); err != nil {
		t.Fatalf("expected 409 to be treated as success, got error: %v", err)
	}
}

func TestCreateInstance_OtherForbidden_StillFails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"status":403,"error":"Forbidden","response":{"message":["invalid api key"]}}`))
	}))
	defer srv.Close()

	client := evolutionapi.NewAdminClient(srv.URL, "wrong-key")
	if err := client.CreateInstance(context.Background(), "abc"); err == nil {
		t.Fatal("expected a genuine 403 (not 'already in use') to still return an error")
	}
}
