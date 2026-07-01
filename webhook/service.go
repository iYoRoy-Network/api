package webhook

import (
	"context"
	"fmt"
	"strings"

	"iyoroynet-api/cloudflare"
	"iyoroynet-api/config"
	"iyoroynet-api/dnsmgr"
	"iyoroynet-api/utils"

	"go.uber.org/zap"
)

// Service rDNS 同步服务
type Service struct {
	cfDns     *cloudflare.DNSRecordService
	cfCfg     *config.CloudflareConfig
	dmgr      *dnsmgr.Client
	dmgrCfg   *config.DnsmgrConfig
	dmgrCache *dnsmgr.DomainCache
}

// NewService 创建 rDNS 同步服务
func NewService(cfClient *cloudflare.Client, cfCfg *config.CloudflareConfig) *Service {
	return &Service{
		cfDns: cloudflare.NewDNSRecordService(cfClient),
		cfCfg: cfCfg,
	}
}

// WithDnsmgr 注入 dnsmgr 客户端和域名缓存
func (s *Service) WithDnsmgr(client *dnsmgr.Client, cfg *config.DnsmgrConfig, cache *dnsmgr.DomainCache) *Service {
	s.dmgr = client
	s.dmgrCfg = cfg
	s.dmgrCache = cache
	return s
}

// SyncResult 单次同步操作的结果
type SyncResult struct {
	Event          string `json:"event"`
	IPAddress      string `json:"ip_address"`
	DNSName        string `json:"dns_name"`
	AAAASuccess    bool   `json:"aaaa_success"`
	PTRSuccess     bool   `json:"ptr_success"`
	AAAAMessage    string `json:"aaaa_message,omitempty"`
	PTRMessage     string `json:"ptr_message,omitempty"`
	NodeDNSSuccess bool   `json:"node_dns_success,omitempty"`
	NodeDNSMessage string `json:"node_dns_message,omitempty"`
}

// ProcessWebhook 处理 NetBox webhook，同步 DNS 记录
func (s *Service) ProcessWebhook(ctx context.Context, webhook *NetBoxWebhook) (*SyncResult, error) {
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
		s.syncNodeDNSForEvent(ctx, webhook, result)

	case "updated":
		result.AAAASuccess, result.AAAAMessage = s.syncAAAA(ctx, dnsName, ip)
		result.PTRSuccess, result.PTRMessage = s.syncPTR(ctx, ip, dnsName)

		// 对比 snapshot 清理旧记录（address / dns_name）
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

			// 清理旧的 Node IANA DNS 记录
			s.cleanupOldNodeDNS(ctx, old.CustomFields, webhook.Data.CustomFields)
		}

		// 同步新的 Node IANA DNS
		s.syncNodeDNSForEvent(ctx, webhook, result)

	case "deleted":
		result.AAAASuccess, result.AAAAMessage = s.deleteAAAA(ctx, dnsName)
		result.PTRSuccess, result.PTRMessage = s.deletePTR(ctx, ip)

		// 清理对应的 Node IANA DNS
		if node := GetNodeIANA(webhook.Data.CustomFields); node != nil {
			s.deleteNodeDNS(ctx, node)
		}

	default:
		return result, nil
	}

	return result, nil
}

// syncNodeDNSForEvent 从 webhook custom_fields 提取 Node IANA 数据并同步
func (s *Service) syncNodeDNSForEvent(ctx context.Context, webhook *NetBoxWebhook, result *SyncResult) {
	if s.dmgr == nil || s.dmgrCache == nil {
		return
	}

	node := GetNodeIANA(webhook.Data.CustomFields)
	if node == nil {
		return
	}

	if err := s.syncNodeDNS(ctx, node); err != nil {
		result.NodeDNSMessage = err.Error()
		zap.L().Warn("Node IANA DNS sync failed", zap.Error(err))
	} else {
		result.NodeDNSSuccess = true
		result.NodeDNSMessage = "ok"
	}
}

