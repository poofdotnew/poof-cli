package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDeployBackendFollowsUploadAndTriggerSequence(t *testing.T) {
	archive := []byte{0x1f, 0x8b, 0x08, 0x00, 'b', 'u', 'n', 'd', 'l', 'e'}
	var sawUploadURL bool
	var sawS3Put bool
	var sawTrigger bool

	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/project/proj123/deploy-backend/upload-url":
			if r.Method != http.MethodPost {
				t.Fatalf("upload-url method = %s, want POST", r.Method)
			}
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode upload-url body: %v", err)
			}
			if body["title"] != "backend v2" || body["description"] != "compiled bundle" {
				t.Fatalf("unexpected upload-url body: %#v", body)
			}
			sawUploadURL = true
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"success": true,
				"data": map[string]interface{}{
					"uploadUrl": srv.URL + "/s3-put",
					"taskId":    "task123",
					"maxSize":   1024,
					"expiresIn": 600,
				},
			})

		case "/s3-put":
			if r.Method != http.MethodPut {
				t.Fatalf("s3 method = %s, want PUT", r.Method)
			}
			if got := r.Header.Get("Content-Type"); got != "application/gzip" {
				t.Fatalf("Content-Type = %q, want application/gzip", got)
			}
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read S3 body: %v", err)
			}
			if !bytes.Equal(body, archive) {
				t.Fatalf("uploaded archive = %v, want %v", body, archive)
			}
			sawS3Put = true
			w.WriteHeader(http.StatusOK)

		case "/api/project/proj123/deploy-backend/trigger":
			if r.Method != http.MethodPost {
				t.Fatalf("trigger method = %s, want POST", r.Method)
			}
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode trigger body: %v", err)
			}
			if body["taskId"] != "task123" {
				t.Fatalf("trigger taskId = %q, want task123", body["taskId"])
			}
			sawTrigger = true
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"success": true,
				"data": map[string]interface{}{
					"projectId":  "proj123",
					"taskId":     "task123",
					"backendUrl": "https://draft-api.poof.new",
					"slug":       "task123",
				},
			})

		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	client := newTestClient(srv.URL, &mockAuthProvider{token: "test-token", walletAddress: "wallet123"})
	resp, err := client.DeployBackend(context.Background(), "proj123", archive, "backend v2", "compiled bundle")
	if err != nil {
		t.Fatalf("DeployBackend returned error: %v", err)
	}

	if !sawUploadURL || !sawS3Put || !sawTrigger {
		t.Fatalf("sequence incomplete: uploadURL=%v s3Put=%v trigger=%v", sawUploadURL, sawS3Put, sawTrigger)
	}
	if resp.ProjectID != "proj123" || resp.TaskID != "task123" || resp.BackendURL != "https://draft-api.poof.new" {
		t.Fatalf("unexpected response: %#v", resp)
	}
}
