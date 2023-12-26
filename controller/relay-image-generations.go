package controller

import (
	"context"
	"net/http"
	"one-api/common"
	"one-api/model"
	providersBase "one-api/providers/base"
	"one-api/types"

	"github.com/gin-gonic/gin"
)

func RelayImageGenerations(c *gin.Context) {

	var imageRequest types.ImageRequest

	if err := common.UnmarshalBodyReusable(c, &imageRequest); err != nil {
		common.AbortWithMessage(c, http.StatusBadRequest, err.Error())
		return
	}

	if imageRequest.Model == "" {
		imageRequest.Model = "dall-e-2"
	}

	if imageRequest.N == 0 {
		imageRequest.N = 1
	}

	if imageRequest.Size == "" {
		imageRequest.Size = "1024x1024"
	}

	if imageRequest.Quality == "" {
		imageRequest.Quality = "standard"
	}

	channel, pass := fetchChannel(c, imageRequest.Model)
	if pass {
		return
	}

	// 解析模型映射
	var isModelMapped bool
	modelMap, err := parseModelMapping(channel.GetModelMapping())
	if err != nil {
		common.AbortWithMessage(c, http.StatusInternalServerError, err.Error())
		return
	}
	if modelMap != nil && modelMap[imageRequest.Model] != "" {
		imageRequest.Model = modelMap[imageRequest.Model]
		isModelMapped = true
	}

	// 获取供应商
	provider, pass := getProvider(c, channel, common.RelayModeImagesGenerations)
	if pass {
		return
	}
	imageGenerationsProvider, ok := provider.(providersBase.ImageGenerationsInterface)
	if !ok {
		common.AbortWithMessage(c, http.StatusNotImplemented, "channel not implemented")
		return
	}

	// 获取Input Tokens
	promptTokens, err := common.CountTokenImage(imageRequest)
	if err != nil {
		common.AbortWithMessage(c, http.StatusInternalServerError, err.Error())
		return
	}

	var quotaInfo *QuotaInfo
	var errWithCode *types.OpenAIErrorWithStatusCode
	var usage *types.Usage
	quotaInfo, errWithCode = generateQuotaInfo(c, imageRequest.Model, promptTokens)
	if errWithCode != nil {
		errorHelper(c, errWithCode)
		return
	}

	usage, errWithCode = imageGenerationsProvider.ImageGenerationsAction(&imageRequest, isModelMapped, promptTokens)

	// 如果报错，则退还配额
	if errWithCode != nil {
		tokenId := c.GetInt("token_id")
		if quotaInfo.HandelStatus {
			go func(ctx context.Context) {
				// return pre-consumed quota
				err := model.PostConsumeTokenQuota(tokenId, -quotaInfo.preConsumedQuota)
				if err != nil {
					common.LogError(ctx, "error return pre-consumed quota: "+err.Error())
				}
			}(c.Request.Context())
		}
		errorHelper(c, errWithCode)
		return
	} else {
		tokenName := c.GetString("token_name")
		// 如果没有报错，则消费配额
		go func(ctx context.Context) {
			err = quotaInfo.completedQuotaConsumption(usage, tokenName, ctx)
			if err != nil {
				common.LogError(ctx, err.Error())
			}
		}(c.Request.Context())
	}
}
