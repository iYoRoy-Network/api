package middleware

import (
	"time"

	"github.com/labstack/echo/v4"
	"go.uber.org/zap"
)

// ZapLogger 使用 Zap 记录请求日志
func ZapLogger() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			start := time.Now()

			err := next(c)
			if err != nil {
				c.Error(err)
			}

			req := c.Request()
			res := c.Response()

			fields := []zap.Field{
				zap.Int("status", res.Status),
				zap.String("method", req.Method),
				zap.String("uri", req.RequestURI),
				zap.String("ip", c.RealIP()),
				zap.String("user_agent", req.UserAgent()),
				zap.Int64("latency_ms", time.Since(start).Milliseconds()),
			}

			id := req.Header.Get(echo.HeaderXRequestID)
			if id == "" {
				id = res.Header().Get(echo.HeaderXRequestID)
			}
			fields = append(fields, zap.String("request_id", id))

			n := res.Status
			switch {
			case n >= 500:
				fields = append(fields, zap.Error(err))
				zap.L().Error("Server error", fields...)
			case n >= 400:
				fields = append(fields, zap.Error(err))
				zap.L().Warn("Client error", fields...)
			case n >= 300:
				zap.L().Info("Redirection", fields...)
			default:
				zap.L().Info("Success", fields...)
			}

			return nil
		}
	}
}
