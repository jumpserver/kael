package jms

import (
	"context"
	"errors"
	"github.com/jumpserver/kael/pkg/httpd/grpc"
	"github.com/jumpserver/kael/pkg/logger"
	"github.com/jumpserver/wisp/protobuf-go/protobuf"
	"net/http"
)

type CheckUserHandler struct{}

func NewCheckUserHandler() *CheckUserHandler {
	return &CheckUserHandler{}
}

func (th *CheckUserHandler) CheckUserByCookies(reqCookies []*http.Cookie) (*protobuf.User, error) {
	ctx := context.Background()

	var cookies = make([]*protobuf.Cookie, 0)
	for _, cookie := range reqCookies {
		cookies = append(cookies, &protobuf.Cookie{Name: cookie.Name, Value: cookie.Value})
	}

	req := &protobuf.CookiesRequest{Cookies: cookies}
	resp, _ := grpc.GlobalGrpcClient.Client.CheckUserByCookies(ctx, req)
	if !resp.Status.Ok {
		errorMessage := "Failed to check user by cookies: " + resp.Status.Err
		logger.GlobalLogger.Error(errorMessage)
		return nil, errors.New(errorMessage)
	}
	return resp.Data, nil
}
