package webhook

import (
	"encoding/json"
	"fmt"
	"slices"
)

// NetBoxWebhook NetBox webhook 请求体
// NetBox 实际发送的字段名在不同版本间有差异：
//   - object_type / model: IP 地址对象的模型标识
//   - snapshots / snapshot: 变更前后的数据快照
//
// 我们通过自定义 UnmarshalJSON 兼容这些差异。
type NetBoxWebhook struct {
	Event     string           `json:"event"`
	Timestamp string           `json:"timestamp"`
	Model     string           `json:"-"`        // 统一后的模型名，由 UnmarshalJSON 填充
	Username  string           `json:"username"`
	RequestID string           `json:"request_id"`
	Data      NetBoxIPAddress  `json:"data"`
	Snapshot  *json.RawMessage `json:"-"`        // 统一后的快照，由 UnmarshalJSON 填充
}

func (w *NetBoxWebhook) UnmarshalJSON(data []byte) error {
	// 用中间结构体兼容 object_type/model 和 snapshots/snapshot
	var raw struct {
		Event     string           `json:"event"`
		Timestamp string           `json:"timestamp"`
		Model     string           `json:"model"`
		ObjectType string          `json:"object_type"`
		Username  string           `json:"username"`
		RequestID string           `json:"request_id"`
		Data      NetBoxIPAddress  `json:"data"`
		Snapshot  *json.RawMessage `json:"snapshot"`
		Snapshots *json.RawMessage `json:"snapshots"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	w.Event = raw.Event
	w.Timestamp = raw.Timestamp
	w.Username = raw.Username
	w.RequestID = raw.RequestID
	w.Data = raw.Data

	// 统一 model: object_type 优先于 model
	if raw.ObjectType != "" {
		w.Model = raw.ObjectType
	} else {
		w.Model = raw.Model
	}

	// 统一 snapshot: snapshots 优先于 snapshot
	if raw.Snapshots != nil {
		w.Snapshot = raw.Snapshots
	} else {
		w.Snapshot = raw.Snapshot
	}

	return nil
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

// snapshotIPData snapshot 中只提取 address 和 dns_name。
// snapshot 里的字段类型可能与主 data 不同（如 status 是字符串而非对象），
// 用最小结构体避免解析失败。
type snapshotIPData struct {
	Address string `json:"address"`
	DNSName string `json:"dns_name"`
}

// PreChangeData 从 snapshot 中提取变更前的 address 和 dns_name。
// 若 snapshot 为 nil 或解析失败，返回 nil。
func (w *NetBoxWebhook) PreChangeData() *snapshotIPData {
	if w.Snapshot == nil {
		return nil
	}

	raw, err := w.Snapshot.MarshalJSON()
	if err != nil {
		return nil
	}

	// {"prechange": {"address": "...", "dns_name": "..."}}
	var wrapper struct {
		PreChange snapshotIPData `json:"prechange"`
	}
	if json.Unmarshal(raw, &wrapper) == nil && wrapper.PreChange.Address != "" {
		return &wrapper.PreChange
	}

	// 平铺格式 {"address": "...", "dns_name": "..."}
	var data snapshotIPData
	if json.Unmarshal(raw, &data) == nil && data.Address != "" {
		return &data
	}

	return nil
}
