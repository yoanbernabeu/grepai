package cli

import (
	"fmt"
	"runtime"

	"github.com/yoanbernabeu/grepai/internal/managedassets"
)

func managedLlamaCPPSupported() bool {
	_, err := managedassets.LookupCurrentRuntime()
	return err == nil
}

func managedLlamaCPPUnsupportedError() error {
	return fmt.Errorf("managed llama.cpp is not available on %s/%s", runtime.GOOS, runtime.GOARCH)
}

func availableInitProviders() []string {
	providers := []string{"ollama", "lmstudio", "openai"}
	if managedLlamaCPPSupported() {
		providers = []string{"ollama", "llamacpp", "lmstudio", "openai"}
	}
	return providers
}

func availableWorkspaceProviders() []string {
	providers := []string{"ollama", "openai", "lmstudio"}
	if managedLlamaCPPSupported() {
		providers = []string{"ollama", "llamacpp", "openai", "lmstudio"}
	}
	return providers
}
