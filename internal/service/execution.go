package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jumpserver/kael/internal/domain"
	"github.com/jumpserver/kael/internal/event"
	"github.com/jumpserver/kael/internal/model"
	"github.com/jumpserver/kael/internal/ports"
	agentruntime "github.com/jumpserver/kael/internal/runtime"
	"go.uber.org/zap"
)

func (s *Service) modelStarted(ctx context.Context, run *domain.Run, sequence int) error {
	now := time.Now().UTC()
	var notify []string
	err := s.store.Transaction(ctx, func(tx ports.Tx) error {
		current, err := tx.RunInternal(run.ID, true)
		if err != nil {
			return err
		}
		if current.State == "cancelling" || current.State == "cancelled" {
			return context.Canceled
		}
		panel, err := tx.PanelInternal(current.PanelSessionID, true)
		if err != nil {
			return err
		}
		call := &domain.ModelCall{ID: uuid.NewString(), RunID: run.ID, Sequence: sequence, Provider: s.provider.Info().Provider, Model: s.provider.Info().Model, State: "running", CreatedAt: now}
		if err = tx.CreateModelCall(call); err != nil {
			return err
		}
		current.State, current.ModelRequestCount, current.UpdatedAt = "running", sequence, now
		if err = tx.SaveRun(current); err != nil {
			return err
		}
		_, deliveries, err := event.Project(tx, "model.requested", "model_call", call.ID, "run", event.References{ConversationID: current.ConversationID, RunID: current.ID, MessageID: current.OutputMessageID}, map[string]any{"sequence": sequence, "provider": call.Provider, "model": call.Model}, []domain.PanelSession{*panel}, now)
		if err != nil {
			return err
		}
		for _, delivery := range deliveries {
			notify = append(notify, delivery.PanelSessionID)
		}
		return nil
	})
	if err == nil {
		s.bus.Notify(notify...)
	}
	return err
}

func (s *Service) modelCompleted(ctx context.Context, run *domain.Run, sequence int, result model.Result, duration time.Duration) error {
	now := time.Now().UTC()
	var notify []string
	err := s.store.Transaction(ctx, func(tx ports.Tx) error {
		current, err := tx.RunInternal(run.ID, true)
		if err != nil {
			return err
		}
		call, err := tx.ModelCall(run.ID, sequence, true)
		if err != nil {
			return err
		}
		panel, err := tx.PanelInternal(current.PanelSessionID, true)
		if err != nil {
			return err
		}
		call.State, call.RequestID, call.InputTokens, call.OutputTokens, call.ReasoningTokens, call.DurationMS, call.FinishedAt = "completed", result.RequestID, result.Usage.InputTokens, result.Usage.OutputTokens, result.Usage.ReasoningTokens, duration.Milliseconds(), &now
		if err = tx.SaveModelCall(call); err != nil {
			return err
		}
		current.InputTokens += result.Usage.InputTokens
		current.OutputTokens += result.Usage.OutputTokens
		current.UpdatedAt = now
		if err = tx.SaveRun(current); err != nil {
			return err
		}
		_, deliveries, err := event.Project(tx, "model.completed", "model_call", call.ID, "run", event.References{ConversationID: current.ConversationID, RunID: current.ID, MessageID: current.OutputMessageID}, map[string]any{"sequence": sequence, "provider": call.Provider, "model": call.Model, "duration_ms": call.DurationMS, "usage": result.Usage, "finish_reason": result.FinishReason, "request_id": result.RequestID}, []domain.PanelSession{*panel}, now)
		if err != nil {
			return err
		}
		for _, delivery := range deliveries {
			notify = append(notify, delivery.PanelSessionID)
		}
		return nil
	})
	if err == nil {
		s.bus.Notify(notify...)
	}
	return err
}

