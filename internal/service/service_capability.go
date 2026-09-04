package service

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jumpserver/kael/internal/domain"
	"github.com/jumpserver/kael/internal/event"
	"github.com/jumpserver/kael/internal/ports"
	agentruntime "github.com/jumpserver/kael/internal/runtime"
)

func (s *Service) callServiceCapability(ctx context.Context, run *domain.Run, registration domain.Registration, arguments json.RawMessage, modelDurationMS int64) (agentruntime.ToolObservation, error) {
	if s.capability == nil {
		return agentruntime.ToolObservation{}, serviceError(Unavailable, "service_capability_unavailable", "service capability provider is unavailable", nil)
	}
	principal := domain.Principal{SubjectID: run.SubjectID, OrganizationID: run.OrganizationID}
	request := ports.CapabilityRequest{Principal: principal, ConversationID: run.ConversationID, RunID: run.ID, Profile: run.Profile, Registration: registration, Arguments: arguments}
	capabilityPolicy, err := s.capability.Prepare(ctx, request)
	if err != nil {
		return agentruntime.ToolObservation{}, serviceError(Forbidden, "service_capability_denied", "service capability request was denied", err)
	}
	now := time.Now().UTC()
	digest := domain.HashBytes(arguments)
	call := &domain.ToolCall{ID: uuid.NewString(), ConversationID: run.ConversationID, RunID: run.ID, PanelSessionID: run.PanelSessionID, BindingKind: "service", ExecutionBindingID: registration.ExecutionBindingID, SubjectID: run.SubjectID, OrganizationID: run.OrganizationID, RegistrationID: registration.ID, DefinitionVersion: registration.DefinitionVersion, DefinitionDigest: registration.DefinitionDigest, ToolName: registration.Name, Arguments: append(json.RawMessage(nil), arguments...), ArgumentsDigest: digest, Risk: capabilityPolicy.Risk, RequiresConfirmation: capabilityPolicy.RequiresConfirmation, InvocationSequence: uint64(now.UnixNano()), InvocationID: uuid.NewString(), State: "created", CreatedAt: now, UpdatedAt: now}
	request.ToolCallID = call.ID
	var approval *domain.Approval
	var notify []string
	err = s.store.Transaction(ctx, func(tx ports.Tx) error {
		current, loadErr := tx.RunInternal(run.ID, true)
		if loadErr != nil {
			return loadErr
		}
		if current.State == "cancelling" || current.State == "cancelled" {
			return context.Canceled
		}
		panel, loadErr := tx.PanelInternal(run.PanelSessionID, true)
		if loadErr != nil {
			return loadErr
		}
		if createErr := tx.CreateToolCall(call); createErr != nil {
			return createErr
		}
		if capabilityPolicy.RequiresConfirmation {
			preview := capabilityPolicy.Preview
			if len(preview) == 0 {
				preview, _ = json.Marshal(map[string]any{"tool_name": registration.Name, "arguments": json.RawMessage(arguments)})
			}
			approval = &domain.Approval{ID: uuid.NewString(), ConversationID: run.ConversationID, RunID: run.ID, ToolCallID: call.ID, RegistrationID: registration.ID, PanelSessionID: run.PanelSessionID, Scope: "service", SubjectID: run.SubjectID, OrganizationID: run.OrganizationID, DefinitionVersion: registration.DefinitionVersion, ArgumentsDigest: digest, Risk: capabilityPolicy.Risk, Preview: preview, PolicyVersion: "1", State: "pending", ExpiresAt: now.Add(10 * time.Minute), CreatedAt: now, UpdatedAt: now}
			call.State, current.State = "waiting_approval", "waiting_approval"
			if createErr := tx.CreateApproval(approval); createErr != nil {
				return createErr
			}
			if saveErr := tx.SaveToolCall(call); saveErr != nil {
				return saveErr
			}
			if saveErr := tx.SaveRun(current); saveErr != nil {
				return saveErr
			}
			payload := map[string]any{"approval_id": approval.ID, "tool_call_id": call.ID, "registration_id": registration.ID, "tool_name": call.ToolName, "arguments": json.RawMessage(arguments), "arguments_digest": digest, "risk": approval.Risk, "preview": json.RawMessage(preview), "scope": "service", "expires_at": approval.ExpiresAt, "model_duration_ms": modelDurationMS}
			_, deliveries, projectErr := event.Project(tx, "approval.required", "approval", approval.ID, "approval", event.References{ConversationID: run.ConversationID, RunID: run.ID, MessageID: current.OutputMessageID, ToolCallID: call.ID, ApprovalID: approval.ID}, payload, []domain.PanelSession{*panel}, now)
			if projectErr != nil {
				return projectErr
			}
			for _, delivery := range deliveries {
				notify = append(notify, delivery.PanelSessionID)
			}
		}
		return s.audit(tx, principal, "tool.created", run.ConversationID, run.PanelSessionID, run.ID, map[string]any{"tool_call_id": call.ID, "registration_id": registration.ID, "binding_kind": "service", "risk": capabilityPolicy.Risk})
	})
	if err != nil {
		return agentruntime.ToolObservation{}, translateOrService(err)
	}
	s.bus.Notify(notify...)
	if approval != nil {
		request.ApprovalID = approval.ID
		if err = s.waitApproval(ctx, run, call, approval); err != nil {
			var serviceErr *Error
			if errors.As(err, &serviceErr) && serviceErr.Code == "approval_rejected" {
				return agentruntime.ToolObservation{ToolCallID: call.ID, Status: "error", Error: json.RawMessage(`{"code":"approval_rejected","message":"The user rejected this operation."}`)}, nil
			}
			return agentruntime.ToolObservation{}, err
		}
	}
	if err = s.startServiceCapability(ctx, run, call, approval); err != nil {
		return agentruntime.ToolObservation{}, err
	}
	result, executeErr := s.capability.Execute(ctx, request)
	if executeErr != nil {
		result.Status = "error"
		result.Error = json.RawMessage(`{"code":"capability_failed","message":"The authorized capability request failed."}`)
	}
	return s.finishServiceCapability(ctx, run, call, result)
}

