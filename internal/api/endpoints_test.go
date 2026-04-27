package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Chat
// ---------------------------------------------------------------------------

func TestCheckAIActive_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/ai/active") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(AIActiveResponse{Active: true, State: "queued", Status: "ok"})
	}))
	defer srv.Close()

	client := newTestClient(srv.URL, &mockAuthProvider{token: "tok", walletAddress: "w"})
	resp, err := client.CheckAIActive(context.Background(), "proj-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Active {
		t.Error("expected Active=true")
	}
	if resp.State != "queued" {
		t.Errorf("expected State=queued, got %q", resp.State)
	}
	if resp.Status != "ok" {
		t.Errorf("expected Status=ok, got %q", resp.Status)
	}
}

func TestCheckAIActive_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error":"project not found"}`))
	}))
	defer srv.Close()

	client := newTestClient(srv.URL, &mockAuthProvider{token: "tok", walletAddress: "w"})
	_, err := client.CheckAIActive(context.Background(), "bad-proj")
	if err == nil {
		t.Fatal("expected error")
	}
	apiErr, ok := IsAPIError(err)
	if !ok {
		t.Fatalf("expected APIError, got %T", err)
	}
	if apiErr.StatusCode != 404 {
		t.Errorf("expected 404, got %d", apiErr.StatusCode)
	}
}

func TestCancelAI_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	client := newTestClient(srv.URL, &mockAuthProvider{token: "tok", walletAddress: "w"})
	err := client.CancelAI(context.Background(), "proj-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestClearAISession_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/project/proj-1/session/clear" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(ClearSessionResponse{
			Success: true,
			Message: "Session cleared successfully",
		})
	}))
	defer srv.Close()

	client := newTestClient(srv.URL, &mockAuthProvider{token: "tok", walletAddress: "w"})
	resp, err := client.ClearAISession(context.Background(), "proj-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Success {
		t.Error("expected Success=true")
	}
	if resp.Message != "Session cleared successfully" {
		t.Errorf("expected clear message, got %q", resp.Message)
	}
}

func TestSteerAI_Success(t *testing.T) {
	var receivedBody map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &receivedBody)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	client := newTestClient(srv.URL, &mockAuthProvider{token: "tok", walletAddress: "w"})
	err := client.SteerAI(context.Background(), "proj-1", "go faster", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if receivedBody["message"] != "go faster" {
		t.Errorf("expected message='go faster', got %q", receivedBody["message"])
	}
}

// ---------------------------------------------------------------------------
// Credits
// ---------------------------------------------------------------------------

func TestGetCredits_Success(t *testing.T) {
	expected := CreditsResponse{
		Credits: CreditDetails{
			Daily: DailyCredits{Remaining: 10, Allotted: 50, ResetsAt: "2025-01-01T00:00:00Z"},
			AddOn: AddOnCredits{Remaining: 100, Purchased: 200},
			Total: 110,
		},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/user/credits" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(expected)
	}))
	defer srv.Close()

	client := newTestClient(srv.URL, &mockAuthProvider{token: "tok", walletAddress: "w"})
	resp, err := client.GetCredits(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Credits.Total != 110 {
		t.Errorf("expected Total=110, got %v", resp.Credits.Total)
	}
	if resp.Credits.Daily.Remaining != 10 {
		t.Errorf("expected Daily.Remaining=10, got %v", resp.Credits.Daily.Remaining)
	}
}

func TestTopupPhase1_Returns402(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusPaymentRequired)
		json.NewEncoder(w).Encode(PaymentRequirements{
			X402Version: 2,
			Accepts: []PaymentAccept{{
				Scheme:  "exact",
				Network: "solana:mainnet",
				Amount:  "15000000",
				PayTo:   "treasury",
				Extra:   PaymentExtra{FeePayer: "facilitator"},
			}},
			Credits: 50,
		})
	}))
	defer srv.Close()

	client := newTestClient(srv.URL, &mockAuthProvider{token: "tok", walletAddress: "w"})
	reqs, err := client.TopupPhase1(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reqs.Credits != 50 {
		t.Errorf("expected Credits=50, got %d", reqs.Credits)
	}
	if len(reqs.Accepts) != 1 {
		t.Fatalf("expected 1 Accept, got %d", len(reqs.Accepts))
	}
}

