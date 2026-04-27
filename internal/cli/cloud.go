package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/poofdotnew/poof-cli/internal/api"
	"github.com/poofdotnew/poof-cli/internal/output"
	"github.com/spf13/cobra"
)

var policyCmd = &cobra.Command{
	Use:   "policy",
	Short: "Validate, deploy, inspect, and roll back Poof policy/constants",
}

var policyGetCmd = &cobra.Command{
	Use:   "get",
	Short: "Fetch the latest policy and constants for a project",
	Example: `  poof policy get -p <id> --json
  poof policy get -p <id> --out-dir policy`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireAuth(); err != nil {
			return err
		}
		projectID, err := getProjectID()
		if err != nil {
			return err
		}

		resp, err := apiClient.GetPolicy(context.Background(), projectID)
		if err != nil {
			return handleError(err)
		}

		outDir, _ := cmd.Flags().GetString("out-dir")
		if outDir != "" {
			if err := writePolicyFiles(outDir, resp.Policy, resp.Constants); err != nil {
				return err
			}
		}

		output.Print(resp, func() {
			if outDir != "" {
				output.Success("Wrote %s and %s", filepath.Join(outDir, "poof.json"), filepath.Join(outDir, "constants.json"))
			}
			output.Info("Project:       %s", resp.ProjectID)
			output.Info("Latest task:   %s", resp.LatestTaskID)
			output.Info("Policy hash:   %s", resp.PolicyHash)
			output.Info("Constants hash: %s", resp.ConstantsHash)
			if resp.ConnectionInfo != nil {
				printConnectionInfo(resp.ConnectionInfo)
			}
		})
		return nil
	},
}

var policyValidateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate a policy/constants pair without deploying it",
	Example: `  poof policy validate -p <id> --policy policy/poof.json --constants policy/constants.json
  poof policy validate -p <id> --env preview`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireAuth(); err != nil {
			return err
		}
		projectID, err := getProjectID()
		if err != nil {
			return err
		}
		req, err := buildPolicyRequestFromFlags(cmd)
		if err != nil {
			return err
		}

		resp, err := apiClient.ValidatePolicy(context.Background(), projectID, &req)
		if err != nil {
			return handleError(err)
		}
		if !resp.Success {
			return fmt.Errorf("policy validation failed: %s", formatValidationErrors(resp.Validation))
		}

		output.Print(resp, func() {
			output.Success("Policy is valid for %s.", resp.Environment)
			output.Info("App ID:        %s", resp.AppID)
			output.Info("Policy hash:   %s", resp.PolicyHash)
			output.Info("Constants hash: %s", resp.ConstantsHash)
		})
		return nil
	},
}

var policyDeployCmd = &cobra.Command{
	Use:   "deploy",
	Short: "Validate, version, and deploy policy/constants to a Poof environment",
	Example: `  poof policy deploy -p <id> --policy policy/poof.json --constants policy/constants.json
  poof policy deploy -p <id> --env preview
  poof policy deploy -p <id> --env production --yes`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireAuth(); err != nil {
			return err
		}
		projectID, err := getProjectID()
		if err != nil {
			return err
		}
		req, err := buildPolicyRequestFromFlags(cmd)
		if err != nil {
			return err
		}
		yes, _ := cmd.Flags().GetBool("yes")
		if err := validatePolicyDeployRequest(&req, yes); err != nil {
			return err
		}

		resp, err := apiClient.DeployPolicy(context.Background(), projectID, &req)
		if err != nil {
			return handleError(err)
		}

		output.Print(resp, func() {
			if resp.DryRun {
				output.Success("Policy dry-run passed for %s.", resp.Environment)
			} else {
				output.Success("Policy deployed to %s.", resp.Environment)
			}
			output.Info("Task ID:       %s", resp.TaskID)
			output.Info("App ID:        %s", resp.AppID)
			output.Info("Policy hash:   %s", resp.PolicyHash)
			output.Info("Constants hash: %s", resp.ConstantsHash)
		})
		return nil
	},
}