func (s *Service) startServiceCapability(ctx context.Context, run *domain.Run, call *domain.ToolCall, approval *domain.Approval) error {
	now := time.Now().UTC()
	var notify []string
	err := s.store.Transaction(ctx, func(tx ports.Tx) error {
		current, err := tx.RunInternal(run.ID, true)
		if err != nil {
			return err
		}
		panel, err := tx.PanelInternal(run.PanelSessionID, true)
		if err != nil {
			return err
		}
		stored, err := tx.ToolCall(call.ID, true)
		if err != nil {
			return err
		}
		if approval != nil {
			storedApproval, approvalErr := tx.ApprovalByToolCall(call.ID, true)
			if approvalErr != nil {
				return approvalErr
			}
			if storedApproval.State != "approved" {
				return serviceError(Conflict, "approval_required", "service capability invocation is not approved", nil)
			}
			storedApproval.State, storedApproval.UpdatedAt = "consumed", now
			if approvalErr = tx.SaveApproval(storedApproval); approvalErr != nil {
				return approvalErr
			}
		}
		stored.State, stored.UpdatedAt, current.State, current.UpdatedAt = "running", now, "running", now
		if err = tx.SaveToolCall(stored); err != nil {
			return err
		}
		if err = tx.SaveRun(current); err != nil {
			return err
		}
		_, deliveries, err := event.Project(tx, "tool.call", "tool_call", stored.ID, "tool", event.References{ConversationID: run.ConversationID, RunID: run.ID, MessageID: current.OutputMessageID, ToolCallID: stored.ID}, map[string]any{"tool_call_id": stored.ID, "registration_id": stored.RegistrationID, "tool_name": stored.ToolName, "arguments": json.RawMessage(stored.Arguments), "risk": stored.Risk, "binding_kind": "service"}, []domain.PanelSession{*panel}, now)
		if err != nil {
			return err
		}
		for _, delivery := range deliveries {
			notify = append(notify, delivery.PanelSessionID)
		}
		return nil
	})
	if err != nil {
		return translateOrService(err)
	}
	s.bus.Notify(notify...)
	return nil
}

