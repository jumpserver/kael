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
