package tarobase

import (
	"context"
	"fmt"

	"github.com/poofdotnew/poof-cli/internal/api"
)

// ResolvedEnv holds everything `poof data` needs to build a Client:
// the tarobase appid for the target environment, plus which chain it runs on.
type ResolvedEnv struct {
	AppID      string
	BackendURL string
	APIURL     string
	AuthURL    string
	Chain      Chain
	Env        Environment
}

// Resolve looks up the tarobase appid for a given Poof project + environment
// by calling the project-management API's /status endpoint. Also maps the
// environment to the right chain (draft -> offchain, preview/production -> mainnet).
func Resolve(ctx context.Context, poofAPI *api.Client, projectID string, env Environment) (*ResolvedEnv, error) {
	status, err := poofAPI.GetProjectStatus(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("fetch project status: %w", err)
	}
	if status.ConnectionInfo == nil {
		return nil, fmt.Errorf("project %s has no connectionInfo — has it been built yet?", projectID)
	}

	var (
		envInfo *api.ConnectionEnv
		chain   Chain
	)
	switch env {
	case EnvDraft:
		envInfo, chain = status.ConnectionInfo.Draft, ChainOffchain
	case EnvPreview:
		envInfo, chain = status.ConnectionInfo.Preview, ChainMainnet
	case EnvProduction:
		envInfo, chain = status.ConnectionInfo.Production, ChainMainnet
	default:
		return nil, fmt.Errorf("invalid environment %q (valid: draft, preview, production)", env)
	}
	if envInfo == nil || envInfo.TarobaseAppId == "" {
		return nil, fmt.Errorf("project has no %s environment provisioned", env)
	}

	apiURL := status.ConnectionInfo.ApiUrl
	if apiURL == "" {
		apiURL = "https://api.tarobase.com"
	}
	authURL := status.ConnectionInfo.AuthApiUrl
	if authURL == "" {
		authURL = "https://auth.tarobase.com"
	}

	return &ResolvedEnv{
		AppID:      envInfo.TarobaseAppId,
		BackendURL: envInfo.BackendUrl,
		APIURL:     apiURL,
		AuthURL:    authURL,
		Chain:      chain,
		Env:        env,
	}, nil
}

// ParseEnvironment maps a CLI flag value to an Environment, with draft as
// the default when empty.
func ParseEnvironment(s string) (Environment, error) {
	switch s {
	case "", "draft":
		return EnvDraft, nil
	case "preview":
		return EnvPreview, nil
	case "production", "prod":
		return EnvProduction, nil
	default:
		return "", fmt.Errorf("invalid environment %q (valid: draft, preview, production)", s)
	}
}