func TestTopupPhase1_EmptyAccepts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusPaymentRequired)
		json.NewEncoder(w).Encode(PaymentRequirements{Accepts: []PaymentAccept{}})
	}))
	defer srv.Close()

	client := newTestClient(srv.URL, &mockAuthProvider{token: "tok", walletAddress: "w"})
	_, err := client.TopupPhase1(context.Background(), 1)
	if err == nil {
		t.Fatal("expected error for empty Accepts")
	}
	if !strings.Contains(err.Error(), "no payment methods") {
		t.Errorf("expected 'no payment methods' in error, got: %v", err)
	}
}

func TestTopupPhase1_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"server down"}`))
	}))
	defer srv.Close()

	client := newTestClient(srv.URL, &mockAuthProvider{token: "tok", walletAddress: "w"})
	_, err := client.TopupPhase1(context.Background(), 1)
	if err == nil {
		t.Fatal("expected error for 500")
	}
}

func TestTopupPhase2_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-PAYMENT") == "" {
			t.Error("expected X-PAYMENT header")
		}
		json.NewEncoder(w).Encode(TopupResult{Credits: 50, PriceUsd: 15.0, TxID: "tx123"})
	}))
	defer srv.Close()

	client := newTestClient(srv.URL, &mockAuthProvider{token: "tok", walletAddress: "w"})
	result, err := client.TopupPhase2(context.Background(), 1, "payment-header")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Credits != 50 {
		t.Errorf("expected Credits=50, got %d", result.Credits)
	}
	if result.TxID != "tx123" {
		t.Errorf("expected TxID=tx123, got %q", result.TxID)
	}
}

func TestTopupPhase2_PaymentFailed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`insufficient funds`))
	}))
	defer srv.Close()

	client := newTestClient(srv.URL, &mockAuthProvider{token: "tok", walletAddress: "w"})
	_, err := client.TopupPhase2(context.Background(), 1, "bad-payment")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "payment failed") {
		t.Errorf("expected 'payment failed' in error, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Deploy
// ---------------------------------------------------------------------------

func TestCheckPublishEligibility_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"data": map[string]interface{}{
				"status":  "approved",
				"message": "Ready to deploy",
			},
		})
	}))
	defer srv.Close()

	client := newTestClient(srv.URL, &mockAuthProvider{token: "tok", walletAddress: "w"})
	resp, err := client.CheckPublishEligibility(context.Background(), "proj-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Eligible() {
		t.Error("expected Eligible()=true")
	}
}

func TestPublishProject_Production(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// PublishProject now calls GetProjectStatus (GET) first to auto-sign permits,
		// then the actual deploy (POST). Handle both.
		if r.Method == http.MethodGet {
			// Return project status with no connection info (no permit needed)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"data": map[string]interface{}{
					"project": map[string]interface{}{"id": "proj-1"},
					"urls":    map[string]string{},
				},
			})
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"success":true,"data":{"deploymentTaskId":"task-1","projectId":"proj-1"}}`))
	}))
	defer srv.Close()

	client := newTestClient(srv.URL, &mockAuthProvider{token: "tok", walletAddress: "w"})
	_, err := client.PublishProject(context.Background(), "proj-1", "production")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPublishProject_Preview(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Handle GET for project status + POST for deploy
		if r.Method == http.MethodGet {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"data": map[string]interface{}{
					"project": map[string]interface{}{"id": "proj-1"},
					"urls":    map[string]string{},
				},
			})
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"success":true,"data":{"deploymentTaskId":"task-1","projectId":"proj-1"}}`))
	}))
	defer srv.Close()

	client := newTestClient(srv.URL, &mockAuthProvider{token: "tok", walletAddress: "w"})
	_, err := client.PublishProject(context.Background(), "proj-1", "preview")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPublishProject_Mobile(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"success":true,"data":{"deploymentTaskId":"task-1","projectId":"proj-1"}}`))
	}))
	defer srv.Close()

	client := newTestClient(srv.URL, &mockAuthProvider{token: "tok", walletAddress: "w"})
	mobileReq := &MobilePublishRequest{
		Platform:   "ios",
		AppName:    "Test App",
		AppIconUrl: "https://example.com/icon.png",
	}
	_, err := client.PublishProject(context.Background(), "proj-1", "mobile", mobileReq)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPublishProject_InvalidTarget(t *testing.T) {
	client := newTestClient("http://localhost:0", &mockAuthProvider{token: "tok", walletAddress: "w"})
	_, err := client.PublishProject(context.Background(), "proj-1", "invalid")
	if err == nil {
		t.Fatal("expected error for invalid target")
	}
	if !strings.Contains(err.Error(), "invalid target") {
		t.Errorf("expected 'invalid target' in error, got: %v", err)
	}
}

