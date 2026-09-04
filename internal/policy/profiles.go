package policy

import (
	"fmt"
	"strings"

	"github.com/jumpserver/kael/internal/domain"
)

type Profile struct {
	ID                  string   `json:"id"`
	Version             string   `json:"version"`
	Name                string   `json:"name"`
	Description         string   `json:"description"`
	Instructions        string   `json:"-"`
	Kind                string   `json:"conversation_kind"`
	AllowedNamespaces   []string `json:"allowed_namespaces"`
	MaxRisk             string   `json:"max_risk"`
	CoreAPIEnabled      bool     `json:"core_api_enabled"`
	AdminOnly           bool     `json:"admin_only"`
	RequiredPermissions []string `json:"required_permissions,omitempty"`
	StarterPrompts      []string `json:"starter_prompts,omitempty"`
}

var profiles = map[string]Profile{
	"general": {
		ID: "general", Version: "1", Name: "JumpServer assistant", Kind: "general", MaxRisk: "read",
		Description:    "General JumpServer questions without access to live environment data.",
		Instructions:   "Act as a general JumpServer product assistant. Answer product concepts and usage questions directly. You cannot access live environment data unless an explicitly registered, server-authorized capability is present. Never claim that you executed an operation without a successful tool result.",
		StarterPrompts: []string{"What can JumpServer help me manage?", "Explain how to use JumpServer safely."},
	},
	"platform.management": {
		ID: "platform.management", Version: "1", Name: "Management assistant", Kind: "general", MaxRisk: "dangerous", CoreAPIEnabled: true, AdminOnly: true,
		AllowedNamespaces: []string{"platform.management"},
		Description:       "Manage permitted JumpServer resources and settings.",
		Instructions:      "Act as a JumpServer management assistant. Inspect before proposing changes. Every write or dangerous action requires explicit approval. Use only registered semantic capabilities or trusted operation registrations, and report uncertainty.",
		StarterPrompts:    []string{"Summarize the current JumpServer environment.", "Check recent operational exceptions that I am allowed to view."},
	},
	"platform.asset": {
		ID: "platform.asset", Version: "1", Name: "Asset assistant", Kind: "general", MaxRisk: "read", CoreAPIEnabled: true,
		AllowedNamespaces: []string{"platform.asset"}, RequiredPermissions: []string{"assets.view_asset", "assets.view_node", "assets.view_platform"},
		Description:  "Asset, node, platform, and protocol inspection.",
		Instructions: "Focus on assets, nodes, platforms, and protocols. Present identifiers, status, address, platform, and node placement compactly. Never infer missing live data.",
	},
	"platform.session_audit": {
		ID: "platform.session_audit", Version: "1", Name: "Session audit assistant", Kind: "general", MaxRisk: "read", CoreAPIEnabled: true,
		AllowedNamespaces: []string{"platform.session_audit"}, RequiredPermissions: []string{"audits.view_activitylog", "audits.view_operatelog", "audits.view_integrationapplicationlog", "audits.view_userloginlog", "terminal.view_command", "terminal.view_session", "terminal.view_task", "tickets.view_ticket"},
		Description:  "Session, command, login, access, and operation audit diagnosis.",
		Instructions: "Build a time-ordered explanation from authorized audit records. Never infer an event absent from tool results.",
	},
	"platform.ops": {
		ID: "platform.ops", Version: "1", Name: "Operations assistant", Kind: "general", MaxRisk: "read", CoreAPIEnabled: true,
		AllowedNamespaces: []string{"platform.ops"}, RequiredPermissions: []string{"audits.view_joblog", "ops.view_celerytask", "ops.view_celerytaskexecution", "ops.view_job", "ops.view_jobexecution", "terminal.view_terminal"},
		Description:  "Job, task, component, and terminal health diagnosis.",
		Instructions: "Focus on jobs, tasks, component metrics, and terminal health. Cite returned timestamps and status values, and recommend the smallest safe follow-up check.",
	},
	"terminal": {
		ID: "terminal", Version: "1", Name: "Terminal assistant", Kind: "capability", MaxRisk: "dangerous", AllowedNamespaces: []string{"luna.terminal"},
		Instructions: "This is a live audited resource terminal. Follow registered tool descriptions and the declared command language. Never assume an operating-system shell when context identifies another language.",
	},
	"file": {
		ID: "file", Version: "1", Name: "File assistant", Kind: "capability", MaxRisk: "dangerous", AllowedNamespaces: []string{"luna.file"},
		Instructions: "This is a live SFTP file session. Preserve complete virtual absolute paths and use returned versions as mutation preconditions.",
	},
	"sql": {
		ID: "sql", Version: "1", Name: "SQL assistant", Kind: "capability", MaxRisk: "write", AllowedNamespaces: []string{"luna.sql"},
		Instructions: "This is a draft-only SQL editor. Read verified editor context before analysis, use only its dialect and scope, and return edits only through the proposal capability. Never claim SQL was executed.",
	},
	"script": {
		ID: "script", Version: "1", Name: "Script assistant", Kind: "capability", MaxRisk: "write", AllowedNamespaces: []string{"luna.script"},
		Instructions: "This is a draft-only script editor. Read current script before analysis, propose changes using the latest revision, and never claim a proposal was saved or executed. Never request secret values.",
	},
}

