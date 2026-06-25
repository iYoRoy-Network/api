package webhook

import (
	"bytes"
	"io"
	"net/http"

	"iyoroynet-api/config"

	"github.com/labstack/echo/v4"
	"go.uber.org/zap"
)

// Handler Webhook HTTP 处理器
type Handler struct {
	svc *Service
	cfg *config.WebhookConfig
}

// NewHandler 创建 Webhook HTTP 处理器
func NewHandler(svc *Service, cfg *config.WebhookConfig) *Handler {
	return &Handler{
		svc: svc,
		cfg: cfg,
	}
}

// HandleIPAMRDNS 处理 /webhook/ipam/rdns 的 POST 请求
// 接收 NetBox webhook，同步 Cloudflare DNS 记录
func (h *Handler) HandleIPAMRDNS(c echo.Context) error {
	// 读取原始请求体用于调试日志
	rawBody, _ := io.ReadAll(c.Request().Body)
	c.Request().Body = io.NopCloser(bytes.NewBuffer(rawBody))

	zap.L().Debug("Webhook request",
		zap.String("method", c.Request().Method),
		zap.String("uri", c.Request().RequestURI),
		zap.String("content_type", c.Request().Header.Get("Content-Type")),
		zap.String("signature", c.Request().Header.Get("X-Hook-Signature")),
		zap.ByteString("body", rawBody),
	)

	var webhook NetBoxWebhook

	if err := c.Bind(&webhook); err != nil {
		zap.L().Warn("Failed to parse webhook body", zap.Error(err))
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error":   "invalid request body",
			"message": err.Error(),
		})
	}

	// 验证载荷
	if err := webhook.Validate(); err != nil {
		zap.L().Warn("Webhook validation failed",
			zap.Error(err),
			zap.String("model", webhook.Model),
			zap.String("event", webhook.Event),
		)
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error":   "validation failed",
			"message": err.Error(),
		})
	}

	// 检查事件是否启用
	if !webhook.IsEventEnabled(h.cfg.EnabledEvents) {
		zap.L().Debug("Webhook event not enabled",
			zap.String("event", webhook.Event),
		)
		return c.JSON(http.StatusOK, map[string]string{
			"status":  "skipped",
			"message": "event type not enabled",
		})
	}

	// 处理 webhook（传入请求 context）
	result, err := h.svc.ProcessWebhook(c.Request().Context(), &webhook)
	if err != nil {
		zap.L().Error("Failed to process webhook", zap.Error(err))
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error":   "sync failed",
			"message": err.Error(),
		})
	}

	zap.L().Info("Webhook processed successfully",
		zap.String("event", result.Event),
		zap.String("ip", result.IPAddress),
		zap.String("dns_name", result.DNSName),
		zap.Bool("aaaa_success", result.AAAASuccess),
		zap.Bool("ptr_success", result.PTRSuccess),
	)

	return c.JSON(http.StatusOK, map[string]any{
		"status": "ok",
		"result": result,
	})
}