func TestDownloadCode_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"data": map[string]interface{}{
				"downloadTaskId": "task-abc",
				"projectId":      "proj-1",
				"status":         "in_progress",
			},
		})
	}))
	defer srv.Close()

	client := newTestClient(srv.URL, &mockAuthProvider{token: "tok", walletAddress: "w"})
	resp, err := client.DownloadCode(context.Background(), "proj-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.TaskID != "task-abc" {
		t.Errorf("expected TaskID=task-abc, got %q", resp.TaskID)
	}
}

func TestGetDownloadURL_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"data": map[string]interface{}{
				"downloadUrl": "https://example.com/download",
				"expiresAt":   "2026-01-01T00:00:00Z",
				"fileName":    "code.zip",
			},
		})
	}))
	defer srv.Close()

	client := newTestClient(srv.URL, &mockAuthProvider{token: "tok", walletAddress: "w"})
	resp, err := client.GetDownloadURL(context.Background(), "proj-1", "task-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.URL != "https://example.com/download" {
		t.Errorf("unexpected URL: %q", resp.URL)
	}
}

// ---------------------------------------------------------------------------
// Domains
// ---------------------------------------------------------------------------

func TestGetDomains_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(DomainsResponse{
			Domains: []Domain{{Domain: "example.com", IsDefault: true, Status: "active"}},
		})
	}))
	defer srv.Close()

	client := newTestClient(srv.URL, &mockAuthProvider{token: "tok", walletAddress: "w"})
	resp, err := client.GetDomains(context.Background(), "proj-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Domains) != 1 {
		t.Fatalf("expected 1 domain, got %d", len(resp.Domains))
	}
	if resp.Domains[0].Domain != "example.com" {
		t.Errorf("expected domain=example.com, got %q", resp.Domains[0].Domain)
	}
}

func TestAddDomain_Success(t *testing.T) {
	var receivedBody map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &receivedBody)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	client := newTestClient(srv.URL, &mockAuthProvider{token: "tok", walletAddress: "w"})
	err := client.AddDomain(context.Background(), "proj-1", "mysite.com", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if receivedBody["domain"] != "mysite.com" {
		t.Errorf("expected domain=mysite.com, got %v", receivedBody["domain"])
	}
	if receivedBody["isDefault"] != true {
		t.Errorf("expected isDefault=true, got %v", receivedBody["isDefault"])
	}
}

// ---------------------------------------------------------------------------
// Files
// ---------------------------------------------------------------------------

func TestGetFiles_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Server returns "filesWithContent" key; client normalizes to "files"
		json.NewEncoder(w).Encode(map[string]interface{}{
			"filesWithContent": map[string]string{"index.html": "<html></html>"},
		})
	}))
	defer srv.Close()

	client := newTestClient(srv.URL, &mockAuthProvider{token: "tok", walletAddress: "w"})
	resp, err := client.GetFiles(context.Background(), "proj-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Files["index.html"] != "<html></html>" {
		t.Errorf("unexpected file content: %q", resp.Files["index.html"])
	}
}

func TestUpdateFiles_Success(t *testing.T) {
	var receivedBody map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &receivedBody)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	client := newTestClient(srv.URL, &mockAuthProvider{token: "tok", walletAddress: "w"})
	files := map[string]string{"app.js": "console.log('hi')"}
	err := client.UpdateFiles(context.Background(), "proj-1", files)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	filesMap, ok := receivedBody["files"].(map[string]interface{})
	if !ok {
		t.Fatal("expected files in body")
	}
	if filesMap["app.js"] != "console.log('hi')" {
		t.Errorf("unexpected file content in request: %v", filesMap["app.js"])
	}
}

// ---------------------------------------------------------------------------
// Project
// ---------------------------------------------------------------------------

