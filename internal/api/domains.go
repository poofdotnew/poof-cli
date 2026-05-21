package api

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

type DNSRecord struct {
	Type        string `json:"type"`
	Name        string `json:"name"`
	Value       string `json:"value"`
	Purpose     string `json:"purpose,omitempty"`
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required,omitempty"`
	Priority    int    `json:"priority,omitempty"`
}

type DomainStatusDetails struct {
	Status               string      `json:"status,omitempty"`
	Message              string      `json:"message,omitempty"`
	Domain               string      `json:"domain,omitempty"`
	Target               string      `json:"target,omitempty"`
	DNSRecords           []DNSRecord `json:"dns_records,omitempty"`
	LastValidated        string      `json:"last_validated,omitempty"`
	CloudflareHostnameID string      `json:"cloudflare_hostname_id,omitempty"`
}

type Domain struct {
	ID            string          `json:"id,omitempty"`
	Domain        string          `json:"domain"`
	IsDefault     bool            `json:"isDefault"`
	Status        string          `json:"status"`
	StatusDetails json.RawMessage `json:"statusDetails,omitempty"`
}

func (d *Domain) UnmarshalJSON(data []byte) error {
	type domainAlias Domain
	var aux struct {
		StatusDetails json.RawMessage `json:"statusDetails"`
		*domainAlias
	}
	aux.domainAlias = (*domainAlias)(d)

	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	statusDetails, err := normalizeStatusDetails(aux.StatusDetails)
	if err != nil {
		return err
	}
	d.StatusDetails = statusDetails
	return nil
}

func normalizeStatusDetails(raw json.RawMessage) (json.RawMessage, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}

	if raw[0] != '"' {
		return raw, nil
	}

	var encoded string
	if err := json.Unmarshal(raw, &encoded); err != nil {
		return nil, fmt.Errorf("failed to parse statusDetails string: %w", err)
	}
	if encoded == "" {
		return nil, nil
	}
	return json.RawMessage(encoded), nil
}

func (d *Domain) ParsedStatusDetails() (*DomainStatusDetails, error) {
	if len(d.StatusDetails) == 0 {
		return nil, nil
	}

	var details DomainStatusDetails
	if err := json.Unmarshal(d.StatusDetails, &details); err != nil {
		return nil, fmt.Errorf("failed to parse statusDetails for %s: %w", d.Domain, err)
	}
	return &details, nil
}

func (d *Domain) RequiredDNSRecords() []DNSRecord {
	details, err := d.ParsedStatusDetails()
	if err != nil || details == nil {
		return nil
	}
	status := strings.TrimSpace(strings.ToLower(d.Status))
	if details.Status != "" {
		status = strings.TrimSpace(strings.ToLower(details.Status))
	}
	if status == "active" {
		return nil
	}

	var records []DNSRecord
	for _, record := range details.DNSRecords {
		if record.Type == "" || record.Name == "" || record.Value == "" {
			continue
		}
		if record.Required || status != "active" {
			records = append(records, record)
		}
	}
	return records
}

type DomainsResponse struct {
	Domains []Domain `json:"domains"`
}

type AddDomainRequest struct {
	Domain        string `json:"domain"`
	IsDefault     bool   `json:"isDefault"`
	TarobaseToken string `json:"tarobaseToken"`
}

type DNSInstructions struct {
	Target                  string                 `json:"target,omitempty"`
	AutomaticallyConfigured bool                   `json:"automaticallyConfigured,omitempty"`
	SetupStatus             string                 `json:"setupStatus,omitempty"`
	Message                 string                 `json:"message,omitempty"`
	Note                    string                 `json:"note,omitempty"`
	Records                 []DNSRecord            `json:"records,omitempty"`
	Instructions            map[string]interface{} `json:"instructions,omitempty"`
	Troubleshooting         map[string]interface{} `json:"troubleshooting,omitempty"`
}

type AddDomainResponse struct {
	Domain          Domain           `json:"domain"`
	DNSInstructions *DNSInstructions `json:"dnsInstructions,omitempty"`
	SetupMethod     string           `json:"setupMethod,omitempty"`
}

type CloudflareDomainStatus struct {
	Status               string                 `json:"status,omitempty"`
	SSLStatus            string                 `json:"ssl_status,omitempty"`
	VerificationErrors   []string               `json:"verification_errors,omitempty"`
	SSLValidationRecords []SSLValidationRecord  `json:"ssl_validation_records,omitempty"`
	SSLValidationErrors  []string               `json:"ssl_validation_errors,omitempty"`
	Raw                  map[string]interface{} `json:"-"`
}

type SSLValidationRecord struct {
	Status   string `json:"status,omitempty"`
	TXTName  string `json:"txt_name,omitempty"`
	TXTValue string `json:"txt_value,omitempty"`
}

type CheckDomainStatusResponse struct {
	Domain           Domain                  `json:"domain"`
	Message          string                  `json:"message,omitempty"`
	CloudflareStatus *CloudflareDomainStatus `json:"cloudflare_status,omitempty"`
}

func (c *Client) GetDomains(ctx context.Context, projectID string) (*DomainsResponse, error) {
	path := fmt.Sprintf("/api/project/%s/domains", projectID)
	body, err := c.Do(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}

	var resp DomainsResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	return &resp, nil
}

func (c *Client) AddDomain(ctx context.Context, projectID, domain string, isDefault bool) (*AddDomainResponse, error) {
	path := fmt.Sprintf("/api/project/%s/domains", projectID)

	body, err := c.doWithTokenBody(ctx, "POST", path, func() (interface{}, error) {
		token, err := c.AuthManager.GetToken()
		if err != nil {
			return nil, err
		}
		return AddDomainRequest{
			Domain:        domain,
			IsDefault:     isDefault,
			TarobaseToken: token,
		}, nil
	})
	if err != nil {
		return nil, err
	}

	var resp AddDomainResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	return &resp, nil
}

func (c *Client) CheckDomainStatus(ctx context.Context, projectID, domainID string) (*CheckDomainStatusResponse, error) {
	path := fmt.Sprintf("/api/project/%s/domains/%s", projectID, domainID)
	body, err := c.Do(ctx, "PATCH", path, map[string]string{"action": "check_status"})
	if err != nil {
		return nil, err
	}

	var resp CheckDomainStatusResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	return &resp, nil
}
