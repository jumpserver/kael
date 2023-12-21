package jms

import (
	"context"
	"errors"
	"github.com/jumpserver/kael/pkg/httpd/grpc"
	"github.com/jumpserver/kael/pkg/logger"
	"github.com/jumpserver/wisp/protobuf-go/protobuf"
)

type PublicSettingHandler struct{}

func NewPublicSettingHandler() *PublicSettingHandler {
	return &PublicSettingHandler{}
}

func (th *PublicSettingHandler) GetPublicSetting() (*protobuf.PublicSetting, error) {
	ctx := context.Background()
	req := &protobuf.Empty{}

	resp, _ := grpc.GlobalGrpcClient.Client.GetPublicSetting(ctx, req)
	if !resp.Status.Ok {
		errorMessage := "Failed to get public setting: " + resp.Status.Err
		logger.GlobalLogger.Error(errorMessage)
		return nil, errors.New(errorMessage)
	}
	return resp.Data, nil
}
