package dnsmgr

import (
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"

	"go.uber.org/zap"
)

// Domain 域名信息
type Domain struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	RecordCount int    `json:"recordcount"`
	Type        string `json:"type"`
	TypeName    string `json:"typename"`
}

// Record DNS 解析记录
type Record struct {
	RecordID   string `json:"RecordId"`
	Domain     string `json:"Domain"`
	Name       string `json:"Name"`
	Type       string `json:"Type"`
	Value      string `json:"Value"`
	Line       string `json:"Line"`
	LineName   string `json:"LineName"`
	TTL        int    `json:"TTL"`
	Status     string `json:"Status"`
	Weight     int    `json:"Weight"`
	UpdateTime string `json:"UpdateTime"`
}

// ListDomains 获取当前用户所有域名
func (c *Client) ListDomains(keyword string) ([]Domain, error) {
	params := url.Values{}
	params.Set("limit", "100")
	if keyword != "" {
		params.Set("kw", keyword)
	}

	resp, err := c.post("/api/domain", params)
	if err != nil {
		return nil, err
	}

	var domains []Domain
	if err := json.Unmarshal(resp.Rows, &domains); err != nil {
		return nil, fmt.Errorf("dnsmgr: parse domain list: %w", err)
	}

	return domains, nil
}

// ListRecords 获取指定域名的解析记录列表
func (c *Client) ListRecords(domainID int, subdomain, recordType string) ([]Record, error) {
	params := url.Values{}
	params.Set("limit", "100")
	if subdomain != "" {
		params.Set("subdomain", subdomain)
	}
	if recordType != "" {
		params.Set("type", recordType)
	}

	resp, err := c.post(fmt.Sprintf("/api/record/data/%d", domainID), params)
	if err != nil {
		return nil, err
	}

	var records []Record
	if err := json.Unmarshal(resp.Rows, &records); err != nil {
		return nil, fmt.Errorf("dnsmgr: parse record list: %w", err)
	}

	return records, nil
}

// CreateRecord 新增解析记录
func (c *Client) CreateRecord(domainID int, name, recordType, value, line string, ttl int) (string, error) {
	params := url.Values{}
	params.Set("name", name)
	params.Set("type", recordType)
	params.Set("value", value)
	params.Set("line", line)
	params.Set("ttl", fmt.Sprintf("%d", ttl))

	_, err := c.post(fmt.Sprintf("/api/record/add/%d", domainID), params)
	if err != nil {
		return "", err
	}

	zap.L().Debug("dnsmgr record created",
		zap.Int("domain_id", domainID),
		zap.String("name", name),
		zap.String("type", recordType),
		zap.String("value", value),
	)
	return "", nil // dnsmgr add 接口不返回 record ID，后续查询获取
}

// UpdateRecord 修改解析记录
func (c *Client) UpdateRecord(domainID int, recordID, name, recordType, value, line string, ttl int) error {
	params := url.Values{}
	params.Set("recordid", recordID)
	params.Set("name", name)
	params.Set("type", recordType)
	params.Set("value", value)
	params.Set("line", line)
	params.Set("ttl", fmt.Sprintf("%d", ttl))

	_, err := c.post(fmt.Sprintf("/api/record/update/%d", domainID), params)
	if err != nil {
		return err
	}

	zap.L().Debug("dnsmgr record updated",
		zap.Int("domain_id", domainID),
		zap.String("record_id", recordID),
		zap.String("name", name),
	)
	return nil
}

// DeleteRecord 删除解析记录
func (c *Client) DeleteRecord(domainID int, recordID string) error {
	params := url.Values{}
	params.Set("recordid", recordID)

	_, err := c.post(fmt.Sprintf("/api/record/delete/%d", domainID), params)
	if err != nil {
		return err
	}

	zap.L().Debug("dnsmgr record deleted",
		zap.Int("domain_id", domainID),
		zap.String("record_id", recordID),
	)
	return nil
}

// FindRecord 按子域名和类型查找记录，返回 (record, found)
func (c *Client) FindRecord(domainID int, subdomain, recordType string) (*Record, error) {
	records, err := c.ListRecords(domainID, subdomain, recordType)
	if err != nil {
		return nil, err
	}
	for _, r := range records {
		if r.Name == subdomain && r.Type == recordType {
			return &r, nil
		}
	}
	return nil, nil
}

// UpsertRecord 创建或更新记录（先查，存在则更新，否则创建）
func (c *Client) UpsertRecord(domainID int, name, recordType, value, line string, ttl int) error {
	existing, err := c.FindRecord(domainID, name, recordType)
	if err != nil {
		return fmt.Errorf("upsert: find: %w", err)
	}

	if existing != nil {
		// 值相同则跳过
		if existing.Value == value {
			return nil
		}
		return c.UpdateRecord(domainID, existing.RecordID, name, recordType, value, line, ttl)
	}

	_, err = c.CreateRecord(domainID, name, recordType, value, line, ttl)
	return err
}

// DeleteRecordByName 按子域名和类型删除记录
func (c *Client) DeleteRecordByName(domainID int, name, recordType string) error {
	existing, err := c.FindRecord(domainID, name, recordType)
	if err != nil {
		return err
	}
	if existing == nil {
		return nil // 不存在，无需删除
	}
	return c.DeleteRecord(domainID, existing.RecordID)
}

// BestDomain 从域名列表中找到最精确匹配目标域名的后缀。
// 例：目标 "a.b.example.com"，候选有 "example.com" 和 "b.example.com"，
// 返回 "b.example.com" 的 Domain（最长后缀匹配）。
// 若没有匹配返回 nil。
func BestDomain(domains []Domain, target string) *Domain {
	target = strings.TrimSuffix(target, ".")
	var best *Domain
	bestLen := 0

	for i := range domains {
		d := &domains[i]
		suffix := "." + d.Name
		if target == d.Name || strings.HasSuffix(target, suffix) {
			if len(d.Name) > bestLen {
				bestLen = len(d.Name)
				best = d
			}
		}
	}

	return best
}

// SplitSubdomain 根据匹配到的域名，拆分出主机记录。
// 例：target="a.b.example.com", domain="b.example.com" -> "a"
func SplitSubdomain(target, domainName string) string {
	target = strings.TrimSuffix(target, ".")
	sub := strings.TrimSuffix(target, "."+domainName)
	if sub == domainName {
		return "@" // 完全匹配
	}
	return strings.TrimSuffix(sub, ".")
}

// LoadAllDomains 加载全部域名（处理分页，获取所有域名）
func (c *Client) LoadAllDomains() ([]Domain, error) {
	return c.ListDomains("")
}

// CacheDomains 缓存域名的内部状态，用于快速匹配
type DomainCache struct {
	domains  []Domain
	sortedBy []string // 按域名长度降序排列
}

// NewDomainCache 创建域名缓存
func NewDomainCache(domains []Domain) *DomainCache {
	sorted := make([]Domain, len(domains))
	copy(sorted, domains)
	sort.Slice(sorted, func(i, j int) bool {
		return len(sorted[i].Name) > len(sorted[j].Name) // 长域名优先
	})
	names := make([]string, len(sorted))
	for i, d := range sorted {
		names[i] = d.Name
	}
	return &DomainCache{domains: sorted, sortedBy: names}
}

// Match 在缓存中匹配目标域名，返回最佳 Domain 和子域名
func (dc *DomainCache) Match(target string) (*Domain, string) {
	d := BestDomain(dc.domains, target)
	if d == nil {
		return nil, ""
	}
	return d, SplitSubdomain(target, d.Name)
}
