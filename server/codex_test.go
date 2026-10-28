package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Autumn-27/artex/db"
)

func TestCodexRuntimeControlDoesNotExposeCredentials(t *testing.T) {
	token := strings.Repeat("local-control-", 3)
	runtime := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+token {
			t.Error("runtime control token was not forwarded")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"available":true,"connected":false,"access_token":"must-not-escape","login":{"login_id":"login-1","state":"pending","verification_url":"https://auth.openai.com/codex/device","user_code":"TEST-CODE","refresh_token":"must-not-escape"}}`))
	}))
	defer runtime.Close()
	t.Setenv("ARTEX_CODEX_RUNTIME_URL", runtime.URL)
	t.Setenv("ARTEX_CODEX_GATEWAY_TOKEN", token)
	status, err := callCodexRuntime(context.Background(), http.MethodPost, "/v1/codex/login", map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(status)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "must-not-escape") || strings.Contains(string(data), token) {
		t.Fatalf("credential included in public response: %s", data)
	}
	if status.Login == nil || status.Login.UserCode != "TEST-CODE" {
		t.Fatal("device code was not returned")
	}
}

func TestCodexRejectsUnexpectedVerificationSites(t *testing.T) {
	for _, value := range []string{"https://auth.openai.com/codex/device", "https://chatgpt.com/"} {
		if !validCodexVerificationURL(value) {
			t.Errorf("valid verification URL rejected: %s", value)
		}
	}
	for _, value := range []string{"http://auth.openai.com/codex/device", "https://auth.openai.com.evil.test/", "https://user@auth.openai.com/", "https://auth.openai.com:1234/", "javascript:alert(1)"} {
		if validCodexVerificationURL(value) {
			t.Errorf("untrusted verification URL accepted: %s", value)
		}
	}
}

func TestCodexProfileUsesManagedRuntimeInsteadOfStoredKey(t *testing.T) {
	token := strings.Repeat("runtime-token-", 3)
	t.Setenv("ARTEX_CODEX_RUNTIME_URL", "http://codex-runtime:8791")
	t.Setenv("ARTEX_CODEX_GATEWAY_TOKEN", token)
	p := &db.LLMProfile{ID: 1, Name: "Codex", Format: "openai", Model: "runtime-model", BaseURL: codexProfileBaseURL, APIKey: "not-a-real-model-key", APIKeyHint: "must-not-display"}
	cfg := profileProviderConfig(p)
	if cfg.BaseURL != "http://codex-runtime:8791/v1" || cfg.APIKey != token {
		t.Fatal("managed runtime settings were not used")
	}
	public := llmProfileDTO(p)
	if public.Format != "codex" || public.APIKeyHint != "" || public.BaseURL != "" {
		t.Fatalf("native provider exposed API transport fields: %+v", public)
	}
}

func TestCodexUnavailableStatusIsNotConnected(t *testing.T) {
	t.Setenv("ARTEX_CODEX_RUNTIME_URL", "")
	t.Setenv("ARTEX_CODEX_GATEWAY_TOKEN", "")
	recorder := httptest.NewRecorder()
	(&Server{}).codexStatus(recorder, httptest.NewRequest(http.MethodGet, "/api/llm/codex/status", nil))
	var status codexStatus
	if err := json.Unmarshal(recorder.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if recorder.Code != http.StatusOK || status.Available || status.Connected {
		t.Fatalf("unavailable runtime reported as connected: %s", recorder.Body.String())
	}
}

func TestCodexProviderRequiresCompletedDeviceLogin(t *testing.T) {
	runtime := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"available":true,"connected":false}`))
	}))
	defer runtime.Close()
	t.Setenv("ARTEX_CODEX_RUNTIME_URL", runtime.URL)
	t.Setenv("ARTEX_CODEX_GATEWAY_TOKEN", strings.Repeat("control-token", 3))
	if _, err := codexProviderConfig(context.Background(), "runtime-model"); err == nil {
		t.Fatal("provider accepted incomplete device authorization")
	}
}
