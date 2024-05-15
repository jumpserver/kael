package router

import (
	"encoding/json"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/jumpserver/kael/pkg/httpd/ws"
	"github.com/jumpserver/kael/pkg/jms"
	"github.com/jumpserver/kael/pkg/manager"
	"github.com/jumpserver/kael/pkg/schemas"
	"net/http"
)

var SystemChatApi = new(_SystemChatApi)

type _SystemChatApi struct{}

func (s *_SystemChatApi) ChatHandler(ctx *gin.Context) {
	conn, err := ws.UpgradeWsConn(ctx)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "websocket upgrade failed"})
		return
	}

	defer conn.Close()

	currentJMSS := make([]*jms.JMSSystemSession, 0)
	publicSettingHandler := jms.NewPublicSettingHandler()
	publicSetting, err := publicSettingHandler.GetPublicSetting()
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	for {
		messageType, msg, err := conn.ReadMessage()
		if err != nil {
			if len(currentJMSS) != 0 {
				for _, jmss := range currentJMSS {
					jmss.Close("Websocket已关闭 会话中断")
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

			jmss := &jms.JMSSystemSession{}
			conversationID := askRequest.ConversationID
			if conversationID == "" {
				jmss = jmss.CreateNewSession(conn, askRequest.Prompt)
				jmss.ActiveSession()
				currentJMSS = append(currentJMSS, jmss)
			} else {
				jmss, err = jms.GlobalSystemSessionManager.GetSystemSession(conversationID)
				if err != nil {
					sendErrorMessage(conn, "current session not found", conversationID)
					return
				}
			}

			chatGPTParam := &manager.ChatGPTParam{
				AuthToken: publicSetting.GptApiKey,
				BaseURL:   publicSetting.GptBaseUrl,
				Proxy:     publicSetting.GptProxy,
				Model:     publicSetting.GptModel,
				Prompt:    jmss.Prompt,
			}
			id := jmss.GetID()
			wsConn := jmss.Websocket
			currentAskInterrupt := &jmss.CurrentAskInterrupt
			jmss.HistoryAsks = append(jmss.HistoryAsks, askRequest.Content)
			go chatFunc(chatGPTParam, jmss.HistoryAsks, id, wsConn, currentAskInterrupt)()
		}
	}
}