func (s *Service) messageDelta(ctx context.Context, run *domain.Run, output *domain.Message, text string) error {
	if text == "" {
		return nil
	}
	now := time.Now().UTC()
	var notify []string
	err := s.store.Transaction(ctx, func(tx ports.Tx) error {
		current, err := tx.RunInternal(run.ID, true)
		if err != nil {
			return err
		}
		if current.State == "cancelling" || current.State == "cancelled" {
			return context.Canceled
		}
		message, err := tx.Message(output.ID, domain.Principal{SubjectID: run.SubjectID, OrganizationID: run.OrganizationID}, true)
		if err != nil {
			return err
		}
		if len(message.Content)+len(text) > domain.MaxMessageBytes {
			return fmt.Errorf("assistant message exceeds configured limit")
		}
		message.Content += text
		parts, _ := json.Marshal([]domain.MessagePart{{Type: "text", Text: message.Content}})
		message.Parts, message.UpdatedAt = parts, now
		if err = tx.SaveMessage(message); err != nil {
			return err
		}
		panel, err := tx.PanelInternal(run.PanelSessionID, true)
		if err != nil {
			return err
		}
		_, deliveries, err := event.Project(tx, "message.delta", "message", message.ID, "run", event.References{ConversationID: run.ConversationID, RunID: run.ID, MessageID: message.ID}, map[string]any{"delta": text, "role": "assistant"}, []domain.PanelSession{*panel}, now)
		if err != nil {
			return err
		}
		for _, delivery := range deliveries {
			notify = append(notify, delivery.PanelSessionID)
		}
		return nil
	})
	if err == nil {
		s.bus.Notify(notify...)
	}
	return err
}

func (s *Service) callTool(ctx context.Context, run *domain.Run, snapshot domain.Registration, arguments json.RawMessage, modelDurationMS int64) (agentruntime.ToolObservation, error) {
	if snapshot.BindingKind == "service" {
		return s.callServiceCapability(ctx, run, snapshot, arguments, modelDurationMS)
	}
	call, approval, err := s.prepareToolCall(ctx, run, snapshot, arguments, modelDurationMS)
	if err != nil {
		return agentruntime.ToolObservation{}, err
	}
	if approval != nil {
		if err = s.waitApproval(ctx, run, call, approval); err != nil {
			var serviceErr *Error
			if errors.As(err, &serviceErr) && serviceErr.Code == "approval_rejected" {
				return agentruntime.ToolObservation{ToolCallID: call.ID, Status: "error", Error: json.RawMessage(`{"code":"approval_rejected","message":"The user rejected this operation."}`)}, nil
			}
			return agentruntime.ToolObservation{}, err
		}
	}
	if err = s.dispatchToolCall(ctx, run, call); err != nil {
		return agentruntime.ToolObservation{}, err
	}
	return s.waitToolResult(ctx, run, call)
}

