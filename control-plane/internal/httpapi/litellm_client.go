package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type liteLLMGatewayClient struct {
	baseURL   string
	masterKey string
	client    *http.Client
}

func newLiteLLMGatewayClient(baseURL, masterKey string) *liteLLMGatewayClient {
	return &liteLLMGatewayClient{
		baseURL:   strings.TrimRight(baseURL, "/"),
		masterKey: strings.TrimSpace(masterKey),
		client:    http.DefaultClient,
	}
}

func (c *liteLLMGatewayClient) ProvisionAgentKey(ctx context.Context, req GatewayKeyRequest) (string, error) {
	body := map[string]any{
		"models": req.Models,
		"metadata": map[string]string{
			"skquad_agent_id": req.AgentID,
			"skquad_squad_id": req.SquadID,
		},
		"key_alias": fmt.Sprintf("skquad-agent-%s", req.AgentID),
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("litellm: marshal key request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/key/generate", bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("litellm: build key request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.masterKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("litellm: generate key: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("litellm: generate key: %s: %s", resp.Status, gatewayResponseSnippet(resp.Body))
	}

	var out struct {
		Key string `json:"key"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("litellm: decode key response: %w", err)
	}
	if strings.TrimSpace(out.Key) == "" {
		return "", fmt.Errorf("litellm: key response did not include key")
	}
	return out.Key, nil
}

func gatewayResponseSnippet(r io.Reader) string {
	body, err := io.ReadAll(io.LimitReader(r, 2048))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(body))
}
