package embedder

import (
	"strings"
	"testing"
)

func TestIsOfficialEndpoint(t *testing.T) {
	tests := []struct {
		endpoint string
		want     bool
	}{
		{"https://api.openai.com/v1", true},
		{"https://api.openai.com/v1/", true},
		{"https://API.OpenAI.com/v1", true},
		{"https://api.openai.com./v1", true},
		{"https://api.openai.com:443/v1", true},
		{"https://api.openai.com", true},
		{"http://api.openai.com/v1", false},
		{"https://api.openai.com:8443/v1", false},
		{"https://api.openai.com@evil.example/v1", false},
		{"https://user:pass@api.openai.com/v1", false},
		{"https://api.openai.com.evil.example/v1", false},
		{"https://evil.example/api.openai.com/v1", false},
		{"https://evil.example?@api.openai.com/v1", false},
		{"https://evil.example#@api.openai.com/v1", false},
		{"https://аpi.openai.com/v1", false}, // Cyrillic "а"
		{"https://xn--pi-7ld.openai.com/v1", false},
		{"https://\u212aapi.openai.com/v1", false}, // Kelvin sign lowercases to "k"
		{"https://api.opena\u0130.com/v1", false},  // "İ" lowercases to "i"
		{"http://127.0.0.1:8080/v1", false},
		{"api.openai.com/v1", false},
		{" https://api.openai.com/v1", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := isOfficialEndpoint(tt.endpoint, defaultOpenAIEndpoint); got != tt.want {
			t.Errorf("isOfficialEndpoint(%q) = %v, want %v", tt.endpoint, got, tt.want)
		}
	}
}

func TestIsOfficialEndpoint_ProviderDefaults(t *testing.T) {
	for _, def := range []string{
		defaultOpenAIEndpoint,
		defaultOpenRouterEndpoint,
		defaultRequestyEndpoint,
		defaultSyntheticEndpoint + defaultSyntheticPath,
	} {
		if !isOfficialEndpoint(def, def) {
			t.Errorf("default endpoint %q must be official", def)
		}
	}
	if isOfficialEndpoint(defaultOpenRouterEndpoint, defaultOpenAIEndpoint) {
		t.Error("another provider's host must not count as official")
	}
}

// The environment key must never be attached to a request sent to an
// endpoint chosen by a project config, and the failure must be explicit.
func TestEnvKeyNotUsedForCustomEndpoint(t *testing.T) {
	const custom = "http://127.0.0.1:1/v1"
	const secret = "sk-env-secret-must-not-leak"
	for _, name := range []string{"OPENAI_API_KEY", "OPENROUTER_API_KEY", "SYNTHETIC_API_KEY", "REQUESTY_API_KEY"} {
		t.Setenv(name, "")
	}

	cases := []struct {
		name   string
		envVar string
		build  func() error
	}{
		{"openai", "OPENAI_API_KEY", func() error {
			_, err := NewOpenAIEmbedder(WithOpenAIEndpoint(custom))
			return err
		}},
		{"openrouter", "OPENROUTER_API_KEY", func() error {
			_, err := NewOpenRouterEmbedder(WithOpenRouterEndpoint(custom))
			return err
		}},
		{"openrouter via OPENAI_API_KEY", "OPENAI_API_KEY", func() error {
			_, err := NewOpenRouterEmbedder(WithOpenRouterEndpoint(custom))
			return err
		}},
		{"synthetic", "SYNTHETIC_API_KEY", func() error {
			_, err := NewSyntheticEmbedder(WithSyntheticEndpoint(custom))
			return err
		}},
		{"synthetic via OPENAI_API_KEY", "OPENAI_API_KEY", func() error {
			_, err := NewSyntheticEmbedder(WithSyntheticEndpoint(custom))
			return err
		}},
		{"requesty", "REQUESTY_API_KEY", func() error {
			_, err := NewRequestyEmbedder(WithRequestyEndpoint(custom))
			return err
		}},
		{"requesty via OPENAI_API_KEY", "OPENAI_API_KEY", func() error {
			_, err := NewRequestyEmbedder(WithRequestyEndpoint(custom))
			return err
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(tc.envVar, secret)
			err := tc.build()
			if err == nil {
				t.Fatal("expected an error: the environment key must not be used for a custom endpoint")
			}
			msg := err.Error()
			if strings.Contains(msg, secret) {
				t.Fatalf("error message leaks the key: %s", msg)
			}
			if !strings.Contains(msg, "embedder.api_key") || !strings.Contains(msg, tc.envVar) {
				t.Fatalf("error message is not actionable: %s", msg)
			}
		})
	}
}

func TestEnvKeyUsedForOfficialEndpoint(t *testing.T) {
	for _, name := range []string{"OPENAI_API_KEY", "OPENROUTER_API_KEY", "SYNTHETIC_API_KEY", "REQUESTY_API_KEY"} {
		t.Setenv(name, "")
	}
	t.Setenv("OPENAI_API_KEY", "env-openai")

	oa, err := NewOpenAIEmbedder(WithOpenAIEndpoint(defaultOpenAIEndpoint))
	if err != nil {
		t.Fatalf("openai: %v", err)
	}
	if oa.apiKey != "env-openai" {
		t.Fatalf("openai: key=%q", oa.apiKey)
	}
	or, err := NewOpenRouterEmbedder(WithOpenRouterEndpoint(defaultOpenRouterEndpoint))
	if err != nil {
		t.Fatalf("openrouter: %v", err)
	}
	if or.apiKey != "env-openai" {
		t.Fatalf("openrouter: key=%q", or.apiKey)
	}
	sy, err := NewSyntheticEmbedder(WithSyntheticEndpoint(defaultSyntheticEndpoint + defaultSyntheticPath))
	if err != nil {
		t.Fatalf("synthetic: %v", err)
	}
	if sy.apiKey != "env-openai" {
		t.Fatalf("synthetic: key=%q", sy.apiKey)
	}
	rq, err := NewRequestyEmbedder(WithRequestyEndpoint(defaultRequestyEndpoint))
	if err != nil {
		t.Fatalf("requesty: %v", err)
	}
	if rq.apiKey != "env-openai" {
		t.Fatalf("requesty: key=%q", rq.apiKey)
	}
}

func TestConfigKeyStillWorksForCustomEndpoint(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "env-secret")
	e, err := NewOpenAIEmbedder(WithOpenAIEndpoint("http://127.0.0.1:1/v1"), WithOpenAIKey("config-key"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if e.apiKey != "config-key" {
		t.Fatalf("expected config key, got %q", e.apiKey)
	}
}

// A config without endpoint (e.g. a hand-written workspace) keeps the
// provider's official endpoint and therefore its environment key.
func TestEmptyEndpointKeepsDefault(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "env-openai")
	t.Setenv("OPENROUTER_API_KEY", "")
	t.Setenv("SYNTHETIC_API_KEY", "")
	t.Setenv("REQUESTY_API_KEY", "")

	oa, err := NewOpenAIEmbedder(WithOpenAIEndpoint(""))
	if err != nil {
		t.Fatalf("openai: %v", err)
	}
	if oa.endpoint != defaultOpenAIEndpoint {
		t.Fatalf("openai: endpoint=%q", oa.endpoint)
	}
	or, err := NewOpenRouterEmbedder(WithOpenRouterEndpoint(""))
	if err != nil {
		t.Fatalf("openrouter: %v", err)
	}
	if or.endpoint != defaultOpenRouterEndpoint {
		t.Fatalf("openrouter: endpoint=%q", or.endpoint)
	}
	sy, err := NewSyntheticEmbedder(WithSyntheticEndpoint(""))
	if err != nil {
		t.Fatalf("synthetic: %v", err)
	}
	if sy.endpoint != defaultSyntheticEndpoint+defaultSyntheticPath {
		t.Fatalf("synthetic: endpoint=%q", sy.endpoint)
	}
	rq, err := NewRequestyEmbedder(WithRequestyEndpoint(""))
	if err != nil {
		t.Fatalf("requesty: %v", err)
	}
	if rq.endpoint != defaultRequestyEndpoint {
		t.Fatalf("requesty: endpoint=%q", rq.endpoint)
	}
}

func TestEnvKeyErrorNamesAllVariablesAndRedactsEndpoint(t *testing.T) {
	t.Setenv("REQUESTY_API_KEY", "env-requesty")
	t.Setenv("OPENAI_API_KEY", "env-openai")
	_, err := NewRequestyEmbedder(WithRequestyEndpoint("https://user-token:pw@proxy.example/v1?api-key=query-secret"))
	if err == nil {
		t.Fatal("expected an error for a custom endpoint")
	}
	msg := err.Error()
	for _, want := range []string{"REQUESTY_API_KEY", "OPENAI_API_KEY", "https://proxy.example/v1"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q should contain %q", msg, want)
		}
	}
	for _, leak := range []string{"user-token", "pw", "query-secret", "env-requesty", "env-openai"} {
		if strings.Contains(msg, leak) {
			t.Errorf("error %q leaks %q", msg, leak)
		}
	}
}

func TestUsesOfficialEndpoint(t *testing.T) {
	tests := []struct {
		provider, endpoint string
		want               bool
	}{
		{"openai", "", true},
		{"openai", "https://API.openai.com:443/v1/", true},
		{"openai", "https://myres.openai.azure.com/v1", false},
		{"synthetic", "https://api.synthetic.new/openai/v1", true},
		{"openrouter", "https://openrouter.ai/api/v1", true},
		{"requesty", "http://localhost:8080/v1", false},
		{"ollama", "http://localhost:11434", false},
	}
	for _, tt := range tests {
		if got := UsesOfficialEndpoint(tt.provider, tt.endpoint); got != tt.want {
			t.Errorf("UsesOfficialEndpoint(%q, %q) = %v, want %v", tt.provider, tt.endpoint, got, tt.want)
		}
	}
}