func (s *Service) prepareToolCall(ctx context.Context, run *domain.Run, snapshot domain.Registration, arguments json.RawMessage, modelDurationMS int64) (*domain.ToolCall, *domain.Approval, error) {
	now := time.Now().UTC()
	var call *domain.ToolCall
	var approval *domain.Approval
	var notify []string
	err := s.store.Transaction(ctx, func(tx ports.Tx) error {
		current, err := tx.RunInternal(run.ID, true)
		if err != nil {
			return err
		}
		if current.State == "cancelling" || current.State == "cancelled" {
			return context.Canceled
		}
		panel, err := tx.PanelInternal(run.PanelSessionID, true)
		if err != nil {
			return err
		}
		if validatePanelBinding(panel, run.ConversationID, now) != nil {
			current.State, current.ErrorCode, current.ErrorDetail, current.UpdatedAt = "waiting_capability", "provider_disconnected", "original panel capability is unavailable", now
			_ = tx.SaveRun(current)
			return serviceError(Unavailable, "provider_disconnected", "original panel capability is unavailable", nil)
		}
		registration, err := tx.Registration(snapshot.ID, true)
		if err != nil {
			return err
		}
		if registration.PanelSessionID != run.PanelSessionID || registration.RegistryRevision != run.RegistryRevision || registration.DefinitionDigest != snapshot.DefinitionDigest || registration.State != "active" || registration.LeaseExpiresAt.Before(now) {
			return serviceError(Conflict, "capability_revoked", "run capability snapshot is no longer executable", nil)
		}
		digest := domain.HashBytes(arguments)
		requiresConfirmation := registration.RequiresConfirmation
		if panel.ApprovalMode == "always" {
			requiresConfirmation = true
		} else if panel.ApprovalMode == "never" {
			requiresConfirmation = false
		}
		call = &domain.ToolCall{ID: uuid.NewString(), ConversationID: run.ConversationID, RunID: run.ID, PanelSessionID: run.PanelSessionID, SubjectID: run.SubjectID, OrganizationID: run.OrganizationID, RegistrationID: registration.ID, DefinitionVersion: registration.DefinitionVersion, DefinitionDigest: registration.DefinitionDigest, ToolName: registration.Name, Arguments: append(json.RawMessage(nil), arguments...), ArgumentsDigest: digest, Risk: registration.Risk, RequiresConfirmation: requiresConfirmation, InvocationSequence: uint64(now.UnixNano()), InvocationID: uuid.NewString(), State: "created", CreatedAt: now, UpdatedAt: now}
		if err = tx.CreateToolCall(call); err != nil {
			return err
		}
		if requiresConfirmation {
			preview, _ := json.Marshal(map[string]any{"tool_name": registration.Name, "arguments": json.RawMessage(arguments), "description": registration.Description})
			approval = &domain.Approval{ID: uuid.NewString(), ConversationID: run.ConversationID, RunID: run.ID, ToolCallID: call.ID, RegistrationID: registration.ID, PanelSessionID: run.PanelSessionID, Scope: "panel", SubjectID: run.SubjectID, OrganizationID: run.OrganizationID, DefinitionVersion: registration.DefinitionVersion, ArgumentsDigest: digest, Risk: registration.Risk, Preview: preview, PolicyVersion: "1", State: "pending", ExpiresAt: minTime(panel.LeaseExpiresAt, now.Add(10*time.Minute)), CreatedAt: now, UpdatedAt: now}
			call.State, current.State = "waiting_approval", "waiting_approval"
			if err = tx.CreateApproval(approval); err != nil {
				return err
			}
			if err = tx.SaveToolCall(call); err != nil {
				return err
			}
			if err = tx.SaveRun(current); err != nil {
				return err
			}
			payload := map[string]any{"approval_id": approval.ID, "tool_call_id": call.ID, "registration_id": registration.ID, "tool_name": call.ToolName, "arguments": json.RawMessage(arguments), "arguments_digest": digest, "risk": approval.Risk, "preview": json.RawMessage(preview), "expires_at": approval.ExpiresAt, "model_duration_ms": modelDurationMS}
			_, deliveries, err := event.Project(tx, "approval.required", "approval", approval.ID, "approval", event.References{ConversationID: run.ConversationID, RunID: run.ID, MessageID: current.OutputMessageID, ToolCallID: call.ID, ApprovalID: approval.ID}, payload, []domain.PanelSession{*panel}, now)
			if err != nil {
				return err
			}
			for _, delivery := range deliveries {
				notify = append(notify, delivery.PanelSessionID)
			}
		}
		return s.audit(tx, domain.Principal{SubjectID: run.SubjectID, OrganizationID: run.OrganizationID}, "tool.created", run.ConversationID, run.PanelSessionID, run.ID, map[string]any{"tool_call_id": call.ID, "registration_id": registration.ID, "risk": registration.Risk})
	})
	if err != nil {
		return nil, nil, translateOrService(err)
	}
	s.bus.Notify(notify...)
	return call, approval, nil
}

func minTime(left, right time.Time) time.Time {
	if left.Before(right) {
		return left
	}
	return right
}