// syncNodeDNS 同步 Node IANA DNS 记录
func (s *Service) syncNodeDNS(_ context.Context, node *NodeIANA) error {
	domain, subdomain := s.dmgrCache.Match(node.DNS)
	if domain == nil {
		return fmt.Errorf("no matching domain in dnsmgr for %s", node.DNS)
	}

	zap.L().Info("Syncing Node IANA DNS via dnsmgr",
		zap.String("target", node.DNS),
		zap.String("domain", domain.Name),
		zap.Int("domain_id", domain.ID),
		zap.String("subdomain", subdomain),
	)

	line := s.dmgrCfg.DefaultLine
	ttl := s.dmgrCfg.DefaultTTL

	var errs []string

	// IPv4 → A 记录
	if node.IPv4Addr != "" {
		// a.example.com → A → IPv4
		if err := s.dmgr.UpsertRecord(domain.ID, subdomain, "A", node.IPv4Addr, line, ttl); err != nil {
			errs = append(errs, fmt.Sprintf("A %s: %v", subdomain, err))
		} else {
			zap.L().Info("Node A record synced", zap.String("name", subdomain), zap.String("ip", node.IPv4Addr))
		}
		// ipv4.a.example.com → A → IPv4
		ipv4sub := "ipv4." + subdomain
		if err := s.dmgr.UpsertRecord(domain.ID, ipv4sub, "A", node.IPv4Addr, line, ttl); err != nil {
			errs = append(errs, fmt.Sprintf("A %s: %v", ipv4sub, err))
		} else {
			zap.L().Info("Node A record synced", zap.String("name", ipv4sub), zap.String("ip", node.IPv4Addr))
		}
	}

	// IPv6 → AAAA 记录
	if node.IPv6Addr != "" {
		// a.example.com → AAAA → IPv6
		if err := s.dmgr.UpsertRecord(domain.ID, subdomain, "AAAA", node.IPv6Addr, line, ttl); err != nil {
			errs = append(errs, fmt.Sprintf("AAAA %s: %v", subdomain, err))
		} else {
			zap.L().Info("Node AAAA record synced", zap.String("name", subdomain), zap.String("ip", node.IPv6Addr))
		}
		// ipv6.a.example.com → AAAA → IPv6
		ipv6sub := "ipv6." + subdomain
		if err := s.dmgr.UpsertRecord(domain.ID, ipv6sub, "AAAA", node.IPv6Addr, line, ttl); err != nil {
			errs = append(errs, fmt.Sprintf("AAAA %s: %v", ipv6sub, err))
		} else {
			zap.L().Info("Node AAAA record synced", zap.String("name", ipv6sub), zap.String("ip", node.IPv6Addr))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("sync errors: %s", strings.Join(errs, "; "))
	}
	return nil
}

// cleanupOldNodeDNS 对比新旧 custom_fields，删除被移除或改变的记录
func (s *Service) cleanupOldNodeDNS(ctx context.Context, oldCF, newCF map[string]any) {
	if s.dmgr == nil || s.dmgrCache == nil {
		return
	}

	oldNode := GetNodeIANA(oldCF)
	newNode := GetNodeIANA(newCF)

	// 如果旧值存在且与新值不同，删除旧记录
	if oldNode != nil && (newNode == nil || oldNode.DNS != newNode.DNS ||
		oldNode.IPv4Addr != newNode.IPv4Addr || oldNode.IPv6Addr != newNode.IPv6Addr) {
		zap.L().Info("Node IANA DNS changed, deleting old records",
			zap.String("old_dns", oldNode.DNS),
		)
		s.deleteNodeDNS(ctx, oldNode)
	}
}

// deleteNodeDNS 删除 Node IANA DNS 的全部记录
func (s *Service) deleteNodeDNS(ctx context.Context, node *NodeIANA) {
	domain, subdomain := s.dmgrCache.Match(node.DNS)
	if domain == nil {
		zap.L().Warn("deleteNodeDNS: no matching domain", zap.String("target", node.DNS))
		return
	}

	zap.L().Info("Deleting Node IANA DNS records",
		zap.String("domain", domain.Name),
		zap.String("subdomain", subdomain),
	)

	// 尝试删除所有可能存在的记录
	dmgrClient := s.dmgr
	if node.IPv4Addr != "" {
		_ = dmgrClient.DeleteRecordByName(domain.ID, subdomain, "A")
		_ = dmgrClient.DeleteRecordByName(domain.ID, "ipv4."+subdomain, "A")
	}
	if node.IPv6Addr != "" {
		_ = dmgrClient.DeleteRecordByName(domain.ID, subdomain, "AAAA")
		_ = dmgrClient.DeleteRecordByName(domain.ID, "ipv6."+subdomain, "AAAA")
	}
}

// ---- Cloudflare 部分（保持不动） ----

func (s *Service) syncAAAA(ctx context.Context, dnsName, ip string) (bool, string) {
	if !utils.IsIPv6(ip) {
		return false, fmt.Sprintf("not an IPv6 address: %s", ip)
	}

	for _, zone := range s.cfCfg.ForwardZones {
		if !strings.HasSuffix(dnsName, "."+zone.ZoneName) && dnsName != zone.ZoneName {
			continue
		}

		recordID, err := s.cfDns.UpsertRecord(ctx, zone.ZoneID, "AAAA", dnsName, ip)
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

	for _, revZone := range s.cfCfg.ReverseZones {
		if !strings.HasSuffix(ptrName, "."+revZone.ZoneName) && ptrName != revZone.ZoneName {
			continue
		}

		recordID, err := s.cfDns.UpsertRecord(ctx, revZone.ZoneID, "PTR", ptrName, dnsName)
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

func (s *Service) deleteAAAA(ctx context.Context, dnsName string) (bool, string) {
	for _, zone := range s.cfCfg.ForwardZones {
		if !strings.HasSuffix(dnsName, "."+zone.ZoneName) && dnsName != zone.ZoneName {
			continue
		}

		if err := s.cfDns.DeleteRecord(ctx, zone.ZoneID, "AAAA", dnsName); err != nil {
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

	for _, revZone := range s.cfCfg.ReverseZones {
		if !strings.HasSuffix(ptrName, "."+revZone.ZoneName) && ptrName != revZone.ZoneName {
			continue
		}

		if err := s.cfDns.DeleteRecord(ctx, revZone.ZoneID, "PTR", ptrName); err != nil {
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
