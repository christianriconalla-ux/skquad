package kube

import "testing"

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
