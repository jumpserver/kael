package schemas

type AskRequest struct {
	ConversationID string `json:"conversation_id,omitempty"`
	Content        string `json:"content"`
	Prompt         string `json:"prompt"`
}

type AskResponseType string

const (
	Waiting AskResponseType = "waiting"
	Reject  AskResponseType = "reject"
	Message AskResponseType = "message"
	Error   AskResponseType = "error"
	Finish  AskResponseType = "finish"
)

type SessionStateType string

const (
	LockSession   SessionStateType = "lock"
	UnlockSession SessionStateType = "unlock"
)

type ResponseMeta struct {
	ActivateReview bool             `json:"activate_review,omitempty"`
	SessionState   SessionStateType `json:"session_state,omitempty"`
}

type AskResponse struct {
	Type           AskResponseType `json:"type"`
	ConversationID string          `json:"conversation_id,omitempty"`
	Message        *ChatGPTMessage `json:"message,omitempty"`
	SystemMessage  string          `json:"system_message,omitempty"`
	Meta           ResponseMeta    `json:"meta,omitempty"`
}
