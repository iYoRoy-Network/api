package router

import (
	"net/http"

	"iyoroynet-api/cloudflare"
	"iyoroynet-api/config"
	"iyoroynet-api/middleware"
	"iyoroynet-api/webhook"

	"github.com/labstack/echo/v4"
	"go.uber.org/zap"
)

// Init 初始化路由，注入配置
func Init(e *echo.Echo, cfg *config.Config) {
	// 0. Health check
	e.GET("/health", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
	})

	// 1. Webhook 路由组（HMAC 签名验证 + 日志）
	webhookGroup := e.Group("/webhook")
	webhookGroup.Use(middleware.ZapLogger())
	webhookGroup.Use(middleware.HookSignature(cfg.Webhook.HMACSecret))

	// 2. 初始化 Cloudflare 客户端（从环境变量读取凭据）
	cfClient, err := cloudflare.NewClient()
	if err != nil {
		zap.L().Warn("Cloudflare client not initialized — webhook routes disabled",
			zap.Error(err),
		)
		zap.L().Info("Set CLOUDFLARE_API_TOKEN environment variable to enable DNS sync")
		return
	}

	// 3. 初始化 rDNS 服务
	rdnsSvc := webhook.NewService(cfClient, &cfg.Cloudflare)

	// 4. 注册 webhook 路由
	rdnsHandler := webhook.NewHandler(rdnsSvc, &cfg.Webhook)
	webhookGroup.POST("/ipam/rdns", rdnsHandler.HandleIPAMRDNS)

	zap.L().Info("Cloudflare client initialized and webhook routes registered",
		zap.Int("forward_zones", len(cfg.Cloudflare.ForwardZones)),
		zap.Int("reverse_zones", len(cfg.Cloudflare.ReverseZones)),
	)
}
