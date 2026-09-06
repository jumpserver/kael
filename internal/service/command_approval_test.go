package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/jumpserver/kael/internal/domain"
	"github.com/jumpserver/kael/internal/event"
	"github.com/jumpserver/kael/internal/policy"
	"github.com/jumpserver/kael/internal/ports"
	"github.com/jumpserver/kael/internal/store"
)

func TestCommandApprovalModes(t *testing.T) {
	for _, tc := range []struct {
		name, mode, command, commandPolicy, explicitRisk string
		force, approve, read                             bool
	}{
		{"default df", "", "df -h", policy.ShellReadOnlyPolicy, "", false, false, true},
		{"auto du", "auto", "du -sh /var/log", policy.ShellReadOnlyPolicy, "", false, false, true},
		{"auto pipeline", "auto", "du -sh /var/log/* 2>/dev/null | sort -hr | head -n 20", policy.ShellReadOnlyPolicy, "", false, false, true},
		{"auto mutation", "auto", "df -h; rm file", policy.ShellReadOnlyPolicy, "", false, true, false},
		{"auto redirection", "auto", "df -h > /tmp/report", policy.ShellReadOnlyPolicy, "", false, true, false},
		{"always read", "always", "df -h", policy.ShellReadOnlyPolicy, "", false, true, true},
		{"never mutation", "never", "rm file", policy.ShellReadOnlyPolicy, "", false, false, false},
		{"legacy registration", "auto", "df -h", "", "", false, true, false},
		{"unknown policy", "auto", "df -h", "unsupported", "", false, true, false},
		{"explicit confirmation", "auto", "df -h", policy.ShellReadOnlyPolicy, "", true, true, false},
		{"explicit risk", "auto", "df -h", policy.ShellReadOnlyPolicy, "dangerous", false, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			s := &Service{store: store.NewMemory(), bus: event.NewBus(), panelLease: time.Minute, registrationLease: time.Minute, eventRetention: time.Hour}
			principal := domain.Principal{SubjectID: "user", OrganizationID: "org"}
			if err := s.store.Transaction(ctx, func(tx ports.Tx) error {
				return tx.CreateConversation(&domain.Conversation{ID: "conversation", SubjectID: principal.SubjectID, OrganizationID: principal.OrganizationID, Kind: "capability", Profile: "terminal", Status: "active"})
			}); err != nil {
				t.Fatal(err)
			}
			panel, err := s.CreatePanel(ctx, principal, CreatePanelRequest{ConversationID: "conversation", ClientInstanceID: "resource", ApprovalMode: tc.mode})
			if err != nil {
				t.Fatal(err)
			}
			registry, err := s.ReplaceRegistrations(ctx, principal, panel.ID, ReplaceRegistrationsRequest{Registrations: []RegistrationDefinition{{
				Name: "custom_shell", InputSchema: json.RawMessage(`{"type":"object","properties":{"command":{"type":"string"}},"required":["command"],"additionalProperties":false}`),
				Annotations: map[string]any{"readOnlyHint": false, "openWorldHint": true}, Meta: map[string]any{policy.CommandPolicyMetaKey: tc.commandPolicy}, Risk: tc.explicitRisk, RequiresConfirmation: tc.force,
			}}})
			if err != nil {
				t.Fatal(err)
			}
			registration := registry.Registrations[0]
			if registration.Risk != "dangerous" || !registration.RequiresConfirmation {
				t.Fatal("generic shell registration lost its conservative default")
			}
			run := &domain.Run{ID: "run", ConversationID: panel.ConversationID, PanelSessionID: panel.ID, SubjectID: principal.SubjectID, OrganizationID: principal.OrganizationID, RegistryRevision: registry.RegistryRevision, State: "running"}
			if err = s.store.Transaction(ctx, func(tx ports.Tx) error { return tx.CreateRun(run) }); err != nil {
				t.Fatal(err)
			}
			args, _ := json.Marshal(map[string]any{"command": tc.command})
			call, approval, err := s.prepareToolCall(ctx, run, registration, args, 0)
			if err != nil {
				t.Fatal(err)
			}
			if (approval != nil) != tc.approve || call.RequiresConfirmation != tc.approve || (call.Risk == "read") != tc.read {
				t.Fatalf("call=%+v approval=%+v", call, approval)
			}
			if approval != nil {
				if _, _, err = s.DecideApproval(ctx, principal, approval.ID, ApprovalDecisionRequest{Decision: "approve"}); err != nil {
					t.Fatal(err)
				}
			}
			if err = s.dispatchToolCall(ctx, run, call); err != nil {
				t.Fatalf("dispatch: %v", err)
			}
			deliveries, _, err := s.Deliveries(ctx, principal, panel.ID, 0, 100)
			if err != nil {
				t.Fatal(err)
			}
			approvals, dispatched := 0, 0
			for _, delivery := range deliveries {
				if delivery.Type == "approval.required" {
					approvals++
				}
				if delivery.Type == "tool.call" {
					dispatched++
					var payload struct {
						Risk string `json:"risk"`
					}
					if json.Unmarshal(delivery.Payload, &payload) != nil || payload.Risk != call.Risk {
						t.Fatal("dispatch did not carry the invocation risk")
					}
				}
			}
			if (approvals == 1) != tc.approve || dispatched != 1 {
				t.Fatalf("approval events=%d dispatch events=%d", approvals, dispatched)
			}
		})
	}
}
