package config

// Environment holds the connection details for a Poof environment.
type Environment struct {
	AppID   string
	BaseURL string
	AuthURL string
}

// Environments maps environment names to their connection details.
var Environments = map[string]Environment{
	"production": {
		AppID:   "697d5189a1e3dd2cc1a82d2b",
		BaseURL: "https://poof.new",
		AuthURL: "https://auth.tarobase.com",
	},
	"staging": {
		AppID:   "6993d4b0b2b6ac08cd334dfb",
		BaseURL: "https://v2-staging.poof.new",
		AuthURL: "https://auth-staging.tarobase.com",
	},
	"local": {
		AppID:   "6993d4b0b2b6ac08cd334dfb",
		BaseURL: "http://localhost:3000",
		AuthURL: "https://auth-staging.tarobase.com",
	},
}
