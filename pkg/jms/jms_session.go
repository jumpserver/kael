package jms

import (
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/jumpserver/kael/pkg/schemas"
	"github.com/jumpserver/wisp/protobuf-go/protobuf"
	"time"
)

type JMSSession struct {
	Session             *protobuf.Session
	Websocket           *websocket.Conn
	Prompt              string
	HistoryAsks         []string
	CurrentAskInterrupt bool
	CommandACLs         []*protobuf.CommandACL
	ExpireTime          time.Time
	MaxIdleTime         int
	MaxSessionTime      int
	SessionHandler      *SessionHandler
	CommandHandler      *CommandHandler
	ReplayHandler       *ReplayHandler
	JMSState            *schemas.JMSState
}

func (sh *JMSSession) GetID() string {
	return sh.Session.Id
}

func (jmss *JMSSession) ActiveSession() {
	GlobalSessionManager.RegisterSession(jmss)
	jmss.ReplayHandler = NewReplayHandler(jmss.Session)
	jmss.CommandHandler = NewCommandHandler(
		jmss.Websocket, jmss.Session, jmss.CommandACLs, jmss.JMSState,
	)
	go jmss.MaximumIdleTimeDetection()
	go jmss.MaxSessionTimeDetection()
}

func (jmss *JMSSession) MaximumIdleTimeDetection() {
	lastActiveTime := time.Now()

	for {
		currentTime := time.Now()
		idleTime := currentTime.Sub(lastActiveTime)

		if idleTime.Seconds() >= float64(jmss.MaxIdleTime*60) {
			reason := fmt.Sprintf("超过当前会话最大空闲时间 %d (分), 会话中断", jmss.MaxIdleTime)
			jmss.Close(reason)
			break
		}

		if jmss.JMSState.NewDialogue {
			lastActiveTime = currentTime
			jmss.JMSState.NewDialogue = false
		}
		time.Sleep(3 * time.Second)
	}
}

func (jmss *JMSSession) MaxSessionTimeDetection() {
	lastActiveTime := time.Now()

	for {
		currentTime := time.Now()
		idleTime := currentTime.Sub(lastActiveTime)

		if idleTime.Seconds() >= float64(jmss.MaxSessionTime*60*60) {
			reason := fmt.Sprintf("超过当前会话最大时间 %d (时), 会话中断", jmss.MaxSessionTime)
			jmss.Close(reason)
			break
		}
		time.Sleep(3 * time.Second)
	}
}

func (jmss *JMSSession) Close(reason string) {
	jmss.CurrentAskInterrupt = true
	time.Sleep(1 * time.Second)
	jmss.ReplayHandler.Upload()
	jmss.SessionHandler.closeSession(jmss.Session)
	GlobalSessionManager.UnregisterSession(jmss)
	jmss.NotifyToClose(reason)
}

func (jmss *JMSSession) NotifyToClose(reason string) {
	response := &schemas.AskResponse{
		Type:           schemas.Finish,
		ConversationID: jmss.Session.Id,
		SystemMessage:  reason,
	}

	jsonResponse, _ := json.Marshal(response)
	_ = jmss.Websocket.WriteMessage(websocket.TextMessage, jsonResponse)
}

func (jmss *JMSSession) WithAudit(command string, chatFunc func() string) (result string) {
	commandRecord := &schemas.CommandRecord{Input: command}
	jmss.CommandHandler.CommandRecord = commandRecord
	isContinue := jmss.CommandHandler.CommandACLFilter()
	go jmss.ReplayHandler.WriteInput(commandRecord.Input)
	if !isContinue {
		return
	}
	result = chatFunc()
	commandRecord.Output = result
	go jmss.ReplayHandler.WriteOutput(commandRecord.Output)
	go jmss.CommandHandler.RecordCommand()
	return result
}

type JMSSystemSession struct {
	Id                  string
	Prompt              string
	HistoryAsks         []string
	CurrentAskInterrupt bool
	Websocket           *websocket.Conn
	JMSState            *schemas.JMSState
}

func (sh *JMSSystemSession) GetID() string {
	return sh.Id
}

func (sh *JMSSystemSession) CreateNewSession(conn *websocket.Conn, prompt string) *JMSSystemSession {
	id := uuid.New().String()
	return &JMSSystemSession{
		Id:                  id,
		Websocket:           conn,
		CurrentAskInterrupt: false,
		Prompt:              prompt,
		HistoryAsks:         make([]string, 0),
		JMSState: &schemas.JMSState{
			ID: id, ActivateReview: schemas.Wait,
		},
	}
}

func (jmss *JMSSystemSession) ActiveSession() {
	GlobalSystemSessionManager.RegisterSession(jmss)
}

func (jmss *JMSSystemSession) Close(reason string) {
	jmss.CurrentAskInterrupt = true
	GlobalSystemSessionManager.UnregisterSession(jmss)
	jmss.NotifyToClose(reason)
}

func (jmss *JMSSystemSession) NotifyToClose(reason string) {
	response := &schemas.AskResponse{
		Type:           schemas.Finish,
		ConversationID: jmss.Id,
		SystemMessage:  reason,
	}

	jsonResponse, _ := json.Marshal(response)
	_ = jmss.Websocket.WriteMessage(websocket.TextMessage, jsonResponse)
}
