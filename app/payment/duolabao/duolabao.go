package duolabao

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/shopspring/decimal"
	"github.com/v03413/bepusdt/app/payment/common"
)

var (
	ErrConfigInvalid       = errors.New("duolabao config invalid")
	ErrRequestFailed       = errors.New("duolabao request failed")
	ErrResponseInvalid     = errors.New("duolabao response invalid")
	ErrSignatureInvalid    = errors.New("duolabao signature invalid")
	ErrTradeTypeNotSupport = errors.New("duolabao trade type not supported")
)

const (
	StatusWaiting = 1 // 等待支付
	StatusSuccess = 2 // 支付成功
	StatusExpired = 3 // 支付超时

	defaultGatewayURL = "https://openapi.duolabao.com"
	createPath        = "/api/generateQRCodeUrl"

	defaultVersion      = "V4.0"
	defaultSubOrderType = "NORMAL"
	defaultOrderType    = "SALES"
	defaultBusinessType = "QRCODE_TRAD"
	defaultPayModel     = "ONCE"
	defaultSource       = "API"
)

// Config 哆啦宝配置。
type Config struct {
	GatewayURL  string `json:"gateway_url"`
	AccessKey   string `json:"access_key"`
	SecretKey   string `json:"secret_key"`
	AgentNum    string `json:"agent_num"`
	CustomerNum string `json:"customer_num"`
	ShopNum     string `json:"shop_num"`
	Version     string `json:"version"`
	NotifyURL   string `json:"callback_url"`
	ReturnURL   string `json:"complete_url"`
}

// CreateInput 哆啦宝下单输入。
type CreateInput struct {
	OrderNo      string
	Amount       string
	NotifyURL    string
	ReturnURL    string
	ClientIP     string
	TimeExpire   string
	SubOrderType string
	OrderType    string
	BusinessType string
	PayModel     string
	Source       string
	ExtraInfo    string
	OutShopID    string
}

// CreateResult 哆啦宝下单返回。
type CreateResult struct {
	Msg     string
	Code    string
	Success bool
	URL     string
	Raw     map[string]interface{}
}

// CallbackData 哆啦宝支付回调。
type CallbackData struct {
	Raw            map[string]interface{}
	CustomerNum    string
	RequestNum     string
	OrderNum       string
	CompleteTime   string
	OrderAmount    string
	BankRequestNum string
	BankStatus     string
	Status         string
	ExtraInfo      string
	BankOutTradeNo string
	SubOpenID      string
	TradeType      string
	OrderType      string
	SubOrderType   string
}