func (s *Service) finishServiceCapability(ctx context.Context, run *domain.Run, call *domain.ToolCall, result ports.CapabilityResult) (agentruntime.ToolObservation, error) {
	if result.Status == "" {
		result.Status = "success"
	}
	if len(result.Result) > domain.MaxToolResultBytes || len(result.Error) > 16*1024 {
		return agentruntime.ToolObservation{}, serviceError(Invalid, "capability_result_too_large", "service capability result exceeds the configured limit", nil)
	}
	now := time.Now().UTC()
	var notify []string
	err := s.store.Transaction(ctx, func(tx ports.Tx) error {
		current, err := tx.RunInternal(run.ID, true)
		if err != nil {
			return err
		}
		panel, err := tx.PanelInternal(run.PanelSessionID, true)
		if err != nil {
			return err
		}
		stored, err := tx.ToolCall(call.ID, true)
		if err != nil {
			return err
		}
		stored.State, stored.UpdatedAt, stored.FinishedAt = "succeeded", now, &now
		eventType := "tool.completed"
		if result.Status != "success" {
			stored.State, eventType = "failed", "tool.failed"
		}
		if err = tx.SaveToolCall(stored); err != nil {
			return err
		}
		current.State, current.UpdatedAt = "running", now
		if len(result.ResultCards) > 0 {
			if len(result.ResultCards) > domain.MaxEventPayloadBytes || !json.Valid(result.ResultCards) {
				return serviceError(Invalid, "capability_result_cards_invalid", "service capability result cards are invalid", nil)
			}
			message, messageErr := tx.Message(current.OutputMessageID, domain.Principal{SubjectID: run.SubjectID, OrganizationID: run.OrganizationID}, true)
			if messageErr != nil {
				return messageErr
			}
			var existing, incoming []any
			if len(message.ResultCards) > 0 {
				_ = json.Unmarshal(message.ResultCards, &existing)
			}
			if json.Unmarshal(result.ResultCards, &incoming) != nil {
				return serviceError(Invalid, "capability_result_cards_invalid", "service capability result cards are invalid", nil)
			}
			existing = append(existing, incoming...)
			if len(existing) > 20 {
				existing = existing[len(existing)-20:]
			}
			message.ResultCards, _ = json.Marshal(existing)
			message.UpdatedAt = now
			if err = tx.SaveMessage(message); err != nil {
				return err
			}
		}
		if err = tx.SaveRun(current); err != nil {
			return err
		}
		payloadDigest, _ := domain.HashValue(result)
		toolResult := &domain.ToolResult{ID: uuid.NewString(), ToolCallID: call.ID, RunID: run.ID, PanelSessionID: run.PanelSessionID, Sequence: 1, Done: true, Status: result.Status, Result: result.Result, ErrorJSON: result.Error, PayloadDigest: payloadDigest, ExecutorAuditReference: bounded(result.ExecutorAuditReference, 512), CreatedAt: now}
		if err = tx.CreateToolResult(toolResult); err != nil {
			return err
		}
		payload := map[string]any{"tool_call_id": call.ID, "registration_id": call.RegistrationID, "tool_name": call.ToolName, "seq": 1, "done": true, "status": result.Status, "executor_audit_reference": result.ExecutorAuditReference, "binding_kind": "service"}
		if len(result.Result) > 0 {
			payload["result"] = json.RawMessage(result.Result)
		}
		if len(result.Error) > 0 {
			payload["error"] = json.RawMessage(result.Error)
		}
		if len(result.ResultCards) > 0 {
			payload["result_cards"] = json.RawMessage(result.ResultCards)
		}
		_, deliveries, err := event.Project(tx, eventType, "tool_call", call.ID, "tool", event.References{ConversationID: run.ConversationID, RunID: run.ID, MessageID: current.OutputMessageID, ToolCallID: call.ID}, payload, []domain.PanelSession{*panel}, now)
		if err != nil {
			return err
		}
		for _, delivery := range deliveries {
			notify = append(notify, delivery.PanelSessionID)
		}
		return s.audit(tx, domain.Principal{SubjectID: run.SubjectID, OrganizationID: run.OrganizationID}, "tool.result", run.ConversationID, run.PanelSessionID, run.ID, map[string]any{"tool_call_id": call.ID, "status": result.Status, "executor_audit_reference": result.ExecutorAuditReference})
	})
	if err != nil {
		return agentruntime.ToolObservation{}, translateOrService(err)
	}
	s.bus.Notify(notify...)
	return agentruntime.ToolObservation{ToolCallID: call.ID, Status: result.Status, Result: result.Result, Error: result.Error}, nil
}
