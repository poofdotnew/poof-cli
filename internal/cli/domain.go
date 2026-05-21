package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/poofdotnew/poof-cli/internal/api"
	"github.com/poofdotnew/poof-cli/internal/output"
	"github.com/spf13/cobra"
)

var domainCmd = &cobra.Command{
	Use:   "domain",
	Short: "Manage custom domains (requires credit purchase)",
}

var domainListCmd = &cobra.Command{
	Use:     "list",
	Short:   "List custom domains",
	Example: `  poof domain list -p <id>`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireAuth(); err != nil {
			return err
		}

		projectID, err := getProjectID()
		if err != nil {
			return err
		}

		resp, err := apiClient.GetDomains(context.Background(), projectID)
		if err != nil {
			return handleError(err)
		}

		output.Print(resp, func() {
			if len(resp.Domains) == 0 {
				output.Info("No custom domains configured.")
				return
			}
			rows := make([][]string, len(resp.Domains))
			for i, d := range resp.Domains {
				def := ""
				if d.IsDefault {
					def = "yes"
				}
				rows[i] = []string{d.Domain, d.Status, def, fmt.Sprintf("%d", len(d.RequiredDNSRecords()))}
			}
			output.Table([]string{"Domain", "Status", "Default", "DNS needed"}, rows)
			printDomainsDNSDetails(resp.Domains)
		})
		return nil
	},
}

var domainAddCmd = &cobra.Command{
	Use:   "add [domain]",
	Short: "Add a custom domain",
	Example: `  poof domain add myapp.com -p <id>
  poof domain add myapp.com -p <id> --default`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireAuth(); err != nil {
			return err
		}

		projectID, err := getProjectID()
		if err != nil {
			return err
		}

		isDefault, _ := cmd.Flags().GetBool("default")

		resp, err := apiClient.AddDomain(context.Background(), projectID, args[0], isDefault)
		if err != nil {
			return handleError(err)
		}

		output.Print(resp, func() {
			domainName := args[0]
			if resp.Domain.Domain != "" {
				domainName = resp.Domain.Domain
			}
			output.Success("Domain %s added.", domainName)
			if resp.Domain.IsDefault || isDefault {
				output.Info("Set as default domain.")
			}
			if resp.DNSInstructions != nil && resp.DNSInstructions.Message != "" {
				output.Info(resp.DNSInstructions.Message)
			}
			printDNSRecords(domainName, recordsForAddedDomain(resp))
		})
		return nil
	},
}

var domainCheckCmd = &cobra.Command{
	Use:   "check [domain-or-id]",
	Short: "Refresh and show custom domain validation status",
	Example: `  poof domain check myapp.com -p <id>
  poof domain check <domain-id> -p <id>`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireAuth(); err != nil {
			return err
		}

		projectID, err := getProjectID()
		if err != nil {
			return err
		}

		domainID, err := resolveDomainID(context.Background(), projectID, args[0])
		if err != nil {
			return handleError(err)
		}

		resp, err := apiClient.CheckDomainStatus(context.Background(), projectID, domainID)
		if err != nil {
			return handleError(err)
		}

		output.Print(resp, func() {
			domainName := resp.Domain.Domain
			if domainName == "" {
				domainName = args[0]
			}
			output.Info("Domain: %s", domainName)
			output.Info("Status: %s", emptyAsUnknown(resp.Domain.Status))
			if resp.CloudflareStatus != nil {
				output.Info("Cloudflare hostname: %s", emptyAsUnknown(resp.CloudflareStatus.Status))
				output.Info("Cloudflare SSL: %s", emptyAsUnknown(resp.CloudflareStatus.SSLStatus))
			}
			if resp.Message != "" {
				output.Info(resp.Message)
			}
			printDNSRecords(domainName, resp.Domain.RequiredDNSRecords())
		})
		return nil
	},
}

func init() {
	domainAddCmd.Flags().Bool("default", false, "Set as default domain")

	domainCmd.AddCommand(domainListCmd)
	domainCmd.AddCommand(domainAddCmd)
	domainCmd.AddCommand(domainCheckCmd)
}

func resolveDomainID(ctx context.Context, projectID, domainOrID string) (string, error) {
	resp, err := apiClient.GetDomains(ctx, projectID)
	if err != nil {
		return "", err
	}

	needle := strings.TrimSpace(domainOrID)
	for _, domain := range resp.Domains {
		if strings.EqualFold(domain.ID, needle) || strings.EqualFold(domain.Domain, needle) {
			if domain.ID == "" {
				return "", fmt.Errorf("domain %s has no id in API response", domain.Domain)
			}
			return domain.ID, nil
		}
	}
	return domainOrID, nil
}

func recordsForAddedDomain(resp *api.AddDomainResponse) []api.DNSRecord {
	if resp == nil {
		return nil
	}
	if resp.DNSInstructions != nil && len(resp.DNSInstructions.Records) > 0 {
		return resp.DNSInstructions.Records
	}
	return resp.Domain.RequiredDNSRecords()
}

func printDomainsDNSDetails(domains []api.Domain) {
	for _, domain := range domains {
		printDNSRecords(domain.Domain, domain.RequiredDNSRecords())
	}
}

func printDNSRecords(domain string, records []api.DNSRecord) {
	if len(records) == 0 {
		return
	}
	output.Info("")
	output.Info("Required DNS records for %s:", domain)
	for _, record := range records {
		purpose := record.Purpose
		if purpose == "" {
			purpose = record.Description
		}
		suffix := ""
		if purpose != "" {
			suffix = fmt.Sprintf(" (%s)", purpose)
		}
		output.Info("  %s %s -> %s%s", strings.ToUpper(record.Type), record.Name, record.Value, suffix)
	}
}

func emptyAsUnknown(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "unknown"
	}
	return value
}
