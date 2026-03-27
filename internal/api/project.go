package api

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Timestamp handles JSON values that may be either a string or a number (epoch ms).
type Timestamp string

func (t *Timestamp) UnmarshalJSON(data []byte) error {
	// Try string first
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		*t = Timestamp(s)
		return nil
	}

	// Try number (epoch ms)
	var n float64
	if err := json.Unmarshal(data, &n); err == nil {
		ts := time.UnixMilli(int64(n))
		*t = Timestamp(ts.Format(time.RFC3339))
		return nil
	}

	// Fallback: store raw
	*t = Timestamp(string(data))
	return nil
}

func (t Timestamp) MarshalJSON() ([]byte, error) {
	return json.Marshal(string(t))
}

// Project represents a Poof project.
type Project struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Slug        string `json:"slug"`
	IsPublic    bool   `json:"isPublic"`
}

type ListProjectsResponse struct {
	Projects []Project `json:"projects"`
	HasMore  bool      `json:"hasMore"`
}

type CreateProjectRequest struct {
	FirstMessage   string `json:"firstMessage"`
	TarobaseToken  string `json:"tarobaseToken"`
	IsPublic       bool   `json:"isPublic"`
	GenerationMode string `json:"generationMode,omitempty"`
}

type CreateProjectResponse struct {
	Success   bool   `json:"success"`
	ProjectID string `json:"projectId"`
	Message   string `json:"message"`
}

func (r *CreateProjectResponse) QuietString() string { return r.ProjectID }

type UpdateProjectRequest struct {
	Title           string                 `json:"title,omitempty"`
	Name            string                 `json:"name,omitempty"`
	Description     string                 `json:"description,omitempty"`
	Slug            string                 `json:"slug,omitempty"`
	IsPublic        *bool                  `json:"isPublic,omitempty"`
	IsAppPagePublic *bool                  `json:"isAppPagePublic,omitempty"`
	Network         string                 `json:"network,omitempty"`
	TarobaseEnv     string                 `json:"tarobaseEnv,omitempty"`
	IsPlanMode      *bool                  `json:"isPlanMode,omitempty"`
	Auth            map[string]interface{} `json:"auth,omitempty"`
	Settings        map[string]interface{} `json:"settings,omitempty"`
	Screenshots     []string               `json:"screenshots,omitempty"`
	AppPageSettings map[string]interface{} `json:"appPageSettings,omitempty"`
	IsFavorite      *bool                  `json:"isFavorite,omitempty"`
	GenerationMode  string                 `json:"generationMode,omitempty"`
}

// ConnectionEnv holds per-environment Tarobase connection details.
type ConnectionEnv struct {
	TarobaseAppId string `json:"tarobaseAppId"`
	BackendUrl    string `json:"backendUrl"`
}

// ConnectionInfo holds Tarobase connection info returned by the status API.
type ConnectionInfo struct {
	Draft      *ConnectionEnv `json:"draft"`
	Preview    *ConnectionEnv `json:"preview"`
	Production *ConnectionEnv `json:"production"`
	WsUrl      string         `json:"wsUrl"`
	ApiUrl     string         `json:"apiUrl"`
	AuthApiUrl string         `json:"authApiUrl"`
}

type ProjectStatus struct {
	Project        Project                `json:"project"`
	LatestTask     map[string]interface{} `json:"latestTask"`
	PublishState   map[string]interface{} `json:"publishState"`
	URLs           map[string]string      `json:"urls"`
	ConnectionInfo *ConnectionInfo        `json:"connectionInfo,omitempty"`
}

type MessagesResponse struct {
	Messages []Message `json:"messages"`
	HasMore  bool      `json:"hasMore"`
}

type Message struct {
	ID        string    `json:"id"`
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	CreatedAt Timestamp `json:"createdAt"`
	Status    string    `json:"status"`
}

func (c *Client) ListProjects(ctx context.Context, limit, offset int) (*ListProjectsResponse, error) {
	path := fmt.Sprintf("/api/project?limit=%d&offset=%d", limit, offset)
	body, err := c.Do(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}

	var resp ListProjectsResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	return &resp, nil
}

func (c *Client) CreateProject(ctx context.Context, req CreateProjectRequest) (*CreateProjectResponse, error) {
	projectID := uuid.New().String()
	path := fmt.Sprintf("/api/project/%s", projectID)

	body, err := c.doWithTokenBody(ctx, "POST", path, func() (interface{}, error) {
		token, err := c.AuthManager.GetToken()
		if err != nil {
			return nil, err
		}
		req.TarobaseToken = token
		return req, nil
	})
	if err != nil {
		return nil, err
	}

	var resp CreateProjectResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	if resp.ProjectID == "" {
		resp.ProjectID = projectID
	}
	return &resp, nil
}

func (c *Client) UpdateProject(ctx context.Context, projectID string, req UpdateProjectRequest) error {
	path := fmt.Sprintf("/api/project/%s", projectID)
	_, err := c.Do(ctx, "PUT", path, req)
	return err
}

func (c *Client) DeleteProject(ctx context.Context, projectID string) error {
	path := fmt.Sprintf("/api/project/%s", projectID)
	_, err := c.Do(ctx, "DELETE", path, nil)
	return err
}

func (c *Client) GetProjectStatus(ctx context.Context, projectID string) (*ProjectStatus, error) {
	path := fmt.Sprintf("/api/project/%s/status", projectID)
	body, err := c.Do(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}

	var resp ProjectStatus
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	return &resp, nil
}

func (c *Client) GetMessages(ctx context.Context, projectID string, limit, offset int) (*MessagesResponse, error) {
	path := fmt.Sprintf("/api/project/%s/messages?limit=%d&offset=%d", projectID, limit, offset)
	body, err := c.Do(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}

	var resp MessagesResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	return &resp, nil
}
