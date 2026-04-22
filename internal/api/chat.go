package api

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
)

type ChatRequest struct {
	Message       string   `json:"message"`
	MessageID     string   `json:"messageId"`
	TarobaseToken string   `json:"tarobaseToken"`
	AttachedFiles []string `json:"attachedFiles,omitempty"`
}

type ChatResponse struct {
	Success   bool   `json:"success"`
	MessageID string `json:"messageId"`
}

func (r *ChatResponse) QuietString() string { return r.MessageID }

type ClearSessionResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

func (r *ClearSessionResponse) QuietString() string {
	if r.Message != "" {
		return r.Message
	}
	if r.Success {
		return "session cleared"
	}
	return ""
}

type AIActiveResponse struct {
	Active bool   `json:"active"`
	State  string `json:"state,omitempty"`
	Status string `json:"status"`
}

func (r *AIActiveResponse) QuietString() string {
	if r.State != "" {
		return r.State
	}
	if r.Active {
		return "active"
	}
	return "inactive"
}

type SteerRequest struct {
	Message   string `json:"message"`
	MessageID string `json:"messageId,omitempty"`
}

func (c *Client) Chat(ctx context.Context, projectID, message string, attachedFiles []string) (*ChatResponse, error) {
	path := fmt.Sprintf("/api/project/%s/chat", projectID)

	body, err := c.doWithTokenBody(ctx, "POST", path, func() (interface{}, error) {
		token, err := c.AuthManager.GetToken()
		if err != nil {
			return nil, err
		}
		return ChatRequest{
			Message:       message,
			MessageID:     uuid.New().String(),
			TarobaseToken: token,
			AttachedFiles: attachedFiles,
		}, nil
	})
	if err != nil {
		return nil, err
	}

	var resp ChatResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	return &resp, nil
}

func (c *Client) CheckAIActive(ctx context.Context, projectID string) (*AIActiveResponse, error) {
	path := fmt.Sprintf("/api/project/%s/ai/active", projectID)
	body, err := c.Do(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}

	var resp AIActiveResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	return &resp, nil
}

func (c *Client) CancelAI(ctx context.Context, projectID string) error {
	path := fmt.Sprintf("/api/project/%s/cancel", projectID)
	_, err := c.Do(ctx, "POST", path, nil)
	return err
}

func (c *Client) ClearAISession(ctx context.Context, projectID string) (*ClearSessionResponse, error) {
	path := fmt.Sprintf("/api/project/%s/session/clear", projectID)
	body, err := c.Do(ctx, "POST", path, nil)
	if err != nil {
		return nil, err
	}

	var resp ClearSessionResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	return &resp, nil
}

func (c *Client) SteerAI(ctx context.Context, projectID, message, messageID string) error {
	path := fmt.Sprintf("/api/project/%s/steer", projectID)
	_, err := c.Do(ctx, "POST", path, SteerRequest{Message: message, MessageID: messageID})
	return err
}
