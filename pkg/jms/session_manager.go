package jms

import (
	"errors"
	"sync"
)

type Session interface {
	GetID() string
}
type SessionManager struct {
	store sync.Map
}

func NewSessionManager() *SessionManager {
	return &SessionManager{}
}

func (sm *SessionManager) RegisterSession(session Session) string {
	sessionID := session.GetID()
	sm.store.Store(sessionID, session)
	return sessionID
}

func (sm *SessionManager) UnregisterSession(session Session) {
	sessionID := session.GetID()
	sm.store.Delete(sessionID)
}

func (sm *SessionManager) GetSession(sessionID string) (*JMSSession, error) {
	if value, ok := sm.store.Load(sessionID); ok {
		if session, ok := value.(*JMSSession); ok {
			return session, nil
		}
	}
	return nil, errors.New("session not found or type mismatch")
}

func (sm *SessionManager) GetSystemSession(sessionID string) (*JMSSystemSession, error) {
	if value, ok := sm.store.Load(sessionID); ok {
		if session, ok := value.(*JMSSystemSession); ok {
			return session, nil
		}
	}
	return nil, errors.New("session not found or type mismatch")
}

func (sm *SessionManager) GetStore() *sync.Map {
	return &sm.store
}

var GlobalSessionManager = NewSessionManager()
var GlobalSystemSessionManager = NewSessionManager()
