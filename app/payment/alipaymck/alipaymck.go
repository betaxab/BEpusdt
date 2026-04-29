package alipaymck

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/v03413/bepusdt/app/payment/common"
)

var (
	ErrConfigInvalid       = errors.New("alipaymck config invalid")
	ErrRequestFailed       = errors.New("alipaymck request failed")
	ErrResponseInvalid     = errors.New("alipaymck response invalid")
	ErrSignatureInvalid    = errors.New("alipaymck signature invalid")
	ErrTradeTypeNotSupport = errors.New("alipaymck trade type not supported")
)

const (
	StatusWaiting = 1 // 等待支付
	StatusSuccess = 2 // 支付成功
	StatusExpired = 3 // 支付超时

	statusTextSuccess = "成功"
	tradeTimeLayout   = "2006-01-02 15:04:05"

	alipayOpenapiHost      = "openapi.alipay.com"
	alipayQueryBillPath    = "/v3/alipay/data/bill/sell/query"
	alipayRequestBodyEmpty = "{}"
)

// Config AlipayMck 配置。
type Config struct {
	AppId      string `json:"appid"`
	PrivateKey string `json:"privatekey"`
	PublicKey  string `json:"publickey"`
}

// CallbackData AlipayMck 账单明细（用于订单回调处理）。
type CallbackData struct {
	Raw             map[string]interface{} `json:"-"`
	TradeStatus     string                 `json:"trade_status"`
	TotalAmount     string                 `json:"total_amount"`
	AlipayOrderNo   string                 `json:"alipay_order_no"`
	MerchantOrderNo string                 `json:"merchant_order_no"`
	OtherAccount    string                 `json:"other_account"`
	GmtPayment      string                 `json:"gmt_payment"`
	GmtCreate       string                 `json:"gmt_create"`
	TransDt         string                 `json:"trans_dt"`
	TradeTime       time.Time              `json:"-"`
}

// Client Alipay OpenAPI v3 请求客户端。
type Client struct {
	AppId      string
	PrivateKey string
	PublicKey  string
	Host       string

	httpClient *http.Client
}

// ParseConfig 解析配置。
func ParseConfig(raw map[string]interface{}) (*Config, error) {
	return common.ParseConfig[Config](raw, ErrConfigInvalid)
}

// ParseConfigText 从 JSON 字符串解析配置。
func ParseConfigText(raw string) (*Config, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, fmt.Errorf("%w: empty config", ErrConfigInvalid)
	}
	var cfgMap map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &cfgMap); err != nil {
		return nil, fmt.Errorf("%w: unmarshal config failed", ErrConfigInvalid)
	}
	return ParseConfig(cfgMap)
}

// ValidateConfig 校验配置。
func ValidateConfig(cfg *Config) error {
	if cfg == nil {
		return fmt.Errorf("%w: config is nil", ErrConfigInvalid)
	}
	if !IsValidAlipayAppId(cfg.AppId) {
		return fmt.Errorf("%w: appid is invalid", ErrConfigInvalid)
	}
	if !IsValidAlipayPublicKey(cfg.PublicKey) {
		return fmt.Errorf("%w: publickey is invalid", ErrConfigInvalid)
	}
	if !IsValidAlipayPrivateKey(cfg.PrivateKey) {
		return fmt.Errorf("%w: privatekey is invalid", ErrConfigInvalid)
	}
	return nil
}

func IsValidAlipayQR(qr string) bool {
	match, err := regexp.MatchString(`^https://qr\.alipay\.com/[a-zA-Z0-9]+$`, qr)

	return match && err == nil
}

func IsValidAlipayAppId(appId string) bool {
	match, err := regexp.MatchString(`^\d{16}$`, appId)
	return match && err == nil
}

func IsValidAlipayPublicKey(key string) bool {
	if key == "" {
		return false
	}
	keyBytes, err := base64.StdEncoding.DecodeString(key)
	if err != nil {
		return false
	}

	if _, err := x509.ParsePKIXPublicKey(keyBytes); err == nil {
		return true
	}
	if _, err := x509.ParsePKCS1PublicKey(keyBytes); err == nil {
		return true
	}

	return false
}

