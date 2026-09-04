package model

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"
)

type Capabilities struct {
	Responses        bool `json:"responses"`
	ChatCompletions  bool `json:"chat_completions"`
	NativeTools      bool `json:"native_tools"`
	StructuredOutput bool `json:"structured_output"`
	Reasoning        bool `json:"reasoning"`
	PreviousResponse bool `json:"previous_response"`
}

type Info struct {
	Provider     string       `json:"provider"`
	Model        string       `json:"model"`
	Capabilities Capabilities `json:"capabilities"`
}

type Tool struct {
	Name        string
	Description string
	Parameters  map[string]any
}

type Message struct {
	Role          string
	Content       string
	Parts         []ContentPart
	ToolCallID    string
	ToolName      string
	ToolArguments json.RawMessage
}

type ContentPart struct {
	Type      string
	Text      string
	MediaType string
	Data      string
}

type Request struct {
	System             string
	Messages           []Message
	Tools              []Tool
	DisableTools       bool
	RequiredTool       string
	PreviousResponseID string
}

type ToolCall struct {
	ID        string
	Name      string
	Arguments json.RawMessage
}

type Usage struct {
	InputTokens     int64 `json:"input_tokens"`
	OutputTokens    int64 `json:"output_tokens"`
	ReasoningTokens int64 `json:"reasoning_tokens,omitempty"`
	CachedTokens    int64 `json:"cached_tokens,omitempty"`
}

type Result struct {
	Content       string
	Reasoning     string
	ToolCall      *ToolCall
	FinishReason  string
	ResponseID    string
	RequestID     string
	Usage         Usage
	DeltasEmitted bool
}

type Provider interface {
	Info() Info
	Complete(context.Context, Request) (Result, error)
}

type StreamingProvider interface {
	CompleteStream(context.Context, Request, func(string) error) (Result, error)
}

type ConfigLoader func(context.Context) (HTTPConfig, error)

type DynamicProvider struct {
	mu          sync.RWMutex
	loader      ConfigLoader
	refresh     time.Duration
	config      HTTPConfig
	provider    *HTTPProvider
	refreshedAt time.Time
}

func NewDynamicProvider(ctx context.Context, loader ConfigLoader, refresh time.Duration) (*DynamicProvider, error) {
	if loader == nil {
		return nil, fmt.Errorf("model config loader is required")
	}
	if refresh <= 0 {
		refresh = time.Minute
	}
	result := &DynamicProvider{loader: loader, refresh: refresh}
	if _, err := result.current(ctx); err != nil {
		return nil, err
	}
	return result, nil
}

func (p *DynamicProvider) Info() Info {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.provider == nil {
		return Info{}
	}
	return p.provider.Info()
}

func (p *DynamicProvider) Complete(ctx context.Context, request Request) (Result, error) {
	provider, err := p.current(ctx)
	if err != nil {
		return Result{}, err
	}
	return provider.Complete(ctx, request)
}

func (p *DynamicProvider) CompleteStream(ctx context.Context, request Request, onDelta func(string) error) (Result, error) {
	provider, err := p.current(ctx)
	if err != nil {
		return Result{}, err
	}
	return provider.CompleteStream(ctx, request, onDelta)
}

func (p *DynamicProvider) current(ctx context.Context) (*HTTPProvider, error) {
	p.mu.RLock()
	if p.provider != nil && time.Since(p.refreshedAt) < p.refresh {
		provider := p.provider
		p.mu.RUnlock()
		return provider, nil
	}
	p.mu.RUnlock()

	p.mu.Lock()
	defer p.mu.Unlock()
	if p.provider != nil && time.Since(p.refreshedAt) < p.refresh {
		return p.provider, nil
	}
	config, err := p.loader(ctx)
	if err != nil {
		return nil, fmt.Errorf("refresh model configuration: %w", err)
	}
	if p.provider != nil && config == p.config {
		p.refreshedAt = time.Now()
		return p.provider, nil
	}
	provider, err := NewHTTPProvider(config)
	if err != nil {
		return nil, fmt.Errorf("apply model configuration: %w", err)
	}
	p.config, p.provider, p.refreshedAt = config, provider, time.Now()
	return provider, nil
}

type ErrorKind string

const (
	ErrorAuthentication ErrorKind = "authentication"
	ErrorRateLimit      ErrorKind = "rate_limit"
	ErrorTimeout        ErrorKind = "timeout"
	ErrorUnavailable    ErrorKind = "unavailable"
	ErrorInvalidRequest ErrorKind = "invalid_request"
	ErrorInvalidOutput  ErrorKind = "invalid_output"
)

type ProviderError struct {
	Kind      ErrorKind
	Retryable bool
	Status    int
	Message   string
	cause     error
}

func (e *ProviderError) Error() string { return e.Message }
func (e *ProviderError) Unwrap() error { return e.cause }
func IsKind(err error, kind ErrorKind) bool {
	var value *ProviderError
	return errors.As(err, &value) && value.Kind == kind
}

type HTTPConfig struct {
	Provider        string
	BaseURL         string
	APIKey          string
	Model           string
	Proxy           string
	ReasoningEffort string
	Timeout         time.Duration
	Store           bool
}
