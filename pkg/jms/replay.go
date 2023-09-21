package jms

import (
	"context"
	"fmt"
	"github.com/jumpserver/kael/pkg/config"
	"github.com/jumpserver/kael/pkg/httpd/grpc"
	"github.com/jumpserver/kael/pkg/logger"
	"github.com/jumpserver/wisp/protobuf-go/protobuf"
	"go.uber.org/zap"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type ReplayHandler struct {
	Session      *protobuf.Session
	ReplayWriter *AsciinemaWriter
	FileWriter   *os.File
	File         *os.File
}

func NewReplayHandler(session *protobuf.Session) *ReplayHandler {
	handler := &ReplayHandler{
		Session: session,
	}
	handler.buildFile()
	return handler
}

func (rh *ReplayHandler) buildFile() {
	rh.ensureReplayDir()
	replayFilePath := filepath.Join(config.GlobalConfig.ReplayFolderPath, fmt.Sprintf("%s.cast", rh.Session.Id))
	file, err := os.Create(replayFilePath)
	if err != nil {
		logger.GlobalLogger.Error("Failed to create replay file", zap.Error(err))
		return
	}
	rh.File = file
	rh.FileWriter = file
	rh.ReplayWriter = NewAsciinemaWriter(file)
	rh.ReplayWriter.writeHeader()
}

func (rh *ReplayHandler) ensureReplayDir() {
	err := os.MkdirAll(config.GlobalConfig.ReplayFolderPath, os.ModePerm)
	if err != nil {
		logger.GlobalLogger.Error("Failed to create replay directory")
	}
}

func (rh *ReplayHandler) writeRow(row string) {
	row = strings.ReplaceAll(row, "\n", "\r\n")
	row = strings.ReplaceAll(row, "\r\r\n", "\r\n")
	row = fmt.Sprintf("%s \r\n", row)
	rh.ReplayWriter.writeRow([]byte(row))
}

func (rh *ReplayHandler) WriteInput(inputStr string) {
	// TODO 后续时间处理要统一
	currentTime := time.Now()
	formattedTime := currentTime.Format("2006-01-02 15:04:05")
	inputStr = fmt.Sprintf("[%s]#: %s", formattedTime, inputStr)
	rh.writeRow(inputStr)
}

func (rh *ReplayHandler) WriteOutput(outputStr string) {
	wrappedText := wrapText(outputStr, Width)
	outputStr = "\r\n" + wrappedText + "\r\n"
	rh.writeRow(outputStr)

}

func (rh *ReplayHandler) Upload() {
	defer rh.FileWriter.Close()

	replayFilePath := rh.File.Name()
	if _, err := os.Stat(replayFilePath); os.IsNotExist(err) {
		return
	}

	ctx := context.Background()
	replayRequest := &protobuf.ReplayRequest{
		SessionId:      rh.Session.Id,
		ReplayFilePath: replayFilePath,
	}
	resp, _ := grpc.GlobalGrpcClient.Client.UploadReplayFile(ctx, replayRequest)
	if !resp.Status.Ok {
		errorMessage := fmt.Sprintf(
			"Failed to upload replay file: %s %s",
			rh.File.Name(),
			resp.Status.Err,
		)
		logger.GlobalLogger.Error(errorMessage)
	}
}

func wrapText(text string, width int) string {
	var wrappedTextBuilder strings.Builder
	words := strings.Fields(text)
	currentLineLength := 0

	for _, word := range words {
		wordLength := len(word)

		if currentLineLength+wordLength > width {
			wrappedTextBuilder.WriteString("\r\n" + word + " ")
			currentLineLength = wordLength + 1
		} else {
			wrappedTextBuilder.WriteString(word + " ")
			currentLineLength += wordLength + 1
		}
	}

	return wrappedTextBuilder.String()
}
