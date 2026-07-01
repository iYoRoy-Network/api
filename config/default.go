package config

import (
	"reflect"

	"github.com/knadh/koanf/v2"

	"iyoroynet-api/utils"
)

// defaultConfig 聚合所有模块的默认配置
func defaultConfig() Config {
	return Config{
		Server: DefaultServerConfig(),
		Log:    utils.DefaultLogConfig(),
		Cloudflare: CloudflareConfig{
			ForwardZones: []ZoneConfig{},
			ReverseZones: []ReverseZoneConfig{},
		},
		Dnsmgr: DnsmgrConfig{
			BaseURL:     "",
			UID:         0,
			Key:         "",
			DefaultLine: "default",
			DefaultTTL:  600,
		},
		Webhook: WebhookConfig{
			EnabledEvents: []string{"created", "updated", "deleted"},
			HMACSecret:    "",
		},
	}
}

// setDefaults 将默认配置设置到 koanf 实例
func setDefaults(k *koanf.Koanf, defaults Config) {
	v := reflect.ValueOf(defaults)
	t := v.Type()

	for i := 0; i < v.NumField(); i++ {
		field := t.Field(i)
		tag := field.Tag.Get("koanf")
		if tag != "" {
			k.Set(tag, v.Field(i).Interface())
		}
	}
}
