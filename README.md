# iYoRoy-Network API

iYoRoy-Network 的后端 API 服务。

## 功能

- **NetBox Webhook → Cloudflare rDNS 自动同步** — 接收 NetBox IPAM webhook，自动同步 IPv6 地址的 AAAA（前向）和 PTR（反向）DNS 记录到 Cloudflare。

## 快速开始

```bash
# 1. 设置 Cloudflare 凭据
export CLOUDFLARE_API_TOKEN="your-token"

# 2. 创建配置文件
cp config.example.yaml config.local.yaml
# 编辑填入你的 DNS zone 信息

# 3. 运行
go run main.go
```

## 配置

配置文件为 `config.local.yaml`（优先级：环境变量 > 文件 > 默认值）。

```yaml
cloudflare:
  forward_zones:     # AAAA 记录所在 zone
    - zone_id: "..."
      zone_name: "yori.moe"
  reverse_zones:     # PTR 记录所在 zone
    - prefix: "2a14:7583:f244::/48"
      zone_id: "..."
      zone_name: "...ip6.arpa"

webhook:
  hmac_secret: ""    # NetBox webhook 签名密钥（可选）

log:
  log_level: info
```

## 环境变量

| 变量 | 说明 |
|------|------|
| `CLOUDFLARE_API_TOKEN` | Cloudflare API Token（需 DNS:Edit 权限） |
| `WEBHOOK__HMAC_SECRET` | NetBox webhook HMAC 签名密钥（可选） |
| `LOG__LOG_LEVEL` | 日志级别：debug / info / warn / error |

## Docker

```bash
docker run -p 8080:8080 \
  -e CLOUDFLARE_API_TOKEN="your-token" \
  -v $(pwd)/config.local.yaml:/app/config.local.yaml \
  ghcr.io/iyoroy-network/api:main
```

## Webhook 端点

```
POST /webhook/ipam/rdns
```

NetBox 配置：Operations → Webhooks → 新建，Content type 选 `IPAM > IP address`，URL 填上述地址。
