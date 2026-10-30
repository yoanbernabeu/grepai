package embedder

import (
	"fmt"
	"net/url"
	"os"
	"strings"
)

// apiKeyFromEnv returns the first non-empty environment variable among names.
//
// The endpoint usually comes from the project's .grepai/config.yaml, which can
// ship with a cloned repository. A key taken from the environment is therefore
// only returned when the requests go to the provider's official host
// (defaultEndpoint). For any other endpoint the key must be set explicitly in
// embedder.api_key, and an error explains it instead of silently sending the
// environment key to a host the user did not choose.
func apiKeyFromEnv(endpoint, defaultEndpoint string, names ...string) (string, error) {
	for _, name := range names {
		key := os.Getenv(name)
		if key == "" {
			continue
		}
		if !isOfficialEndpoint(endpoint, defaultEndpoint) {
			return "", fmt.Errorf(
				"%s is only sent to the official endpoint %s, not to the custom endpoint %s: set embedder.api_key in the grepai config to use a key with this endpoint",
				name, defaultEndpoint, redactEndpoint(endpoint))
		}
		return key, nil
	}
	return "", nil
}

// isOfficialEndpoint reports whether requests built from endpoint (the
// embedders send POST "<endpoint>/embeddings") go to the same host as
// defaultEndpoint, over HTTPS, on the default port and without userinfo.
func isOfficialEndpoint(endpoint, defaultEndpoint string) bool {
	official, err := url.Parse(defaultEndpoint)
	if err != nil || official.Hostname() == "" {
		return false
	}
	target, err := url.Parse(endpoint + "/embeddings")
	if err != nil {
		return false
	}
	if target.Scheme != "https" || target.User != nil || target.Opaque != "" {
		return false
	}
	if port := target.Port(); port != "" && port != "443" {
		return false
	}
	return normalizeHost(target.Hostname()) == normalizeHost(official.Hostname())
}

func normalizeHost(host string) string {
	return strings.TrimSuffix(strings.ToLower(host), ".")
}

// redactEndpoint hides credentials that may be embedded in an endpoint URL.
func redactEndpoint(endpoint string) string {
	u, err := url.Parse(endpoint)
	if err != nil {
		return "(invalid URL)"
	}
	return u.Redacted()
}
