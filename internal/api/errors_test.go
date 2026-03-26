package api

import (
	"errors"
	"testing"
)

func TestAPIError_Error(t *testing.T) {
	err := &APIError{StatusCode: 401, Message: "Not authenticated"}
	want := "poof API error (401): Not authenticated"
	if err.Error() != want {
		t.Errorf("got %q, want %q", err.Error(), want)
	}
}

func TestAPIError_IsAuthError(t *testing.T) {
	err := &APIError{StatusCode: 401}
	if !err.IsAuthError() {
		t.Error("expected IsAuthError() to be true")
	}
	err2 := &APIError{StatusCode: 200}
	if err2.IsAuthError() {
		t.Error("expected IsAuthError() to be false for 200")
	}
}

func TestAPIError_IsNotFound(t *testing.T) {
	err := &APIError{StatusCode: 404}
	if !err.IsNotFound() {
		t.Error("expected IsNotFound() to be true")
	}
}

func TestAPIError_IsPaymentRequired(t *testing.T) {
	err402 := &APIError{StatusCode: 402}
	if !err402.IsPaymentRequired() {
		t.Error("expected IsPaymentRequired() for 402")
	}

	errMember := &APIError{StatusCode: 200, MembershipRequired: true}
	if !errMember.IsPaymentRequired() {
		t.Error("expected IsPaymentRequired() for membershipRequired")
	}

	errOK := &APIError{StatusCode: 200}
	if errOK.IsPaymentRequired() {
		t.Error("expected IsPaymentRequired() to be false")
	}
}

func TestAPIError_IsCreditsExhausted(t *testing.T) {
	err := &APIError{Message: "You have run out of credits"}
	if !err.IsCreditsExhausted() {
		t.Error("expected IsCreditsExhausted() to be true")
	}

	err2 := &APIError{Message: "other error"}
	if err2.IsCreditsExhausted() {
		t.Error("expected IsCreditsExhausted() to be false")
	}
}

func TestIsAPIError(t *testing.T) {
	apiErr := &APIError{StatusCode: 500, Message: "internal"}
	wrappedErr := errors.New("wrapper")

	got, ok := IsAPIError(apiErr)
	if !ok || got != apiErr {
		t.Error("expected IsAPIError to unwrap APIError")
	}

	_, ok = IsAPIError(wrappedErr)
	if ok {
		t.Error("expected IsAPIError to return false for non-APIError")
	}
}
