package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type AIService struct {
	baseURL string
	model   string
	client  *http.Client
}

func NewAIService(baseURL, model string) *AIService {
	return &AIService{
		baseURL: baseURL,
		model:   model,
		client: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
}

type OllamaRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Stream bool   `json:"stream"`
	Format string `json:"format,omitempty"`
}

type OllamaResponse struct {
	Response string `json:"response"`
}

type FixResponse struct {
	FixedCode   string `json:"fixed_code"`
	Explanation string `json:"explanation"`
}

func (s *AIService) GenerateFix(codeSnippet, issueType, errorMessage, language string) (*FixResponse, error) {
	prompt := fmt.Sprintf(`You are an expert Senior Developer.
Fix the following technical debt issue in %s.

Issue Type: %s
Error/Warning: %s

Code to Fix:
%s

Respond ONLY with a valid JSON object containing:
1. "fixed_code": The refactored code string.
2. "explanation": A brief 1-sentence explanation of what you changed.
`, language, issueType, errorMessage, codeSnippet)

	reqBody := OllamaRequest{
		Model:  s.model,
		Prompt: prompt,
		Stream: false,
		Format: "json",
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", s.baseURL+"/api/generate", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to call AI provider: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("AI provider returned status %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var ollamaResp OllamaResponse
	if err := json.Unmarshal(body, &ollamaResp); err != nil {
		return nil, fmt.Errorf("failed to parse AI response wrapper: %w", err)
	}

	var fix FixResponse
	if err := json.Unmarshal([]byte(ollamaResp.Response), &fix); err != nil {
		return &FixResponse{
			Explanation: ollamaResp.Response,
			FixedCode:   "",
		}, nil
	}

	return &fix, nil
}