func TestListProjects_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.RawQuery, "limit=10") {
			t.Errorf("expected limit=10 in query, got %s", r.URL.RawQuery)
		}
		if !strings.Contains(r.URL.RawQuery, "offset=0") {
			t.Errorf("expected offset=0 in query, got %s", r.URL.RawQuery)
		}
		json.NewEncoder(w).Encode(ListProjectsResponse{
			Projects: []Project{
				{ID: "p1", Title: "Test Project", Slug: "test"},
			},
		})
	}))
	defer srv.Close()

	client := newTestClient(srv.URL, &mockAuthProvider{token: "tok", walletAddress: "w"})
	resp, err := client.ListProjects(context.Background(), 10, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Projects) != 1 {
		t.Fatalf("expected 1 project, got %d", len(resp.Projects))
	}
	if resp.Projects[0].Title != "Test Project" {
		t.Errorf("expected title='Test Project', got %q", resp.Projects[0].Title)
	}
}

func TestCreateProject_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		json.NewEncoder(w).Encode(CreateProjectResponse{
			Success:   true,
			ProjectID: "new-proj-id",
			Message:   "created",
		})
	}))
	defer srv.Close()

	client := newTestClient(srv.URL, &mockAuthProvider{token: "tok", walletAddress: "w"})
	resp, err := client.CreateProject(context.Background(), CreateProjectRequest{
		FirstMessage: "Build me an app",
		IsPublic:     true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Success {
		t.Error("expected Success=true")
	}
	if resp.ProjectID != "new-proj-id" {
		t.Errorf("expected ProjectID=new-proj-id, got %q", resp.ProjectID)
	}
}

func TestCreateProject_FallbackProjectID(t *testing.T) {
	// When server returns empty ProjectID, client uses the generated UUID.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(CreateProjectResponse{Success: true, ProjectID: ""})
	}))
	defer srv.Close()

	client := newTestClient(srv.URL, &mockAuthProvider{token: "tok", walletAddress: "w"})
	resp, err := client.CreateProject(context.Background(), CreateProjectRequest{FirstMessage: "test"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.ProjectID == "" {
		t.Error("expected non-empty ProjectID (generated UUID)")
	}
}

func TestUpdateProject_Success(t *testing.T) {
	var receivedMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedMethod = r.Method
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	client := newTestClient(srv.URL, &mockAuthProvider{token: "tok", walletAddress: "w"})
	err := client.UpdateProject(context.Background(), "proj-1", &UpdateProjectRequest{Title: "New Title"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if receivedMethod != http.MethodPut {
		t.Errorf("expected PUT, got %s", receivedMethod)
	}
}

func TestDeleteProject_Success(t *testing.T) {
	var receivedMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedMethod = r.Method
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	client := newTestClient(srv.URL, &mockAuthProvider{token: "tok", walletAddress: "w"})
	err := client.DeleteProject(context.Background(), "proj-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if receivedMethod != http.MethodDelete {
		t.Errorf("expected DELETE, got %s", receivedMethod)
	}
}

func TestGetProjectStatus_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(ProjectStatus{
			Project: Project{ID: "p1", Title: "My Project"},
			URLs:    map[string]string{"preview": "https://preview.example.com"},
		})
	}))
	defer srv.Close()

	client := newTestClient(srv.URL, &mockAuthProvider{token: "tok", walletAddress: "w"})
	resp, err := client.GetProjectStatus(context.Background(), "p1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Project.Title != "My Project" {
		t.Errorf("expected title='My Project', got %q", resp.Project.Title)
	}
	if resp.URLs["preview"] != "https://preview.example.com" {
		t.Errorf("unexpected preview URL: %q", resp.URLs["preview"])
	}
}

func TestGetMessages_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(MessagesResponse{
			Messages: []Message{
				{ID: "m1", Role: "user", Content: "hello"},
				{ID: "m2", Role: "assistant", Content: "hi there"},
			},
		})
	}))
	defer srv.Close()

	client := newTestClient(srv.URL, &mockAuthProvider{token: "tok", walletAddress: "w"})
	resp, err := client.GetMessages(context.Background(), "proj-1", 50, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(resp.Messages))
	}
	if resp.Messages[0].Content != "hello" {
		t.Errorf("expected first message content='hello', got %q", resp.Messages[0].Content)
	}
}

