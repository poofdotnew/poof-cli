package api

import (
	"context"
	"encoding/json"
	"fmt"
)

// filesRawResponse matches the server's response shape for deserialization.
type filesRawResponse struct {
	FilesWithContent map[string]string `json:"filesWithContent"`
}

// FilesResponse is the normalized output shape (matches MCP response).
type FilesResponse struct {
	Files map[string]string `json:"files"`
}

type UpdateFilesRequest struct {
	Files         map[string]string `json:"files"`
	TarobaseToken string            `json:"tarobaseToken"`
}

func (c *Client) GetFiles(ctx context.Context, projectID string) (*FilesResponse, error) {
	path := fmt.Sprintf("/api/project/%s/files", projectID)
	body, err := c.Do(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}

	var raw filesRawResponse
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	return &FilesResponse{Files: raw.FilesWithContent}, nil
}

func (c *Client) UpdateFiles(ctx context.Context, projectID string, files map[string]string) error {
	path := fmt.Sprintf("/api/project/%s/files/update", projectID)

	_, err := c.doWithTokenBody(ctx, "POST", path, func() (interface{}, error) {
		token, err := c.AuthManager.GetToken()
		if err != nil {
			return nil, err
		}
		return UpdateFilesRequest{
			Files:         files,
			TarobaseToken: token,
		}, nil
	})
	return err
}

type UploadImageRequest struct {
	ImageBase64   string `json:"imageBase64"`
	ContentType   string `json:"contentType"`
	FileName      string `json:"fileName,omitempty"`
	TarobaseToken string `json:"tarobaseToken"`
}

type uploadImageEnvelope struct {
	Success bool `json:"success"`
	Data    struct {
		URL     string `json:"url"`
		FileKey string `json:"fileKey"`
	} `json:"data"`
	Error string `json:"error,omitempty"`
}

type UploadImageResponse struct {
	URL     string `json:"url"`
	FileKey string `json:"fileKey"`
}

func (r *UploadImageResponse) QuietString() string { return r.URL }

func (c *Client) UploadImage(ctx context.Context, projectID, imageBase64, contentType, fileName string) (*UploadImageResponse, error) {
	path := fmt.Sprintf("/api/project/%s/files/upload-image", projectID)

	body, err := c.doWithTokenBody(ctx, "POST", path, func() (interface{}, error) {
		token, err := c.AuthManager.GetToken()
		if err != nil {
			return nil, err
		}
		return UploadImageRequest{
			ImageBase64:   imageBase64,
			ContentType:   contentType,
			FileName:      fileName,
			TarobaseToken: token,
		}, nil
	})
	if err != nil {
		return nil, err
	}

	var envelope uploadImageEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	if !envelope.Success {
		return nil, fmt.Errorf("upload failed: %s", envelope.Error)
	}
	return &UploadImageResponse{
		URL:     envelope.Data.URL,
		FileKey: envelope.Data.FileKey,
	}, nil
}

// UploadImageGlobal uploads an image to the global Tarobase app (no project
// required). Used by `poof build --file` so the image URL can be included in
// the initial firstMessage before any project exists.
func (c *Client) UploadImageGlobal(ctx context.Context, imageBase64, contentType, fileName string) (*UploadImageResponse, error) {
	body, err := c.doWithTokenBody(ctx, "POST", "/api/upload-image", func() (interface{}, error) {
		token, err := c.AuthManager.GetToken()
		if err != nil {
			return nil, err
		}
		return UploadImageRequest{
			ImageBase64:   imageBase64,
			ContentType:   contentType,
			FileName:      fileName,
			TarobaseToken: token,
		}, nil
	})
	if err != nil {
		return nil, err
	}

	var envelope uploadImageEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	if !envelope.Success {
		return nil, fmt.Errorf("upload failed: %s", envelope.Error)
	}
	return &UploadImageResponse{
		URL:     envelope.Data.URL,
		FileKey: envelope.Data.FileKey,
	}, nil
}
