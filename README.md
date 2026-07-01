# iYoRoy-Network API

iYoRoy-Network 的后端 API 服务。

## 功能

- **NetBox Webhook → Cloudflare rDNS 自动同步** — 接收 NetBox IPAM webhook，自动同步 IPv6 地址的 AAAA（前向）和 PTR（反向）DNS 记录到 Cloudflare，支持 create/update/delete 事件。
- **NetBox Webhook → Node IANA DNS 自动同步** — 通过 [dnsmgr](https://github.com/netcccyun/dnsmgr) 同步 NetBox IP 地址 `custom_fields` 中的 `Node_IANA_DNS` / `Node_IANA_IPv4_Address` / `Node_IANA_IPv6_Address`，自动创建 A 和 AAAA 记录（含 `ipv4.` / `ipv6.` 前缀子域名）。

## 快速开始

```bash
# 1. 设置 Cloudflare 凭据（Cloudflare 功能需要）
export CLOUDFLARE_API_TOKEN="your-token"

# 2. 创建配置文件
cp config.example.yaml config.local.yaml
# 编辑填入 DNS zone 和 dnsmgr 信息

# 3. 运行
go run main.go
```

## 配置

配置文件 `config.local.yaml`，优先级：环境变量 > 配置文件 > 默认值。

```yaml
# Cloudflare 直连（AAAA / PTR）
cloudflare:
  forward_zones:
    - zone_id: "..."
      zone_name: "yori.moe"
  reverse_zones:
    - prefix: "2a14:7583:f244::/48"
      zone_id: "..."
      zone_name: "...ip6.arpa"

# 聚合 DNS 管理系统（Node IANA DNS）
dnsmgr:
  base_url: "https://dns.example.com"
  uid: 1
  key: "your-api-key"
  default_line: "default"
  default_ttl: 600

# Webhook 配置
webhook:
  hmac_secret: ""          # NetBox HMAC-SHA512 签名密钥（可选）
  enabled_events:          # 启用的事件类型
    - created
    - updated
    - deleted

log:
  log_level: info
```

## 环境变量

| 变量 | 说明 |
|------|------|
| `CLOUDFLARE_API_TOKEN` | Cloudflare API Token（需 DNS:Edit 权限） |
| `WEBHOOK__HMAC_SECRET` | NetBox webhook HMAC 签名密钥 |
| `LOG__LOG_LEVEL` | 日志级别：debug / info / warn / error |

## Webhook 端点

```
POST /webhook/ipam/dns
```

NetBox 配置：Operations → Webhooks → 新建，Content type 选 `IPAM > IP address`，URL 填上述地址。

## Docker

```bash
docker run -p 8080:8080 \
  -e CLOUDFLARE_API_TOKEN="your-token" \
  -v $(pwd)/config.local.yaml:/app/config.local.yaml \
  ghcr.io/iyoroy-network/api:main
```
