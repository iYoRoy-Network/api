package config

import "iyoroynet-api/utils"

// Config 对外暴露的配置结构，包含各模块独立的配置
type Config struct {
	Server     ServerConfig     `koanf:"server"`
	Log        utils.LogConfig  `koanf:"log"`
	Cloudflare CloudflareConfig `koanf:"cloudflare"`
	Dnsmgr     DnsmgrConfig     `koanf:"dnsmgr"`
	Webhook    WebhookConfig    `koanf:"webhook"`
}

// CloudflareConfig Cloudflare 模块配置
// 注意：Cloudflare 认证凭据从环境变量读取（CLOUDFLARE_API_TOKEN），不在此配置中
type CloudflareConfig struct {
	ForwardZones []ZoneConfig        `koanf:"forward_zones"`
	ReverseZones []ReverseZoneConfig `koanf:"reverse_zones"`
}

// ZoneConfig 前向 DNS Zone 配置（AAAA 记录）
type ZoneConfig struct {
	ZoneID   string `koanf:"zone_id"`
	ZoneName string `koanf:"zone_name"`
}

// ReverseZoneConfig 反向 DNS Zone 配置（PTR 记录）
type ReverseZoneConfig struct {
	Prefix   string `koanf:"prefix"`
	ZoneID   string `koanf:"zone_id"`
	ZoneName string `koanf:"zone_name"`
}

// DnsmgrConfig 聚合 DNS 管理系统配置
type DnsmgrConfig struct {
	BaseURL     string `koanf:"base_url"`
	UID         int    `koanf:"uid"`
	Key         string `koanf:"key"`
	DefaultLine string `koanf:"default_line"`
	DefaultTTL  int    `koanf:"default_ttl"`
}

// WebhookConfig Webhook 模块配置
type WebhookConfig struct {
	EnabledEvents []string `koanf:"enabled_events"`
	HMACSecret    string   `koanf:"hmac_secret"` // NetBox webhook HMAC-SHA512 签名密钥
}