func (s *Service) waitApproval(ctx context.Context, run *domain.Run, call *domain.ToolCall, approval *domain.Approval) error {
	notifications, unsubscribe := s.bus.Subscribe(run.PanelSessionID)
	defer unsubscribe()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		state, err := s.approvalState(ctx, approval.ID)
		if err != nil {
			return err
		}
		switch state {
		case "approved":
			return nil
		case "rejected":
			return serviceError(Conflict, "approval_rejected", "user rejected the capability invocation", nil)
		case "cancelled", "expired":
			return context.Canceled
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-notifications:
		case <-ticker.C:
		}
	}
}

func (s *Service) approvalState(ctx context.Context, id string) (string, error) {
	state := ""
	err := s.store.Transaction(ctx, func(tx ports.Tx) error {
		value, err := tx.ApprovalInternal(id, true)
		if err != nil {
			return err
		}
		state = value.State
		if state == "pending" && value.ExpiresAt.Before(time.Now().UTC()) {
			value.State, value.Reason, value.UpdatedAt = "expired", "approval expired", time.Now().UTC()
			state = value.State
			return tx.SaveApproval(value)
		}
		return nil
	})
	return state, err
}

func (s *Service) dispatchToolCall(ctx context.Context, run *domain.Run, call *domain.ToolCall) error {
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
		registration, err := tx.Registration(call.RegistrationID, true)
		if err != nil {
			return err
		}
		if validatePanelBinding(panel, run.ConversationID, now) != nil || registration.State != "active" || registration.LeaseExpiresAt.Before(now) || registration.DefinitionDigest != call.DefinitionDigest {
			return serviceError(Conflict, "capability_expired", "capability expired before invocation", nil)
		}
		if call.RequiresConfirmation {
			approval, err := tx.ApprovalByToolCall(call.ID, true)
			if err != nil {
				return err
			}
			if approval.State != "approved" {
				return serviceError(Conflict, "approval_required", "capability invocation is not approved", nil)
			}
			approval.State, approval.UpdatedAt = "consumed", now
			if err = tx.SaveApproval(approval); err != nil {
				return err
			}
		}
		call.State, call.UpdatedAt, current.State, current.UpdatedAt = "dispatched", now, "waiting_capability", now
		if err = tx.SaveToolCall(call); err != nil {
			return err
		}
		if err = tx.SaveRun(current); err != nil {
			return err
		}
		payload := map[string]any{"tool_call_id": call.ID, "invocation_id": call.InvocationID, "registration_id": call.RegistrationID, "definition_version": call.DefinitionVersion, "definition_digest": call.DefinitionDigest, "tool_name": call.ToolName, "arguments": json.RawMessage(call.Arguments), "risk": call.Risk, "revision": run.RegistryRevision}
		_, deliveries, err := event.Project(tx, "tool.call", "tool_call", call.ID, "tool", event.References{ConversationID: run.ConversationID, RunID: run.ID, MessageID: current.OutputMessageID, ToolCallID: call.ID}, payload, []domain.PanelSession{*panel}, now)
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

func (s *Service) waitToolResult(ctx context.Context, run *domain.Run, call *domain.ToolCall) (agentruntime.ToolObservation, error) {
	notifications, unsubscribe := s.bus.Subscribe(run.PanelSessionID)
	defer unsubscribe()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		result, err := s.latestToolResult(ctx, call.ID)
		if err == nil && result.Done {
			return agentruntime.ToolObservation{ToolCallID: call.ID, Status: result.Status, Result: result.Result, Error: result.ErrorJSON}, nil
		}
		if err != nil && !errors.Is(err, ports.ErrNotFound) {
			return agentruntime.ToolObservation{}, err
		}
		select {
		case <-ctx.Done():
			return agentruntime.ToolObservation{}, ctx.Err()
		case <-notifications:
		case <-ticker.C:
		}
	}
}

