package ports

import (
	"context"
	"encoding/json"

	"github.com/jumpserver/kael/internal/domain"
)

type CapabilityRequest struct {
	Principal      domain.Principal
	ConversationID string
	RunID          string
	ToolCallID     string
	ApprovalID     string
	Profile        string
	Registration   domain.Registration
	Arguments      json.RawMessage
}

type CapabilityPolicy struct {
	Risk                 string
	RequiresConfirmation bool
	Preview              json.RawMessage
}

type CapabilityResult struct {
	Status                 string
	Result                 json.RawMessage
	Error                  json.RawMessage
	ResultCards            json.RawMessage
	ExecutorAuditReference string
}

type CapabilityProvider interface {
	Registrations(context.Context, domain.Principal, string) ([]domain.Registration, error)
	Prepare(context.Context, CapabilityRequest) (CapabilityPolicy, error)
	Execute(context.Context, CapabilityRequest) (CapabilityResult, error)
	Refresh(context.Context) (map[string]any, error)
}
