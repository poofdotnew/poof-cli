package api

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/gagliardetto/solana-go"
	"github.com/mr-tron/base58"
)

// PublishEligibility matches the server's nested response shape.
type PublishEligibility struct {
	Status  string                 `json:"status"`
	Message string                 `json:"message"`
	Details map[string]interface{} `json:"details,omitempty"`
}

// Eligible returns true when the server reports status "approved".
func (e *PublishEligibility) Eligible() bool {
	return e.Status == "approved"
}

type publishEligibilityEnvelope struct {
	Data PublishEligibility `json:"data"`
}

// Publish request types per target.

type PublishRequest struct {
	AuthToken               string                 `json:"authToken"`
	SignedPermitTransaction string                 `json:"signedPermitTransaction,omitempty"`
	ProdConstantsOverrides  map[string]interface{} `json:"prodConstantsOverrides,omitempty"`
	ProdConfig              map[string]interface{} `json:"prodConfig,omitempty"`
}

type PreviewPublishRequest struct {
	AuthToken                        string                 `json:"authToken"`
	SignedPermitTransaction          string                 `json:"signedPermitTransaction"`
	AllowedAddresses                 []string               `json:"allowedAddresses,omitempty"`
	MainnetPreviewConstantsOverrides map[string]interface{} `json:"mainnetPreviewConstantsOverrides,omitempty"`
	MainnetPreviewConfig             map[string]interface{} `json:"mainnetPreviewConfig,omitempty"`
}

// PublishOptions holds optional parameters for preview/production deploys.
type PublishOptions struct {
	AllowedAddresses   []string
	ConstantsOverrides map[string]interface{}
	Config             map[string]interface{}
}

type MobilePublishRequest struct {
	Platform          string `json:"platform"`
	AppName           string `json:"appName"`
	AppIconUrl        string `json:"appIconUrl"`
	AppDescription    string `json:"appDescription,omitempty"`
	ThemeColor        string `json:"themeColor,omitempty"`
	IsDraft           bool   `json:"isDraft,omitempty"`
	TargetEnvironment string `json:"targetEnvironment,omitempty"`
}

// Download response types — server wraps in { data: {...} }.

type downloadDataEnvelope struct {
	Data struct {
		DownloadTaskID string `json:"downloadTaskId"`
		ProjectID      string `json:"projectId"`
		Status         string `json:"status"`
	} `json:"data"`
}

type DownloadResponse struct {
	TaskID    string `json:"taskId"`
	ProjectID string `json:"projectId"`
	Status    string `json:"status"`
}

func (r *DownloadResponse) QuietString() string { return r.TaskID }

type downloadURLDataEnvelope struct {
	Data struct {
		DownloadURL string `json:"downloadUrl"`
		ExpiresAt   string `json:"expiresAt"`
		FileName    string `json:"fileName"`
	} `json:"data"`
}

type DownloadURLRequest struct {
	TaskID string `json:"taskId"`
}

type DownloadURLResponse struct {
	URL       string `json:"url"`
	ExpiresAt string `json:"expiresAt"`
	FileName  string `json:"fileName"`
}

func (r *DownloadURLResponse) QuietString() string { return r.URL }

// StaticDeployResponse is the response from deploy-static.
type StaticDeployResponse struct {
	ProjectID string `json:"projectId"`
	TaskID    string `json:"taskId"`
	BundleURL string `json:"bundleUrl"`
	Slug      string `json:"slug"`
}

func (r *StaticDeployResponse) QuietString() string { return r.BundleURL }

type staticDeployEnvelope struct {
	Success bool                 `json:"success"`
	Data    StaticDeployResponse `json:"data"`
	Error   string               `json:"error,omitempty"`
}

type uploadURLEnvelope struct {
	Success bool `json:"success"`
	Data    struct {
		UploadURL string `json:"uploadUrl"`
		TaskID    string `json:"taskId"`
		MaxSize   int    `json:"maxSize"`
		ExpiresIn int    `json:"expiresIn"`
	} `json:"data"`
	Error string `json:"error,omitempty"`
}

