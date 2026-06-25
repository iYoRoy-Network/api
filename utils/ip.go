package utils

import (
	"fmt"
	"net"
	"net/netip"
	"strings"
)

// IPv6ToPTRName 将 IPv6 地址（可含 :: 简写）转换为 PTR 记录名称（反向 nibble 格式）
// 例: 2a14:7583:f244::3b06 → 6.0.b.3.0.0.0.0.0.0.0.0.0.0.0.0.4.4.2.f.3.8.5.7.4.1.a.2.ip6.arpa
func IPv6ToPTRName(ip string) (string, error) {
	addr, err := netip.ParseAddr(ip)
	if err != nil {
		return "", fmt.Errorf("invalid IPv6 address %q: %w", ip, err)
	}

	if !addr.Is6() {
		return "", fmt.Errorf("address %q is not an IPv6 address", ip)
	}

	// 展开为完整的 32 位十六进制字符串
	expanded := addr.StringExpanded()
	// 去掉冒号，得到纯十六进制字符串
	hexStr := strings.ReplaceAll(expanded, ":", "")
	// 反转 nibble
	var reversed strings.Builder
	for i := len(hexStr) - 1; i >= 0; i-- {
		if reversed.Len() > 0 {
			reversed.WriteByte('.')
		}
		reversed.WriteByte(hexStr[i])
	}
	reversed.WriteString(".ip6.arpa")

	return reversed.String(), nil
}

// IPv4ToPTRName 将 IPv4 地址转换为 PTR 记录名称（反向 octet 格式）
// 例: 10.0.0.1 → 1.0.0.10.in-addr.arpa
func IPv4ToPTRName(ip string) (string, error) {
	addr, err := netip.ParseAddr(ip)
	if err != nil {
		return "", fmt.Errorf("invalid IPv4 address %q: %w", ip, err)
	}

	if !addr.Is4() {
		return "", fmt.Errorf("address %q is not an IPv4 address", ip)
	}

	ip4 := addr.As4()
	return fmt.Sprintf("%d.%d.%d.%d.in-addr.arpa",
		ip4[3], ip4[2], ip4[1], ip4[0]), nil
}

// StripCIDRMask 去掉 IP 地址中的 /prefix 掩码部分
// 例: 2a14:7583:f244::3b06/128 → 2a14:7583:f244::3b06
func StripCIDRMask(address string) string {
	ip, _, found := strings.Cut(address, "/")
	if found {
		return ip
	}
	return address
}

// LongestPrefixMatch IPv6 最长前缀匹配：根据 IP 地址找到最匹配的反向 Zone 配置索引
// 返回匹配到的配置索引，无匹配时返回 -1
func LongestPrefixMatch(ipStr string, prefixes []string) int {
	addr, err := netip.ParseAddr(ipStr)
	if err != nil {
		return -1
	}

	bestIdx := -1
	bestBits := -1

	for i, prefixStr := range prefixes {
		prefix, err := netip.ParsePrefix(prefixStr)
		if err != nil {
			continue
		}

		if prefix.Contains(addr) {
			bits := prefix.Bits()
			if bits > bestBits {
				bestBits = bits
				bestIdx = i
			}
		}
	}

	return bestIdx
}

// IsIPv6 判断是否为 IPv6 地址
func IsIPv6(ip string) bool {
	addr, err := netip.ParseAddr(ip)
	if err != nil {
		return false
	}
	return addr.Is6()
}

// IsIPv4 判断是否为 IPv4 地址
func IsIPv4(ip string) bool {
	addr, err := netip.ParseAddr(ip)
	if err != nil {
		return false
	}
	return addr.Is4()
}

// NormalizeIP 标准化 IP 地址：去掉掩码、展开简写
func NormalizeIP(address string) (string, error) {
	ip := StripCIDRMask(address)
	addr, err := netip.ParseAddr(ip)
	if err != nil {
		return "", fmt.Errorf("invalid IP address %q: %w", address, err)
	}
	return addr.String(), nil
}

// ValidateDomainName 验证域名格式（基本校验）
func ValidateDomainName(name string) bool {
	if len(name) == 0 || len(name) > 253 {
		return false
	}
	// 简单校验：不能以 . 开头或结尾
	if strings.HasPrefix(name, ".") || strings.HasSuffix(name, ".") {
		return false
	}
	// 尝试解析为 IP 的一部分，如果能解析为 IP 说明不是合法域名
	if ip := net.ParseIP(name); ip != nil {
		return false
	}
	return true
}
