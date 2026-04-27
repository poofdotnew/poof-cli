package cli

import (
	"strings"
	"testing"

	"github.com/poofdotnew/poof-cli/internal/api"
	"github.com/spf13/cobra"
)

func TestValidatePolicyDeployRequest_ProductionRequiresLatestStoredPolicy(t *testing.T) {
	req := api.PolicyRequest{
		Environment: "production",
		Policy:      "{}",
	}

	err := validatePolicyDeployRequest(&req, true)
	if err == nil || !strings.Contains(err.Error(), "latest stored policy") {
		t.Fatalf("expected latest-stored-policy error, got %v", err)
	}
}

func TestPolicyEnvironmentFromArgs_SeparatesGlobalAndPolicyEnv(t *testing.T) {
	args := []string{"poof", "--env", "local", "policy", "validate", "-p", "proj-1"}
	if got := policyEnvironmentFromArgs(args, "validate"); got != "draft" {
		t.Fatalf("expected default draft policy env, got %q", got)
	}

	args = []string{"poof", "--env", "local", "policy", "deploy", "-p", "proj-1", "--env", "preview"}
	if got := policyEnvironmentFromArgs(args, "deploy"); got != "preview" {
		t.Fatalf("expected preview policy env, got %q", got)
	}

	args = []string{"poof", "--env=local", "policy", "deploy", "-p", "proj-1", "--env=production"}
	if got := policyEnvironmentFromArgs(args, "deploy"); got != "production" {
		t.Fatalf("expected production policy env, got %q", got)
	}
}

func TestValidatePolicyDeployRequest_ProductionRequiresConfirmation(t *testing.T) {
	req := api.PolicyRequest{Environment: "production"}

	err := validatePolicyDeployRequest(&req, false)
	if err == nil || !strings.Contains(err.Error(), "requires --yes") {
		t.Fatalf("expected --yes error, got %v", err)
	}
}

func TestValidatePolicyDeployRequest_AllowsSafeProductionAndDryRun(t *testing.T) {
	if err := validatePolicyDeployRequest(&api.PolicyRequest{Environment: "production"}, true); err != nil {
		t.Fatalf("expected production deploy of latest stored policy to pass: %v", err)
	}

	err := validatePolicyDeployRequest(&api.PolicyRequest{
		Environment: "production",
		DryRun:      true,
		Policy:      "{}",
	}, false)
	if err != nil {
		t.Fatalf("expected production dry-run with policy to pass: %v", err)
	}
}

func TestValidatePolicyRollbackRequest_ProductionMustBeStaged(t *testing.T) {
	err := validatePolicyRollbackRequest("proj-1", "task-1", &api.PolicyRequest{Environment: "production"})
	if err == nil || !strings.Contains(err.Error(), "staged through draft first") {
		t.Fatalf("expected staged-through-draft error, got %v", err)
	}
}

func TestValidatePolicyRollbackRequest_AllowsDraftAndDryRun(t *testing.T) {
	if err := validatePolicyRollbackRequest("proj-1", "task-1", &api.PolicyRequest{Environment: "draft"}); err != nil {
		t.Fatalf("expected draft rollback to pass: %v", err)
	}
	if err := validatePolicyRollbackRequest("proj-1", "task-1", &api.PolicyRequest{Environment: "production", DryRun: true}); err != nil {
		t.Fatalf("expected production rollback dry-run to pass: %v", err)
	}
}

func TestResolveNoAIProjectMode(t *testing.T) {
	cmd := &cobra.Command{Use: "create"}
	cmd.Flags().String("mode", "full", "")
	got, err := resolveNoAIProjectMode(cmd)
	if err != nil {
		t.Fatalf("unexpected default error: %v", err)
	}
	if got != "backend,policy" {
		t.Fatalf("expected default backend,policy, got %q", got)
	}

	cmd = &cobra.Command{Use: "create"}
	cmd.Flags().String("mode", "full", "")
	if err := cmd.Flags().Set("mode", "backend"); err != nil {
		t.Fatalf("set mode: %v", err)
	}
	got, err = resolveNoAIProjectMode(cmd)
	if err != nil {
		t.Fatalf("unexpected backend error: %v", err)
	}
	if got != "backend,policy" {
		t.Fatalf("expected backend alias to normalize to backend,policy, got %q", got)
	}

	cmd = &cobra.Command{Use: "create"}
	cmd.Flags().String("mode", "full", "")
	if err := cmd.Flags().Set("mode", "ui,policy"); err != nil {
		t.Fatalf("set mode: %v", err)
	}
	if _, err := resolveNoAIProjectMode(cmd); err == nil || !strings.Contains(err.Error(), "--no-ai supports") {
		t.Fatalf("expected unsupported mode error, got %v", err)
	}
}
