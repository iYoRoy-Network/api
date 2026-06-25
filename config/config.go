package config

import (
	"encoding/json"
	"log"
	"regexp"

	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
)

// loadConfigFiles 从本地配置文件加载配置（覆盖默认值）
// 按顺序尝试以下路径，第一个存在的文件会被加载：
// 1. ./config.local.yaml
// 2. ./config/config.local.yaml
func loadConfigFiles(k *koanf.Koanf) {
	configPaths := []string{
		"config.local.yaml",
		"config/config.local.yaml",
	}

	for _, path := range configPaths {
		if err := k.Load(file.Provider(path), yaml.Parser()); err == nil {
			log.Printf("info: loaded config from %s", path)
			return
		}
	}
	log.Printf("info: no local config found")
}

// LoadConfig 加载所有配置
func LoadConfig() *Config {
	k := koanf.New(".")

	// 1. 设置所有模块的默认值
	setDefaults(k, defaultConfig())

	// 2. 从配置文件读取
	loadConfigFiles(k)

	// 3. 从环境变量读取
	loadEnvVars(k)

	// 4. 反序列化到 Config 结构
	var cfg Config
	if err := k.Unmarshal("", &cfg); err != nil {
		log.Fatalf("error unmarshalling config: %v", err)
	}

	return &cfg
}

// 用于检测敏感字段的正则表达式
var re = regexp.MustCompile(`(?i)password|secret|key|token|access|databaseurl|dsn`)

// GetDebugConfig 返回脱敏后的配置数据
func (c *Config) GetDebugConfig() (m map[string]any) {
	b, _ := json.Marshal(c)
	json.Unmarshal(b, &m)

	var f func(any)
	f = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			for k, v := range x {
				if re.MatchString(k) {
					x[k] = "***MASKED***"
				} else {
					f(v)
				}
			}
		case []any:
			for _, v := range x {
				f(v)
			}
		}
	}

	f(m)
	return
}
