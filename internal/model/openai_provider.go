package model

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/packages/respjson"
	"github.com/openai/openai-go/responses"
	"github.com/openai/openai-go/shared"
)

type HTTPProvider struct {
	config HTTPConfig
	client openai.Client
	info   Info
}

func NewHTTPProvider(config HTTPConfig) (*HTTPProvider, error) {
	endpoint, endpointErr := url.Parse(config.BaseURL)
	if endpointErr != nil || endpoint.Scheme != "http" && endpoint.Scheme != "https" || endpoint.Host == "" || endpoint.User != nil || endpoint.RawQuery != "" || endpoint.Fragment != "" || config.Model == "" {
		return nil, fmt.Errorf("model endpoint is incomplete")
	}
	config.BaseURL = strings.TrimRight(config.BaseURL, "/")
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	transport.DialContext = (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext
	if strings.TrimSpace(config.Proxy) != "" {
		parsed, err := url.Parse(config.Proxy)
		if err != nil || parsed.Scheme != "http" && parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil {
			return nil, fmt.Errorf("model proxy is invalid")
		}
		transport.Proxy = http.ProxyURL(parsed)
	} else {
		transport.Proxy = nil
	}
	httpClient := &http.Client{Transport: transport, Timeout: config.Timeout}
	sdkClient := openai.NewClient(
		option.WithAPIKey(config.APIKey),
		option.WithBaseURL(config.BaseURL),
		option.WithHTTPClient(httpClient),
		option.WithMaxRetries(0),
	)
	provider := strings.ToLower(strings.TrimSpace(config.Provider))
	capabilities := Capabilities{ChatCompletions: true, NativeTools: true, StructuredOutput: true}
	if provider == "openai" || provider == "gpt" {
		capabilities.Responses, capabilities.PreviousResponse, capabilities.Reasoning = true, true, true
	}
	if provider == "deepseek" || provider == "deep-seek" {
		capabilities.Reasoning = true
	}
	return &HTTPProvider{config: config, client: sdkClient, info: Info{Provider: provider, Model: config.Model, Capabilities: capabilities}}, nil
}

func (p *HTTPProvider) Info() Info { return p.info }

func (p *HTTPProvider) Complete(ctx context.Context, request Request) (Result, error) {
	if p.info.Capabilities.Responses {
		result, err := p.completeResponses(ctx, request)
		if err == nil {
			return result, nil
		}
		var providerErr *ProviderError
		if !errors.As(err, &providerErr) || providerErr.Status != http.StatusNotFound && providerErr.Status != http.StatusBadRequest {
			return Result{}, err
		}
	}
	return p.completeChat(ctx, request)
}

func (p *HTTPProvider) CompleteStream(ctx context.Context, request Request, onDelta func(string) error) (Result, error) {
	if onDelta == nil {
		return p.Complete(ctx, request)
	}
	if p.info.Capabilities.Responses {
		result, err := p.streamResponses(ctx, request, onDelta)
		if err == nil {
			return result, nil
		}
		var providerErr *ProviderError
		if !errors.As(err, &providerErr) || providerErr.Status != http.StatusNotFound && providerErr.Status != http.StatusBadRequest {
			return Result{}, err
		}
	}
	return p.streamChat(ctx, request, onDelta)
}

func (p *HTTPProvider) completeResponses(ctx context.Context, request Request) (Result, error) {
	var httpResponse *http.Response
	response, err := p.client.Responses.New(ctx, p.responsesParams(request), option.WithResponseInto(&httpResponse))
	if err != nil {
		return Result{}, mapProviderError(err, "model request failed")
	}
	result, err := resultFromResponse(response)
	result.RequestID = requestID(httpResponse)
	return result, err
}

func (p *HTTPProvider) streamResponses(ctx context.Context, request Request, onDelta func(string) error) (Result, error) {
	var httpResponse *http.Response
	stream := p.client.Responses.NewStreaming(ctx, p.responsesParams(request), option.WithResponseInto(&httpResponse))
	defer stream.Close()
	var final *responses.Response
	emitted := false
	for stream.Next() {
		switch event := stream.Current().AsAny().(type) {
		case responses.ResponseTextDeltaEvent:
			if event.Delta != "" {
				if err := onDelta(event.Delta); err != nil {
					return Result{}, err
				}
				emitted = true
			}
		case responses.ResponseCompletedEvent:
			value := event.Response
			final = &value
		case responses.ResponseIncompleteEvent:
			value := event.Response
			final = &value
		case responses.ResponseFailedEvent:
			return Result{}, responseFailure(event.Response)
		case responses.ResponseErrorEvent:
			return Result{}, &ProviderError{Kind: ErrorUnavailable, Message: boundedProviderMessage(event.Message)}
		}
	}
	if err := stream.Err(); err != nil {
		return Result{}, mapProviderError(err, "model stream request failed")
	}
	if final == nil {
		return Result{}, &ProviderError{Kind: ErrorInvalidOutput, Message: "model stream ended without a completed response"}
	}
	result, err := resultFromResponse(final)
	result.RequestID, result.DeltasEmitted = requestID(httpResponse), emitted
	return result, err
}

func (p *HTTPProvider) responsesParams(request Request) responses.ResponseNewParams {
	input := make(responses.ResponseInputParam, 0, len(request.Messages))
	for _, message := range request.Messages {
		switch {
		case message.Role == "assistant" && message.ToolCallID != "":
			input = append(input, responses.ResponseInputItemParamOfFunctionCall(string(message.ToolArguments), message.ToolCallID, message.ToolName))
		case message.Role == "tool":
			input = append(input, responses.ResponseInputItemParamOfFunctionCallOutput(message.ToolCallID, message.Content))
		default:
			role := responses.EasyInputMessageRoleUser
			if message.Role == "assistant" {
				role = responses.EasyInputMessageRoleAssistant
			}
			if content := responseMessageContent(message); content != nil {
				input = append(input, responses.ResponseInputItemParamOfMessage(content, role))
			} else {
				input = append(input, responses.ResponseInputItemParamOfMessage(message.Content, role))
			}
		}
	}
	params := responses.ResponseNewParams{
		Model: shared.ResponsesModel(p.config.Model),
		Input: responses.ResponseNewParamsInputUnion{OfInputItemList: input},
		Store: openai.Bool(p.config.Store),
	}
	if request.System != "" {
		params.Instructions = openai.String(request.System)
	}
	if request.PreviousResponseID != "" && p.config.Store {
		params.PreviousResponseID = openai.String(request.PreviousResponseID)
	}
	if effort := strings.TrimSpace(p.config.ReasoningEffort); effort != "" {
		params.Reasoning = shared.ReasoningParam{Effort: shared.ReasoningEffort(effort)}
	}
	if !request.DisableTools && len(request.Tools) > 0 {
		params.ParallelToolCalls = openai.Bool(false)
		params.Tools = responseTools(request.Tools)
		if request.RequiredTool != "" {
			params.ToolChoice = responses.ResponseNewParamsToolChoiceUnion{OfFunctionTool: &responses.ToolChoiceFunctionParam{Name: request.RequiredTool}}
		} else {
			params.ToolChoice = responses.ResponseNewParamsToolChoiceUnion{OfToolChoiceMode: openai.Opt(responses.ToolChoiceOptionsAuto)}
		}
	}
	return params
}

func responseMessageContent(message Message) responses.ResponseInputMessageContentListParam {
	if len(message.Parts) == 0 || message.Role == "assistant" {
		return nil
	}
	content := make(responses.ResponseInputMessageContentListParam, 0, len(message.Parts))
	for _, part := range message.Parts {
		switch part.Type {
		case "text":
			if part.Text != "" {
				content = append(content, responses.ResponseInputContentParamOfInputText(part.Text))
			}
		case "image":
			if part.Data != "" && strings.HasPrefix(part.MediaType, "image/") {
				item := responses.ResponseInputContentParamOfInputImage(responses.ResponseInputImageDetailAuto)
				item.OfInputImage.ImageURL = openai.String("data:" + part.MediaType + ";base64," + part.Data)
				content = append(content, item)
			}
		}
	}
	if len(content) == 0 {
		return nil
	}
	return content
}

func responseTools(tools []Tool) []responses.ToolUnionParam {
	result := make([]responses.ToolUnionParam, 0, len(tools))
	for _, tool := range tools {
		item := responses.ToolParamOfFunction(tool.Name, tool.Parameters, true)
		if tool.Description != "" {
			item.OfFunction.Description = openai.String(tool.Description)
		}
		result = append(result, item)
	}
	return result
}

func resultFromResponse(response *responses.Response) (Result, error) {
	if response == nil {
		return Result{}, &ProviderError{Kind: ErrorInvalidOutput, Message: "model returned no response"}
	}
	if response.Status == responses.ResponseStatusFailed {
		return Result{}, responseFailure(*response)
	}
	finishReason := string(response.Status)
	if response.Status == responses.ResponseStatusIncomplete && response.IncompleteDetails.Reason != "" {
		finishReason = response.IncompleteDetails.Reason
	}
	result := Result{
		Content:      response.OutputText(),
		FinishReason: finishReason,
		ResponseID:   response.ID,
		Usage: Usage{
			InputTokens:     response.Usage.InputTokens,
			OutputTokens:    response.Usage.OutputTokens,
			ReasoningTokens: response.Usage.OutputTokensDetails.ReasoningTokens,
			CachedTokens:    response.Usage.InputTokensDetails.CachedTokens,
		},
	}
	for _, output := range response.Output {
		if output.Type == "function_call" && result.ToolCall == nil {
			callID := output.CallID
			if callID == "" {
				callID = output.ID
			}
			result.ToolCall = &ToolCall{ID: callID, Name: output.Name, Arguments: normalizeArguments(json.RawMessage(output.Arguments))}
		}
		if output.Type == "reasoning" {
			for _, summary := range output.Summary {
				result.Reasoning += summary.Text
			}
		}
	}
	if result.Content == "" && result.ToolCall == nil {
		return Result{}, &ProviderError{Kind: ErrorInvalidOutput, Message: "model returned no usable output"}
	}
	return result, nil
}

func responseFailure(response responses.Response) error {
	message := response.Error.Message
	if message == "" {
		message = "model response failed"
	}
	kind, retryable := ErrorUnavailable, true
	if response.Error.Code == responses.ResponseErrorCodeRateLimitExceeded {
		kind = ErrorRateLimit
	}
	if response.Error.Code == responses.ResponseErrorCodeInvalidPrompt {
		kind, retryable = ErrorInvalidRequest, false
	}
	return &ProviderError{Kind: kind, Retryable: retryable, Message: boundedProviderMessage(message)}
}

func (p *HTTPProvider) completeChat(ctx context.Context, request Request) (Result, error) {
	var httpResponse *http.Response
	response, err := p.client.Chat.Completions.New(ctx, p.chatParams(request), option.WithResponseInto(&httpResponse))
	if err != nil {
		return Result{}, mapProviderError(err, "model request failed")
	}
	result, err := resultFromChat(response)
	result.RequestID = requestID(httpResponse)
	return result, err
}

func (p *HTTPProvider) streamChat(ctx context.Context, request Request, onDelta func(string) error) (Result, error) {
	params := p.chatParams(request)
	params.StreamOptions = openai.ChatCompletionStreamOptionsParam{IncludeUsage: openai.Bool(true)}
	var httpResponse *http.Response
	stream := p.client.Chat.Completions.NewStreaming(ctx, params, option.WithResponseInto(&httpResponse))
	defer stream.Close()
	accumulator := openai.ChatCompletionAccumulator{}
	var usage openai.CompletionUsage
	reasoning := strings.Builder{}
	emitted := false
	for stream.Next() {
		chunk := stream.Current()
		if chunk.ID == "" && accumulator.ID != "" {
			chunk.ID = accumulator.ID
		}
		if !accumulator.AddChunk(chunk) {
			return Result{}, &ProviderError{Kind: ErrorInvalidOutput, Message: "model stream chunks are inconsistent"}
		}
		if chunk.JSON.Usage.Valid() {
			usage = chunk.Usage
		}
		for _, choice := range chunk.Choices {
			delta := choice.Delta.Content
			if delta == "" {
				delta = choice.Delta.Refusal
			}
			if delta != "" {
				if err := onDelta(delta); err != nil {
					return Result{}, err
				}
				emitted = true
			}
			reasoning.WriteString(extraString(choice.Delta.JSON.ExtraFields, "reasoning_content"))
		}
	}
	if err := stream.Err(); err != nil {
		return Result{}, mapProviderError(err, "model stream request failed")
	}
	result, err := resultFromChat(&accumulator.ChatCompletion)
	result.RequestID, result.DeltasEmitted = requestID(httpResponse), emitted
	result.Reasoning = reasoning.String()
	if usage.PromptTokens != 0 || usage.CompletionTokens != 0 {
		result.Usage = chatUsage(usage)
	}
	return result, err
}

func (p *HTTPProvider) chatParams(request Request) openai.ChatCompletionNewParams {
	messages := make([]openai.ChatCompletionMessageParamUnion, 0, len(request.Messages)+1)
	if request.System != "" {
		messages = append(messages, openai.SystemMessage(request.System))
	}
	for _, message := range request.Messages {
		messages = append(messages, chatMessage(message))
	}
	params := openai.ChatCompletionNewParams{Model: p.config.Model, Messages: messages}
	if effort := strings.TrimSpace(p.config.ReasoningEffort); effort != "" {
		params.ReasoningEffort = shared.ReasoningEffort(effort)
	}
	if !request.DisableTools && len(request.Tools) > 0 {
		params.ParallelToolCalls = openai.Bool(false)
		params.Tools = chatTools(request.Tools)
		if request.RequiredTool != "" {
			params.ToolChoice = openai.ChatCompletionToolChoiceOptionParamOfChatCompletionNamedToolChoice(openai.ChatCompletionNamedToolChoiceFunctionParam{Name: request.RequiredTool})
		} else {
			params.ToolChoice = openai.ChatCompletionToolChoiceOptionUnionParam{OfAuto: openai.String("auto")}
		}
	}
	return params
}

func chatMessage(message Message) openai.ChatCompletionMessageParamUnion {
	switch message.Role {
	case "assistant":
		if message.ToolCallID != "" {
			return openai.ChatCompletionMessageParamUnion{OfAssistant: &openai.ChatCompletionAssistantMessageParam{
				ToolCalls: []openai.ChatCompletionMessageToolCallParam{{
					ID: message.ToolCallID,
					Function: openai.ChatCompletionMessageToolCallFunctionParam{
						Name:      message.ToolName,
						Arguments: string(message.ToolArguments),
					},
				}},
			}}
		}
		return openai.AssistantMessage(message.Content)
	case "tool":
		return openai.ToolMessage(message.Content, message.ToolCallID)
	case "system":
		return openai.SystemMessage(message.Content)
	default:
		if content := chatMessageContent(message); content != nil {
			return openai.UserMessage(content)
		}
		return openai.UserMessage(message.Content)
	}
}

func chatMessageContent(message Message) []openai.ChatCompletionContentPartUnionParam {
	if len(message.Parts) == 0 {
		return nil
	}
	content := make([]openai.ChatCompletionContentPartUnionParam, 0, len(message.Parts))
	for _, part := range message.Parts {
		switch part.Type {
		case "text":
			if part.Text != "" {
				content = append(content, openai.TextContentPart(part.Text))
			}
		case "image":
			if part.Data != "" && strings.HasPrefix(part.MediaType, "image/") {
				content = append(content, openai.ImageContentPart(openai.ChatCompletionContentPartImageImageURLParam{URL: "data:" + part.MediaType + ";base64," + part.Data}))
			}
		}
	}
	if len(content) == 0 {
		return nil
	}
	return content
}

func chatTools(tools []Tool) []openai.ChatCompletionToolParam {
	result := make([]openai.ChatCompletionToolParam, 0, len(tools))
	for _, tool := range tools {
		definition := shared.FunctionDefinitionParam{Name: tool.Name, Parameters: tool.Parameters, Strict: openai.Bool(true)}
		if tool.Description != "" {
			definition.Description = openai.String(tool.Description)
		}
		result = append(result, openai.ChatCompletionToolParam{Function: definition})
	}
	return result
}

func resultFromChat(response *openai.ChatCompletion) (Result, error) {
	if response == nil || len(response.Choices) == 0 {
		return Result{}, &ProviderError{Kind: ErrorInvalidOutput, Message: "model returned no choices"}
	}
	choice := response.Choices[0]
	content := choice.Message.Content
	if content == "" {
		content = choice.Message.Refusal
	}
	result := Result{
		Content:      content,
		Reasoning:    extraString(choice.Message.JSON.ExtraFields, "reasoning_content"),
		FinishReason: choice.FinishReason,
		ResponseID:   response.ID,
		Usage:        chatUsage(response.Usage),
	}
	if len(choice.Message.ToolCalls) > 0 {
		call := choice.Message.ToolCalls[0]
		result.ToolCall = &ToolCall{ID: call.ID, Name: call.Function.Name, Arguments: normalizeArguments(json.RawMessage(call.Function.Arguments))}
	}
	if result.Content == "" && result.ToolCall == nil {
		return Result{}, &ProviderError{Kind: ErrorInvalidOutput, Message: "model returned no usable output"}
	}
	return result, nil
}

func chatUsage(usage openai.CompletionUsage) Usage {
	return Usage{
		InputTokens:     usage.PromptTokens,
		OutputTokens:    usage.CompletionTokens,
		ReasoningTokens: usage.CompletionTokensDetails.ReasoningTokens,
		CachedTokens:    usage.PromptTokensDetails.CachedTokens,
	}
}

func extraString(fields map[string]respjson.Field, key string) string {
	field, ok := fields[key]
	if !ok {
		return ""
	}
	var value string
	_ = json.Unmarshal([]byte(field.Raw()), &value)
	return value
}

func normalizeArguments(value json.RawMessage) json.RawMessage {
	if len(value) == 0 || string(value) == "null" {
		return json.RawMessage(`{}`)
	}
	var encoded string
	if json.Unmarshal(value, &encoded) == nil {
		return json.RawMessage(encoded)
	}
	return value
}

func requestID(response *http.Response) string {
	if response == nil {
		return ""
	}
	return response.Header.Get("x-request-id")
}

func mapProviderError(err error, fallback string) error {
	if err == nil {
		return nil
	}
	var apiError *openai.Error
	if errors.As(err, &apiError) {
		kind, retryable := classifyStatus(apiError.StatusCode)
		message := apiError.Message
		if message == "" {
			message = fallback
		}
		return &ProviderError{Kind: kind, Retryable: retryable, Status: apiError.StatusCode, Message: boundedProviderMessage(message), cause: err}
	}
	kind, retryable := ErrorUnavailable, true
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		kind = ErrorTimeout
	}
	var networkError net.Error
	if errors.As(err, &networkError) && networkError.Timeout() {
		kind = ErrorTimeout
	}
	return &ProviderError{Kind: kind, Retryable: retryable, Message: fallback, cause: err}
}

func boundedProviderMessage(value string) string {
	if len(value) > 1024 {
		return value[:1024]
	}
	return value
}

func classifyStatus(status int) (ErrorKind, bool) {
	switch status {
	case http.StatusUnauthorized, http.StatusForbidden:
		return ErrorAuthentication, false
	case http.StatusTooManyRequests:
		return ErrorRateLimit, true
	case http.StatusRequestTimeout, http.StatusGatewayTimeout:
		return ErrorTimeout, true
	case http.StatusBadRequest, http.StatusUnprocessableEntity:
		return ErrorInvalidRequest, false
	default:
		return ErrorUnavailable, status >= 500
	}
}