// DeployStatic uploads a pre-built static frontend (tar.gz) to a project.
// Uses a 3-step presigned URL flow:
//  1. Get a presigned S3 upload URL from the API
//  2. Upload the archive directly to S3
//  3. Trigger the deploy pipeline
func (c *Client) DeployStatic(ctx context.Context, projectID string, archive []byte, title, description string) (*StaticDeployResponse, error) {
	// Step 1: Get presigned upload URL
	uploadURLPath := fmt.Sprintf("/api/project/%s/deploy-static/upload-url", projectID)
	uploadReqBody := map[string]string{}
	if title != "" {
		uploadReqBody["title"] = title
	}
	if description != "" {
		uploadReqBody["description"] = description
	}

	respBody, err := c.Do(ctx, "POST", uploadURLPath, uploadReqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to get upload URL: %w", err)
	}

	var uploadURLResp uploadURLEnvelope
	if err := json.Unmarshal(respBody, &uploadURLResp); err != nil {
		return nil, fmt.Errorf("failed to parse upload URL response: %w", err)
	}
	if !uploadURLResp.Success || uploadURLResp.Data.UploadURL == "" {
		if uploadURLResp.Error != "" {
			return nil, fmt.Errorf("failed to get upload URL: %s", uploadURLResp.Error)
		}
		return nil, fmt.Errorf("server returned empty upload URL")
	}

	// Validate archive size against server-provided max
	if uploadURLResp.Data.MaxSize > 0 && len(archive) > uploadURLResp.Data.MaxSize {
		return nil, fmt.Errorf("archive size (%d bytes) exceeds maximum (%d bytes)", len(archive), uploadURLResp.Data.MaxSize)
	}

	// Step 2: Upload directly to S3 via presigned URL
	s3Req, err := http.NewRequestWithContext(ctx, "PUT", uploadURLResp.Data.UploadURL, bytes.NewReader(archive))
	if err != nil {
		return nil, fmt.Errorf("failed to create S3 upload request: %w", err)
	}
	s3Req.Header.Set("Content-Type", "application/gzip")
	s3Req.ContentLength = int64(len(archive))

	s3Resp, err := c.HTTPClient.Do(s3Req)
	if err != nil {
		return nil, fmt.Errorf("S3 upload failed: %w", err)
	}
	defer s3Resp.Body.Close()

	if s3Resp.StatusCode >= 400 {
		s3ErrBody, _ := io.ReadAll(io.LimitReader(s3Resp.Body, 1024))
		return nil, fmt.Errorf("S3 upload failed (HTTP %d): %s", s3Resp.StatusCode, string(s3ErrBody))
	}

	// Step 3: Trigger the deploy
	triggerPath := fmt.Sprintf("/api/project/%s/deploy-static/trigger", projectID)
	triggerBody := map[string]string{"taskId": uploadURLResp.Data.TaskID}

	triggerRespBody, err := c.Do(ctx, "POST", triggerPath, triggerBody)
	if err != nil {
		return nil, fmt.Errorf("deploy trigger failed: %w", err)
	}

	var envelope staticDeployEnvelope
	if err := json.Unmarshal(triggerRespBody, &envelope); err != nil {
		return nil, fmt.Errorf("failed to parse deploy response: %w", err)
	}
	return &envelope.Data, nil
}

func (c *Client) CheckPublishEligibility(ctx context.Context, projectID string) (*PublishEligibility, error) {
	path := fmt.Sprintf("/api/project/%s/check-publish-eligibility", projectID)
	body, err := c.Do(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}

	var envelope publishEligibilityEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	return &envelope.Data, nil
}

