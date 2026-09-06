package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestToolReceiptDeadlineReturnsObservationAndRejectsLateResult(t *testing.T) {
	s, principal, run, call, approval := approvalFixture(t)
	s.toolResultTimeout = 10 * time.Millisecond
	ctx := context.Background()
	if _, _, err := s.DecideApproval(ctx, principal, approval.ID, ApprovalDecisionRequest{Decision: "approve"}); err != nil {
		t.Fatal(err)
	}
	if err := s.dispatchToolCall(ctx, run, call); err != nil {
		t.Fatal(err)
	}
	observation, err := s.waitToolResult(ctx, run, call)
	if err != nil || observation.Status != "timeout" || len(observation.Error) == 0 {
		t.Fatalf("receipt timeout aborted the model: %#v %v", observation, err)
	}
	if _, _, err = s.SubmitToolResult(ctx, principal, call.ID, ToolResultRequest{RunID: run.ID, Sequence: 2, Done: true, Status: "success", Result: json.RawMessage(`{"ok":true}`)}); err == nil {
		t.Fatal("late result overwrote unknown execution outcome")
	}
	stored, err := s.latestToolResult(ctx, call.ID)
	if err != nil || !stored.Done || stored.Status != "timeout" {
		t.Fatalf("missing terminal receipt: %#v %v", stored, err)
	}
}