var policyHistoryCmd = &cobra.Command{
	Use:   "history",
	Short: "List recent policy/constants versions",
	Example: `  poof policy history -p <id>
  poof policy history -p <id> --limit 50 --json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireAuth(); err != nil {
			return err
		}
		projectID, err := getProjectID()
		if err != nil {
			return err
		}
		limit, _ := cmd.Flags().GetInt("limit")
		resp, err := apiClient.PolicyHistory(context.Background(), projectID, limit)
		if err != nil {
			return handleError(err)
		}

		output.Print(resp, func() {
			if len(resp.History) == 0 {
				output.Info("No policy history found.")
				return
			}
			rows := make([][]string, 0, len(resp.History))
			for _, entry := range resp.History {
				rows = append(rows, []string{entry.TaskID, entry.CreatedAt, entry.Title, entry.PolicyHash, entry.ConstantsHash})
			}
			output.Table([]string{"Task", "Created", "Title", "Policy Hash", "Constants Hash"}, rows)
		})
		return nil
	},
}

var policyRollbackCmd = &cobra.Command{
	Use:   "rollback",
	Short: "Deploy policy/constants from a previous task",
	Example: `  poof policy rollback -p <id> --task <taskId>
  poof policy deploy -p <id> --env production --yes`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireAuth(); err != nil {
			return err
		}
		projectID, err := getProjectID()
		if err != nil {
			return err
		}
		taskID, _ := cmd.Flags().GetString("task")
		if taskID == "" {
			return fmt.Errorf("--task is required")
		}
		req, err := buildPolicyRequestFromFlags(cmd)
		if err != nil {
			return err
		}
		req.TaskID = taskID
		if err := validatePolicyRollbackRequest(projectID, taskID, &req); err != nil {
			return err
		}

		resp, err := apiClient.RollbackPolicy(context.Background(), projectID, &req)
		if err != nil {
			return handleError(err)
		}

		output.Print(resp, func() {
			if resp.DryRun {
				output.Success("Rollback dry-run passed for %s.", resp.Environment)
			} else {
				output.Success("Policy rolled back on %s.", resp.Environment)
			}
			output.Info("Task ID:       %s", resp.TaskID)
			output.Info("App ID:        %s", resp.AppID)
			output.Info("Policy hash:   %s", resp.PolicyHash)
			output.Info("Constants hash: %s", resp.ConstantsHash)
		})
		return nil
	},
}

func buildPolicyRequestFromFlags(cmd *cobra.Command) (api.PolicyRequest, error) {
	env := resolvePolicyEnvironment(cmd)
	if err := validatePolicyEnv(env); err != nil {
		return api.PolicyRequest{}, err
	}
	policy, err := readJSONFlagFile(cmd, "policy")
	if err != nil {
		return api.PolicyRequest{}, err
	}
	constants, err := readJSONFlagFile(cmd, "constants")
	if err != nil {
		return api.PolicyRequest{}, err
	}
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	return api.PolicyRequest{
		Environment: env,
		Policy:      policy,
		Constants:   constants,
		DryRun:      dryRun,
	}, nil
}

func resolvePolicyEnvironment(cmd *cobra.Command) string {
	if env := policyEnvironmentFromArgs(os.Args, cmd.Name()); env != "" {
		return env
	}
	env, _ := cmd.Flags().GetString("env")
	return env
}

func policyEnvironmentFromArgs(args []string, commandName string) string {
	commandIndex := -1
	for i, arg := range args {
		if arg == commandName {
			commandIndex = i
		}
	}
	if commandIndex == -1 {
		return ""
	}

	for i := commandIndex + 1; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			break
		}
		if arg == "--env" {
			if i+1 >= len(args) {
				return ""
			}
			return args[i+1]
		}
		if strings.HasPrefix(arg, "--env=") {
			return strings.TrimPrefix(arg, "--env=")
		}
	}
	return "draft"
}

func validatePolicyDeployRequest(req *api.PolicyRequest, confirmProduction bool) error {
	if req.Environment != "production" || req.DryRun {
		return nil
	}
	if req.Policy != "" || req.Constants != "" {
		return fmt.Errorf("production policy deploy uses the latest stored policy; deploy changes to draft first, run the production gate, then rerun without --policy/--constants")
	}
	if !confirmProduction {
		return fmt.Errorf("production policy deploy requires --yes")
	}
	return nil
}

func validatePolicyRollbackRequest(projectID, taskID string, req *api.PolicyRequest) error {
	if req.Environment == "production" && !req.DryRun {
		return fmt.Errorf("production rollback must be staged through draft first: poof policy rollback -p %s --task %s, then poof policy deploy -p %s --env production --yes", projectID, taskID, projectID)
	}
	return nil
}

func readJSONFlagFile(cmd *cobra.Command, name string) (string, error) {
	path, _ := cmd.Flags().GetString(name)
	if path == "" {
		return "", nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read --%s %s: %w", name, path, err)
	}
	if !json.Valid(data) {
		return "", fmt.Errorf("--%s must point to a valid JSON file: %s", name, path)
	}
	return string(data), nil
}

func writePolicyFiles(outDir, policy, constants string) error {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", outDir, err)
	}
	if err := os.WriteFile(filepath.Join(outDir, "poof.json"), []byte(policy), 0o644); err != nil {
		return fmt.Errorf("write poof.json: %w", err)
	}
	if err := os.WriteFile(filepath.Join(outDir, "constants.json"), []byte(constants), 0o644); err != nil {
		return fmt.Errorf("write constants.json: %w", err)
	}
	return nil
}

func validatePolicyEnv(env string) error {
	switch env {
	case "draft", "preview", "production":
		return nil
	default:
		return fmt.Errorf("invalid --env %q (valid: draft, preview, production)", env)
	}
}

func formatValidationErrors(v api.PolicyValidation) string {
	if len(v.Errors) > 0 {
		return strings.Join(v.Errors, "; ")
	}
	if v.Message != "" {
		return v.Message
	}
	return "unknown validation error"
}

func mustGetString(cmd *cobra.Command, name string) string {
	value, _ := cmd.Flags().GetString(name)
	return value
}

func printConnectionInfo(info *api.ConnectionInfo) {
	if info == nil {
		return
	}
	if info.Draft != nil {
		output.Info("Draft app:    %s", info.Draft.TarobaseAppId)
	}
	if info.Preview != nil {
		output.Info("Preview app:  %s", info.Preview.TarobaseAppId)
	}
	if info.Production != nil {
		output.Info("Prod app:     %s", info.Production.TarobaseAppId)
	}
	if info.ApiUrl != "" {
		output.Info("API URL:      %s", info.ApiUrl)
	}
}

func init() {
	for _, cmd := range []*cobra.Command{policyValidateCmd, policyDeployCmd} {
		cmd.Flags().String("env", "draft", "Environment: draft, preview, production")
		cmd.Flags().String("policy", "", "Path to policy JSON file")
		cmd.Flags().String("constants", "", "Path to constants JSON file")
		cmd.Flags().Bool("dry-run", false, "Validate without deploying or creating a task")
	}
	policyRollbackCmd.Flags().String("env", "draft", "Environment: draft, preview, production")
	policyRollbackCmd.Flags().Bool("dry-run", false, "Validate rollback without deploying or creating a task")
	policyDeployCmd.Flags().Bool("yes", false, "Confirm production deploy")
	policyRollbackCmd.Flags().String("task", "", "Source task ID to roll back to")
	policyGetCmd.Flags().String("out-dir", "", "Directory to write poof.json and constants.json")
	policyHistoryCmd.Flags().Int("limit", 20, "Max history entries to return")

	policyCmd.AddCommand(policyGetCmd)
	policyCmd.AddCommand(policyValidateCmd)
	policyCmd.AddCommand(policyDeployCmd)
	policyCmd.AddCommand(policyHistoryCmd)
	policyCmd.AddCommand(policyRollbackCmd)
}
