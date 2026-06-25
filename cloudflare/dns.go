package cloudflare

import (
	"context"
	"fmt"

	cloudflare "github.com/cloudflare/cloudflare-go/v4"
	dns "github.com/cloudflare/cloudflare-go/v4/dns"
	"go.uber.org/zap"
)

// DNSRecordService DNS 记录管理服务（封装官方 SDK）
type DNSRecordService struct {
	client *Client
}

// NewDNSRecordService 创建 DNS 记录服务
func NewDNSRecordService(client *Client) *DNSRecordService {
	return &DNSRecordService{client: client}
}

// CreateAAAARecord 创建或更新 AAAA 记录（前向 DNS：域名 → IPv6）
// 返回记录 ID，空字符串表示记录不存在
func (s *DNSRecordService) CreateAAAARecord(ctx context.Context, zoneID, name, ipv6 string) (string, error) {
	record, err := s.client.SDK().DNS.Records.New(ctx, dns.RecordNewParams{
		ZoneID: cloudflare.F(zoneID),
		Body: dns.AAAARecordParam{
			Name:    cloudflare.F(name),
			Type:    cloudflare.F(dns.AAAARecordTypeAAAA),
			Content: cloudflare.F(ipv6),
			TTL:     cloudflare.F(dns.TTL(1)), // Auto TTL
			Proxied: cloudflare.F(false),
		},
	})
	if err != nil {
		return "", fmt.Errorf("create AAAA record: %w", err)
	}

	zap.L().Debug("AAAA record created",
		zap.String("zone_id", zoneID),
		zap.String("name", name),
		zap.String("record_id", record.ID),
	)
	return record.ID, nil
}

// CreatePTRRecord 创建或更新 PTR 记录（反向 DNS：IPv6 反转名 → 域名）
func (s *DNSRecordService) CreatePTRRecord(ctx context.Context, zoneID, ptrName, target string) (string, error) {
	record, err := s.client.SDK().DNS.Records.New(ctx, dns.RecordNewParams{
		ZoneID: cloudflare.F(zoneID),
		Body: dns.PTRRecordParam{
			Name:    cloudflare.F(ptrName),
			Type:    cloudflare.F(dns.PTRRecordTypePTR),
			Content: cloudflare.F(target),
			TTL:     cloudflare.F(dns.TTL(1)), // Auto TTL
		},
	})
	if err != nil {
		return "", fmt.Errorf("create PTR record: %w", err)
	}

	zap.L().Debug("PTR record created",
		zap.String("zone_id", zoneID),
		zap.String("ptr_name", ptrName),
		zap.String("record_id", record.ID),
	)
	return record.ID, nil
}

// FindRecord 按类型和名称查找 DNS 记录
// 返回 (recordID, found)，未找到时 recordID 为空字符串
func (s *DNSRecordService) FindRecord(ctx context.Context, zoneID, recordType, name string) (string, error) {
	iter := s.client.SDK().DNS.Records.ListAutoPaging(ctx, dns.RecordListParams{
		ZoneID: cloudflare.F(zoneID),
		Type:   cloudflare.F(dns.RecordListParamsType(recordType)),
		Name:   cloudflare.F(dns.RecordListParamsName{Exact: cloudflare.F(name)}),
	})

	for iter.Next() {
		return iter.Current().ID, nil
	}

	if err := iter.Err(); err != nil {
		return "", fmt.Errorf("list records: %w", err)
	}

	return "", nil // 未找到
}

// UpsertRecord 创建或更新一条 DNS 记录（先查后建/改）
func (s *DNSRecordService) UpsertRecord(ctx context.Context, zoneID, recordType, name, content string) (string, error) {
	recordID, err := s.FindRecord(ctx, zoneID, recordType, name)
	if err != nil {
		return "", fmt.Errorf("upsert: find: %w", err)
	}

	if recordID != "" {
		// 记录已存在，更新内容
		_, err := s.client.SDK().DNS.Records.Edit(ctx, recordID, dns.RecordEditParams{
			ZoneID: cloudflare.F(zoneID),
			Body: dns.RecordEditParamsBodyUnion(
				buildEditBody(recordType, content),
			),
		})
		if err != nil {
			return "", fmt.Errorf("upsert: edit: %w", err)
		}

		zap.L().Debug("DNS record updated",
			zap.String("zone_id", zoneID),
			zap.String("record_id", recordID),
			zap.String("type", recordType),
			zap.String("name", name),
		)
		return recordID, nil
	}

	// 记录不存在，创建新纪录
	return s.CreateRecord(ctx, zoneID, recordType, name, content)
}

// CreateRecord 创建一条通用 DNS 记录
func (s *DNSRecordService) CreateRecord(ctx context.Context, zoneID, recordType, name, content string) (string, error) {
	switch recordType {
	case "AAAA":
		return s.CreateAAAARecord(ctx, zoneID, name, content)
	case "PTR":
		return s.CreatePTRRecord(ctx, zoneID, name, content)
	default:
		return "", fmt.Errorf("unsupported record type: %s", recordType)
	}
}

// DeleteRecord 按类型和名称删除 DNS 记录
func (s *DNSRecordService) DeleteRecord(ctx context.Context, zoneID, recordType, name string) error {
	recordID, err := s.FindRecord(ctx, zoneID, recordType, name)
	if err != nil {
		return fmt.Errorf("delete: find: %w", err)
	}

	if recordID == "" {
		return fmt.Errorf("delete: record not found (type=%s, name=%s, zone=%s)", recordType, name, zoneID)
	}

	_, err = s.client.SDK().DNS.Records.Delete(ctx, recordID, dns.RecordDeleteParams{
		ZoneID: cloudflare.F(zoneID),
	})
	if err != nil {
		return fmt.Errorf("delete record %s: %w", recordID, err)
	}

	zap.L().Debug("DNS record deleted",
		zap.String("zone_id", zoneID),
		zap.String("record_id", recordID),
		zap.String("type", recordType),
		zap.String("name", name),
	)
	return nil
}

// buildEditBody 根据记录类型构建编辑请求体
func buildEditBody(recordType, content string) dns.RecordEditParamsBodyUnion {
	switch recordType {
	case "AAAA":
		return dns.AAAARecordParam{
			Content: cloudflare.F(content),
			TTL:     cloudflare.F(dns.TTL(1)),
		}
	case "PTR":
		return dns.PTRRecordParam{
			Content: cloudflare.F(content),
			TTL:     cloudflare.F(dns.TTL(1)),
		}
	default:
		return nil
	}
}
