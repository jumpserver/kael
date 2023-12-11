package router

import (
	"errors"
	"github.com/gin-gonic/gin"
	"github.com/jumpserver/kael/pkg/jms"
	"github.com/jumpserver/kael/pkg/schemas"
	"net/http"
)

var HandlerApi = new(_HandlerApi)

type _HandlerApi struct{}

func GetSession(sessionID string) (*jms.JMSSession, error) {
	jmsSession, _ := jms.GlobalSessionManager.GetSession(sessionID)
	if jmsSession != nil {
		return jmsSession, nil
	}
	return nil, errors.New("not found conversation")
}

func (s *_HandlerApi) InterruptCurrentAskHandler(ctx *gin.Context) {
	var conversation schemas.Conversation
	if err := ctx.ShouldBindJSON(&conversation); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid data"})
		return
	}

	jmsSession, err := GetSession(conversation.ID)
	if err != nil {
		ctx.JSON(http.StatusNotFound, gin.H{"message": err.Error()})
		return
	}
	jmsSession.CurrentAskInterrupt = true
	ctx.JSON(http.StatusOK, gin.H{"message": "Success"})
}

func (s *_HandlerApi) JmsStateHandler(ctx *gin.Context) {
	var jmsState schemas.JMSState
	if err := ctx.ShouldBindJSON(&jmsState); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid data"})
		return
	}
	jmsSession, err := GetSession(jmsState.ID)
	if err != nil {
		ctx.JSON(http.StatusNotFound, gin.H{"message": err.Error()})
		return
	}

	jmsSession.JMSState.ActivateReview = jmsState.ActivateReview
	ctx.JSON(http.StatusOK, gin.H{"message": "Success"})
}
