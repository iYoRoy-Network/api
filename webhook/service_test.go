package webhook

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"iyoroynet-api/cloudflare"
	"iyoroynet-api/config"

	"github.com/cloudflare/cloudflare-go/v4/option"
)

// setupMockCFClient 创建一个模拟 Cloudflare API 服务器和对应的 Client
func setupMockCFClient(handler http.HandlerFunc) (*cloudflare.Client, *httptest.Server) {
	server := httptest.NewServer(handler)
	client := cloudflare.NewClientWithOptions(
		option.WithAPIToken("test-token"),
		option.WithBaseURL(server.URL),
	)
	return client, server
}

// mockDNSResponse 返回标准的 Cloudflare API 模拟响应
func mockDNSResponse(w http.ResponseWriter, r *http.Request, result any) {
	w.Header().Set("Content-Type", "application/json")

	// Cloudflare SDK 期望的响应结构
	var resp any
	switch r.Method {
	case "POST", "PUT", "PATCH":
		resp = map[string]any{
			"success":  true,
			"errors":   []any{},
			"messages": []any{},
			"result":   result,
		}
	case "GET":
		// List response
		switch v := result.(type) {
		case []any:
			resp = map[string]any{
				"success":  true,
				"errors":   []any{},
				"messages": []any{},
				"result":   v,
				"result_info": map[string]any{
					"page":        1,
					"per_page":    100,
					"count":       len(v),
					"total_count": len(v),
					"total_pages": 1,
				},
			}
		default:
			resp = map[string]any{
				"success":  true,
				"errors":   []any{},
				"messages": []any{},
				"result":   result,
			}
		}
	case "DELETE":
		resp = map[string]any{
			"success":  true,
			"errors":   []any{},
			"messages": []any{},
			"result": map[string]string{
				"id": "deleted-record-id",
			},
		}
	}

	json.NewEncoder(w).Encode(resp)
}

func TestNetBoxWebhook_Validate(t *testing.T) {
	tests := []struct {
		name    string
		payload NetBoxWebhook
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid payload",
			payload: NetBoxWebhook{
				Event: "created",
				Model: "ipam.ipaddress",
				Data: NetBoxIPAddress{
					Address: "2a14:7583:f244::3b06/128",
					DNSName: "3b06.fra-de.backbone.yori.moe",
				},
			},
			wantErr: false,
		},
		{
			name: "missing event",
			payload: NetBoxWebhook{
				Model: "ipam.ipaddress",
				Data: NetBoxIPAddress{
					Address: "2a14:7583:f244::3b06/128",
					DNSName: "3b06.fra-de.backbone.yori.moe",
				},
			},
			wantErr: true,
			errMsg:  "event is required",
		},
		{
			name: "unsupported model",
			payload: NetBoxWebhook{
				Event: "created",
				Model: "dcim.device",
				Data: NetBoxIPAddress{
					Address: "2a14:7583:f244::3b06/128",
					DNSName: "3b06.fra-de.backbone.yori.moe",
				},
			},
			wantErr: true,
			errMsg:  "unsupported model",
		},
		{
			name: "missing dns_name",
			payload: NetBoxWebhook{
				Event: "created",
				Model: "ipam.ipaddress",
				Data: NetBoxIPAddress{
					Address: "2a14:7583:f244::3b06/128",
				},
			},
			wantErr: true,
			errMsg:  "dns_name is empty",
		},
		{
			name: "missing address",
			payload: NetBoxWebhook{
				Event: "created",
				Model: "ipam.ipaddress",
				Data: NetBoxIPAddress{
					DNSName: "3b06.fra-de.backbone.yori.moe",
				},
			},
			wantErr: true,
			errMsg:  "ip address is empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.payload.Validate()
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil")
				} else if tt.errMsg != "" && !containsStr(err.Error(), tt.errMsg) {
					t.Errorf("expected error containing %q, got %q", tt.errMsg, err.Error())
				}
			} else if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestNetBoxWebhook_EventEnabled(t *testing.T) {
	w := NetBoxWebhook{Event: "created"}
	if !w.IsEventEnabled([]string{"created", "updated"}) {
		t.Error("expected 'created' to be enabled")
	}
	if w.IsEventEnabled([]string{"deleted"}) {
		t.Error("expected 'created' to not be enabled")
	}
}