// ParseConfig 解析配置。
func ParseConfig(raw map[string]interface{}) (*Config, error) {
	cfg, err := common.ParseConfig[Config](raw, ErrConfigInvalid)
	if err != nil {
		return nil, err
	}

	cfg.Normalize()
	return cfg, nil
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

// ValidateConfig 校验配置完整性。
func ValidateConfig(cfg *Config) error {
	if cfg == nil {
		return fmt.Errorf("%w: config is nil", ErrConfigInvalid)
	}
	if strings.TrimSpace(cfg.GatewayURL) == "" {
		return fmt.Errorf("%w: gateway_url is required", ErrConfigInvalid)
	}
	if strings.TrimSpace(cfg.AccessKey) == "" {
		return fmt.Errorf("%w: access_key is required", ErrConfigInvalid)
	}
	if strings.TrimSpace(cfg.SecretKey) == "" {
		return fmt.Errorf("%w: secret_key is required", ErrConfigInvalid)
	}
	if strings.TrimSpace(cfg.CustomerNum) == "" {
		return fmt.Errorf("%w: customer_num is required", ErrConfigInvalid)
	}
	if _, err := url.ParseRequestURI(strings.TrimSpace(cfg.GatewayURL)); err != nil {
		return fmt.Errorf("%w: gateway_url is invalid", ErrConfigInvalid)
	}
	return nil
}

func (c *Config) Normalize() {
	c.GatewayURL = strings.TrimRight(strings.TrimSpace(c.GatewayURL), "/")
	c.AccessKey = strings.TrimSpace(c.AccessKey)
	c.SecretKey = strings.TrimSpace(c.SecretKey)
	c.AgentNum = strings.TrimSpace(c.AgentNum)
	c.CustomerNum = strings.TrimSpace(c.CustomerNum)
	c.ShopNum = strings.TrimSpace(c.ShopNum)
	c.Version = strings.TrimSpace(c.Version)
	c.NotifyURL = strings.TrimSpace(c.NotifyURL)
	c.ReturnURL = strings.TrimSpace(c.ReturnURL)
	if c.GatewayURL == "" {
		c.GatewayURL = defaultGatewayURL
	}
	if c.Version == "" {
		c.Version = defaultVersion
	}
}

// CreatePayment 发起哆啦宝下单。
func CreatePayment(ctx context.Context, cfg *Config, input CreateInput) (*CreateResult, error) {
	if cfg == nil {
		return nil, ErrConfigInvalid
	}
	cfgCopy := *cfg
	cfgCopy.Normalize()
	if err := ValidateConfig(&cfgCopy); err != nil {
		return nil, err
	}

	orderNo := strings.TrimSpace(input.OrderNo)
	if orderNo == "" {
		return nil, fmt.Errorf("%w: order_no is required", ErrConfigInvalid)
	}
	amount, err := decimal.NewFromString(strings.TrimSpace(input.Amount))
	if err != nil || amount.LessThanOrEqual(decimal.Zero) {
		return nil, fmt.Errorf("%w: amount is invalid", ErrConfigInvalid)
	}

	notifyURL := strings.TrimSpace(input.NotifyURL)
	if notifyURL == "" {
		notifyURL = cfgCopy.NotifyURL
	}
	if notifyURL == "" {
		return nil, fmt.Errorf("%w: callbackUrl is required", ErrConfigInvalid)
	}
	returnURL := strings.TrimSpace(input.ReturnURL)
	if returnURL == "" {
		returnURL = cfgCopy.ReturnURL
	}

	timeExpire := strings.TrimSpace(input.TimeExpire)
	if timeExpire == "" {
		timeExpire = time.Now().Add(2 * time.Hour).Format("2006-01-02 15:04:05")
	}
	subOrderType := strings.ToUpper(strings.TrimSpace(input.SubOrderType))
	if subOrderType == "" {
		subOrderType = defaultSubOrderType
	}
	orderType := strings.ToUpper(strings.TrimSpace(input.OrderType))
	if orderType == "" {
		orderType = defaultOrderType
	}
	businessType := strings.ToUpper(strings.TrimSpace(input.BusinessType))
	if businessType == "" {
		businessType = defaultBusinessType
	}
	payModel := strings.ToUpper(strings.TrimSpace(input.PayModel))
	if payModel == "" {
		payModel = defaultPayModel
	}
	source := strings.ToUpper(strings.TrimSpace(input.Source))
	if source == "" {
		source = defaultSource
	}

	payload := map[string]interface{}{
		"version":      cfgCopy.Version,
		"customerNum":  cfgCopy.CustomerNum,
		"requestNum":   orderNo,
		"orderAmount":  amount.Round(2).StringFixed(2),
		"callbackUrl":  notifyURL,
		"subOrderType": subOrderType,
		"orderType":    orderType,
		"timeExpire":   timeExpire,
		"businessType": businessType,
		"payModel":     payModel,
		"source":       source,
	}
	if cfgCopy.AgentNum != "" {
		payload["agentNum"] = cfgCopy.AgentNum
	}
	if cfgCopy.ShopNum != "" {
		payload["shopNum"] = cfgCopy.ShopNum
	}
	if returnURL != "" {
		payload["completeUrl"] = returnURL
	}
	if value := strings.TrimSpace(input.ClientIP); value != "" {
		payload["clientIp"] = value
	}
	if value := strings.TrimSpace(input.ExtraInfo); value != "" {
		payload["extraInfo"] = value
	}
	if value := strings.TrimSpace(input.OutShopID); value != "" {
		payload["outShopId"] = value
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("%w: marshal payload failed", ErrConfigInvalid)
	}
	timestamp := fmt.Sprintf("%d", time.Now().Unix())
	token := SignRequestToken(cfgCopy.SecretKey, timestamp, createPath, body)

	respBody, err := postJSON(ctx, cfgCopy.GatewayURL+createPath, body, cfgCopy.AccessKey, timestamp, token)
	if err != nil {
		return nil, err
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(respBody, &raw); err != nil {
		return nil, fmt.Errorf("%w: decode response failed", ErrResponseInvalid)
	}

	result := &CreateResult{
		Msg:     strings.TrimSpace(common.ReadString(raw, "msg")),
		Code:    strings.TrimSpace(common.ReadString(raw, "code")),
		Success: parseBool(raw, "success") || parseBool(raw, "result"),
		URL:     strings.TrimSpace(common.ReadString(raw, "url")),
		Raw:     raw,
	}
	if result.Success {
		if result.URL == "" {
			return nil, fmt.Errorf("%w: url is empty", ErrResponseInvalid)
		}
		return result, nil
	}

	if errCode := strings.TrimSpace(common.ReadString(raw, "errorCode")); errCode != "" {
		errMsg := strings.TrimSpace(common.ReadString(raw, "errorMsg"))
		if errMsg == "" {
			errMsg = strings.TrimSpace(common.ReadString(raw, "message"))
		}
		if errMsg == "" {
			errMsg = "request failed"
		}
		return nil, fmt.Errorf("%w: [%s] %s", ErrResponseInvalid, errCode, errMsg)
	}

	errMsg := result.Msg
	if errMsg == "" {
		errMsg = strings.TrimSpace(common.ReadString(raw, "message"))
	}
	if errMsg == "" {
		errMsg = "request failed"
	}
	return nil, fmt.Errorf("%w: %s", ErrResponseInvalid, errMsg)
}

// ParseCallback 解析哆啦宝回调请求体。
func ParseCallback(body []byte) (*CallbackData, error) {
	if len(body) == 0 {
		return nil, fmt.Errorf("%w: callback body is empty", ErrResponseInvalid)
	}

	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var payload map[string]interface{}
	if err := decoder.Decode(&payload); err != nil {
		return nil, fmt.Errorf("%w: decode callback failed", ErrResponseInvalid)
	}
	if len(payload) == 0 {
		return nil, fmt.Errorf("%w: callback payload is empty", ErrResponseInvalid)
	}

	// 文档示例包含 text 字段（JSON string），优先展开。
	if text := strings.TrimSpace(common.ReadString(payload, "text")); text != "" && common.ReadString(payload, "requestNum") == "" {
		var textPayload map[string]interface{}
		if err := json.Unmarshal([]byte(text), &textPayload); err == nil && len(textPayload) > 0 {
			payload = textPayload
		}
	}

	return &CallbackData{
		Raw:            payload,
		CustomerNum:    strings.TrimSpace(common.ReadString(payload, "customerNum")),
		RequestNum:     strings.TrimSpace(common.ReadString(payload, "requestNum")),
		OrderNum:       strings.TrimSpace(common.ReadString(payload, "orderNum")),
		CompleteTime:   strings.TrimSpace(common.ReadString(payload, "completeTime")),
		OrderAmount:    strings.TrimSpace(common.ReadString(payload, "orderAmount")),
		BankRequestNum: strings.TrimSpace(common.ReadString(payload, "bankRequestNum")),
		BankStatus:     strings.TrimSpace(common.ReadString(payload, "bankStatus")),
		Status:         strings.ToUpper(strings.TrimSpace(common.ReadString(payload, "status"))),
		ExtraInfo:      strings.TrimSpace(common.ReadString(payload, "extraInfo")),
		BankOutTradeNo: strings.TrimSpace(common.ReadString(payload, "bankOutTradeNum")),
		SubOpenID:      strings.TrimSpace(common.ReadString(payload, "subOpenId")),
		TradeType:      strings.TrimSpace(common.ReadString(payload, "tradeType")),
		OrderType:      strings.ToUpper(strings.TrimSpace(common.ReadString(payload, "orderType"))),
		SubOrderType:   strings.ToUpper(strings.TrimSpace(common.ReadString(payload, "subOrderType"))),
	}, nil
}

// VerifyCallback 校验回调业务状态。
func VerifyCallback(cfg *Config, data *CallbackData) error {
	if cfg == nil {
		return fmt.Errorf("%w: config is nil", ErrConfigInvalid)
	}
	if data == nil {
		return fmt.Errorf("%w: callback is nil", ErrResponseInvalid)
	}
	if data.RequestNum == "" {
		return fmt.Errorf("%w: requestNum is empty", ErrResponseInvalid)
	}
	if strings.TrimSpace(cfg.CustomerNum) != "" && data.CustomerNum != "" &&
		!strings.EqualFold(strings.TrimSpace(cfg.CustomerNum), strings.TrimSpace(data.CustomerNum)) {
		return fmt.Errorf("%w: customerNum mismatch", ErrResponseInvalid)
	}

	// 按文档“支付回调请求参数”进行校验，退款回调参数不在此解析范围。
	if data.OrderType == "REFUND" {
		return fmt.Errorf("%w: refund callback is not supported in payment parser", ErrTradeTypeNotSupport)
	}

	if data.Status != "SUCCESS" {
		return fmt.Errorf("%w: status is not success", ErrResponseInvalid)
	}
	return nil
}

// VerifyNotifySignature 按文档规则验签：
// POST: secretKey + timestamp + body
// GET : secretKey + timestamp
func VerifyNotifySignature(cfg *Config, body []byte, timestamp, token, method string) error {
	if cfg == nil {
		return fmt.Errorf("%w: config is nil", ErrConfigInvalid)
	}
	timestamp = strings.TrimSpace(timestamp)
	token = strings.TrimSpace(token)
	if timestamp == "" || token == "" {
		return fmt.Errorf("%w: timestamp/token is required", ErrSignatureInvalid)
	}

	method = strings.ToUpper(strings.TrimSpace(method))
	useBody := method == "POST" || method == "PUT" || method == "PATCH"
	expected := SignNotifyToken(cfg.SecretKey, timestamp, body, useBody)
	if !strings.EqualFold(expected, token) {
		return fmt.Errorf("%w: token mismatch", ErrSignatureInvalid)
	}
	return nil
}

// VerifyNotifySignatureByHeader 从 HTTP Header 中读取 timestamp/token 并验签。
func VerifyNotifySignatureByHeader(cfg *Config, body []byte, headers http.Header, method string) error {
	if headers == nil {
		return fmt.Errorf("%w: headers is nil", ErrSignatureInvalid)
	}
	timestamp := firstNonEmpty(
		headers.Get("timestamp"),
		headers.Get("Timestamp"),
		headers.Get("X-Timestamp"),
		headers.Get("x-timestamp"),
	)
	token := firstNonEmpty(
		headers.Get("token"),
		headers.Get("Token"),
		headers.Get("X-Token"),
		headers.Get("x-token"),
	)
	return VerifyNotifySignature(cfg, body, timestamp, token, method)
}

// SignRequestToken 生成下单签名（secretKey+timestamp+path+body）。
func SignRequestToken(secretKey, timestamp, path string, body []byte) string {
	signText := fmt.Sprintf("secretKey=%s&timestamp=%s", strings.TrimSpace(secretKey), strings.TrimSpace(timestamp))
	if strings.TrimSpace(path) != "" {
		signText += "&path=" + strings.TrimSpace(path)
	}
	if len(body) > 0 {
		signText += "&body=" + string(body)
	}
	return sha1Upper(signText)
}

// SignNotifyToken 生成通知验签 token。
func SignNotifyToken(secretKey, timestamp string, body []byte, includeBody bool) string {
	signText := fmt.Sprintf("secretKey=%s&timestamp=%s", strings.TrimSpace(secretKey), strings.TrimSpace(timestamp))
	if includeBody && len(body) > 0 {
		signText += "&body=" + string(body)
	}
	return sha1Upper(signText)
}

func postJSON(ctx context.Context, endpoint string, body []byte, accessKey, timestamp, token string) ([]byte, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := common.WithDefaultTimeout(ctx)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimSpace(endpoint), bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("%w: build request failed", ErrRequestFailed)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "*/*")
	req.Header.Set("accessKey", strings.TrimSpace(accessKey))
	req.Header.Set("timestamp", strings.TrimSpace(timestamp))
	req.Header.Set("token", strings.TrimSpace(token))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: http request failed", ErrRequestFailed)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%w: read response failed", ErrRequestFailed)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%w: status %d", ErrResponseInvalid, resp.StatusCode)
	}
	return respBody, nil
}

func sha1Upper(text string) string {
	sum := sha1.Sum([]byte(text))
	return strings.ToUpper(hex.EncodeToString(sum[:]))
}

func parseBool(raw map[string]interface{}, key string) bool {
	value, ok := raw[key]
	if !ok || value == nil {
		return false
	}
	if v, ok := value.(bool); ok {
		return v
	}
	text := strings.ToLower(strings.TrimSpace(fmt.Sprintf("%v", value)))
	return text == "1" || text == "true"
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
