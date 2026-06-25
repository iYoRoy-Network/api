package config

import (
	"log"
	"strings"

	"github.com/knadh/koanf/providers/env"
	"github.com/knadh/koanf/v2"
)

// loadEnvVars 从环境变量加载配置。
// 环境变量名遵循: 配置键名大写 + 下划线分隔 + 层级用双下划线连接
// 例: cloudflare.api_token → CLOUDFLARE__API_TOKEN
func loadEnvVars(k *koanf.Koanf) {
	callback := func(key string, value string) (string, any) {
		// 转换为小写并用点分隔的 key
		key = strings.ToLower(key)
		key = strings.ReplaceAll(key, "__", ".")

		return key, value
	}

	if err := k.Load(env.ProviderWithValue("", ".", callback), nil); err != nil {
		log.Printf("warn: error loading env vars: %v", err)
	}
}