// ---------------------------------------------------------------------------
// Direct Project / Policy
// ---------------------------------------------------------------------------

func TestCreateCloudProject_Success(t *testing.T) {
	var receivedBody map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/cloud/project" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &receivedBody)
		json.NewEncoder(w).Encode(CloudProjectCreateResponse{
			Success:   true,
			ProjectID: "cloud-1",
			Message:   "created",
		})
	}))
	defer srv.Close()

	client := newTestClient(srv.URL, &mockAuthProvider{token: "tok", walletAddress: "w"})
	req := &CloudProjectCreateRequest{
		Title:          "Direct Backend",
		GenerationMode: "backend,policy",
		Policy:         `{"items/$id":{"rules":{"read":"true"}}}`,
	}
	resp, err := client.CreateCloudProject(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.ProjectID != "cloud-1" {
		t.Errorf("expected ProjectID=cloud-1, got %q", resp.ProjectID)
	}
	if receivedBody["tarobaseToken"] != "tok" {
		t.Errorf("expected tarobaseToken to be set, got %v", receivedBody["tarobaseToken"])
	}
	if receivedBody["generationMode"] != "backend,policy" {
		t.Errorf("expected generationMode to be sent, got %v", receivedBody["generationMode"])
	}
	if req.TarobaseToken != "" {
		t.Errorf("request should not be mutated, got TarobaseToken=%q", req.TarobaseToken)
	}
}

func TestGetPolicy_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/project/proj-1/policy" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(PolicyStateResponse{
			ProjectID:     "proj-1",
			LatestTaskID:  "task-1",
			Policy:        "{}",
			Constants:     "{}",
			PolicyHash:    "ph",
			ConstantsHash: "ch",
		})
	}))
	defer srv.Close()

	client := newTestClient(srv.URL, &mockAuthProvider{token: "tok", walletAddress: "w"})
	resp, err := client.GetPolicy(context.Background(), "proj-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.PolicyHash != "ph" || resp.ConstantsHash != "ch" {
		t.Fatalf("unexpected hashes: %#v", resp)
	}
}

func TestValidatePolicy_Success(t *testing.T) {
	var receivedBody map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/project/proj-1/policy/validate" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &receivedBody)
		json.NewEncoder(w).Encode(PolicyDeployResult{
			Success:     true,
			ProjectID:   "proj-1",
			Environment: "draft",
			AppID:       "app-1",
			PolicyHash:  "ph",
			Validation:  PolicyValidation{Valid: true},
		})
	}))
	defer srv.Close()

	client := newTestClient(srv.URL, &mockAuthProvider{token: "tok", walletAddress: "w"})
	resp, err := client.ValidatePolicy(context.Background(), "proj-1", &PolicyRequest{
		Environment: "draft",
		Policy:      "{}",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Validation.Valid {
		t.Fatal("expected validation to be valid")
	}
	if receivedBody["tarobaseToken"] != "tok" {
		t.Errorf("expected tarobaseToken to be set, got %v", receivedBody["tarobaseToken"])
	}
}

func TestDeployPolicy_DraftSuccess(t *testing.T) {
	var receivedBody map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/project/proj-1/policy/deploy" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &receivedBody)
		json.NewEncoder(w).Encode(PolicyDeployResult{
			Success:     true,
			ProjectID:   "proj-1",
			TaskID:      "task-2",
			Environment: "draft",
			AppID:       "app-1",
			PolicyHash:  "ph",
			Validation:  PolicyValidation{Valid: true},
			Deployed:    true,
		})
	}))
	defer srv.Close()

	client := newTestClient(srv.URL, &mockAuthProvider{token: "tok", walletAddress: "w"})
	resp, err := client.DeployPolicy(context.Background(), "proj-1", &PolicyRequest{
		Environment: "draft",
		Policy:      "{}",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.TaskID != "task-2" {
		t.Errorf("expected task-2, got %q", resp.TaskID)
	}
	if _, ok := receivedBody["signedPermitTransaction"]; ok {
		t.Error("draft deploy should not include signedPermitTransaction")
	}
}

func TestRollbackPolicy_DraftSuccess(t *testing.T) {
	var receivedBody map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/project/proj-1/policy/rollback" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &receivedBody)
		json.NewEncoder(w).Encode(PolicyDeployResult{
			Success:     true,
			ProjectID:   "proj-1",
			TaskID:      "task-rollback",
			Environment: "draft",
			AppID:       "app-1",
			PolicyHash:  "ph",
			Validation:  PolicyValidation{Valid: true},
			Deployed:    true,
		})
	}))
	defer srv.Close()

	client := newTestClient(srv.URL, &mockAuthProvider{token: "tok", walletAddress: "w"})
	req := &PolicyRequest{
		Environment: "draft",
		TaskID:      "task-1",
	}
	resp, err := client.RollbackPolicy(context.Background(), "proj-1", req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.TaskID != "task-rollback" {
		t.Errorf("expected task-rollback, got %q", resp.TaskID)
	}
	if receivedBody["taskId"] != "task-1" {
		t.Errorf("expected taskId=task-1, got %v", receivedBody["taskId"])
	}
	if receivedBody["tarobaseToken"] != "tok" {
		t.Errorf("expected tarobaseToken to be set, got %v", receivedBody["tarobaseToken"])
	}
	if req.TarobaseToken != "" {
		t.Errorf("request should not be mutated, got TarobaseToken=%q", req.TarobaseToken)
	}
}

