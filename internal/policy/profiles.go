package policy

import (
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
		ID: "general", Version: "3", Name: "JumpServer assistant", Kind: "general", MaxRisk: "dangerous", CoreAPIEnabled: true,
		AllowedNamespaces: []string{"general"},
		Description:       "Unified JumpServer assistant for product questions and authorized live Core operations.",
		Instructions:      "Act as the unified JumpServer assistant. Answer product concepts and usage questions directly. For live environment work, immediately search only server-authorized Core operations and call only an operation returned by that search. Resolve platform, node, and other related identifiers with authorized read operations before asking the user. Ask one concise question only for irreducible missing values such as a target host address; combine all missing values in that question. For a Linux host create request, the address is the only irreducible value: derive a concise name from it, look up the Linux platform, and use Core defaults unless the user specified otherwise. Once required values are available, call the write operation immediately: the trusted approval UI provides confirmation, so never ask the user to type confirmation in chat. Never claim execution without a successful tool result.",
		StarterPrompts:    []string{"What can JumpServer help me manage?", "Explain how to use JumpServer safely."},
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
	"workspace": {
		ID: "workspace", Version: "1", Name: "Workspace assistant", Kind: "capability", MaxRisk: "dangerous", AllowedNamespaces: []string{"luna.workspace"},
		Description:    "Find authorized assets, follow them in the Luna workspace, and start an approved connection when the target is unambiguous.",
		Instructions:   "Act as the Luna workspace assistant. Search only the user's authorized connectable assets. Never choose among multiple assets, protocols, or accounts: present the candidates and ask the user to select. Reveal the selected asset in the workspace before preparing a connection. When the asset, protocol, and account are all unique, connect it only through the trusted connection capability, following its registered policy and the user's approval mode. Otherwise open the prefilled connection setup and wait for the user's selection. Never request, expose, or infer passwords, tokens, tickets, cookies, or connection URLs. Report a session_started tool result only as a Luna session start; never claim that remote authentication or login completed.",
		StarterPrompts: []string{"Connect me to an authorized asset.", "Find an asset in my workspace."},
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

func Get(id string) (Profile, bool) {
	if alias := map[string]string{"management": "platform.management", "asset": "platform.asset", "session_audit": "platform.session_audit", "ops": "platform.ops"}[id]; alias != "" {
		id = alias
	}
	profile, ok := profiles[id]
	return profile, ok
}

func Available(principal domain.Principal) []Profile {
	result := make([]Profile, 0, len(profiles))
	for _, id := range []string{"general", "workspace", "platform.management", "platform.asset", "platform.session_audit", "platform.ops", "terminal", "file", "sql", "script"} {
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
		if _, ok := available[permission]; !ok {
			return false
		}
	}
	return true
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

// ConfirmationRequired combines the registered policy with the user's panel mode.
func ConfirmationRequired(registered bool, approvalMode string) bool {
	if approvalMode == "always" {
		return true
	}
	if approvalMode == "never" {
		return false
	}
	return registered
}

// RegistrationPolicy derives risk from the authenticated Luna client's annotations.
// Missing annotations default to write with confirmation; names do not affect policy.
func RegistrationPolicy(annotations domain.ToolAnnotations) (string, bool) {
	risk := "write"
	if annotations.ReadOnly {
		risk = "read"
	}
	if annotations.Destructive || annotations.OpenWorld && !annotations.ReadOnly {
		risk = "dangerous"
	}
	return risk, risk != "read" || annotations.OpenWorld
}

func RiskAllowed(maximum, actual string) bool {
	levels := map[string]int{"read": 1, "write": 2, "dangerous": 3}
	return levels[actual] > 0 && levels[actual] <= levels[maximum]
}