func IsValidAlipayPrivateKey(key string) bool {
	if key == "" {
		return false
	}
	keyBytes, err := base64.StdEncoding.DecodeString(key)
	if err != nil {
		return false
	}

	if _, err := x509.ParsePKCS1PrivateKey(keyBytes); err == nil {
		return true
	}
	if _, err := x509.ParsePKCS8PrivateKey(keyBytes); err == nil {
		return true
	}

	return false
}

func (c *Config) Normalize() {
	c.AppId = strings.TrimSpace(c.AppId)
	c.PrivateKey = strings.TrimSpace(c.PrivateKey)
	c.PublicKey = strings.TrimSpace(c.PublicKey)
}

// NewClient 根据配置创建 AlipayMck 请求客户端。
func NewClient(cfg *Config) (*Client, error) {
	if cfg == nil {
		return nil, fmt.Errorf("%w: config is nil", ErrConfigInvalid)
	}
	copyCfg := *cfg
	copyCfg.Normalize()
	if err := ValidateConfig(&copyCfg); err != nil {
		return nil, err
	}

	return &Client{
		AppId:      copyCfg.AppId,
		PrivateKey: wrapPEM(copyCfg.PrivateKey, false),
		PublicKey:  wrapPEM(copyCfg.PublicKey, true),
		Host:       alipayOpenapiHost,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}, nil
}

// QuerySellBill 查询支付宝收款明细。
func (c *Client) QuerySellBill(ctx context.Context, startTime, endTime time.Time, pageNo, pageSize int) (map[string]interface{}, error) {
	if c == nil {
		return nil, fmt.Errorf("%w: client is nil", ErrConfigInvalid)
	}
	if pageNo <= 0 {
		pageNo = 1
	}
	if pageSize <= 0 {
		pageSize = 2000
	}
	if startTime.IsZero() || endTime.IsZero() {
		return nil, fmt.Errorf("%w: start/end time is required", ErrConfigInvalid)
	}
	queryParams := map[string]string{
		"start_time": startTime.Format("2006-01-02 15:04:05"),
		"end_time":   endTime.Format("2006-01-02 15:04:05"),
		"page_no":    strconv.Itoa(pageNo),
		"page_size":  strconv.Itoa(pageSize),
	}
	return c.Request(ctx, alipayQueryBillPath, queryParams)
}

// Request 发起 Alipay OpenAPI v3 GET 请求并验签。
func (c *Client) Request(ctx context.Context, path string, queryParams map[string]string) (map[string]interface{}, error) {
	if c == nil {
		return nil, fmt.Errorf("%w: client is nil", ErrConfigInvalid)
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("%w: path is required", ErrConfigInvalid)
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	q := url.Values{}
	for key, value := range queryParams {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			continue
		}
		q.Set(key, value)
	}
	queryString := q.Encode()
	httpURI := path
	if queryString != "" {
		httpURI += "?" + queryString
	}

	requestURL := "https://" + strings.TrimSpace(c.Host) + httpURI
	nonce := strconv.FormatInt(time.Now().UnixNano(), 10)
	timestamp := strconv.FormatInt(time.Now().UnixMilli(), 10)
	authString := fmt.Sprintf("app_id=%s,nonce=%s,timestamp=%s", c.AppId, nonce, timestamp)

	sign, err := c.generateSign(authString, http.MethodGet, httpURI, alipayRequestBodyEmpty)
	if err != nil {
		return nil, err
	}
	authHeader := fmt.Sprintf("ALIPAY-SHA256withRSA %s,sign=%s", authString, sign)

	if ctx == nil {
		ctx = context.Background()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, strings.NewReader(alipayRequestBodyEmpty))
	if err != nil {
		return nil, fmt.Errorf("%w: build request failed", ErrRequestFailed)
	}
	req.Header.Set("Authorization", authHeader)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrRequestFailed, err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%w: read body failed", ErrRequestFailed)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%w: status code %d", ErrResponseInvalid, resp.StatusCode)
	}

	respBody := string(bodyBytes)
	signHeader := strings.TrimSpace(resp.Header.Get("alipay-signature"))
	if signHeader != "" {
		if err := c.verifyResponse(respBody, signHeader, resp.Header.Get("alipay-timestamp"), resp.Header.Get("alipay-nonce")); err != nil {
			return nil, err
		}
	}

	var result map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &result); err != nil {
		return nil, fmt.Errorf("%w: decode response failed", ErrResponseInvalid)
	}
	return result, nil
}