var capabilityCatalog = map[string]map[string]domain.ToolAnnotations{
	"terminal": {
		"terminal_context":  {ReadOnly: true, Idempotent: true},
		"terminal_snapshot": {ReadOnly: true, Idempotent: true, OpenWorld: true},
		"database_schema":   {ReadOnly: true, Idempotent: true, OpenWorld: true},
		"execute_command":   {OpenWorld: true}, "execute_shell": {OpenWorld: true}, "execute_sql": {OpenWorld: true}, "execute_redis": {OpenWorld: true}, "execute_mongodb": {OpenWorld: true},
	},
	"file": {
		"list_directory": {ReadOnly: true, Idempotent: true}, "stat": {ReadOnly: true, Idempotent: true},
		"read_text": {ReadOnly: true, Idempotent: true, OpenWorld: true},
		"save_text": {Destructive: true}, "mkdir": {Destructive: true}, "rename": {Destructive: true}, "delete": {Destructive: true},
	},
	"sql": {
		"read_sql_context": {ReadOnly: true, Idempotent: true}, "inspect_schema": {ReadOnly: true, Idempotent: true, OpenWorld: true},
		"validate_sql": {ReadOnly: true, Idempotent: true}, "propose_sql": {ReadOnly: true, Idempotent: true},
	},
	"script": {
		"read_script": {ReadOnly: true, Idempotent: true}, "propose_script": {ReadOnly: true, Idempotent: true},
	},
}

func Get(id string) (Profile, bool) {
	if alias := map[string]string{"management": "platform.management", "asset": "platform.asset", "session_audit": "platform.session_audit", "ops": "platform.ops"}[id]; alias != "" {
		id = alias
	}
	profile, ok := profiles[id]
	return profile, ok
}

func Available(principal domain.Principal) []Profile {
	result := make([]Profile, 0, len(profiles))
	for _, id := range []string{"general", "platform.management", "platform.asset", "platform.session_audit", "platform.ops", "terminal", "file", "sql", "script"} {
		profile := profiles[id]
		if Authorized(profile, principal) {
			result = append(result, profile)
		}
	}
	return result
}

func Authorized(profile Profile, principal domain.Principal) bool {
	if profile.AdminOnly && !principal.IsSuperuser && !principal.IsOrgAdmin {
		return false
	}
	if len(profile.RequiredPermissions) == 0 {
		return true
	}
	available := map[string]struct{}{}
	for _, permission := range principal.Permissions {
		available[permission] = struct{}{}
	}
	for _, permission := range profile.RequiredPermissions {
		if _, ok := available[permission]; ok {
			return true
		}
	}
	return false
}

func Namespace(profile Profile) string {
	if strings.HasPrefix(profile.ID, "platform.") {
		return profile.ID
	}
	if profile.Kind == "capability" {
		return "luna." + profile.ID
	}
	return ""
}

func EnforceRegistration(profile Profile, name string, supplied domain.ToolAnnotations) (domain.ToolAnnotations, string, bool, error) {
	name = strings.TrimSpace(name)
	trusted, known := capabilityCatalog[profile.ID][name]
	if !known {
		if !strings.HasPrefix(profile.ID, "platform.") {
			return domain.ToolAnnotations{}, "", false, fmt.Errorf("capability %q is not allowed by profile", name)
		}
		trusted = domain.ToolAnnotations{Destructive: true, OpenWorld: true}
	}
	result := trusted
	result.ReadOnly = trusted.ReadOnly && supplied.ReadOnly
	result.Destructive = trusted.Destructive || supplied.Destructive
	result.OpenWorld = trusted.OpenWorld || supplied.OpenWorld
	result.Idempotent = trusted.Idempotent && supplied.Idempotent
	result.FinalResult = supplied.FinalResult && (profile.ID == "sql" || profile.ID == "script")
	risk := "write"
	if result.ReadOnly {
		risk = "read"
	}
	if result.Destructive || result.OpenWorld && !result.ReadOnly {
		risk = "dangerous"
	}
	requiresConfirmation := risk != "read" || result.OpenWorld
	if profile.ID == "sql" && name == "inspect_schema" {
		requiresConfirmation = true
	}
	return result, risk, requiresConfirmation, nil
}

func RiskAllowed(maximum, actual string) bool {
	levels := map[string]int{"read": 1, "write": 2, "dangerous": 3}
	return levels[actual] > 0 && levels[actual] <= levels[maximum]
}