func (s *Service) latestToolResult(ctx context.Context, id string) (*domain.ToolResult, error) {
	var result *domain.ToolResult
	err := s.store.View(ctx, func(tx ports.Tx) error { var err error; result, err = tx.LatestToolResult(id); return err })
	return result, err
}

func (s *Service) finishCompleted(run *domain.Run, output *domain.Message, completion agentruntime.Completion) {
	s.finishRun(run, output, "completed", "", "", completion.Partial, completion.FinishReason, completion.Usage)
}
func (s *Service) finishFailed(run *domain.Run, output *domain.Message, failure error) {
	internalDetail := bounded(sanitizeAuditText(failure.Error()), 1024)
	s.logger.Error("run failed", zap.String("run_id", run.ID), zap.String("error", internalDetail))
	s.finishRun(run, output, "failed", "run_failed", "The AI request could not be completed. Please try again.", false, "error", model.Usage{})
}
func (s *Service) finishCancelled(run *domain.Run, output *domain.Message, failure error) {
	detail := "run cancelled"
	if errors.Is(failure, context.DeadlineExceeded) {
		detail = "run timed out"
	}
	s.finishRun(run, output, "cancelled", "cancelled", detail, true, "cancelled", model.Usage{})
}

func (s *Service) finishRun(run *domain.Run, output *domain.Message, state, code, detail string, partial bool, finishReason string, usage model.Usage) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	now := time.Now().UTC()
	var notify []string
	err := s.store.Transaction(ctx, func(tx ports.Tx) error {
		current, err := tx.RunInternal(run.ID, true)
		if err != nil {
			return err
		}
		if current.Terminal() {
			return nil
		}
		messageID := current.OutputMessageID
		if messageID == "" && output != nil {
			messageID = output.ID
		}
		var message *domain.Message
		if messageID != "" {
			message, err = tx.Message(messageID, domain.Principal{SubjectID: run.SubjectID, OrganizationID: run.OrganizationID}, true)
			if err != nil && !errors.Is(err, ports.ErrNotFound) {
				return err
			}
		}
		if message != nil {
			if state != "completed" && strings.TrimSpace(message.Content) != "" {
				partial = true
			}
			switch state {
			case "completed":
				message.Status = "completed"
			case "cancelled":
				message.Status = "cancelled"
			default:
				message.Status, message.ErrorCode, message.ErrorDetail = "failed", code, detail
			}
			message.InputTokens, message.OutputTokens, message.UpdatedAt = usage.InputTokens, usage.OutputTokens, now
			if err = tx.SaveMessage(message); err != nil {
				return err
			}
		}
		current.State, current.ErrorCode, current.ErrorDetail = state, code, detail
		current.Partial, current.FinishReason = partial, finishReason
		current.ClaimOwner, current.ClaimExpiresAt = "", nil
		current.UpdatedAt, current.FinishedAt = now, &now
		if err = tx.SaveRun(current); err != nil {
			return err
		}
		panels, err := tx.ListConversationPanels(run.ConversationID, run.OrganizationID)
		if err != nil {
			return err
		}
		executionPanels := make([]domain.PanelSession, 0, 1)
		for _, panel := range panels {
			if panel.ID == run.PanelSessionID {
				executionPanels = append(executionPanels, panel)
				break
			}
		}
		if len(executionPanels) == 0 {
			panel, panelErr := tx.PanelInternal(run.PanelSessionID, true)
			if panelErr == nil {
				executionPanels = append(executionPanels, *panel)
			}
		}
		if message != nil {
			_, deliveries, eventErr := event.Project(tx, "message.completed", "message", message.ID, "conversation", event.References{ConversationID: run.ConversationID, RunID: run.ID, MessageID: message.ID}, message, panels, now)
			if eventErr != nil {
				return eventErr
			}
			for _, delivery := range deliveries {
				notify = append(notify, delivery.PanelSessionID)
			}
		}
		_, deliveries, err := event.Project(tx, "run."+state, "run", run.ID, "run", event.References{ConversationID: run.ConversationID, RunID: run.ID, MessageID: messageID}, runEventPayload(current), executionPanels, now)
		if err != nil {
			return err
		}
		for _, delivery := range deliveries {
			notify = append(notify, delivery.PanelSessionID)
		}
		return s.audit(tx, domain.Principal{SubjectID: run.SubjectID, OrganizationID: run.OrganizationID}, "run."+state, run.ConversationID, run.PanelSessionID, run.ID, map[string]any{"finish_reason": finishReason, "partial": partial})
	})
	if err != nil {
		s.logger.Error("persist run terminal state", zap.String("run_id", run.ID), zap.Error(err))
		return
	}
	s.bus.Notify(notify...)
}

