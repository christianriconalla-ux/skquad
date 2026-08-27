package kube

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSecretNameFromRef(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"k8s://squad-test/agent-credential": "agent-credential",
		"plain-secret":                      "plain-secret",
		"llm-gateway://virtual-keys/agent":  "",
		"squad-test/agent-credential":       "",
		"":                                  "",
	}
	for ref, want := range tests {
		if got := secretNameFromRef(ref); got != want {
			t.Fatalf("secretNameFromRef(%q) = %q, want %q", ref, got, want)
		}
	}
}

func TestSecretTargetFromRef(t *testing.T) {
	t.Parallel()

	namespace, name := secretTargetFromRef("k8s://squad-test/agent-credential")
	if namespace != "squad-test" || name != "agent-credential" {
		t.Fatalf("secret target = %q/%q, want squad-test/agent-credential", namespace, name)
	}

	namespace, name = secretTargetFromRef("llm-gateway://virtual-keys/agent")
	if namespace != "" || name != "" {
		t.Fatalf("non-Kubernetes ref target = %q/%q, want empty", namespace, name)
	}
}

func TestWriteAgentCredentialAppliesOpaqueSecret(t *testing.T) {
	t.Parallel()

	var gotPath string
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.RequestURI()
		if r.Method != http.MethodPatch {
			t.Fatalf("method = %s, want PATCH", r.Method)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Fatalf("authorization = %q, want bearer token", got)
		}
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatal(err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	writer := &CRWriter{
		baseURL: server.URL,
		token:   "test-token",
		client:  server.Client(),
	}

	if err := writer.WriteAgentCredential(
		context.Background(),
		"k8s://squad-test/agent-credential",
		"agent-1",
		"runtime-token",
	); err != nil {
		t.Fatal(err)
	}

	if gotPath != "/api/v1/namespaces/squad-test/secrets/agent-credential?fieldManager=skquad-control-plane&force=true" {
		t.Fatalf("path = %q", gotPath)
	}
	data := gotBody["data"].(map[string]any)
	if got := data["token"]; got != base64.StdEncoding.EncodeToString([]byte("runtime-token")) {
		t.Fatalf("encoded token = %q", got)
	}
}
