package runtime

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/jumpserver/kael/internal/domain"
	"github.com/jumpserver/kael/internal/model"
)

type sequenceProvider struct {
	results  []model.Result
	index    int
	requests []model.Request
}

func (p *sequenceProvider) Info() model.Info { return model.Info{} }
func (p *sequenceProvider) Complete(_ context.Context, request model.Request) (model.Result, error) {
	p.requests = append(p.requests, request)
	result := p.results[p.index]
	p.index++
	return result, nil
}

func TestLoopRepairsArgumentsOnceAndCompletes(t *testing.T) {
	provider := &sequenceProvider{results: []model.Result{
		{ToolCall: &model.ToolCall{ID: "call-1", Name: "inspect", Arguments: json.RawMessage(`{"limit":"bad"}`)}},
		{ToolCall: &model.ToolCall{ID: "call-1", Name: "inspect", Arguments: json.RawMessage(`{"limit":1}`)}},
		{Content: "done", FinishReason: "stop"},
	}}
	annotations, _ := json.Marshal(domain.ToolAnnotations{ReadOnly: true})
	registration := domain.Registration{ID: "registration-1", Name: "inspect", InputSchema: json.RawMessage(`{"type":"object","properties":{"limit":{"type":"integer"}},"required":["limit"],"additionalProperties":false}`), AnnotationsJSON: annotations}
	modelCalls, toolCalls := 0, 0
	result, err := New(provider).Execute(context.Background(), Input{Messages: []domain.Message{{Role: "user", Content: "inspect"}}, Registrations: []domain.Registration{registration}, MaxRounds: 3, MaxModelRequests: 4}, Callbacks{
		ModelStarted:   func(int) error { modelCalls++; return nil },
		ModelCompleted: func(int, model.Result, time.Duration) error { return nil },
		MessageDelta:   func(string) error { return nil },
		CallTool: func(context.Context, domain.Registration, json.RawMessage, int64) (ToolObservation, error) {
			toolCalls++
			return ToolObservation{Status: "success", Result: json.RawMessage(`{"ok":true}`)}, nil
		},
	})
	if err != nil || result.Answer != "done" || modelCalls != 3 || toolCalls != 1 {
		t.Fatalf("unexpected loop result: result=%+v model_calls=%d tool_calls=%d err=%v", result, modelCalls, toolCalls, err)
	}
}

func TestLoopRecoversWhenArgumentRepairDoesNotCallTool(t *testing.T) {
	provider := &sequenceProvider{results: []model.Result{
		{ToolCall: &model.ToolCall{ID: "call-1", Name: "inspect", Arguments: json.RawMessage(`{"limit":"bad"}`)}},
		{Content: "repair explanation"},
		{ToolCall: &model.ToolCall{ID: "call-2", Name: "inspect", Arguments: json.RawMessage(`{"limit":1}`)}},
		{Content: "done", FinishReason: "stop"},
	}}
	registration := domain.Registration{ID: "registration-1", Name: "inspect", InputSchema: json.RawMessage(`{"type":"object","properties":{"limit":{"type":"integer"}},"required":["limit"],"additionalProperties":false}`)}
	toolCalls := 0
	result, err := New(provider).Execute(context.Background(), Input{Messages: []domain.Message{{Role: "user", Content: "inspect"}}, Registrations: []domain.Registration{registration}, MaxRounds: 3, MaxModelRequests: 5}, Callbacks{
		ModelStarted:   func(int) error { return nil },
		ModelCompleted: func(int, model.Result, time.Duration) error { return nil },
		MessageDelta:   func(string) error { return nil },
		CallTool: func(context.Context, domain.Registration, json.RawMessage, int64) (ToolObservation, error) {
			toolCalls++
			return ToolObservation{Status: "success", Result: json.RawMessage(`{"ok":true}`)}, nil
		},
	})
	if err != nil || result.Answer != "done" || toolCalls != 1 {
		t.Fatalf("unexpected recovery result: result=%+v tool_calls=%d err=%v", result, toolCalls, err)
	}
}

