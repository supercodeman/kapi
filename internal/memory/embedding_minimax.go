package memory

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type MiniMaxEmbeddingProvider struct {
	baseURL string
	apiKey  string
	model   string
	client  *http.Client
}

func NewMiniMaxEmbeddingProvider(baseURL, apiKey, model string) *MiniMaxEmbeddingProvider {
	if model == "" {
		model = "embo-01"
	}
	return &MiniMaxEmbeddingProvider{
		baseURL: baseURL,
		apiKey:  apiKey,
		model:   model,
		client:  &http.Client{Timeout: 30 * time.Second},
	}
}

type minimaxEmbeddingRequest struct {
	Model string   `json:"model"`
	Texts []string `json:"texts"`
	Type  string   `json:"type"`
}

type minimaxEmbeddingResponse struct {
	Vectors  [][]float32 `json:"vectors"`
	BaseResp struct {
		StatusCode int    `json:"status_code"`
		StatusMsg  string `json:"status_msg"`
	} `json:"base_resp"`
}

func (p *MiniMaxEmbeddingProvider) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	reqBody := minimaxEmbeddingRequest{
		Model: p.model,
		Texts: texts,
		Type:  "db",
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal embedding request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create embedding request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("embedding http request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read embedding response: %w", err)
	}

	var embResp minimaxEmbeddingResponse
	if err := json.Unmarshal(respBody, &embResp); err != nil {
		return nil, fmt.Errorf("decode embedding response: %w", err)
	}

	if embResp.BaseResp.StatusCode != 0 {
		return nil, fmt.Errorf("embedding api error %d: %s", embResp.BaseResp.StatusCode, embResp.BaseResp.StatusMsg)
	}

	if len(embResp.Vectors) == 0 {
		return nil, nil
	}

	return embResp.Vectors, nil
}