func TestPolicyHistory_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/project/proj-1/policy/history" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("limit") != "5" {
			t.Errorf("expected limit=5, got %q", r.URL.Query().Get("limit"))
		}
		json.NewEncoder(w).Encode(PolicyHistoryResponse{
			ProjectID: "proj-1",
			History:   []PolicyHistoryEntry{{TaskID: "task-1", Title: "Bootstrap"}},
		})
	}))
	defer srv.Close()

	client := newTestClient(srv.URL, &mockAuthProvider{token: "tok", walletAddress: "w"})
	resp, err := client.PolicyHistory(context.Background(), "proj-1", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.History) != 1 || resp.History[0].TaskID != "task-1" {
		t.Fatalf("unexpected history: %#v", resp.History)
	}
}

// ---------------------------------------------------------------------------
// Security
// ---------------------------------------------------------------------------

func TestSecurityScan_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		json.NewEncoder(w).Encode(SecurityScanResponse{
			Success:   true,
			MessageID: "msg-1",
			Message:   "Security scan initiated successfully",
			TaskID:    "task-1",
			TaskTitle: "Security Audit",
		})
	}))
	defer srv.Close()

	client := newTestClient(srv.URL, &mockAuthProvider{token: "tok", walletAddress: "w"})
	resp, err := client.SecurityScan(context.Background(), "proj-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Success {
		t.Error("expected Success=true")
	}
	if resp.TaskID != "task-1" {
		t.Errorf("expected TaskID=task-1, got %q", resp.TaskID)
	}
	if resp.Message != "Security scan initiated successfully" {
		t.Errorf("unexpected message: %q", resp.Message)
	}
}

// ---------------------------------------------------------------------------
// Tasks
// ---------------------------------------------------------------------------

func TestListTasks_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("changeId"); got != "change-1" {
			t.Errorf("expected changeId=change-1, got %q", got)
		}
		if got := r.URL.Query().Get("limit"); got != "20" {
			t.Errorf("expected limit=20, got %q", got)
		}
		if got := r.URL.Query().Get("offset"); got != "5" {
			t.Errorf("expected offset=5, got %q", got)
		}
		json.NewEncoder(w).Encode(TasksResponse{
			Tasks: []map[string]interface{}{
				{"id": "t1", "status": "complete"},
			},
			HasMore: true,
		})
	}))
	defer srv.Close()

	client := newTestClient(srv.URL, &mockAuthProvider{token: "tok", walletAddress: "w"})
	resp, err := client.ListTasks(context.Background(), "proj-1", "change-1", 20, 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(resp.Tasks))
	}
	if !resp.HasMore {
		t.Fatalf("expected HasMore=true")
	}
}

