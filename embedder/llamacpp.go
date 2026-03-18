package embedder

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/yoanbernabeu/grepai/internal/managedassets"
)

const (
	defaultLlamaCPPModel = managedassets.DefaultModelID
)

type LlamaCPPEmbedder struct {
	model       string
	modelPath   string
	endpoint    string
	dimensions  int
	runtimePath string
	queryPrefix string
	docPrefix   string
	client      *http.Client
}

type LlamaCPPOption func(*LlamaCPPEmbedder)

type llamaCPPEmbedRequest struct {
	Content string `json:"content,omitempty"`
	Input   string `json:"input,omitempty"`
}

type llamaCPPEmbedResponse struct {
	Embedding []float32 `json:"embedding"`
	Data      []struct {
		Embedding []float32 `json:"embedding"`
	} `json:"data"`
}

func WithLlamaCPPModel(model string) LlamaCPPOption {
	return func(e *LlamaCPPEmbedder) {
		e.model = model
	}
}

func WithLlamaCPPModelPath(path string) LlamaCPPOption {
	return func(e *LlamaCPPEmbedder) {
		e.modelPath = path
	}
}

func WithLlamaCPPEndpoint(endpoint string) LlamaCPPOption {
	return func(e *LlamaCPPEmbedder) {
		e.endpoint = endpoint
	}
}

func WithLlamaCPPDimensions(dimensions int) LlamaCPPOption {
	return func(e *LlamaCPPEmbedder) {
		e.dimensions = dimensions
	}
}

func WithLlamaCPPRuntimePath(path string) LlamaCPPOption {
	return func(e *LlamaCPPEmbedder) {
		e.runtimePath = path
	}
}

func NewLlamaCPPEmbedder(opts ...LlamaCPPOption) (*LlamaCPPEmbedder, error) {
	e := &LlamaCPPEmbedder{
		model:      defaultLlamaCPPModel,
		endpoint:   managedassets.DefaultSidecarEndpoint(),
		dimensions: 384,
		client: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
	for _, opt := range opts {
		opt(e)
	}
	if e.dimensions <= 0 {
		e.dimensions = 768
	}
	modelPath, dims, err := managedassets.ResolveModelPath(e.model, e.modelPath)
	if err != nil {
		return nil, err
	}
	modelDef, err := managedassets.LookupModel(e.model)
	if err == nil {
		e.queryPrefix = modelDef.QueryPrefix
		e.docPrefix = modelDef.DocPrefix
	}
	e.modelPath = modelPath
	if e.dimensions == 384 && dims > 0 {
		e.dimensions = dims
	}
	if e.runtimePath == "" {
		runtimeDef, err := managedassets.LookupCurrentRuntime()
		if err != nil {
			return nil, err
		}
		runtimePath, err := managedassets.ManagedRuntimeBinaryPath(runtimeDef)
		if err != nil {
			return nil, err
		}
		e.runtimePath = runtimePath
	}
	return e, nil
}

func (e *LlamaCPPEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	return e.EmbedWithRole(ctx, text, RoleGeneric)
}

func (e *LlamaCPPEmbedder) EmbedWithRole(ctx context.Context, text string, role InputRole) ([]float32, error) {
	if err := e.ensureRunning(ctx); err != nil {
		return nil, err
	}
	text = e.applyRolePrefix(text, role)
	body, err := json.Marshal(llamaCPPEmbedRequest{
		Content: text,
		Input:   text,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal llama.cpp request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(e.endpoint, "/")+"/embedding", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create llama.cpp request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request to llama.cpp: %w", err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read llama.cpp response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("llama.cpp returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	var result llamaCPPEmbedResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to decode llama.cpp response: %w", err)
	}
	switch {
	case len(result.Embedding) > 0:
		return result.Embedding, nil
	case len(result.Data) > 0 && len(result.Data[0].Embedding) > 0:
		return result.Data[0].Embedding, nil
	default:
		return nil, fmt.Errorf("llama.cpp returned empty embedding")
	}
}

func (e *LlamaCPPEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	return e.EmbedBatchWithRole(ctx, texts, RoleGeneric)
}

func (e *LlamaCPPEmbedder) EmbedBatchWithRole(ctx context.Context, texts []string, role InputRole) ([][]float32, error) {
	results := make([][]float32, len(texts))
	for i, text := range texts {
		embedding, err := e.EmbedWithRole(ctx, text, role)
		if err != nil {
			return nil, fmt.Errorf("failed to embed text %d: %w", i, err)
		}
		results[i] = embedding
	}
	return results, nil
}

func (e *LlamaCPPEmbedder) applyRolePrefix(text string, role InputRole) string {
	switch role {
	case RoleQuery:
		if e.queryPrefix != "" && !strings.HasPrefix(text, e.queryPrefix) {
			return e.queryPrefix + text
		}
	case RoleDocument:
		if e.docPrefix != "" && !strings.HasPrefix(text, e.docPrefix) {
			return e.docPrefix + text
		}
	}
	return text
}

func (e *LlamaCPPEmbedder) Dimensions() int {
	return e.dimensions
}

func (e *LlamaCPPEmbedder) Close() error {
	return nil
}

func (e *LlamaCPPEmbedder) Ping(ctx context.Context) error {
	if err := e.ensureRunning(ctx); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(e.endpoint, "/")+"/health", nil)
	if err != nil {
		return fmt.Errorf("failed to create llama.cpp health request: %w", err)
	}
	resp, err := e.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to reach llama.cpp at %s: %w", e.endpoint, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("llama.cpp returned status %d", resp.StatusCode)
	}
	return nil
}

