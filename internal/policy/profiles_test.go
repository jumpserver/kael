package policy

import (
	"testing"

	"github.com/jumpserver/kael/internal/domain"
)

func TestWorkspaceProfileIsAvailable(t *testing.T) {
	profile, ok := Get("workspace")
	if !ok || profile.Kind != "capability" || profile.MaxRisk != "dangerous" || Namespace(profile) != "luna.workspace" {
		t.Fatalf("unexpected workspace profile: profile=%+v available=%t", profile, ok)
	}

	available := Available(domain.Principal{})
	if len(available) < 2 || available[0].ID != "general" || available[1].ID != "workspace" {
		t.Fatalf("workspace profile is not available after general: %+v", available)
	}
}

func TestWorkspaceProfileRejectsUnknownCapability(t *testing.T) {
	profile, _ := Get("workspace")
	if _, _, _, err := EnforceRegistration(profile, "click", domain.ToolAnnotations{}); err == nil {
		t.Fatal("workspace profile accepted an unknown capability")
	}
}

func TestWorkspaceCapabilityPolicy(t *testing.T) {
	profile, _ := Get("workspace")
	readAnnotations := domain.ToolAnnotations{ReadOnly: true, Idempotent: true, FinalResult: true}
	for _, name := range []string{"search_connectable_assets", "reveal_asset"} {
		annotations, risk, confirmation, err := EnforceRegistration(profile, name, readAnnotations)
		if err != nil || risk != "read" || confirmation || !annotations.ReadOnly || !annotations.Idempotent || annotations.FinalResult {
			t.Fatalf("unexpected %s policy: annotations=%+v risk=%q confirmation=%t err=%v", name, annotations, risk, confirmation, err)
		}
	}
	annotations, risk, confirmation, err := EnforceRegistration(profile, "prepare_asset_connection", readAnnotations)
	if err != nil || risk != "read" || confirmation || !annotations.ReadOnly || annotations.Idempotent || annotations.FinalResult {
		t.Fatalf("unexpected prepare_asset_connection policy: annotations=%+v risk=%q confirmation=%t err=%v", annotations, risk, confirmation, err)
	}

	annotations, risk, confirmation, err = EnforceRegistration(profile, "connect_asset", domain.ToolAnnotations{ReadOnly: true, Idempotent: true, FinalResult: true})
	if err != nil || risk != "dangerous" || !confirmation || annotations.ReadOnly || annotations.Idempotent || !annotations.OpenWorld || !annotations.FinalResult {
		t.Fatalf("unexpected connect_asset policy: annotations=%+v risk=%q confirmation=%t err=%v", annotations, risk, confirmation, err)
	}
}

func TestWorkspaceConnectionConfirmationCannotBeDisabled(t *testing.T) {
	workspace, _ := Get("workspace")
	if ApprovalModeAllowed(workspace, "never") {
		t.Fatal("workspace profile accepted approval mode never")
	}
	for _, mode := range []string{"always", "auto", "never"} {
		if !ConfirmationRequired(workspace, "connect_asset", false, mode) {
			t.Fatalf("workspace connect_asset confirmation was disabled in mode %q", mode)
		}
	}

	general, _ := Get("general")
	if !ApprovalModeAllowed(general, "never") || ConfirmationRequired(general, "other_tool", true, "never") {
		t.Fatal("workspace safeguard changed the existing general profile approval behavior")
	}
}