type ToolResultRequest struct {
	RunID                  string          `json:"run_id"`
	PanelSessionID         string          `json:"panel_session_id"`
	Sequence               uint64          `json:"seq"`
	Done                   bool            `json:"done"`
	Status                 string          `json:"status"`
	Result                 json.RawMessage `json:"result"`
	Error                  json.RawMessage `json:"error"`
	ExecutorAuditReference string          `json:"executor_audit_reference"`
}

func (s *Service) SubmitToolResult(ctx context.Context, principal domain.Principal, id string, request ToolResultRequest) (*domain.ToolResult, bool, error) {
	if request.Sequence < 1 || request.Status != "running" && request.Status != "success" && request.Status != "error" && request.Status != "cancelled" && request.Status != "timeout" || len(request.Result) > domain.MaxToolResultBytes || len(request.Error) > 16*1024 {
		return nil, false, serviceError(Invalid, "invalid_tool_result", "tool result is invalid", nil)
	}
	digest, _ := domain.HashValue(request)
	now := time.Now().UTC()
	var result *domain.ToolResult
	duplicate := false
	var notify []string
	err := s.store.Transaction(ctx, func(tx ports.Tx) error {
		call, err := tx.ToolCall(id, true)
		if err != nil {
			return err
		}
		run, err := tx.Run(call.RunID, principal, true)
		if err != nil {
			return err
		}
		if request.RunID != "" && request.RunID != run.ID || request.PanelSessionID != "" && request.PanelSessionID != call.PanelSessionID {
			return serviceError(Forbidden, "tool_result_binding_mismatch", "tool result binding is invalid", nil)
		}
		panel, err := tx.Panel(call.PanelSessionID, principal, true)
		if err != nil {
			return err
		}
		latest, latestErr := tx.LatestToolResult(id)
		if latestErr == nil {
			if request.Sequence < latest.Sequence {
				return serviceError(Conflict, "tool_result_sequence_conflict", "tool result sequence did not advance", nil)
			}
			if request.Sequence == latest.Sequence {
				if latest.PayloadDigest != digest {
					return serviceError(Conflict, "tool_result_conflict", "tool result sequence was reused with another payload", nil)
				}
				result, duplicate = latest, true
				return nil
			}
			if latest.Done {
				return serviceError(Conflict, "tool_result_terminal", "tool call already has a terminal result", nil)
			}
		} else if !errors.Is(latestErr, ports.ErrNotFound) {
			return latestErr
		}
		result = &domain.ToolResult{ID: uuid.NewString(), ToolCallID: id, RunID: run.ID, PanelSessionID: call.PanelSessionID, Sequence: request.Sequence, Done: request.Done, Status: request.Status, Result: append(json.RawMessage(nil), request.Result...), ErrorJSON: append(json.RawMessage(nil), request.Error...), PayloadDigest: digest, ExecutorAuditReference: bounded(request.ExecutorAuditReference, 512), CreatedAt: now}
		if err = tx.CreateToolResult(result); err != nil {
			return err
		}
		eventType := "tool.progress"
		if request.Done {
			switch request.Status {
			case "success":
				eventType, call.State = "tool.completed", "succeeded"
			case "cancelled":
				eventType, call.State = "tool.cancelled", "cancelled"
			default:
				eventType, call.State = "tool.failed", "failed"
			}
			call.FinishedAt = &now
		}
		call.UpdatedAt = now
		if err = tx.SaveToolCall(call); err != nil {
			return err
		}
		payload := map[string]any{"tool_call_id": call.ID, "registration_id": call.RegistrationID, "tool_name": call.ToolName, "seq": result.Sequence, "done": result.Done, "status": result.Status, "executor_audit_reference": result.ExecutorAuditReference}
		if len(result.Result) > 0 {
			payload["result"] = json.RawMessage(result.Result)
		}
		if len(result.ErrorJSON) > 0 {
			payload["error"] = json.RawMessage(result.ErrorJSON)
		}
		_, deliveries, err := event.Project(tx, eventType, "tool_call", call.ID, "tool", event.References{ConversationID: run.ConversationID, RunID: run.ID, MessageID: run.OutputMessageID, ToolCallID: call.ID}, payload, []domain.PanelSession{*panel}, now)
		if err != nil {
			return err
		}
		for _, delivery := range deliveries {
			notify = append(notify, delivery.PanelSessionID)
		}
		return s.audit(tx, principal, "tool.result", run.ConversationID, panel.ID, run.ID, map[string]any{"tool_call_id": call.ID, "status": result.Status, "executor_audit_reference": result.ExecutorAuditReference})
	})
	if err != nil {
		return nil, false, translateOrService(err)
	}
	if !duplicate {
		s.bus.Notify(notify...)
	}
	return result, duplicate, nil
}