func (e *LlamaCPPEmbedder) ensureRunning(ctx context.Context) error {
	if ok := waitForHealth(ctx, e.client, e.endpoint, 250*time.Millisecond); ok {
		return nil
	}

	state, err := managedassets.LoadRuntimeState()
	if err != nil {
		return err
	}
	if state != nil && state.Binary == e.runtimePath && state.Endpoint == e.endpoint {
		if ok := waitForHealth(ctx, e.client, e.endpoint, 250*time.Millisecond); ok {
			return nil
		}
	}
	return e.startSidecar(ctx)
}

func (e *LlamaCPPEmbedder) startSidecar(ctx context.Context) error {
	if err := managedassets.EnsureManagedDirs(); err != nil {
		return err
	}
	runtimePath, _, err := managedassets.EnsureRuntime(ctx, nil)
	if err != nil {
		return err
	}
	e.runtimePath = runtimePath
	u, err := net.ResolveTCPAddr("tcp", strings.TrimPrefix(strings.TrimPrefix(e.endpoint, "http://"), "https://"))
	if err != nil {
		return fmt.Errorf("invalid llama.cpp endpoint %s: %w", e.endpoint, err)
	}
	port := u.Port
	host := u.IP.String()
	if host == "" || host == "<nil>" {
		host = "127.0.0.1"
	}
	cmd := exec.CommandContext(ctx, e.runtimePath,
		"--host", host,
		"--port", strconv.Itoa(port),
		"--model", e.modelPath,
		"--embeddings",
		"--batch-size", "4096",
		"--ubatch-size", "4096",
	)
	logPath, err := managedassets.GetManagedRuntimeStatePath()
	if err != nil {
		return err
	}
	logPath = strings.TrimSuffix(logPath, ".json") + ".log"
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("failed to open llama.cpp log file: %w", err)
	}
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if err := cmd.Start(); err != nil {
		logFile.Close()
		return fmt.Errorf("failed to start managed llama.cpp runtime: %w", err)
	}
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
		_ = logFile.Close()
	}()
	state := managedassets.RuntimeState{
		Version:  managedassets.DefaultRuntimeVersion,
		Platform: runtime.GOOS,
		Arch:     runtime.GOARCH,
		Binary:   e.runtimePath,
		Endpoint: e.endpoint,
		PID:      cmd.Process.Pid,
		Started:  time.Now().UTC(),
	}
	if err := managedassets.SaveRuntimeState(state); err != nil {
		return err
	}
	healthCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	if err := waitForRuntimeReady(healthCtx, e.client, e.endpoint, done); err != nil {
		_ = managedassets.ClearRuntimeState()
		return err
	}
	return nil
}

func waitForHealth(ctx context.Context, client *http.Client, endpoint string, interval time.Duration) bool {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(endpoint, "/")+"/health", nil)
		resp, err := client.Do(req)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return true
			}
		}
		select {
		case <-ctx.Done():
			return false
		case <-ticker.C:
		}
	}
}

func waitForRuntimeReady(ctx context.Context, client *http.Client, endpoint string, done <-chan error) error {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		if checkHealth(client, endpoint) {
			return nil
		}
		select {
		case err := <-done:
			if checkHealth(client, endpoint) {
				return nil
			}
			if err != nil {
				return fmt.Errorf("managed llama.cpp runtime exited before becoming ready: %w", err)
			}
			return fmt.Errorf("managed llama.cpp runtime exited before becoming ready")
		case <-ctx.Done():
			if checkHealth(client, endpoint) {
				return nil
			}
			return fmt.Errorf("managed llama.cpp runtime did not become ready at %s", endpoint)
		case <-ticker.C:
		}
	}
}

func checkHealth(client *http.Client, endpoint string) bool {
	req, err := http.NewRequest(http.MethodGet, strings.TrimRight(endpoint, "/")+"/health", nil)
	if err != nil {
		return false
	}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}
