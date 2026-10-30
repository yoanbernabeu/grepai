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
	key := ""
	var set []string
	for _, name := range names {
		if value := os.Getenv(name); value != "" {
			if key == "" {
				key = value
			}
			set = append(set, name)
		}
	}
	if key == "" {
		return "", nil
	}
	if !isOfficialEndpoint(endpoint, defaultEndpoint) {
		return "", fmt.Errorf(
			"API keys from the environment (%s) are only sent to the official endpoint %s, not to the custom endpoint %s: set embedder.api_key in the grepai config to use a key with this endpoint",
			strings.Join(set, ", "), defaultEndpoint, redactEndpoint(endpoint))
	}
	return key, nil
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

// redactEndpoint keeps only the scheme, host and path of an endpoint, so
// that credentials in the userinfo or the query never reach error messages.
func redactEndpoint(endpoint string) string {
	u, err := url.Parse(endpoint)
	if err != nil || u.Host == "" {
		return "(invalid URL)"
	}
	return (&url.URL{Scheme: u.Scheme, Host: u.Host, Path: u.Path}).String()
}
