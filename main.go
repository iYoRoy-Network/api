package main

import (
	"iyoroynet-api/config"
	"iyoroynet-api/router"
	"iyoroynet-api/utils"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"go.uber.org/zap"
)

func main() {
	// 1. 加载配置
	cfg := config.LoadConfig()

	// 2. 初始化日志
	utils.InitLogger(&cfg.Log)
	defer zap.L().Sync()

	// 3. 打印已加载的配置（脱敏）
	zap.L().Info("Configuration loaded", zap.Any("config", cfg.GetDebugConfig()))

	// 4. 创建 Echo 实例
	e := echo.New()

	// 5. 添加中间件
	e.Use(middleware.Recover())
	e.Use(middleware.RequestID())

	// 6. 注册路由
	router.Init(e, cfg)

	// 7. 启动服务器
	zap.L().Info("Starting server", zap.String("address", cfg.Server.ServerAddress))
	if err := e.Start(cfg.Server.ServerAddress); err != nil {
		zap.L().Fatal("Failed to start server", zap.Error(err))
	}
}
