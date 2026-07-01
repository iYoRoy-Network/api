package dnsmgr

import (
	"crypto/md5"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Client dnsmgr API 客户端
type Client struct {
	baseURL string
	uid     string
	key     string
	http    *http.Client
}

// NewClient 创建 dnsmgr 客户端。
// baseURL: dnsmgr 实例地址（如 https://dns.example.com）
// uid: 用户 ID
// key: API 密钥
func NewClient(baseURL, uid, key string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		uid:     uid,
		key:     key,
		http:    &http.Client{Timeout: 30 * time.Second},
	}
}

// sign 生成请求签名: md5(uid + timestamp + key)
func (c *Client) sign(timestamp string) string {
	s := c.uid + timestamp + c.key
	h := md5.Sum([]byte(s))
	return fmt.Sprintf("%x", h)
}

// apiResponse dnsmgr 通用响应
type apiResponse struct {
	Code int             `json:"code"`
	Msg  string          `json:"msg"`
	Data json.RawMessage `json:"data"`
	// 列表接口返回的字段
	Total int              `json:"total"`
	Rows  json.RawMessage  `json:"rows"`
}

// post 发送 POST 请求，自动附加签名参数，返回解析后的通用响应
func (c *Client) post(path string, params url.Values) (*apiResponse, error) {
	ts := strconv.FormatInt(time.Now().Unix(), 10)

	if params == nil {
		params = url.Values{}
	}
	params.Set("uid", c.uid)
	params.Set("timestamp", ts)
	params.Set("sign", c.sign(ts))

	req, err := http.NewRequest("POST", c.baseURL+path, strings.NewReader(params.Encode()))
	if err != nil {
		return nil, fmt.Errorf("dnsmgr: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("dnsmgr: %s: %w", path, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("dnsmgr: read response: %w", err)
	}

	var ar apiResponse
	if err := json.Unmarshal(body, &ar); err != nil {
		return nil, fmt.Errorf("dnsmgr: parse response: %w", err)
	}

	if ar.Code != 0 {
		return nil, fmt.Errorf("dnsmgr: API error (code=%d): %s", ar.Code, ar.Msg)
	}

	return &ar, nil
}
