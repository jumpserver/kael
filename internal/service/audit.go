package service

import (
	"context"
	"regexp"
	"strings"
	"time"

	"github.com/jumpserver/kael/internal/domain"
	"github.com/jumpserver/kael/internal/ports"
)

type AuditConversation struct {
	ID             string     `json:"id"`
	Title          string     `json:"title,omitempty"`
	Assistant      string     `json:"assistant"`
	Profile        string     `json:"profile"`
	Status         string     `json:"status"`
	User           AuditUser  `json:"user"`
	MessageCount   int64      `json:"message_count"`
	QuestionCount  int64      `json:"question_count"`
	LastQuestionAt *time.Time `json:"last_question_at,omitempty"`
	CreatedAt      time.Time  `json:"date_created"`
	UpdatedAt      time.Time  `json:"date_updated"`
}

type AuditUser struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Username string `json:"username"`
}

type AuditMessage struct {
	ID           string    `json:"id"`
	Role         string    `json:"role"`
	Content      string    `json:"content"`
	Status       string    `json:"status"`
	InputTokens  int64     `json:"input_tokens,omitempty"`
	OutputTokens int64     `json:"output_tokens,omitempty"`
	Error        string    `json:"error,omitempty"`
	CreatedAt    time.Time `json:"date_created"`
}

type AuditConversationDetail struct {
	AuditConversation
	Messages []AuditMessage `json:"messages"`
}

func (s *Service) AdminAuditConversations(ctx context.Context, principal domain.Principal, offset, limit int) (domain.Page[AuditConversation], error) {
	if !principal.IsSuperuser {
		return domain.Page[AuditConversation]{}, serviceError(Forbidden, "admin_required", "administrator permission is required", nil)
	}
	offset, limit = pageBounds(offset, limit)
	var conversations []domain.Conversation
	var count int64
	var summaries []AuditConversation
	err := s.store.View(ctx, func(tx ports.Tx) error {
		var err error
		conversations, count, err = tx.ListConversationsByOrganization(principal.OrganizationID, offset, limit)
		if err != nil {
			return err
		}
		summaries = make([]AuditConversation, 0, len(conversations))
		for _, conversation := range conversations {
			summary := auditConversation(conversation, principal)
			summary.MessageCount, summary.QuestionCount, summary.LastQuestionAt, err = tx.MessageAuditStats(conversation.ID, principal.OrganizationID)
			if err != nil {
				return err
			}
			summaries = append(summaries, summary)
		}
		return nil
	})
	if err != nil {
		return domain.Page[AuditConversation]{}, translateStore(err)
	}
	return domain.Page[AuditConversation]{Results: summaries, Count: count}, nil
}

func (s *Service) AdminAuditConversation(ctx context.Context, principal domain.Principal, id string) (*AuditConversationDetail, error) {
	if !principal.IsSuperuser {
		return nil, serviceError(Forbidden, "admin_required", "administrator permission is required", nil)
	}
	var result *AuditConversationDetail
	err := s.store.Transaction(ctx, func(tx ports.Tx) error {
		conversation, err := tx.ConversationByOrganization(id, principal.OrganizationID)
		if err != nil {
			return err
		}
		messages := []domain.Message{}
		for offset := 0; ; offset += domain.MaxPageSize {
			page, count, listErr := tx.ListMessagesByOrganization(id, principal.OrganizationID, offset, domain.MaxPageSize)
			if listErr != nil {
				return listErr
			}
			messages = append(messages, page...)
			if int64(len(messages)) >= count {
				break
			}
		}
		summary := auditConversation(*conversation, principal)
		summary.MessageCount = int64(len(messages))
		auditMessages := make([]AuditMessage, 0, len(messages))
		for _, message := range messages {
			if message.Role != "user" && message.Role != "assistant" {
				continue
			}
			if message.Role == "user" {
				summary.QuestionCount++
				created := message.CreatedAt
				summary.LastQuestionAt = &created
			}
			auditMessages = append(auditMessages, AuditMessage{ID: message.ID, Role: message.Role, Content: sanitizeAuditText(message.Content), Status: message.Status, InputTokens: message.InputTokens, OutputTokens: message.OutputTokens, Error: sanitizeAuditText(message.ErrorDetail), CreatedAt: message.CreatedAt})
		}
		result = &AuditConversationDetail{AuditConversation: summary, Messages: auditMessages}
		return s.audit(tx, principal, "admin.conversation_audit.viewed", id, "", "", map[string]any{"conversation_id": id})
	})
	if err != nil {
		return nil, translateStore(err)
	}
	return result, nil
}

func auditConversation(value domain.Conversation, principal domain.Principal) AuditConversation {
	name, username := value.SubjectName, value.SubjectUsername
	if value.SubjectID == principal.SubjectID {
		if name == "" {
			name = principal.Name
		}
		if username == "" {
			username = principal.Username
		}
	}
	return AuditConversation{ID: value.ID, Title: value.Title, Assistant: value.Assistant, Profile: value.Profile, Status: value.Status, User: AuditUser{ID: value.SubjectID, Name: name, Username: username}, CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt}
}

var auditSecretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?is)-----BEGIN [^-]*PRIVATE KEY-----.*?-----END [^-]*PRIVATE KEY-----`),
	regexp.MustCompile(`(?i)\bBearer\s+[A-Za-z0-9._~+/=-]+`),
	regexp.MustCompile(`\bsk-[A-Za-z0-9_-]{16,}\b`),
	regexp.MustCompile(`\bAKIA[A-Z0-9]{16}\b`),
	regexp.MustCompile(`(?i)\b(password|passphrase|secret|token|api[ _-]?key|authorization|cookie)\b\s*(?:is|[:=])\s*[^\s,;]+`),
	regexp.MustCompile(`(密码|口令|密钥|令牌|私钥)\s*(?:是|为|[:：=])\s*[^\s,，；;]+`),
}

func sanitizeAuditText(value string) string {
	result := value
	for _, pattern := range auditSecretPatterns {
		result = pattern.ReplaceAllString(result, "[REDACTED]")
	}
	return strings.TrimSpace(bounded(result, 32*1024))
}
