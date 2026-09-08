package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jumpserver/kael/internal/component"
	"github.com/jumpserver/kael/internal/ports"
)

const (
	coreStorePageSize       = 1000
	coreStoreCommitAttempts = 3
)

type corePersistence struct {
	mu                   sync.Mutex
	client               *component.Client
	revision             uint64
	recordsSinceSnapshot int
	poisoned             error
	snapshotDisabled     bool
}

func NewCore(client *component.Client) (*Memory, error) {
	if client == nil {
		return nil, fmt.Errorf("Core runtime store client is required")
	}
	persistence, state, err := openCorePersistence(client)
	if err != nil {
		return nil, err
	}
	next := state.clone()
	recoverProcessLocalState(next, time.Now().UTC())
	if !reflect.DeepEqual(state, next) {
		if err = persistence.Commit(state, next); err != nil {
			return nil, fmt.Errorf("recover persisted runtime state: %w", err)
		}
		state = next
	}
	return &Memory{state: state, persistence: persistence}, nil
}

func openCorePersistence(client *component.Client) (*corePersistence, *memoryState, error) {
	state := newMemoryState()
	after := uint64(0)
	recordsSinceSnapshot := 0
	for {
		page, err := client.LoadRuntimeStore(after, coreStorePageSize)
		if err != nil {
			return nil, nil, err
		}
		for _, item := range page.Results {
			if item.Revision <= after {
				return nil, nil, fmt.Errorf("Core runtime store returned non-increasing revision %d", item.Revision)
			}
			if item.Revision != after+1 && !item.Snapshot {
				return nil, nil, fmt.Errorf("Core runtime store skipped from revision %d to non-snapshot revision %d", after, item.Revision)
			}
			payload, err := decodeCoreRecord(item.Record)
			if err != nil {
				return nil, nil, fmt.Errorf("decode Core runtime store revision %d: %w", item.Revision, err)
			}
			if item.Snapshot != (payload.Snapshot != nil) {
				return nil, nil, fmt.Errorf("Core runtime store revision %d has inconsistent snapshot metadata", item.Revision)
			}
			if payload.Snapshot != nil {
				state = payload.Snapshot.memoryState()
				recordsSinceSnapshot = 0
			}
			if payload.Delta != nil {
				applyPersistentDelta(state, *payload.Delta)
				recordsSinceSnapshot++
			}
			after = item.Revision
		}
		if page.HasMore {
			if len(page.Results) == 0 {
				return nil, nil, fmt.Errorf("Core runtime store pagination did not advance")
			}
			continue
		}
		if page.Revision != after {
			return nil, nil, fmt.Errorf("Core runtime store load ended at revision %d, expected %d", after, page.Revision)
		}
		return &corePersistence{client: client, revision: after, recordsSinceSnapshot: recordsSinceSnapshot}, state, nil
	}
}

func decodeCoreRecord(line string) (journalPayload, error) {
	line = strings.TrimSuffix(line, "\n")
	if line == "" || strings.ContainsAny(line, "\r\n") {
		return journalPayload{}, fmt.Errorf("runtime store record must be one non-empty line")
	}
	var record journalRecord
	if err := json.Unmarshal([]byte(line), &record); err != nil {
		return journalPayload{}, fmt.Errorf("runtime store record is invalid: %w", err)
	}
	if record.Version != journalVersion {
		return journalPayload{}, fmt.Errorf("runtime store record version %d is unsupported", record.Version)
	}
	payload, err := decodeJournalPayload(record)
	if err != nil {
		return journalPayload{}, err
	}
	if (payload.Snapshot == nil) == (payload.Delta == nil) {
		return journalPayload{}, fmt.Errorf("runtime store record must contain exactly one snapshot or delta")
	}
	return payload, nil
}

