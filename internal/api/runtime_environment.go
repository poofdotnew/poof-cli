package api

import "fmt"

func normalizeProjectRuntimeEnvironment(environment string) (string, error) {
	switch environment {
	case "":
		return "", nil
	case "development", "dev", "draft":
		return "development", nil
	case "mainnet-preview", "preview":
		return "mainnet-preview", nil
	case "production", "prod", "live":
		return "production", nil
	default:
		return "", fmt.Errorf("invalid environment %q (valid: draft, preview, production, live)", environment)
	}
}