func TestListTasks_DefaultProjectWideQuery(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("changeId"); got != "" {
			t.Errorf("expected no changeId, got %q", got)
		}
		if got := r.URL.Query().Get("limit"); got != "20" {
			t.Errorf("expected limit=20, got %q", got)
		}
		if got := r.URL.Query().Get("offset"); got != "" {
			t.Errorf("expected no offset, got %q", got)
		}
		json.NewEncoder(w).Encode(TasksResponse{})
	}))
	defer srv.Close()

	client := newTestClient(srv.URL, &mockAuthProvider{token: "tok", walletAddress: "w"})
	if _, err := client.ListTasks(context.Background(), "proj-1", "", 20, 0); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGetTask_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(TaskResponse{Task: TaskDetail{ID: "t1", Status: "running", Title: "Build"}})
	}))
	defer srv.Close()

	client := newTestClient(srv.URL, &mockAuthProvider{token: "tok", walletAddress: "w"})
	resp, err := client.GetTask(context.Background(), "proj-1", "t1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Task.ID != "t1" {
		t.Errorf("expected ID=t1, got %q", resp.Task.ID)
	}
	if resp.Task.Status != "running" {
		t.Errorf("expected Status=running, got %q", resp.Task.Status)
	}
}

func TestGetTestResults_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(TestResultsResponse{
			Summary: TestSummary{Total: 5, Passed: 4, Failed: 1},
			Results: []TestResult{
				{ID: "r1", Source: "ui", TestName: "test_login", Status: "passed"},
			},
		})
	}))
	defer srv.Close()

	client := newTestClient(srv.URL, &mockAuthProvider{token: "tok", walletAddress: "w"})
	resp, err := client.GetTestResults(context.Background(), "proj-1", 100, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Summary.Total != 5 {
		t.Errorf("expected Total=5, got %d", resp.Summary.Total)
	}
	if resp.Summary.Failed != 1 {
		t.Errorf("expected Failed=1, got %d", resp.Summary.Failed)
	}
	if got := resp.Results[0].Source; got != "ui" {
		t.Errorf("expected Source=ui, got %q", got)
	}
}

// ---------------------------------------------------------------------------
// Templates
// ---------------------------------------------------------------------------

func TestListTemplates_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("category") != "defi" {
			t.Errorf("expected category=defi, got %q", r.URL.Query().Get("category"))
		}
		json.NewEncoder(w).Encode(TemplatesResponse{
			Templates: []Template{
				{ID: "t1", Name: "DEX", Slug: "dex", Category: "defi"},
			},
		})
	}))
	defer srv.Close()

	client := newTestClient(srv.URL, &mockAuthProvider{token: "tok", walletAddress: "w"})
	resp, err := client.ListTemplates(context.Background(), "defi", "", "", 20, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Templates) != 1 {
		t.Fatalf("expected 1 template, got %d", len(resp.Templates))
	}
	if resp.Templates[0].Name != "DEX" {
		t.Errorf("expected name=DEX, got %q", resp.Templates[0].Name)
	}
}

func TestListTemplates_NoParams(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RawQuery != "" {
			t.Errorf("expected no query params, got %q", r.URL.RawQuery)
		}
		json.NewEncoder(w).Encode(TemplatesResponse{Templates: []Template{}})
	}))
	defer srv.Close()

	client := newTestClient(srv.URL, &mockAuthProvider{token: "tok", walletAddress: "w"})
	resp, err := client.ListTemplates(context.Background(), "", "", "", 0, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Templates) != 0 {
		t.Errorf("expected 0 templates, got %d", len(resp.Templates))
	}
}

// ---------------------------------------------------------------------------
// Logs
// ---------------------------------------------------------------------------

func TestGetLogs_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("environment") != "preview" {
			t.Errorf("expected environment=preview, got %q", r.URL.Query().Get("environment"))
		}
		if r.URL.Query().Get("limit") != "50" {
			t.Errorf("expected limit=50, got %q", r.URL.Query().Get("limit"))
		}
		json.NewEncoder(w).Encode(LogsResponse{
			Logs: []LogEntry{
				{Timestamp: "2025-01-01T00:00:00Z", Level: "info", Message: "started"},
			},
		})
	}))
	defer srv.Close()

	client := newTestClient(srv.URL, &mockAuthProvider{token: "tok", walletAddress: "w"})
	resp, err := client.GetLogs(context.Background(), "proj-1", "preview", 50, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Logs) != 1 {
		t.Fatalf("expected 1 log entry, got %d", len(resp.Logs))
	}
	if resp.Logs[0].Level != "info" {
		t.Errorf("expected level=info, got %q", resp.Logs[0].Level)
	}
}