func (p *corePersistence) Commit(previous, next *memoryState) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.poisoned != nil {
		return p.unavailableLocked()
	}
	previous = corePersistableState(previous)
	next = corePersistableState(next)
	delta := diffPersistentState(previous, next)
	if delta.empty() {
		return nil
	}
	snapshot := !p.snapshotDisabled && p.recordsSinceSnapshot >= compactJournalRecords
	payload := journalPayload{Delta: &delta}
	if snapshot {
		state := persistentStateFromMemory(next)
		payload = journalPayload{Snapshot: &state}
	}
	line, err := encodeJournalRecord(payload)
	if snapshot && errors.Is(err, errJournalRecordTooLarge) {
		p.snapshotDisabled = true
		snapshot = false
		payload = journalPayload{Delta: &delta}
		line, err = encodeJournalRecord(payload)
	}
	if err != nil {
		return err
	}
	record := strings.TrimSuffix(string(line), "\n")
	commitID := uuid.NewString()
	var revision uint64
	uncertainSeen := false
	for attempt := 0; attempt < coreStoreCommitAttempts; attempt++ {
		revision, err = p.client.AppendRuntimeStore(commitID, p.revision, snapshot, record)
		if err == nil {
			break
		}
		if errors.Is(err, component.ErrRuntimeStoreRevisionConflict) {
			return p.poisonLocked(fmt.Errorf("runtime store revision conflict: %v", err))
		}
		if !errors.Is(err, component.ErrRuntimeStoreCommitUncertain) {
			if uncertainSeen {
				return p.poisonLocked(fmt.Errorf("%w: retry after uncertain outcome failed: %v", component.ErrRuntimeStoreCommitUncertain, err))
			}
			return err
		}
		uncertainSeen = true
		if attempt+1 == coreStoreCommitAttempts {
			return p.poisonLocked(err)
		}
		time.Sleep(time.Duration(attempt+1) * 50 * time.Millisecond)
	}
	p.revision = revision
	if snapshot {
		p.recordsSinceSnapshot = 0
	} else {
		p.recordsSinceSnapshot++
	}
	return nil
}

