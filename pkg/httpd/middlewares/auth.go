package middlewares

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/jumpserver/kael/pkg/jms"
	"github.com/jumpserver/kael/pkg/logger"
	"go.uber.org/zap"
	"net/http"
	"net/url"
)

func AuthMiddleware() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		reqCookies := ctx.Request.Cookies()
		checkUserHandler := jms.NewCheckUserHandler()
		user, err := checkUserHandler.CheckUserByCookies(reqCookies)
		if err != nil {
			logger.GlobalLogger.Error("Check user cookie failed", zap.Error(err))
			loginUrl := fmt.Sprintf("/core/auth/login/?next=%s", url.QueryEscape(ctx.Request.URL.RequestURI()))
			ctx.Redirect(http.StatusFound, loginUrl)
			ctx.Abort()
			return
		}
		ctx.Set("CONTEXT_USER", user)
	}
}