func (c *Client) PublishProject(ctx context.Context, projectID, target string, opts ...interface{}) error {
	var path string
	switch target {
	case "preview":
		path = fmt.Sprintf("/api/project/%s/deploy-mainnet-preview", projectID)
	case "production":
		path = fmt.Sprintf("/api/project/%s/deploy-prod", projectID)
	case "mobile":
		path = fmt.Sprintf("/api/project/%s/mobile/publish", projectID)
	default:
		return fmt.Errorf("invalid target %q (valid: preview, production, mobile)", target)
	}

	// Mobile uses a different payload shape (no auth token in body).
	if target == "mobile" {
		if len(opts) == 0 {
			return fmt.Errorf("mobile publish requires a MobilePublishRequest")
		}
		mobileReq, ok := opts[0].(*MobilePublishRequest)
		if !ok {
			return fmt.Errorf("mobile publish requires a *MobilePublishRequest")
		}
		_, err := c.Do(ctx, "POST", path, mobileReq)
		return err
	}

	var publishOpts PublishOptions
	if len(opts) > 0 {
		if v, ok := opts[0].(*PublishOptions); ok && v != nil {
			publishOpts = *v
		}
	}

	// Auto-generate signed permit for preview/production deploys.
	// Get the appropriate Tarobase app ID from project status.
	signedPermit, err := c.getSignedPermitForDeploy(ctx, projectID, target)
	if err != nil {
		return fmt.Errorf("failed to generate deploy permit: %w", err)
	}

	_, err = c.doWithTokenBody(ctx, "POST", path, func() (interface{}, error) {
		token, err := c.AuthManager.GetToken()
		if err != nil {
			return nil, err
		}
		if target == "preview" {
			return PreviewPublishRequest{
				AuthToken:                        token,
				SignedPermitTransaction:          signedPermit,
				AllowedAddresses:                 publishOpts.AllowedAddresses,
				MainnetPreviewConstantsOverrides: publishOpts.ConstantsOverrides,
				MainnetPreviewConfig:             publishOpts.Config,
			}, nil
		}
		// production
		return PublishRequest{
			AuthToken:               token,
			SignedPermitTransaction: signedPermit,
			ProdConstantsOverrides:  publishOpts.ConstantsOverrides,
			ProdConfig:              publishOpts.Config,
		}, nil
	})
	return err
}

func (c *Client) DownloadCode(ctx context.Context, projectID string) (*DownloadResponse, error) {
	path := fmt.Sprintf("/api/project/%s/download", projectID)
	body, err := c.Do(ctx, "POST", path, nil)
	if err != nil {
		return nil, err
	}

	var envelope downloadDataEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	return &DownloadResponse{
		TaskID:    envelope.Data.DownloadTaskID,
		ProjectID: envelope.Data.ProjectID,
		Status:    envelope.Data.Status,
	}, nil
}

func (c *Client) GetDownloadURL(ctx context.Context, projectID, taskID string) (*DownloadURLResponse, error) {
	path := fmt.Sprintf("/api/project/%s/download/get-signed-url", projectID)
	body, err := c.Do(ctx, "POST", path, DownloadURLRequest{TaskID: taskID})
	if err != nil {
		return nil, err
	}

	var envelope downloadURLDataEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	return &DownloadURLResponse{
		URL:       envelope.Data.DownloadURL,
		ExpiresAt: envelope.Data.ExpiresAt,
		FileName:  envelope.Data.FileName,
	}, nil
}

// getSignedPermitForDeploy fetches an unsigned permit transaction from the
// Tarobase developer API and signs it with the wallet's private key.
func (c *Client) getSignedPermitForDeploy(ctx context.Context, projectID, target string) (string, error) {
	// Get the Tarobase app ID for this target
	status, err := c.GetProjectStatus(ctx, projectID)
	if err != nil {
		return "", fmt.Errorf("failed to get project status: %w", err)
	}

	var appID string
	if status.ConnectionInfo != nil {
		switch target {
		case "preview":
			if status.ConnectionInfo.Preview != nil {
				appID = status.ConnectionInfo.Preview.TarobaseAppId
			}
		case "production":
			if status.ConnectionInfo.Production != nil {
				appID = status.ConnectionInfo.Production.TarobaseAppId
			}
		}
	}
	if appID == "" {
		// No app ID means first deploy — permit not needed
		return "", nil
	}

	// Get auth token for Tarobase API call
	token, err := c.AuthManager.GetToken()
	if err != nil {
		return "", fmt.Errorf("failed to get auth token: %w", err)
	}

	// Call Tarobase developer API to get unsigned permit
	unsignedPermit, err := c.fetchUnsignedPermit(ctx, appID, token)
	if err != nil {
		return "", fmt.Errorf("failed to fetch permit: %w", err)
	}

	// Sign the permit transaction with the wallet's private key
	signedPermit, err := c.signPermitTransaction(unsignedPermit)
	if err != nil {
		return "", fmt.Errorf("failed to sign permit: %w", err)
	}

	return signedPermit, nil
}

