package model

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestChatCompletionStreaming(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("Content-Type", "text/event-stream")
		_, _ = response.Write([]byte("data: {\"id\":\"response-1\",\"choices\":[{\"delta\":{\"content\":\"hel\"}}]}\n\n"))
		_, _ = response.Write([]byte("data: {\"id\":\"response-1\",\"choices\":[{\"delta\":{\"content\":\"lo\"},\"finish_reason\":\"stop\"}]}\n\n"))
		_, _ = response.Write([]byte("data: {\"id\":\"response-1\",\"usage\":{\"prompt_tokens\":2,\"completion_tokens\":1}}\n\ndata: [DONE]\n\n"))
	}))
	defer server.Close()
	provider, err := NewHTTPProvider(HTTPConfig{Provider: "compatible", BaseURL: server.URL, Model: "test", Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	deltas := ""
	result, err := provider.CompleteStream(context.Background(), Request{Messages: []Message{{Role: "user", Content: "hi"}}}, func(delta string) error {
		deltas += delta
		return nil
	})
	if err != nil || result.Content != "hello" || deltas != "hello" || !result.DeltasEmitted || result.Usage.InputTokens != 2 {
		t.Fatalf("unexpected stream result: result=%+v deltas=%q err=%v", result, deltas, err)
	}
}

func TestResponsesCompletion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/responses" {
			t.Errorf("unexpected request path %q", request.URL.Path)
		}
		response.Header().Set("Content-Type", "application/json")
		response.Header().Set("x-request-id", "request-1")
		_, _ = response.Write([]byte(`{"id":"response-1","object":"response","model":"test","status":"completed","output":[{"id":"message-1","type":"message","role":"assistant","status":"completed","content":[{"type":"output_text","text":"done","annotations":[]}]}],"usage":{"input_tokens":2,"output_tokens":1,"total_tokens":3,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":0}}}`))
	}))
	defer server.Close()
	provider, err := NewHTTPProvider(HTTPConfig{Provider: "openai", BaseURL: server.URL, Model: "test", Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	result, err := provider.Complete(context.Background(), Request{Messages: []Message{{Role: "user", Content: "hi"}}})
	if err != nil || result.Content != "done" || result.RequestID != "request-1" || result.Usage.InputTokens != 2 {
		t.Fatalf("unexpected Responses result: result=%+v err=%v", result, err)
	}
}

func TestDynamicProviderRefreshesConfiguration(t *testing.T) {
	loads := 0
	provider, err := NewDynamicProvider(context.Background(), func(context.Context) (HTTPConfig, error) {
		loads++
		return HTTPConfig{Provider: "compatible", BaseURL: "https://model.example.test/v1", Model: "model-1", Timeout: time.Minute}, nil
	}, time.Nanosecond)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(time.Millisecond)
	if _, err = provider.current(context.Background()); err != nil {
		t.Fatal(err)
	}
	if loads != 2 || provider.Info().Model != "model-1" {
		t.Fatalf("unexpected dynamic provider state: loads=%d info=%#v", loads, provider.Info())
	}
}

func TestRequiredToolChoice(t *testing.T) {
	provider := &HTTPProvider{config: HTTPConfig{Model: "test"}}
	request := Request{Tools: []Tool{{Name: "inspect", Parameters: map[string]any{"type": "object"}}}, RequiredTool: "inspect"}
	responsesJSON, err := json.Marshal(provider.responsesParams(request))
	if err != nil {
		t.Fatal(err)
	}
	chatJSON, err := json.Marshal(provider.chatParams(request))
	if err != nil {
		t.Fatal(err)
	}
	var responsesPayload, chatPayload map[string]any
	if json.Unmarshal(responsesJSON, &responsesPayload) != nil || json.Unmarshal(chatJSON, &chatPayload) != nil {
		t.Fatal("SDK request payload is invalid")
	}
	responsesChoice := responsesPayload["tool_choice"].(map[string]any)
	if responsesChoice["type"] != "function" || responsesChoice["name"] != "inspect" {
		t.Fatal("Responses API did not require the repair tool")
	}
	chatChoice := chatPayload["tool_choice"].(map[string]any)
	function := chatChoice["function"].(map[string]any)
	if chatChoice["type"] != "function" || function["name"] != "inspect" {
		t.Fatal("Chat Completions API did not require the repair tool")
	}
}

func TestChatRefusalIsUsableOutput(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{"id":"response-1","choices":[{"finish_reason":"stop","message":{"role":"assistant","content":null,"refusal":"I cannot do that."}}]}`))
	}))
	defer server.Close()
	provider, err := NewHTTPProvider(HTTPConfig{Provider: "compatible", BaseURL: server.URL, Model: "test", Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	result, err := provider.Complete(context.Background(), Request{Messages: []Message{{Role: "user", Content: "hi"}}})
	if err != nil || result.Content != "I cannot do that." {
		t.Fatalf("unexpected refusal result: result=%+v err=%v", result, err)
	}
}