func TestSyncAAAA_Created(t *testing.T) {
	cfClient, server := setupMockCFClient(func(w http.ResponseWriter, r *http.Request) {
		record := map[string]any{
			"id":      "record-aaaa-001",
			"type":    "AAAA",
			"name":    "3b06.fra-de.backbone.yori.moe",
			"content": "2a14:7583:f244::3b06",
			"ttl":     1,
			"proxied": false,
		}

		switch r.Method {
		case "GET":
			mockDNSResponse(w, r, []any{}) // 空列表 = 记录不存在
		case "POST":
			mockDNSResponse(w, r, record) // 创建成功
		default:
			mockDNSResponse(w, r, record)
		}
	})
	defer server.Close()

	cfg := &config.CloudflareConfig{
		ForwardZones: []config.ZoneConfig{
			{ZoneID: "zone-yori", ZoneName: "yori.moe"},
		},
	}

	svc := NewService(cfClient, cfg)
	ctx := context.Background()

	result, err := svc.ProcessWebhook(ctx, &NetBoxWebhook{
		Event: "created",
		Model: "ipam.ipaddress",
		Data: NetBoxIPAddress{
			Address: "2a14:7583:f244::3b06/128",
			DNSName: "3b06.fra-de.backbone.yori.moe",
		},
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.AAAASuccess {
		t.Errorf("AAAA sync should succeed, got: %s", result.AAAAMessage)
	}
	if result.IPAddress != "2a14:7583:f244::3b06" {
		t.Errorf("expected IP 2a14:7583:f244::3b06, got %s", result.IPAddress)
	}
	if result.DNSName != "3b06.fra-de.backbone.yori.moe" {
		t.Errorf("expected DNS name 3b06.fra-de.backbone.yori.moe, got %s", result.DNSName)
	}
}

func TestProcessWebhook_Deleted(t *testing.T) {
	cfClient, server := setupMockCFClient(func(w http.ResponseWriter, r *http.Request) {
		record := map[string]any{
			"id":      "record-aaaa-001",
			"type":    "AAAA",
			"name":    "3b06.fra-de.backbone.yori.moe",
			"content": "2a14:7583:f244::3b06",
		}

		switch r.Method {
		case "GET":
			mockDNSResponse(w, r, []any{record}) // 记录存在
		case "DELETE":
			mockDNSResponse(w, r, nil)
		default:
			mockDNSResponse(w, r, record)
		}
	})
	defer server.Close()

	cfg := &config.CloudflareConfig{
		ForwardZones: []config.ZoneConfig{
			{ZoneID: "zone-yori", ZoneName: "yori.moe"},
		},
	}

	svc := NewService(cfClient, cfg)
	ctx := context.Background()

	result, err := svc.ProcessWebhook(ctx, &NetBoxWebhook{
		Event: "deleted",
		Model: "ipam.ipaddress",
		Data: NetBoxIPAddress{
			Address: "2a14:7583:f244::3b06/128",
			DNSName: "3b06.fra-de.backbone.yori.moe",
		},
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.AAAASuccess {
		t.Errorf("AAAA delete should succeed, got: %s", result.AAAAMessage)
	}
}

func TestProcessWebhook_EventHandling(t *testing.T) {
	// service 层面不校验 enabled_events（那是 handler 的职责）
	// event 类型为 "updated" 时，service 仍然正常处理
	cfClient, server := setupMockCFClient(func(w http.ResponseWriter, r *http.Request) {
		record := map[string]any{
			"id":      "record-001",
			"type":    "AAAA",
			"name":    "3b06.fra-de.backbone.yori.moe",
			"content": "2a14:7583:f244::3b06",
			"ttl":     1,
		}
		switch r.Method {
		case "GET":
			mockDNSResponse(w, r, []any{})
		case "POST":
			mockDNSResponse(w, r, record)
		default:
			mockDNSResponse(w, r, record)
		}
	})
	defer server.Close()

	cfg := &config.CloudflareConfig{
		ForwardZones: []config.ZoneConfig{
			{ZoneID: "zone-yori", ZoneName: "yori.moe"},
		},
	}
	svc := NewService(cfClient, cfg)
	ctx := context.Background()

	result, err := svc.ProcessWebhook(ctx, &NetBoxWebhook{
		Event: "updated",
		Model: "ipam.ipaddress",
		Data: NetBoxIPAddress{
			Address: "2a14:7583:f244::3b06/128",
			DNSName: "3b06.fra-de.backbone.yori.moe",
		},
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.AAAASuccess {
		t.Error("AAAA sync should proceed regardless of event enablement (that's handler's job)")
	}
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
