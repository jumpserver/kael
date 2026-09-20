package service

import (
	"context"
	"errors"

	"github.com/jumpserver/kael/internal/domain"
	"github.com/jumpserver/kael/internal/identity"
	"github.com/jumpserver/kael/internal/ports"
)

// The caller holds credentialsMu across queueing and binding so a worker
// cannot execute a committed run before its request credentials are available.
func (s *Service) bindRunCredentials(ctx context.Context, run *domain.Run) {
	if run.CapabilityMode != "service" {
		return
	}
	if s.runCredentials == nil {
		s.runCredentials = make(map[string]identity.CoreCredentials)
	}
	s.runCredentials[run.ID] = identity.CoreCredentialsFromContext(ctx)
}

func (s *Service) capabilityContext(ctx context.Context, runID string) context.Context {
	s.credentialsMu.Lock()
	credentials := s.runCredentials[runID]
	s.credentialsMu.Unlock()
	return identity.WithCoreCredentials(ctx, credentials)
}

func (s *Service) forgetRunCredentials(runID string) {
	s.credentialsMu.Lock()
	delete(s.runCredentials, runID)
	s.credentialsMu.Unlock()
}

func (s *Service) pruneRunCredentials(ctx context.Context) error {
	s.credentialsMu.Lock()
	defer s.credentialsMu.Unlock()
	if len(s.runCredentials) == 0 {
		return nil
	}
	return s.store.View(ctx, func(tx ports.Tx) error {
		for id := range s.runCredentials {
			run, err := tx.RunInternal(id, false)
			if errors.Is(err, ports.ErrNotFound) || err == nil && run.Terminal() {
				delete(s.runCredentials, id)
			} else if err != nil {
				return err
			}
		}
		return nil
	})
}
