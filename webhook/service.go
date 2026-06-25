package webhook

import (
	"context"
	"fmt"
	"strings"

	"iyoroynet-api/cloudflare"
	"iyoroynet-api/config"
	"iyoroynet-api/utils"

	"go.uber.org/zap"
)

// Service rDNS 同步服务
type Service struct {
	dns    *cloudflare.DNSRecordService
	cfg    *config.CloudflareConfig
}

// NewService 创建 rDNS 同步服务
func NewService(cfClient *cloudflare.Client, cfg *config.CloudflareConfig) *Service {
	return &Service{
		dns: cloudflare.NewDNSRecordService(cfClient),
		cfg: cfg,
	}
}

// SyncResult 单次同步操作的结果
type SyncResult struct {
	Event       string `json:"event"`
	IPAddress   string `json:"ip_address"`
	DNSName     string `json:"dns_name"`
	AAAASuccess bool   `json:"aaaa_success"`
	PTRSuccess  bool   `json:"ptr_success"`
	AAAAMessage string `json:"aaaa_message,omitempty"`
	PTRMessage  string `json:"ptr_message,omitempty"`
}

// ProcessWebhook 处理 NetBox webhook，同步 DNS 记录
func (s *Service) ProcessWebhook(ctx context.Context, webhook *NetBoxWebhook) (*SyncResult, error) {
	// 标准化 IP 地址（去掉掩码）
	ip, err := utils.NormalizeIP(webhook.Data.Address)
	if err != nil {
		return nil, fmt.Errorf("invalid IP address: %w", err)
	}

	dnsName := strings.TrimSuffix(webhook.Data.DNSName, ".")

	result := &SyncResult{
		Event:     webhook.Event,
		IPAddress: ip,
		DNSName:   dnsName,
	}

	zap.L().Info("Processing webhook",
		zap.String("event", webhook.Event),
		zap.String("ip", ip),
		zap.String("dns_name", dnsName),
	)

	switch webhook.Event {
	case "created":
		result.AAAASuccess, result.AAAAMessage = s.syncAAAA(ctx, dnsName, ip)
		result.PTRSuccess, result.PTRMessage = s.syncPTR(ctx, ip, dnsName)

	case "updated":
		// 同步新数据
		result.AAAASuccess, result.AAAAMessage = s.syncAAAA(ctx, dnsName, ip)
		result.PTRSuccess, result.PTRMessage = s.syncPTR(ctx, ip, dnsName)

		// 清理旧记录：对比 snapshot 中的旧值，不同则删除
		if old := webhook.PreChangeData(); old != nil {
			oldIP, _ := utils.NormalizeIP(old.Address)
			oldDNS := strings.TrimSuffix(old.DNSName, ".")

			if oldDNS != "" && oldDNS != dnsName {
				zap.L().Info("DNS name changed, deleting old AAAA record",
					zap.String("old", oldDNS), zap.String("new", dnsName))
				s.deleteAAAA(ctx, oldDNS)
			}
			if oldIP != "" && oldIP != ip {
				zap.L().Info("IP changed, deleting old PTR record",
					zap.String("old", oldIP), zap.String("new", ip))
				s.deletePTR(ctx, oldIP)
			}
		}

	case "deleted":
		result.AAAASuccess, result.AAAAMessage = s.deleteAAAA(ctx, dnsName)
		result.PTRSuccess, result.PTRMessage = s.deletePTR(ctx, ip)

	default:
		return result, nil
	}

	return result, nil
}

// syncAAAA 同步 AAAA 记录（前向 DNS：域名 → IP）
func (s *Service) syncAAAA(ctx context.Context, dnsName, ip string) (bool, string) {
	if !utils.IsIPv6(ip) {
		return false, fmt.Sprintf("not an IPv6 address: %s", ip)
	}

	for _, zone := range s.cfg.ForwardZones {
		if !strings.HasSuffix(dnsName, "."+zone.ZoneName) && dnsName != zone.ZoneName {
			continue
		}

		recordID, err := s.dns.UpsertRecord(ctx, zone.ZoneID, "AAAA", dnsName, ip)
		if err != nil {
			return false, fmt.Sprintf("AAAA upsert failed: %v", err)
		}

		zap.L().Info("AAAA record synced",
			zap.String("zone", zone.ZoneName),
			zap.String("record_id", recordID),
			zap.String("name", dnsName),
			zap.String("ip", ip),
		)
		return true, fmt.Sprintf("AAAA record %s → %s (zone: %s)", dnsName, ip, zone.ZoneName)
	}

	return false, fmt.Sprintf("no matching forward zone for %s", dnsName)
}

