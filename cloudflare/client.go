package cloudflare

import (
	"fmt"
	"os"

	cloudflare "github.com/cloudflare/cloudflare-go/v4"
	"github.com/cloudflare/cloudflare-go/v4/option"
)

// Client 封装官方 Cloudflare Go SDK 客户端
type Client struct {
	sdk *cloudflare.Client
}

// NewClient 从环境变量创建 Cloudflare 客户端。
// 支持的认证方式（优先级从高到低）：
//  1. CLOUDFLARE_API_TOKEN  — API Token（推荐）
//  2. CLOUDFLARE_API_KEY + CLOUDFLARE_EMAIL — API Key + 邮箱
//
// 若设置了 CLOUDFLARE_API_TOKEN，则优先使用 API Token。
// 若环境变量均未设置，返回错误。
func NewClient() (*Client, error) {
	var opts []option.RequestOption

	if token := os.Getenv("CLOUDFLARE_API_TOKEN"); token != "" {
		opts = append(opts, option.WithAPIToken(token))
	} else if apiKey := os.Getenv("CLOUDFLARE_API_KEY"); apiKey != "" {
		email := os.Getenv("CLOUDFLARE_EMAIL")
		if email == "" {
			return nil, fmt.Errorf("CLOUDFLARE_API_KEY is set but CLOUDFLARE_EMAIL is not")
		}
		opts = append(opts, option.WithAPIKey(apiKey), option.WithAPIEmail(email))
	} else {
		return nil, fmt.Errorf(
			"no Cloudflare credentials found in environment: set CLOUDFLARE_API_TOKEN, or CLOUDFLARE_API_KEY + CLOUDFLARE_EMAIL",
		)
	}

	sdk := cloudflare.NewClient(opts...)
	return &Client{sdk: sdk}, nil
}

// NewClientWithToken 使用显式 API Token 创建客户端（用于测试或特殊场景）
func NewClientWithToken(token string) *Client {
	sdk := cloudflare.NewClient(option.WithAPIToken(token))
	return &Client{sdk: sdk}
}

// NewClientWithOptions 使用自定义 option 创建客户端（用于测试，如更改 BaseURL）
func NewClientWithOptions(opts ...option.RequestOption) *Client {
	sdk := cloudflare.NewClient(opts...)
	return &Client{sdk: sdk}
}

// SDK 返回底层的官方 SDK 客户端，用于高级操作
func (c *Client) SDK() *cloudflare.Client {
	return c.sdk
}