func TestLoopReturnsSafeAnswerWhenArgumentRepairExhaustsBudget(t *testing.T) {
	provider := &sequenceProvider{results: []model.Result{
		{ToolCall: &model.ToolCall{ID: "call-1", Name: "inspect", Arguments: json.RawMessage(`{"limit":"bad"}`)}},
		{Content: "repair explanation"},
	}}
	registration := domain.Registration{ID: "registration-1", Name: "inspect", InputSchema: json.RawMessage(`{"type":"object","properties":{"limit":{"type":"integer"}},"required":["limit"],"additionalProperties":false}`)}
	deltas := ""
	result, err := New(provider).Execute(context.Background(), Input{Messages: []domain.Message{{Role: "user", Content: "inspect"}}, Registrations: []domain.Registration{registration}, MaxRounds: 3, MaxModelRequests: 2}, Callbacks{
		ModelStarted:   func(int) error { return nil },
		ModelCompleted: func(int, model.Result, time.Duration) error { return nil },
		MessageDelta:   func(delta string) error { deltas += delta; return nil },
		CallTool: func(context.Context, domain.Registration, json.RawMessage, int64) (ToolObservation, error) {
			t.Fatal("invalid arguments must not execute the tool")
			return ToolObservation{}, nil
		},
	})
	if err != nil || result.Answer != invalidArgumentsFallback || deltas != invalidArgumentsFallback {
		t.Fatalf("unexpected budget fallback: result=%+v deltas=%q err=%v", result, deltas, err)
	}
}

func TestLoopSummarizesFinalResultAfterArgumentRepair(t *testing.T) {
	provider := &sequenceProvider{results: []model.Result{
		{ToolCall: &model.ToolCall{ID: "call-1", Name: "propose", Arguments: json.RawMessage(`{"value":1}`)}},
		{Content: "repair chatter", DeltasEmitted: true, ToolCall: &model.ToolCall{ID: "call-2", Name: "propose", Arguments: json.RawMessage(`{"value":"ready"}`)}},
		{Content: "The proposal updates the script and was applied to the editor, but it was not saved or executed.", FinishReason: "stop"},
	}}
	annotations, _ := json.Marshal(domain.ToolAnnotations{FinalResult: true})
	registration := domain.Registration{ID: "registration-1", Name: "propose", InputSchema: json.RawMessage(`{"type":"object","properties":{"value":{"type":"string"}},"required":["value"],"additionalProperties":false}`), AnnotationsJSON: annotations}
	deltas := ""
	result, err := New(provider).Execute(context.Background(), Input{Messages: []domain.Message{{Role: "user", Content: "propose"}}, Registrations: []domain.Registration{registration}, MaxRounds: 2, MaxModelRequests: 3}, Callbacks{
		ModelStarted:   func(int) error { return nil },
		ModelCompleted: func(int, model.Result, time.Duration) error { return nil },
		MessageDelta:   func(delta string) error { deltas += delta; return nil },
		CallTool: func(context.Context, domain.Registration, json.RawMessage, int64) (ToolObservation, error) {
			return ToolObservation{Status: "success", Result: json.RawMessage(`{"content":[{"type":"text","text":"The user applied the proposal to the editor. It has not been saved or executed."}],"structuredContent":{"status":"applied"}}`)}, nil
		},
	})
	if err != nil || result.Answer != "The proposal updates the script and was applied to the editor, but it was not saved or executed." || deltas != result.Answer {
		t.Fatalf("unexpected final repair result: result=%+v deltas=%q err=%v", result, deltas, err)
	}
	if len(provider.requests) != 3 || !provider.requests[2].DisableTools || len(provider.requests[2].Tools) != 0 || provider.requests[2].PreviousResponseID != "" {
		t.Fatalf("final result summary request = %#v", provider.requests)
	}
}