// syncPTR 同步 PTR 记录（反向 DNS：IP → 域名）
func (s *Service) syncPTR(ctx context.Context, ip, dnsName string) (bool, string) {
	var ptrName string
	var err error

	if utils.IsIPv6(ip) {
		ptrName, err = utils.IPv6ToPTRName(ip)
	} else if utils.IsIPv4(ip) {
		ptrName, err = utils.IPv4ToPTRName(ip)
	} else {
		return false, fmt.Sprintf("unsupported IP address: %s", ip)
	}

	if err != nil {
		return false, fmt.Sprintf("PTR name computation failed: %v", err)
	}

	// 通过最长前缀匹配找到对应的反向 zone
	for _, revZone := range s.cfg.ReverseZones {
		if !strings.HasSuffix(ptrName, "."+revZone.ZoneName) && ptrName != revZone.ZoneName {
			continue
		}

		recordID, err := s.dns.UpsertRecord(ctx, revZone.ZoneID, "PTR", ptrName, dnsName)
		if err != nil {
			return false, fmt.Sprintf("PTR upsert failed: %v", err)
		}

		zap.L().Info("PTR record synced",
			zap.String("zone", revZone.ZoneName),
			zap.String("record_id", recordID),
			zap.String("ptr_name", ptrName),
			zap.String("dns_name", dnsName),
		)
		return true, fmt.Sprintf("PTR record %s → %s (zone: %s)", ptrName, dnsName, revZone.ZoneName)
	}

	return false, fmt.Sprintf("no matching reverse zone for %s (PTR: %s)", ip, ptrName)
}

// deleteAAAA 删除 AAAA 记录
func (s *Service) deleteAAAA(ctx context.Context, dnsName string) (bool, string) {
	for _, zone := range s.cfg.ForwardZones {
		if !strings.HasSuffix(dnsName, "."+zone.ZoneName) && dnsName != zone.ZoneName {
			continue
		}

		if err := s.dns.DeleteRecord(ctx, zone.ZoneID, "AAAA", dnsName); err != nil {
			return false, fmt.Sprintf("AAAA delete failed: %v", err)
		}

		zap.L().Info("AAAA record deleted",
			zap.String("zone", zone.ZoneName),
			zap.String("name", dnsName),
		)
		return true, fmt.Sprintf("AAAA record %s deleted (zone: %s)", dnsName, zone.ZoneName)
	}

	return false, fmt.Sprintf("no matching forward zone for %s", dnsName)
}

// deletePTR 删除 PTR 记录
func (s *Service) deletePTR(ctx context.Context, ip string) (bool, string) {
	var ptrName string
	var err error

	if utils.IsIPv6(ip) {
		ptrName, err = utils.IPv6ToPTRName(ip)
	} else if utils.IsIPv4(ip) {
		ptrName, err = utils.IPv4ToPTRName(ip)
	} else {
		return false, fmt.Sprintf("unsupported IP address: %s", ip)
	}

	if err != nil {
		return false, fmt.Sprintf("PTR name computation failed: %v", err)
	}

	for _, revZone := range s.cfg.ReverseZones {
		if !strings.HasSuffix(ptrName, "."+revZone.ZoneName) && ptrName != revZone.ZoneName {
			continue
		}

		if err := s.dns.DeleteRecord(ctx, revZone.ZoneID, "PTR", ptrName); err != nil {
			return false, fmt.Sprintf("PTR delete failed: %v", err)
		}

		zap.L().Info("PTR record deleted",
			zap.String("zone", revZone.ZoneName),
			zap.String("ptr_name", ptrName),
		)
		return true, fmt.Sprintf("PTR record %s deleted (zone: %s)", ptrName, revZone.ZoneName)
	}

	return false, fmt.Sprintf("no matching reverse zone for %s (PTR: %s)", ip, ptrName)
}