type ApprovalDecisionRequest struct {
	Decision        string `json:"decision"`
	RunID           string `json:"run_id"`
	ArgumentsDigest string `json:"arguments_digest"`
}

func (s *Service) DecideApproval(ctx context.Context, principal domain.Principal, id string, request ApprovalDecisionRequest) (*domain.Approval, bool, error) {
	if request.Decision != "approve" && request.Decision != "reject" {
		return nil, false, serviceError(Invalid, "invalid_decision", "approval decision is invalid", nil)
	}
	digest, _ := domain.HashValue(request)
	now := time.Now().UTC()
	var approval *domain.Approval
	duplicate := false
	var notify []string
	err := s.store.Transaction(ctx, func(tx ports.Tx) error {
		var err error
		approval, err = tx.Approval(id, principal, true)
		if err != nil {
			return err
		}
		if approval.State != "pending" {
			expected := "approved"
			if request.Decision == "reject" {
				expected = "rejected"
			}
			if approval.State == expected && approval.DecisionDigest == digest {
				duplicate = true
				return nil
			}
			return serviceError(Conflict, "approval_terminal", "approval already has a terminal decision", nil)
		}
		if approval.ExpiresAt.Before(now) {
			approval.State, approval.Reason, approval.UpdatedAt, approval.ResolvedAt = "expired", "approval expired", now, &now
			_ = tx.SaveApproval(approval)
			return serviceError(Conflict, "approval_expired", "approval has expired", nil)
		}
		if request.RunID != "" && request.RunID != approval.RunID || request.ArgumentsDigest != "" && request.ArgumentsDigest != approval.ArgumentsDigest {
			return serviceError(Forbidden, "approval_binding_mismatch", "approval binding is invalid", nil)
		}
		panel, err := tx.Panel(approval.PanelSessionID, principal, true)
		if err != nil {
			return err
		}
		if approval.Scope != "service" && (panel.State != "active" || panel.LeaseExpiresAt.Before(now)) {
			return serviceError(Conflict, "panel_expired", "original panel session is unavailable", nil)
		}
		approval.State = "approved"
		if request.Decision == "reject" {
			approval.State = "rejected"
		}
		approval.DecisionDigest, approval.UpdatedAt, approval.ResolvedAt = digest, now, &now
		if err = tx.SaveApproval(approval); err != nil {
			return err
		}
		if approval.State == "rejected" {
			call, callErr := tx.ToolCall(approval.ToolCallID, true)
			if callErr != nil {
				return callErr
			}
			call.State, call.UpdatedAt, call.FinishedAt = "rejected", now, &now
			if callErr = tx.SaveToolCall(call); callErr != nil {
				return callErr
			}
		}
		payload := map[string]any{"approval_id": approval.ID, "tool_call_id": approval.ToolCallID, "status": approval.State, "approved": approval.State == "approved", "reason": approval.Reason}
		_, deliveries, err := event.Project(tx, "approval.resolved", "approval", approval.ID, "approval", event.References{ConversationID: approval.ConversationID, RunID: approval.RunID, ToolCallID: approval.ToolCallID, ApprovalID: approval.ID}, payload, []domain.PanelSession{*panel}, now)
		if err != nil {
			return err
		}
		for _, delivery := range deliveries {
			notify = append(notify, delivery.PanelSessionID)
		}
		return s.audit(tx, principal, "approval.decided", approval.ConversationID, approval.PanelSessionID, approval.RunID, map[string]any{"approval_id": approval.ID, "decision": request.Decision})
	})
	if err != nil {
		return nil, false, translateOrService(err)
	}
	if !duplicate {
		s.bus.Notify(notify...)
	}
	return approval, duplicate, nil
}

