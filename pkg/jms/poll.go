package jms

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/gorilla/websocket"
	"github.com/jumpserver/kael/pkg/config"
	"github.com/jumpserver/kael/pkg/httpd/grpc"
	"github.com/jumpserver/kael/pkg/logger"
	"github.com/jumpserver/kael/pkg/schemas"
	"github.com/jumpserver/wisp/protobuf-go/protobuf"
	"go.uber.org/zap"
)

type PollJMSEvent struct{}

func NewPollJMSEvent() *PollJMSEvent {
	return &PollJMSEvent{}
}

func (p *PollJMSEvent) clearZombieSession() {
	ctx := context.Background()
	req := &protobuf.RemainReplayRequest{
		ReplayDir: config.GlobalConfig.ReplayFolderPath,
	}

	resp, err := grpc.GlobalGrpcClient.Client.ScanRemainReplays(ctx, req)
	if err != nil || !resp.Status.Ok {
		logger.GlobalLogger.Error("Failed to scan remain replay")
	}
}

func (p *PollJMSEvent) waitForKillSessionMessage() {
	stream, err := grpc.GlobalGrpcClient.Client.DispatchTask(context.Background())
	if err != nil {
		logger.GlobalLogger.Error("dispatch task err", zap.Error(err))
		return
	}
	logger.GlobalLogger.Info("start dispatch task success")

	closeStreamChan := make(chan struct{})
	for {
		taskResponse, err := stream.Recv()

		if err != nil {
			_ = stream.CloseSend()
			close(closeStreamChan)
			break
		}

		task := taskResponse.Task
		sessionId := task.SessionId
		taskAction := task.Action
		targetSession := GlobalSessionManager.GetJMSSession(sessionId)
		if targetSession != nil {
			switch taskAction {
			case protobuf.TaskAction_KillSession:
				reason := "当前会话已被中断"
				p.sendFinishTask(stream, task.Id)
				p.sendFinishSession(task.SessionId)
				targetSession.Close(reason)
			case protobuf.TaskAction_LockSession:
				msg := "当前会话已被锁定"
				p.sendFinishTask(stream, task.Id)
				p.sendSessionState(targetSession, schemas.LockSession, msg)
			case protobuf.TaskAction_UnlockSession:
				msg := "当前会话已解锁"
				p.sendFinishTask(stream, task.Id)
				p.sendSessionState(targetSession, schemas.UnlockSession, msg)
			}
		}
	}
	<-closeStreamChan
	p.waitForKillSessionMessage()
}
func (p *PollJMSEvent) sendFinishTask(stream protobuf.Service_DispatchTaskClient, TaskId string) {
	req := &protobuf.FinishedTaskRequest{
		TaskId: TaskId,
	}

	_ = stream.Send(req)
}

func (p *PollJMSEvent) sendFinishSession(SessionId string) {
	req := &protobuf.SessionFinishRequest{
		Id: SessionId,
	}

	resp, _ := grpc.GlobalGrpcClient.Client.FinishSession(context.Background(), req)
	if !resp.Status.Ok {
		errorMessage := fmt.Sprintf("Failed to finish session: %s", resp.Status.Err)
		logger.GlobalLogger.Error(errorMessage)
	}
}

func (p *PollJMSEvent) sendSessionState(jmss *JMSSession, state schemas.SessionStateType, msg string) {
	response := &schemas.AskResponse{
		Type:           schemas.Waiting,
		ConversationID: jmss.Session.Id,
		SystemMessage:  msg,
		Meta:           schemas.ResponseMeta{SessionState: state},
	}

	jsonResponse, _ := json.Marshal(response)
	_ = jmss.Websocket.WriteMessage(websocket.TextMessage, jsonResponse)
}

func (p *PollJMSEvent) start() {
	p.clearZombieSession()
	p.waitForKillSessionMessage()
}

func SetupPollJMSEvent() {
	jmsEvent := NewPollJMSEvent()
	go jmsEvent.start()
}
