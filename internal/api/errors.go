package api

import (
	"errors"
	"fmt"
)

// APIError represents an error from the Poof API.
type APIError struct {
	StatusCode         int    `json:"-"`
	Message            string `json:"error"`
	Code               string `json:"code,omitempty"`
	MembershipRequired bool   `json:"membershipRequired,omitempty"`
}

func (e *APIError) Error() string {
	return fmt.Sprintf("poof API error (%d): %s", e.StatusCode, e.Message)
}

func (e *APIError) IsAuthError() bool        { return e.StatusCode == 401 }
func (e *APIError) IsNotFound() bool         { return e.StatusCode == 404 }
func (e *APIError) IsPaymentRequired() bool  { return e.StatusCode == 402 || e.MembershipRequired }
func (e *APIError) IsCreditsExhausted() bool { return e.Message == "You have run out of credits" }

// IsAPIError checks if the error is an APIError and returns it.
func IsAPIError(err error) (*APIError, bool) {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr, true
	}
	return nil, false
}