// fetchUnsignedPermit calls the Tarobase developer API to get an unsigned
// update authority permit transaction for the given app.
func (c *Client) fetchUnsignedPermit(ctx context.Context, appID, authToken string) (string, error) {
	if c.DevAPIURL == "" {
		return "", fmt.Errorf("Tarobase developer API URL not configured")
	}

	reqBody, err := json.Marshal(map[string]string{
		"action": "createUpdateAppAuthorityPermit",
		"appId":  appID,
	})
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.DevAPIURL, bytes.NewReader(reqBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+authToken)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("Tarobase API error (%d): %s", resp.StatusCode, string(body))
	}

	// Response is a JSON string (base64-encoded transaction)
	var unsignedTx string
	if err := json.Unmarshal(body, &unsignedTx); err != nil {
		return "", fmt.Errorf("failed to parse permit response: %w", err)
	}

	return unsignedTx, nil
}

// signPermitTransaction deserializes a base64-encoded Solana transaction,
// signs it with the wallet's private key, and returns the signed transaction
// as a base64-encoded string.
func (c *Client) signPermitTransaction(unsignedBase64 string) (string, error) {
	if c.PrivateKey == "" {
		return "", fmt.Errorf("wallet private key not configured")
	}

	// Decode the unsigned transaction
	txBytes, err := base64.StdEncoding.DecodeString(unsignedBase64)
	if err != nil {
		return "", fmt.Errorf("invalid base64 transaction: %w", err)
	}

	tx, err := solana.TransactionFromBytes(txBytes)
	if err != nil {
		return "", fmt.Errorf("invalid Solana transaction: %w", err)
	}

	// Load wallet keypair
	secretBytes, err := base58.Decode(c.PrivateKey)
	if err != nil {
		return "", fmt.Errorf("invalid private key: %w", err)
	}
	if len(secretBytes) != 64 {
		return "", fmt.Errorf("invalid private key length: %d", len(secretBytes))
	}
	privKey := ed25519.NewKeyFromSeed(secretBytes[:32])
	pubKey := privKey.Public().(ed25519.PublicKey)
	walletPubkey := solana.PublicKeyFromBytes(pubKey)

	// Ensure signature slots exist
	numSigners := int(tx.Message.Header.NumRequiredSignatures)
	if len(tx.Signatures) < numSigners {
		sigs := make([]solana.Signature, numSigners)
		copy(sigs, tx.Signatures)
		tx.Signatures = sigs
	}

	// Find our wallet in the signers and sign
	walletIdx := -1
	for i, key := range tx.Message.AccountKeys {
		if key.Equals(walletPubkey) {
			walletIdx = i
			break
		}
	}
	if walletIdx < 0 {
		return "", fmt.Errorf("wallet %s not found in transaction signers", walletPubkey)
	}

	// Sign the transaction message
	messageBytes, err := tx.Message.MarshalBinary()
	if err != nil {
		return "", fmt.Errorf("failed to marshal transaction message: %w", err)
	}
	sig := ed25519.Sign(privKey, messageBytes)
	var solSig solana.Signature
	copy(solSig[:], sig)
	tx.Signatures[walletIdx] = solSig

	// Serialize with requireAllSignatures=false (facilitator signs later)
	signedBytes, err := tx.MarshalBinary()
	if err != nil {
		return "", fmt.Errorf("failed to serialize signed transaction: %w", err)
	}

	return base64.StdEncoding.EncodeToString(signedBytes), nil
}