// ParseCallbacks 从账单响应中解析明细回调。
func ParseCallbacks(raw map[string]interface{}) ([]*CallbackData, error) {
	if raw == nil {
		return nil, nil
	}
	detailListRaw, exists := raw["detail_list"]
	if !exists || detailListRaw == nil {
		return nil, nil
	}
	detailList, ok := detailListRaw.([]interface{})
	if !ok {
		return nil, fmt.Errorf("%w: detail_list format invalid", ErrResponseInvalid)
	}

	callbacks := make([]*CallbackData, 0, len(detailList))
	for _, item := range detailList {
		itemMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		callback, err := ParseCallbackMap(itemMap)
		if err != nil {
			continue
		}
		callbacks = append(callbacks, callback)
	}
	return callbacks, nil
}

// ParseCallback 解析单条回调 JSON。
func ParseCallback(body []byte) (*CallbackData, error) {
	if len(body) == 0 {
		return nil, fmt.Errorf("%w: callback body is empty", ErrResponseInvalid)
	}
	var raw map[string]interface{}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("%w: decode callback failed", ErrResponseInvalid)
	}
	return ParseCallbackMap(raw)
}

// ParseCallbackMap 解析单条回调对象。
func ParseCallbackMap(raw map[string]interface{}) (*CallbackData, error) {
	if raw == nil {
		return nil, fmt.Errorf("%w: callback is nil", ErrResponseInvalid)
	}
	callback := &CallbackData{
		Raw:             raw,
		TradeStatus:     strings.TrimSpace(common.ReadString(raw, "trade_status")),
		TotalAmount:     strings.TrimSpace(common.ReadString(raw, "total_amount")),
		AlipayOrderNo:   strings.TrimSpace(common.ReadString(raw, "alipay_order_no")),
		MerchantOrderNo: strings.TrimSpace(common.ReadString(raw, "merchant_order_no")),
		OtherAccount:    strings.TrimSpace(common.ReadString(raw, "other_account")),
		GmtPayment:      readStringField(raw, "gmt_payment"),
		GmtCreate:       readStringField(raw, "gmt_create"),
		TransDt:         readStringField(raw, "trans_dt"),
	}
	callback.TradeTime = parseTradeTime(callback.GmtPayment, callback.GmtCreate, callback.TransDt)
	return callback, nil
}

// VerifyCallback 校验账单回调业务数据。
func VerifyCallback(cfg *Config, data *CallbackData) error {
	if cfg == nil {
		return fmt.Errorf("%w: config is nil", ErrConfigInvalid)
	}
	if data == nil {
		return fmt.Errorf("%w: callback is nil", ErrResponseInvalid)
	}
	if strings.TrimSpace(data.TradeStatus) != statusTextSuccess {
		return fmt.Errorf("%w: trade_status is not success", ErrResponseInvalid)
	}
	if strings.TrimSpace(data.AlipayOrderNo) == "" {
		return fmt.Errorf("%w: alipay_order_no is empty", ErrResponseInvalid)
	}
	if strings.TrimSpace(data.MerchantOrderNo) == "" {
		return fmt.Errorf("%w: merchant_order_no is empty", ErrResponseInvalid)
	}
	return nil
}

func (c *Client) generateSign(authString, method, httpURI, httpBody string) (string, error) {
	privateKey, err := parsePrivateKey(c.PrivateKey)
	if err != nil {
		return "", fmt.Errorf("%w: parse private key failed", ErrSignatureInvalid)
	}
	content := fmt.Sprintf("%s\n%s\n%s\n%s\n", authString, method, httpURI, httpBody)
	hashed := sha256.Sum256([]byte(content))
	signature, err := rsa.SignPKCS1v15(rand.Reader, privateKey, crypto.SHA256, hashed[:])
	if err != nil {
		return "", fmt.Errorf("%w: sign failed", ErrSignatureInvalid)
	}
	return base64.StdEncoding.EncodeToString(signature), nil
}