func (s *Service) Approval(ctx context.Context, principal domain.Principal, id string) (*domain.Approval, error) {
	var value *domain.Approval
	err := s.store.View(ctx, func(tx ports.Tx) error { var err error; value, err = tx.Approval(id, principal, false); return err })
	if err != nil {
		return nil, translateStore(err)
	}
	return value, nil
}
func (s *Service) ListApprovals(ctx context.Context, principal domain.Principal, conversationID string, offset, limit int) (domain.Page[domain.Approval], error) {
	offset, limit = pageBounds(offset, limit)
	var values []domain.Approval
	var count int64
	err := s.store.View(ctx, func(tx ports.Tx) error {
		if _, err := tx.Conversation(conversationID, principal, false); err != nil {
			return err
		}
		var err error
		values, count, err = tx.ListApprovals(conversationID, principal, offset, limit)
		return err
	})
	if err != nil {
		return domain.Page[domain.Approval]{}, translateStore(err)
	}
	return domain.Page[domain.Approval]{Results: values, Count: count}, nil
}

func (s *Service) AdminStats(ctx context.Context, principal domain.Principal) (map[string]int64, error) {
	if !principal.IsSuperuser {
		return nil, serviceError(Forbidden, "admin_required", "administrator permission is required", nil)
	}
	var result map[string]int64
	err := s.store.View(ctx, func(tx ports.Tx) error {
		var err error
		result, err = tx.Stats(principal.OrganizationID, time.Now().UTC().Add(-24*time.Hour))
		return err
	})
	if err != nil {
		return nil, translateStore(err)
	}
	return result, nil
}
func (s *Service) AdminAudit(ctx context.Context, principal domain.Principal, offset, limit int) (domain.Page[domain.AuditRecord], error) {
	if !principal.IsSuperuser {
		return domain.Page[domain.AuditRecord]{}, serviceError(Forbidden, "admin_required", "administrator permission is required", nil)
	}
	offset, limit = pageBounds(offset, limit)
	var values []domain.AuditRecord
	var count int64
	err := s.store.View(ctx, func(tx ports.Tx) error {
		var err error
		values, count, err = tx.ListAudit(principal.OrganizationID, offset, limit)
		return err
	})
	if err != nil {
		return domain.Page[domain.AuditRecord]{}, translateStore(err)
	}
	return domain.Page[domain.AuditRecord]{Results: values, Count: count}, nil
}
