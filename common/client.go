package common

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"one-api/types"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/net/proxy"
)

var clientPool = &sync.Pool{
	New: func() interface{} {
		return &http.Client{}
	},
}

func GetHttpClient(proxyAddr string) *http.Client {
	client := clientPool.Get().(*http.Client)

	if RelayTimeout > 0 {
		client.Timeout = time.Duration(RelayTimeout) * time.Second
	}

	if proxyAddr != "" {
		proxyURL, err := url.Parse(proxyAddr)
		if err != nil {
			SysError("Error parsing proxy address: " + err.Error())
			return client
		}

		switch proxyURL.Scheme {
		case "http", "https":
			client.Transport = &http.Transport{
				Proxy: http.ProxyURL(proxyURL),
			}
		case "socks5":
			dialer, err := proxy.SOCKS5("tcp", proxyURL.Host, nil, proxy.Direct)
			if err != nil {
				SysError("Error creating SOCKS5 dialer: " + err.Error())
				return client
			}
			client.Transport = &http.Transport{
				Dial: dialer.Dial,
			}
		default:
			SysError("Unsupported proxy scheme: " + proxyURL.Scheme)
		}
	}

	return client

}

func PutHttpClient(c *http.Client) {
	clientPool.Put(c)
}

type Client struct {
	requestBuilder    RequestBuilder
	CreateFormBuilder func(io.Writer) FormBuilder
}

func NewClient() *Client {
	return &Client{
		requestBuilder: NewRequestBuilder(),
		CreateFormBuilder: func(body io.Writer) FormBuilder {
			return NewFormBuilder(body)
		},
	}
}

type requestOptions struct {
	body   any
	header http.Header
}

type requestOption func(*requestOptions)

type Stringer interface {
	GetString() *string
}

func WithBody(body any) requestOption {
	return func(args *requestOptions) {
		args.body = body
	}
}

func WithHeader(header map[string]string) requestOption {
	return func(args *requestOptions) {
		for k, v := range header {
			args.header.Set(k, v)
		}
	}
}

func WithContentType(contentType string) requestOption {
	return func(args *requestOptions) {
		args.header.Set("Content-Type", contentType)
	}
}

type RequestError struct {
	HTTPStatusCode int
	Err            error
}

func (c *Client) NewRequest(method, url string, setters ...requestOption) (*http.Request, error) {
	// Default Options
	args := &requestOptions{
		body:   nil,
		header: make(http.Header),
	}
	for _, setter := range setters {
		setter(args)
	}
	req, err := c.requestBuilder.Build(method, url, args.body, args.header)
	if err != nil {
		return nil, err
	}

	return req, nil
}

func SendRequest(req *http.Request, response any, outputResp bool, proxyAddr string) (*http.Response, *types.OpenAIErrorWithStatusCode) {
	// 发送请求
	client := GetHttpClient(proxyAddr)
	resp, err := client.Do(req)
	if err != nil {
		return nil, ErrorWrapper(err, "http_request_failed", http.StatusInternalServerError)
	}
	PutHttpClient(client)

	if !outputResp {
		defer resp.Body.Close()
	}

	// 处理响应
	if IsFailureStatusCode(resp) {
		return nil, HandleErrorResp(resp)
	}

	// 解析响应
	if outputResp {
		var buf bytes.Buffer
		tee := io.TeeReader(resp.Body, &buf)
		err = DecodeResponse(tee, response)

		// 将响应体重新写入 resp.Body
		resp.Body = io.NopCloser(&buf)
	} else {
		err = DecodeResponse(resp.Body, response)
	}
	if err != nil {
		return nil, ErrorWrapper(err, "decode_response_failed", http.StatusInternalServerError)
	}

	if outputResp {
		return resp, nil
	}

	return nil, nil
}

type GeneralErrorResponse struct {
	Error    types.OpenAIError `json:"error"`
	Message  string            `json:"message"`
	Msg      string            `json:"msg"`
	Err      string            `json:"err"`
	ErrorMsg string            `json:"error_msg"`
	Header   struct {
		Message string `json:"message"`
	} `json:"header"`
	Response struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	} `json:"response"`
}

func (e GeneralErrorResponse) ToMessage() string {
	if e.Error.Message != "" {
		return e.Error.Message
	}
	if e.Message != "" {
		return e.Message
	}
	if e.Msg != "" {
		return e.Msg
	}
	if e.Err != "" {
		return e.Err
	}
	if e.ErrorMsg != "" {
		return e.ErrorMsg
	}
	if e.Header.Message != "" {
		return e.Header.Message
	}
	if e.Response.Error.Message != "" {
		return e.Response.Error.Message
	}
	return ""
}

// 处理错误响应
func HandleErrorResp(resp *http.Response) (openAIErrorWithStatusCode *types.OpenAIErrorWithStatusCode) {
	openAIErrorWithStatusCode = &types.OpenAIErrorWithStatusCode{
		StatusCode: resp.StatusCode,
		OpenAIError: types.OpenAIError{
			Message: "",
			Type:    "upstream_error",
			Code:    "bad_response_status_code",
			Param:   strconv.Itoa(resp.StatusCode),
		},
	}
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return
	}
	err = resp.Body.Close()
	if err != nil {
		return
	}
	// var errorResponse types.OpenAIErrorResponse
	var errorResponse GeneralErrorResponse
	err = json.Unmarshal(responseBody, &errorResponse)
	if err != nil {
		return
	}

	if errorResponse.Error.Message != "" {
		// OpenAI format error, so we override the default one
		openAIErrorWithStatusCode.OpenAIError = errorResponse.Error
	} else {
		openAIErrorWithStatusCode.OpenAIError.Message = errorResponse.ToMessage()
	}
	if openAIErrorWithStatusCode.OpenAIError.Message == "" {
		openAIErrorWithStatusCode.OpenAIError.Message = fmt.Sprintf("bad response status code %d", resp.StatusCode)
	}

	return
}

func (c *Client) SendRequestRaw(req *http.Request, proxyAddr string) (body io.ReadCloser, err error) {
	client := GetHttpClient(proxyAddr)
	resp, err := client.Do(req)
	PutHttpClient(client)
	if err != nil {
		return
	}

	return resp.Body, nil
}

func IsFailureStatusCode(resp *http.Response) bool {
	return resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusBadRequest
}

func DecodeResponse(body io.Reader, v any) error {
	if v == nil {
		return nil
	}

	if result, ok := v.(*string); ok {
		return DecodeString(body, result)
	}

	if stringer, ok := v.(Stringer); ok {
		return DecodeString(body, stringer.GetString())
	}

	return json.NewDecoder(body).Decode(v)
}

func DecodeString(body io.Reader, output *string) error {
	b, err := io.ReadAll(body)
	if err != nil {
		return err
	}
	*output = string(b)
	return nil
}

func SetEventStreamHeaders(c *gin.Context) {
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("Transfer-Encoding", "chunked")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
}