func TestGetLogs_NoParams(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("environment") != "" {
			t.Errorf("expected no environment param")
		}
		if r.URL.Query().Get("limit") != "" {
			t.Errorf("expected no limit param")
		}
		json.NewEncoder(w).Encode(LogsResponse{Logs: []LogEntry{}})
	}))
	defer srv.Close()

	client := newTestClient(srv.URL, &mockAuthProvider{token: "tok", walletAddress: "w"})
	_, err := client.GetLogs(context.Background(), "proj-1", "", 0, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Preferences
// ---------------------------------------------------------------------------

func TestGetPreferences_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/user/ai-preferences" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(PreferencesResponse{
			Preferences: map[string]interface{}{"mainChat": "smart"},
		})
	}))
	defer srv.Close()

	client := newTestClient(srv.URL, &mockAuthProvider{token: "tok", walletAddress: "w"})
	resp, err := client.GetPreferences(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Preferences["mainChat"] != "smart" {
		t.Errorf("expected mainChat=smart, got %v", resp.Preferences["mainChat"])
	}
}

func TestSetPreferences_Success(t *testing.T) {
	var receivedMethod string
	var receivedBody map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedMethod = r.Method
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &receivedBody)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	client := newTestClient(srv.URL, &mockAuthProvider{token: "tok", walletAddress: "w"})
	err := client.SetPreferences(context.Background(), map[string]interface{}{"mainChat": "genius"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if receivedMethod != http.MethodPut {
		t.Errorf("expected PUT, got %s", receivedMethod)
	}
}

// ---------------------------------------------------------------------------
// Secrets
// ---------------------------------------------------------------------------

func TestGetSecrets_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(SecretsResponse{
			SecretRequirements: SecretRequirements{
				Required: []SecretEntry{{Key: "API_KEY", Label: "API Key", IsRequired: true}},
				Optional: []SecretEntry{{Key: "DEBUG", Label: "Debug Mode"}},
			},
			Summary: SecretsSummary{TotalRequired: 1, TotalOptional: 1},
		})
	}))
	defer srv.Close()

	client := newTestClient(srv.URL, &mockAuthProvider{token: "tok", walletAddress: "w"})
	resp, err := client.GetSecrets(context.Background(), "proj-1", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.SecretRequirements.Required) != 1 || resp.SecretRequirements.Required[0].Key != "API_KEY" {
		t.Errorf("unexpected required secrets: %v", resp.SecretRequirements.Required)
	}
}

func TestSetSecrets_Success(t *testing.T) {
	var receivedBody map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &receivedBody)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	client := newTestClient(srv.URL, &mockAuthProvider{token: "tok", walletAddress: "w"})
	err := client.SetSecrets(context.Background(), "proj-1", map[string]string{"API_KEY": "secret"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	secretsMap, ok := receivedBody["secrets"].(map[string]interface{})
	if !ok {
		t.Fatal("expected secrets in body")
	}
	if secretsMap["API_KEY"] != "secret" {
		t.Errorf("expected API_KEY=secret, got %v", secretsMap["API_KEY"])
	}
}

// ---------------------------------------------------------------------------
// doWithTokenBody 401 retry
// ---------------------------------------------------------------------------

func TestDoWithTokenBody_401Retry(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error":"expired"}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	tokenCall := 0
	auth := &mockAuthProvider{
		walletAddress: "w",
		tokenFunc: func() (string, error) {
			tokenCall++
			if tokenCall == 1 {
				return "old-tok", nil
			}
			return "new-tok", nil
		},
	}
	client := newTestClient(srv.URL, auth)

	// Use doWithTokenBody via a public method that uses it (e.g. AddDomain)
	err := client.AddDomain(context.Background(), "proj-1", "test.com", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if callCount != 2 {
		t.Errorf("expected 2 server calls (retry), got %d", callCount)
	}
}

// ---------------------------------------------------------------------------
// JSON parse errors
// ---------------------------------------------------------------------------

func TestGetCredits_BadJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`not json`))
	}))
	defer srv.Close()

	client := newTestClient(srv.URL, &mockAuthProvider{token: "tok", walletAddress: "w"})
	_, err := client.GetCredits(context.Background())
	if err == nil {
		t.Fatal("expected error for bad JSON")
	}
	if !strings.Contains(err.Error(), "failed to parse response") {
		t.Errorf("expected parse error, got: %v", err)
	}
}
