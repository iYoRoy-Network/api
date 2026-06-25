package middleware

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha512"
	"encoding/hex"
	"io"
	"net/http"

	"github.com/labstack/echo/v4"
	"go.uber.org/zap"
)

const (
	// HeaderHookSignature NetBox webhook 签名头
	HeaderHookSignature = "X-Hook-Signature"
)

// HookSignature 验证 NetBox webhook 的 HMAC-SHA512 签名。
//
// NetBox 使用 webhook 的 secret 对请求体做 HMAC-SHA512 签名，
// 签名结果以十六进制形式放在 X-Hook-Signature 头中。
//
// 若 secret 为空，跳过验证（不推荐用于生产环境）。
func HookSignature(secret string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// 无密钥时跳过验证
			if secret == "" {
				zap.L().Warn("Webhook HMAC secret is not configured — skipping signature verification")
				return next(c)
			}

			// 读取 X-Hook-Signature 头
			signatureHex := c.Request().Header.Get(HeaderHookSignature)
			if signatureHex == "" {
				zap.L().Warn("Missing X-Hook-Signature header",
					zap.String("method", c.Request().Method),
					zap.String("uri", c.Request().RequestURI),
					zap.String("ip", c.RealIP()),
				)
				return c.JSON(http.StatusUnauthorized, map[string]string{
					"error":   "missing signature",
					"message": "X-Hook-Signature header is required",
				})
			}

			// 解码十六进制签名
			expectedMAC, err := hex.DecodeString(signatureHex)
			if err != nil {
				zap.L().Warn("Invalid X-Hook-Signature format",
					zap.String("signature", signatureHex),
					zap.Error(err),
				)
				return c.JSON(http.StatusUnauthorized, map[string]string{
					"error":   "invalid signature format",
					"message": "X-Hook-Signature must be a hex-encoded HMAC-SHA512",
				})
			}

			// 读取请求体（需要缓冲以便后续 handler 使用）
			body, err := io.ReadAll(c.Request().Body)
			if err != nil {
				zap.L().Error("Failed to read request body", zap.Error(err))
				return c.JSON(http.StatusInternalServerError, map[string]string{
					"error": "failed to read request body",
				})
			}
			// 恢复请求体供下游读取
			c.Request().Body = io.NopCloser(bytes.NewBuffer(body))

			// 计算 HMAC-SHA512
			mac := hmac.New(sha512.New, []byte(secret))
			mac.Write(body)
			actualMAC := mac.Sum(nil)

			// 常量时间比较，防止时序攻击
			if !hmac.Equal(expectedMAC, actualMAC) {
				zap.L().Warn("HMAC signature mismatch",
					zap.String("method", c.Request().Method),
					zap.String("uri", c.Request().RequestURI),
					zap.String("ip", c.RealIP()),
				)
				return c.JSON(http.StatusUnauthorized, map[string]string{
					"error":   "signature mismatch",
					"message": "HMAC signature verification failed",
				})
			}

			zap.L().Debug("Webhook HMAC signature verified",
				zap.String("ip", c.RealIP()),
			)

			return next(c)
		}
	}
}
