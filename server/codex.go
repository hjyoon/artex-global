package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/Autumn-27/artex/agent"
	"github.com/Autumn-27/artex/db"
)

// Persist a provider reference, never an OAuth credential or runtime token.
const codexProfileBaseURL = "codex://managed"
const codexCredentialMarker = "managed-by-codex"

type codexLogin struct {
	LoginID         string `json:"login_id"`
	State           string `json:"state"`
	VerificationURL string `json:"verification_url,omitempty"`
	UserCode        string `json:"user_code,omitempty"`
}

type codexAccount struct {
	Email    string `json:"email,omitempty"`
	PlanType string `json:"plan_type,omitempty"`
}

type codexStatus struct {
	Available    bool          `json:"available"`
	Connected    bool          `json:"connected"`
	Account      *codexAccount `json:"account,omitempty"`
	Login        *codexLogin   `json:"login,omitempty"`
	Models       []string      `json:"models,omitempty"`
	DefaultModel string        `json:"default_model,omitempty"`
}

func codexRuntimeSettings() (string, string, bool) {
	base := strings.TrimRight(strings.TrimSpace(os.Getenv("ARTEX_CODEX_RUNTIME_URL")), "/")
	token := os.Getenv("ARTEX_CODEX_GATEWAY_TOKEN")
	u, err := url.Parse(base)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", "", false
	}
	return base, token, len(token) >= 24
}

func isCodexProfile(p *db.LLMProfile) bool {
	return p != nil && p.Format == "openai" && p.BaseURL == codexProfileBaseURL
}

func profileProviderConfig(p *db.LLMProfile) agent.Config {
	if isCodexProfile(p) {
		base, token, ok := codexRuntimeSettings()
		if !ok {
			return agent.ConfigFrom("openai", p.Model, "", "", "")
		}
		return agent.ConfigFrom("openai", p.Model, base+"/v1", token, "")
	}
	return agent.ConfigFrom(p.Format, p.Model, p.BaseURL, p.APIKey, p.Proxy)
}

func validCodexVerificationURL(value string) bool {
	u, err := url.Parse(value)
	return err == nil && u.Scheme == "https" && u.User == nil &&
		(u.Port() == "" || u.Port() == "443") &&
		(u.Hostname() == "auth.openai.com" || u.Hostname() == "chatgpt.com")
}

func callCodexRuntime(ctx context.Context, method, path string, body any) (codexStatus, error) {
	var result codexStatus
	base, token, ok := codexRuntimeSettings()
	if !ok {
		return result, errors.New("Codex runtime is not configured; start runtime/start.sh")
	}
	var input io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return result, err
		}
		input = bytes.NewReader(encoded)
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, method, base+path, input)
	if err != nil {
		return result, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		return result, errors.New("Codex runtime is unavailable; check the runtime container")
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return result, err
	}
	if resp.StatusCode != http.StatusOK {
		return result, fmt.Errorf("Codex runtime request failed (HTTP %d)", resp.StatusCode)
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return result, errors.New("Invalid Codex runtime response")
	}
	if result.Login != nil && result.Login.VerificationURL != "" && !validCodexVerificationURL(result.Login.VerificationURL) {
		return codexStatus{}, errors.New("Invalid Codex verification URL")
	}
	if result.Login != nil && result.Login.State == "pending" && (result.Login.LoginID == "" || result.Login.UserCode == "" || result.Login.VerificationURL == "") {
		return codexStatus{}, errors.New("Incomplete Codex device login response")
	}
	return result, nil
}

func (s *Server) codexStatus(w http.ResponseWriter, r *http.Request) {
	status, err := callCodexRuntime(r.Context(), http.MethodGet, "/v1/codex/status", nil)
	if err != nil {
		writeJSON(w, http.StatusOK, codexStatus{Available: false})
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (s *Server) codexLogin(w http.ResponseWriter, r *http.Request) {
	s.codexAction(w, r, "/v1/codex/login", map[string]any{})
}

func (s *Server) codexCancel(w http.ResponseWriter, r *http.Request) {
	var body struct {
		LoginID string `json:"login_id"`
	}
	if err := decode(r, &body); err != nil || body.LoginID == "" || len(body.LoginID) > 128 {
		writeErr(w, http.StatusBadRequest, "A valid Codex login_id is required")
		return
	}
	s.codexAction(w, r, "/v1/codex/cancel", body)
}

func (s *Server) codexLogout(w http.ResponseWriter, r *http.Request) {
	s.codexAction(w, r, "/v1/codex/logout", map[string]any{})
}

func (s *Server) codexAction(w http.ResponseWriter, r *http.Request, path string, body any) {
	status, err := callCodexRuntime(r.Context(), http.MethodPost, path, body)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func codexProviderConfig(ctx context.Context, model string) (agent.Config, error) {
	status, err := callCodexRuntime(ctx, http.MethodGet, "/v1/codex/status", nil)
	if err != nil {
		return agent.Config{}, err
	}
	if !status.Connected {
		return agent.Config{}, errors.New("Sign in with Codex device code first")
	}
	if strings.TrimSpace(model) == "" {
		model = status.DefaultModel
	}
	if model == "" {
		return agent.Config{}, errors.New("Select a Codex model")
	}
	base, token, _ := codexRuntimeSettings()
	return agent.ConfigFrom("openai", model, base+"/v1", token, ""), nil
}

func prepareCodexProfile(ctx context.Context, p *db.LLMProfile) error {
	cfg, err := codexProviderConfig(ctx, p.Model)
	if err != nil {
		return err
	}
	p.Format, p.Model = "openai", cfg.Model
	p.BaseURL, p.APIKey, p.APIKeyHint = codexProfileBaseURL, codexCredentialMarker, ""
	p.Proxy, p.ThinkingType, p.SessionHeaderKey, p.MaxTokensField = "", "", "", ""
	p.Streaming, p.MaxTokens = true, 0
	return nil
}
