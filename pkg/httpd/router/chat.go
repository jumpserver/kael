package router

import (
	"encoding/json"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/jumpserver/kael/pkg/httpd/ws"
	"github.com/jumpserver/kael/pkg/jms"
	"github.com/jumpserver/kael/pkg/manager"
	"github.com/jumpserver/kael/pkg/schemas"
	"github.com/sashabaranov/go-openai"
	"net/http"
	"time"
)

var ChatApi = new(_ChatApi)

type _ChatApi struct{}

func (s *_ChatApi) ChatHandler(ctx *gin.Context) {
	remoteIP := ctx.ClientIP()
	conn, err := ws.UpgradeWsConn(ctx)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "websocket upgrade failed"})
		return
	}

	defer conn.Close()

	token, ok := ctx.GetQuery("token")
	if !ok {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "token required"})
		return
	}

	currentJMSS := make([]*jms.JMSSession, 0)
	tokenHandler := jms.NewTokenHandler()
	sessionHandler := jms.NewSessionHandler(conn, remoteIP)
	authInfo, err := tokenHandler.GetTokenAuthInfo(token)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	for {
		messageType, msg, err := conn.ReadMessage()
		if err != nil {
			if len(currentJMSS) != 0 {
				for _, jmss := range currentJMSS {
					jmss.Close("Websocket已关闭, 会话中断")
				}
			}
			return
		}

		if messageType == websocket.TextMessage {
			if string(msg) == "ping" {
				_ = conn.WriteMessage(websocket.TextMessage, []byte("pong"))
				continue
			}

			var askRequest schemas.AskRequest
			_ = json.Unmarshal(msg, &askRequest)

			jmss := &jms.JMSSession{}
			conversationID := askRequest.ConversationID
			if conversationID == "" {
				jmss = sessionHandler.CreateNewSession(authInfo, askRequest.Prompt)
				jmss.ActiveSession()
				currentJMSS = append(currentJMSS, jmss)
			} else {
				jmss, err = jms.GlobalSessionManager.GetSession(conversationID)
				if err != nil {
					sendErrorMessage(conn, "current session not found", conversationID)
					return
				} else {
					jmss.JMSState.NewDialogue = true
				}
			}

			chatGPTParam := &manager.ChatGPTParam{
				AuthToken: authInfo.Account.Secret,
				BaseURL:   authInfo.Asset.Address,
				Proxy:     authInfo.Asset.Specific.HttpProxy,
				Model:     authInfo.Platform.Protocols[0].Settings["api_mode"],
				Prompt:    jmss.Prompt,
			}

			id := jmss.GetID()
			wsConn := jmss.Websocket
			currentAskInterrupt := &jmss.CurrentAskInterrupt
			jmss.HistoryAsks = append(jmss.HistoryAsks, askRequest.Content)
			go jmss.WithAudit(
				askRequest.Content,
				chatFunc(
					chatGPTParam, jmss.HistoryAsks,
					id, wsConn, currentAskInterrupt,
				),
			)
		}
	}
}

func chatFunc(
	chatGPTParam *manager.ChatGPTParam, historyAsks []string,
	id string, wsConn *websocket.Conn, currentAskInterrupt *bool,
) func() string {
	return func() string {
		doneCh := make(chan string)
		answerCh := make(chan string)
		defer close(doneCh)
		defer close(answerCh)

		c := manager.NewClient(
			chatGPTParam.AuthToken,
			chatGPTParam.BaseURL,
			chatGPTParam.Proxy,
		)

		askChatGPT := &manager.AskChatGPT{
			Client:   c,
			Model:    chatGPTParam.Model,
			Contents: historyAsks,
			AnswerCh: answerCh,
			DoneCh:   doneCh,
		}

		go manager.ChatGPT(askChatGPT, chatGPTParam.Prompt, currentAskInterrupt)
		return processChatMessages(id, answerCh, doneCh, wsConn)
	}
}

func processChatMessages(
	id string, answerCh <-chan string, doneCh <-chan string, wsConn *websocket.Conn,
) string {
	messageID := uuid.New()

	for {
		select {
		case answer := <-answerCh:
			sendChatResponse(id, wsConn, answer, messageID, schemas.Message)
		case answer := <-doneCh:
			sendChatResponse(id, wsConn, answer, messageID, schemas.Finish)
			return answer
		}
	}
}

func sendErrorMessage(conn *websocket.Conn, message, conversationID string) {
	response := schemas.AskResponse{
		Type:           schemas.Error,
		ConversationID: conversationID,
		SystemMessage:  message,
	}
	jsonResponse, _ := json.Marshal(response)
	_ = conn.WriteMessage(websocket.TextMessage, jsonResponse)
}

func sendChatResponse(
	id string, ws *websocket.Conn, chatContent string,
	messageID uuid.UUID, messageType schemas.AskResponseType) {
	response := schemas.AskResponse{
		Type:           schemas.Message,
		ConversationID: id,
		Message: &schemas.ChatGPTMessage{
			Content:    chatContent,
			ID:         messageID,
			CreateTime: time.Now(),
			Type:       messageType,
			Role:       openai.ChatMessageRoleAssistant,
		},
	}
	jsonResponse, _ := json.Marshal(response)
	_ = ws.WriteMessage(websocket.TextMessage, jsonResponse)
}
