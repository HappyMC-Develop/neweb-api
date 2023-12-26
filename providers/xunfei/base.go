package xunfei

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/url"
	"one-api/common"
	"one-api/providers/base"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type XunfeiProviderFactory struct{}

// 创建 XunfeiProvider
func (f XunfeiProviderFactory) Create(c *gin.Context) base.ProviderInterface {
	return &XunfeiProvider{
		BaseProvider: base.BaseProvider{
			BaseURL:         "wss://spark-api.xf-yun.com",
			ChatCompletions: "true",
			Context:         c,
		},
	}
}

// https://www.xfyun.cn/doc/spark/Web.html
type XunfeiProvider struct {
	base.BaseProvider
	domain string
	apiId  string
}

// 获取请求头
func (p *XunfeiProvider) GetRequestHeaders() (headers map[string]string) {
	headers = make(map[string]string)
	return headers
}

// 获取完整请求 URL
func (p *XunfeiProvider) GetFullRequestURL(requestURL string, modelName string) string {
	splits := strings.Split(p.Channel.Key, "|")
	if len(splits) != 3 {
		return ""
	}
	domain, authUrl := p.getXunfeiAuthUrl(splits[2], splits[1])

	p.domain = domain
	p.apiId = splits[0]

	return authUrl
}

func (p *XunfeiProvider) getXunfeiAuthUrl(apiKey string, apiSecret string) (string, string) {
	query := p.Context.Request.URL.Query()
	apiVersion := query.Get("api-version")
	if apiVersion == "" {
		apiVersion = p.Channel.Key
	}
	if apiVersion == "" {
		apiVersion = "v1.1"
		common.SysLog("api_version not found, use default: " + apiVersion)
	}
	domain := "general"
	if apiVersion != "v1.1" {
		domain += strings.Split(apiVersion, ".")[0]
	}
	authUrl := p.buildXunfeiAuthUrl(fmt.Sprintf("%s/%s/chat", p.BaseURL, apiVersion), apiKey, apiSecret)
	return domain, authUrl
}

func (p *XunfeiProvider) buildXunfeiAuthUrl(hostUrl string, apiKey, apiSecret string) string {
	HmacWithShaToBase64 := func(algorithm, data, key string) string {
		mac := hmac.New(sha256.New, []byte(key))
		mac.Write([]byte(data))
		encodeData := mac.Sum(nil)
		return base64.StdEncoding.EncodeToString(encodeData)
	}
	ul, err := url.Parse(hostUrl)
	if err != nil {
		fmt.Println(err)
	}
	date := time.Now().UTC().Format(time.RFC1123)
	signString := []string{"host: " + ul.Host, "date: " + date, "GET " + ul.Path + " HTTP/1.1"}
	sign := strings.Join(signString, "\n")
	sha := HmacWithShaToBase64("hmac-sha256", sign, apiSecret)
	authUrl := fmt.Sprintf("hmac username=\"%s\", algorithm=\"%s\", headers=\"%s\", signature=\"%s\"", apiKey,
		"hmac-sha256", "host date request-line", sha)
	authorization := base64.StdEncoding.EncodeToString([]byte(authUrl))
	v := url.Values{}
	v.Add("host", ul.Host)
	v.Add("date", date)
	v.Add("authorization", authorization)
	callUrl := hostUrl + "?" + v.Encode()
	return callUrl
}