func (c *Client) verifyResponse(respBody, sign, timestamp, nonce string) error {
	sign = strings.TrimSpace(sign)
	timestamp = strings.TrimSpace(timestamp)
	nonce = strings.TrimSpace(nonce)
	if respBody == "" || sign == "" {
		return fmt.Errorf("%w: response/sign is empty", ErrSignatureInvalid)
	}

	publicKey, err := parsePublicKey(c.PublicKey)
	if err != nil {
		return fmt.Errorf("%w: parse public key failed", ErrSignatureInvalid)
	}
	signBytes, err := base64.StdEncoding.DecodeString(sign)
	if err != nil {
		return fmt.Errorf("%w: decode sign failed", ErrSignatureInvalid)
	}
	content := fmt.Sprintf("%s\n%s\n%s\n", timestamp, nonce, respBody)
	hashed := sha256.Sum256([]byte(content))
	if err := rsa.VerifyPKCS1v15(publicKey, crypto.SHA256, hashed[:], signBytes); err != nil {
		return fmt.Errorf("%w: verify failed", ErrSignatureInvalid)
	}
	return nil
}

func parsePrivateKey(key string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(key))
	if block == nil {
		return nil, errors.New("private key pem decode failed")
	}
	if privateKey, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return privateKey, nil
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	privateKey, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("private key is not rsa")
	}
	return privateKey, nil
}

func parsePublicKey(key string) (*rsa.PublicKey, error) {
	block, _ := pem.Decode([]byte(key))
	if block == nil {
		return nil, errors.New("public key pem decode failed")
	}
	if parsed, err := x509.ParsePKIXPublicKey(block.Bytes); err == nil {
		if publicKey, ok := parsed.(*rsa.PublicKey); ok {
			return publicKey, nil
		}
	}
	publicKey, err := x509.ParsePKCS1PublicKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	return publicKey, nil
}

func wrapPEM(raw string, isPublic bool) string {
	raw = strings.TrimSpace(raw)
	raw = strings.ReplaceAll(raw, "-----BEGIN RSA PRIVATE KEY-----", "")
	raw = strings.ReplaceAll(raw, "-----END RSA PRIVATE KEY-----", "")
	raw = strings.ReplaceAll(raw, "-----BEGIN PRIVATE KEY-----", "")
	raw = strings.ReplaceAll(raw, "-----END PRIVATE KEY-----", "")
	raw = strings.ReplaceAll(raw, "-----BEGIN PUBLIC KEY-----", "")
	raw = strings.ReplaceAll(raw, "-----END PUBLIC KEY-----", "")
	raw = strings.ReplaceAll(raw, "-----BEGIN RSA PUBLIC KEY-----", "")
	raw = strings.ReplaceAll(raw, "-----END RSA PUBLIC KEY-----", "")
	raw = strings.ReplaceAll(raw, "\r", "")
	raw = strings.ReplaceAll(raw, "\n", "")
	raw = strings.ReplaceAll(raw, " ", "")

	header := "-----BEGIN RSA PRIVATE KEY-----"
	footer := "-----END RSA PRIVATE KEY-----"
	if isPublic {
		header = "-----BEGIN PUBLIC KEY-----"
		footer = "-----END PUBLIC KEY-----"
	}

	var sb strings.Builder
	sb.WriteString(header)
	sb.WriteByte('\n')
	for i := 0; i < len(raw); i += 64 {
		end := i + 64
		if end > len(raw) {
			end = len(raw)
		}
		sb.WriteString(raw[i:end])
		sb.WriteByte('\n')
	}
	sb.WriteString(footer)
	return sb.String()
}

func readStringField(raw map[string]interface{}, key string) string {
	value, ok := raw[key]
	if !ok || value == nil {
		return ""
	}
	switch value.(type) {
	case map[string]interface{}, []interface{}:
		return ""
	default:
		return strings.TrimSpace(common.ReadString(raw, key))
	}
}

func parseTradeTime(values ...string) time.Time {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		for _, layout := range []string{tradeTimeLayout, time.RFC3339, time.RFC3339Nano} {
			parsed, err := time.ParseInLocation(layout, value, time.Local)
			if err == nil {
				return parsed
			}
		}
	}
	return time.Time{}
}