// corePersistableState keeps pre-question sessions process-local. Luna prepares
// conversations, panels, context, and registrations before the user sends a
// prompt; persisting that preparation would expose empty conversations in Core
// audit views. The first user message makes the entire conversation subgraph
// durable in the same transaction.
func corePersistableState(state *memoryState) *memoryState {
	result := state.clone()
	durableConversations := make(map[string]struct{})
	for _, message := range state.messages {
		if message.Role == "user" {
			durableConversations[message.ConversationID] = struct{}{}
		}
	}

	transientConversations := make(map[string]struct{})
	for id := range result.conversations {
		if _, durable := durableConversations[id]; durable {
			continue
		}
		transientConversations[id] = struct{}{}
		delete(result.conversations, id)
	}
	if len(transientConversations) == 0 {
		return result
	}

	transientMessages := make(map[string]struct{})
	for id, message := range result.messages {
		if _, transient := transientConversations[message.ConversationID]; transient {
			transientMessages[id] = struct{}{}
			delete(result.messages, id)
		}
	}
	for id, artifact := range result.artifacts {
		if _, transient := transientMessages[artifact.MessageID]; transient {
			delete(result.artifacts, id)
		}
	}

	transientPanels := make(map[string]struct{})
	for id, panel := range result.panels {
		if _, transient := transientConversations[panel.ConversationID]; transient {
			transientPanels[id] = struct{}{}
			delete(result.panels, id)
		}
	}
	for id, snapshot := range result.contexts {
		if _, transient := transientPanels[snapshot.PanelSessionID]; transient {
			delete(result.contexts, id)
		}
	}
	for id, registration := range result.registrations {
		if _, transient := transientPanels[registration.PanelSessionID]; transient {
			delete(result.registrations, id)
		}
	}

	transientRuns := make(map[string]struct{})
	for id, run := range result.runs {
		if _, transient := transientConversations[run.ConversationID]; transient {
			transientRuns[id] = struct{}{}
			delete(result.runs, id)
		}
	}
	for id, call := range result.modelCalls {
		if _, transient := transientRuns[call.RunID]; transient {
			delete(result.modelCalls, id)
		}
	}

	transientToolCalls := make(map[string]struct{})
	for id, call := range result.toolCalls {
		_, transientConversation := transientConversations[call.ConversationID]
		_, transientRun := transientRuns[call.RunID]
		_, transientPanel := transientPanels[call.PanelSessionID]
		if transientConversation || transientRun || transientPanel {
			transientToolCalls[id] = struct{}{}
			delete(result.toolCalls, id)
		}
	}
	for id, value := range result.toolResults {
		_, transientCall := transientToolCalls[value.ToolCallID]
		_, transientRun := transientRuns[value.RunID]
		_, transientPanel := transientPanels[value.PanelSessionID]
		if transientCall || transientRun || transientPanel {
			delete(result.toolResults, id)
		}
	}

	transientApprovals := make(map[string]struct{})
	for id, approval := range result.approvals {
		_, transientConversation := transientConversations[approval.ConversationID]
		_, transientRun := transientRuns[approval.RunID]
		_, transientPanel := transientPanels[approval.PanelSessionID]
		_, transientCall := transientToolCalls[approval.ToolCallID]
		if transientConversation || transientRun || transientPanel || transientCall {
			transientApprovals[id] = struct{}{}
			delete(result.approvals, id)
		}
	}

	transientEvents := make(map[string]struct{})
	for id, value := range result.events {
		if _, transient := transientConversations[value.ConversationID]; transient {
			transientEvents[id] = struct{}{}
			delete(result.events, id)
		}
	}
	for id := range transientConversations {
		delete(result.eventHighWater, id)
	}
	for id, delivery := range result.deliveries {
		_, transientConversation := transientConversations[delivery.ConversationID]
		_, transientPanel := transientPanels[delivery.PanelSessionID]
		_, transientRun := transientRuns[delivery.RunID]
		_, transientMessage := transientMessages[delivery.MessageID]
		_, transientCall := transientToolCalls[delivery.ToolCallID]
		_, transientApproval := transientApprovals[delivery.ApprovalID]
		_, transientEvent := transientEvents[delivery.EventID]
		if transientConversation || transientPanel || transientRun || transientMessage || transientCall || transientApproval || transientEvent {
			delete(result.deliveries, id)
		}
	}
	for id := range transientPanels {
		delete(result.deliveryHighWater, id)
	}

	for id, audit := range result.audits {
		_, transientConversation := transientConversations[audit.ConversationID]
		_, transientPanel := transientPanels[audit.PanelSessionID]
		_, transientRun := transientRuns[audit.RunID]
		_, transientCall := transientToolCalls[audit.ToolCallID]
		_, transientApproval := transientApprovals[audit.ApprovalID]
		if transientConversation || transientPanel || transientRun || transientCall || transientApproval {
			delete(result.audits, id)
		}
	}
	return result
}

func (p *corePersistence) Ready(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if p.poisoned != nil {
		return p.unavailableLocked()
	}
	page, err := p.client.LoadRuntimeStoreContext(ctx, p.revision, 1)
	if err != nil {
		return fmt.Errorf("check Core runtime store: %w", err)
	}
	if page.Revision != p.revision || len(page.Results) > 0 || page.HasMore {
		return p.poisonLocked(fmt.Errorf("Core runtime store advanced from local revision %d to %d", p.revision, page.Revision))
	}
	return nil
}

func (p *corePersistence) poisonLocked(err error) error {
	if p.poisoned == nil {
		p.poisoned = err
	}
	return p.unavailableLocked()
}

func (p *corePersistence) unavailableLocked() error {
	return fmt.Errorf("%w: Core runtime store requires restart: %v", ports.ErrUnavailable, p.poisoned)
}

func (p *corePersistence) RuntimeMetrics() map[string]int64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	disabled := int64(0)
	if p.snapshotDisabled {
		disabled = 1
	}
	return map[string]int64{
		"runtime_store_snapshot_disabled":      disabled,
		"runtime_store_revision":               int64(p.revision),
		"runtime_store_records_since_snapshot": int64(p.recordsSinceSnapshot),
	}
}

func (p *corePersistence) Close() error { return nil }
