package webhook

import (
	"encoding/json"
	"fmt"
	"slices"
)

// NetBoxWebhook NetBox webhook 请求体
// 参考: https://docs.netbox.dev/en/stable/integrations/webhooks/
type NetBoxWebhook struct {
	Event     string           `json:"event"`
	Timestamp string           `json:"timestamp"`
	Model     string           `json:"model"`
	Username  string           `json:"username"`
	RequestID string           `json:"request_id"`
	Data      NetBoxIPAddress  `json:"data"`
	Snapshot  *json.RawMessage `json:"snapshot,omitempty"`
}

// NetBoxIPAddress NetBox IPAM IP 地址数据
type NetBoxIPAddress struct {
	ID      int    `json:"id"`
	Address string `json:"address"` // 例: 2a14:7583:f244::3b06/128
	DNSName string `json:"dns_name"` // 例: 3b06.fra-de.backbone.yori.moe
	Status  *struct {
		Value string `json:"value"`
		Label string `json:"label"`
	} `json:"status,omitempty"`
	Description string                 `json:"description,omitempty"`
	Tenant      json.RawMessage        `json:"tenant,omitempty"`
	VRF         json.RawMessage        `json:"vrf,omitempty"`
	Tags        []json.RawMessage      `json:"tags,omitempty"`
	CustomFields map[string]any `json:"custom_fields,omitempty"`
}

// Validate 验证 webhook 载荷是否包含必要的同步信息。
// 不强制校验 model 字段 — 不同 NetBox 版本的模型标识可能不同，
// 只要 event、address、dns_name 齐全就允许处理。
func (w *NetBoxWebhook) Validate() error {
	if w.Event == "" {
		return fmt.Errorf("webhook event is required")
	}

	if w.Data.Address == "" {
		return fmt.Errorf("ip address is empty")
	}

	if w.Data.DNSName == "" {
		return fmt.Errorf("dns_name is empty, cannot sync")
	}

	return nil
}

// IsEventEnabled 检查事件类型是否在启用列表中
func (w *NetBoxWebhook) IsEventEnabled(enabledEvents []string) bool {
	return slices.Contains(enabledEvents, w.Event)
}

// PreChangeData 从 snapshot 中提取变更前的 IP 地址数据。
// NetBox 的 snapshot 格式有两种：
//   - {"prechange": {"address": "...", "dns_name": "..."}}   -- 标准格式
//   - {"address": "...", "dns_name": "..."}                   -- 旧版平铺格式
//
// 若 snapshot 为 nil 或解析失败，返回空结构体。
func (w *NetBoxWebhook) PreChangeData() *NetBoxIPAddress {
	if w.Snapshot == nil {
		return nil
	}

	raw, err := w.Snapshot.MarshalJSON()
	if err != nil {
		return nil
	}

	// 先尝试 {"prechange": {...}} 格式
	var wrapper struct {
		PreChange NetBoxIPAddress `json:"prechange"`
	}
	if err := json.Unmarshal(raw, &wrapper); err == nil && wrapper.PreChange.Address != "" {
		return &wrapper.PreChange
	}

	// 回退：平铺格式
	var data NetBoxIPAddress
	if err := json.Unmarshal(raw, &data); err == nil && data.Address != "" {
		return &data
	}

	return nil
}
