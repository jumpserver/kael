package grpc

import (
	"fmt"
	"github.com/jumpserver/kael/pkg/config"
	"github.com/jumpserver/kael/pkg/logger"
	pb "github.com/jumpserver/wisp/protobuf-go/protobuf"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var GlobalGrpcClient = &Client{}

type Client struct {
	Conn   *grpc.ClientConn
	Client pb.ServiceClient
}

func (c *Client) Start() {
	conn, err := grpc.Dial(
		fmt.Sprintf("localhost:%s", config.GlobalConfig.WispPort),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		logger.GlobalLogger.Error("grpc client start error", zap.Error(err))
		return
	}
	client := pb.NewServiceClient(conn)
	c.Conn = conn
	c.Client = client
}

func (c *Client) Stop() {
	_ = c.Conn.Close()
}
