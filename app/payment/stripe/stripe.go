package stripe

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"
	"github.com/v03413/bepusdt/app/payment/common"
)

var (
	ErrConfigInvalid    = errors.New("stripe config invalid")
	ErrRequestFailed    = errors.New("stripe request failed")
	ErrResponseInvalid  = errors.New("stripe response invalid")
	ErrSignatureInvalid = errors.New("stripe signature invalid")
)

const (
	defaultAPIBaseURL = "https://api.stripe.com"

	MethodAlipay    = "alipay"
	MethodWechatPay = "wechat_pay"
	MethodCard      = "card"

	StatusPending = "pending"
	StatusSuccess = "success"
	StatusFailed  = "failed"
	StatusExpired = "expired"

	eventCheckoutSessionCompleted           = "checkout.session.completed"
	eventCheckoutSessionAsyncPaymentSuccess = "checkout.session.async_payment_succeeded"
	eventCheckoutSessionExpired             = "checkout.session.expired"
	eventCheckoutSessionAsyncPaymentFailed  = "checkout.session.async_payment_failed"
	eventPaymentIntentSucceeded             = "payment_intent.succeeded"
	eventPaymentIntentFailed                = "payment_intent.payment_failed"
	eventPaymentIntentCanceled              = "payment_intent.canceled"
	eventPaymentIntentProcessing            = "payment_intent.processing"

	objectCheckoutSession = "checkout.session"
	objectPaymentIntent   = "payment_intent"
)

// Config Stripe 通道配置。
type Config struct {
	SecretKey          string   `json:"secret_key"`
	WebhookSecret      string   `json:"webhook_secret"`
	APIBaseURL         string   `json:"api_base_url"`
	SuccessURL         string   `json:"success_url"`
	CancelURL          string   `json:"cancel_url"`
	PaymentMethodTypes []string `json:"payment_method_types"`
}

// CreateInput Stripe Checkout Session 创建参数。
type CreateInput struct {
	OrderNo            string
	MerchantOrderID    string
	Amount             string
	Currency           string
	Description        string
	SuccessURL         string
	CancelURL          string
	PaymentMethodTypes []string
}

// CreateResult Stripe Checkout Session 创建结果。
type CreateResult struct {
	SessionID       string
	PaymentIntentID string
	URL             string
	Status          string
	Raw             map[string]interface{}
}

// WebhookResult Stripe webhook 解析结果。
type WebhookResult struct {
	EventID         string
	EventType       string
	ProviderRef     string
	SessionID       string
	PaymentIntentID string
	OrderNo         string
	MerchantOrderID string
	Amount          string
	Currency        string
	Status          string
	PaidAt          *time.Time
	Raw             map[string]interface{}
}

// ParseConfig 解析 Stripe 配置。
func ParseConfig(raw map[string]interface{}) (*Config, error) {
	cfg, err := common.ParseConfig[Config](raw, ErrConfigInvalid)
	if err != nil {
		return nil, err
	}
	cfg.Normalize()
	return cfg, nil
}

// ParseConfigText 从 JSON 字符串解析 Stripe 配置。
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

// ValidateConfig 校验 Stripe 配置。
func ValidateConfig(cfg *Config) error {
	if cfg == nil {
		return fmt.Errorf("%w: config is nil", ErrConfigInvalid)
	}
	if strings.TrimSpace(cfg.SecretKey) == "" {
		return fmt.Errorf("%w: secret_key is required", ErrConfigInvalid)
	}
	if strings.TrimSpace(cfg.WebhookSecret) == "" {
		return fmt.Errorf("%w: webhook_secret is required", ErrConfigInvalid)
	}
	if strings.TrimSpace(cfg.APIBaseURL) == "" {
		return fmt.Errorf("%w: api_base_url is required", ErrConfigInvalid)
	}
	if _, err := url.ParseRequestURI(strings.TrimSpace(cfg.APIBaseURL)); err != nil {
		return fmt.Errorf("%w: api_base_url is invalid", ErrConfigInvalid)
	}
	if value := strings.TrimSpace(cfg.SuccessURL); value != "" {
		if _, err := url.ParseRequestURI(value); err != nil {
			return fmt.Errorf("%w: success_url is invalid", ErrConfigInvalid)
		}
	}
	if value := strings.TrimSpace(cfg.CancelURL); value != "" {
		if _, err := url.ParseRequestURI(value); err != nil {
			return fmt.Errorf("%w: cancel_url is invalid", ErrConfigInvalid)
		}
	}
	for _, method := range cfg.PaymentMethodTypes {
		if !isSupportedMethod(method) {
			return fmt.Errorf("%w: unsupported payment method %s", ErrConfigInvalid, method)
		}
	}
	return nil
}

// Normalize 规范化 Stripe 配置。
func (c *Config) Normalize() {
	if c == nil {
		return
	}
	c.SecretKey = strings.TrimSpace(c.SecretKey)
	c.WebhookSecret = strings.TrimSpace(c.WebhookSecret)
	c.APIBaseURL = strings.TrimRight(strings.TrimSpace(c.APIBaseURL), "/")
	c.SuccessURL = strings.TrimSpace(c.SuccessURL)
	c.CancelURL = strings.TrimSpace(c.CancelURL)
	for i := range c.PaymentMethodTypes {
		c.PaymentMethodTypes[i] = strings.ToLower(strings.TrimSpace(c.PaymentMethodTypes[i]))
	}
	if c.APIBaseURL == "" {
		c.APIBaseURL = defaultAPIBaseURL
	}
}

// CreatePayment 创建 Stripe Checkout Session。
func CreatePayment(ctx context.Context, cfg *Config, input CreateInput) (*CreateResult, error) {
	if cfg == nil {
		return nil, ErrConfigInvalid
	}
	cfgCopy := *cfg
	cfgCopy.Normalize()
	if err := ValidateConfig(&cfgCopy); err != nil {
		return nil, err
	}
	if ctx == nil {
		ctx = context.Background()
	}

	orderNo := strings.TrimSpace(input.OrderNo)
	if orderNo == "" {
		return nil, fmt.Errorf("%w: order_no is required", ErrConfigInvalid)
	}
	currency := strings.ToUpper(strings.TrimSpace(input.Currency))
	if currency == "" {
		return nil, fmt.Errorf("%w: currency is required", ErrConfigInvalid)
	}
	amount, err := toMinorAmount(input.Amount, currency)
	if err != nil {
		return nil, err
	}
	successURL := strings.TrimSpace(input.SuccessURL)
	if successURL == "" {
		successURL = cfgCopy.SuccessURL
	}
	cancelURL := strings.TrimSpace(input.CancelURL)
	if cancelURL == "" {
		cancelURL = cfgCopy.CancelURL
	}
	if successURL == "" {
		return nil, fmt.Errorf("%w: success_url is required", ErrConfigInvalid)
	}
	if cancelURL == "" {
		return nil, fmt.Errorf("%w: cancel_url is required", ErrConfigInvalid)
	}
	subject := strings.TrimSpace(input.Description)
	if subject == "" {
		subject = orderNo
	}

	methods := input.PaymentMethodTypes
	if len(methods) == 0 {
		methods = cfgCopy.PaymentMethodTypes
	}

	form := url.Values{}
	form.Set("mode", "payment")
	form.Set("success_url", successURL)
	form.Set("cancel_url", cancelURL)
	form.Set("client_reference_id", orderNo)
	form.Set("line_items[0][quantity]", "1")
	form.Set("line_items[0][price_data][currency]", strings.ToLower(currency))
	form.Set("line_items[0][price_data][unit_amount]", strconv.FormatInt(amount, 10))
	form.Set("line_items[0][price_data][product_data][name]", subject)
	form.Set("metadata[trade_id]", orderNo)
	form.Set("payment_intent_data[metadata][trade_id]", orderNo)
	if merchantOrderID := strings.TrimSpace(input.MerchantOrderID); merchantOrderID != "" {
		form.Set("metadata[order_id]", merchantOrderID)
		form.Set("payment_intent_data[metadata][order_id]", merchantOrderID)
	}
	for _, method := range methods {
		method = strings.ToLower(strings.TrimSpace(method))
		if method == "" {
			continue
		}
		if !isSupportedMethod(method) {
			return nil, fmt.Errorf("%w: unsupported payment method %s", ErrConfigInvalid, method)
		}
		form.Add("payment_method_types[]", method)
	}

	respBody, statusCode, err := doFormRequest(ctx, &cfgCopy, http.MethodPost, "/v1/checkout/sessions", form)
	if err != nil {
		return nil, err
	}
	if statusCode < 200 || statusCode >= 300 {
		return nil, fmt.Errorf("%w: create checkout session status %d: %s", ErrResponseInvalid, statusCode, string(respBody))
	}
	raw, err := decodeRawMap(respBody)
	if err != nil {
		return nil, err
	}
	result := &CreateResult{
		SessionID:       strings.TrimSpace(common.ReadString(raw, "id")),
		PaymentIntentID: strings.TrimSpace(readPaymentIntentID(raw)),
		URL:             strings.TrimSpace(common.ReadString(raw, "url")),
		Status:          strings.TrimSpace(common.ReadString(raw, "status")),
		Raw:             raw,
	}
	if result.SessionID == "" || result.URL == "" {
		return nil, fmt.Errorf("%w: missing session id or url", ErrResponseInvalid)
	}
	return result, nil
}

// VerifyAndParseWebhook 校验 Stripe-Signature 并解析 webhook。
func VerifyAndParseWebhook(cfg *Config, headers http.Header, body []byte, now time.Time) (*WebhookResult, error) {
	if cfg == nil {
		return nil, ErrConfigInvalid
	}
	cfgCopy := *cfg
	cfgCopy.Normalize()
	if err := ValidateConfig(&cfgCopy); err != nil {
		return nil, err
	}
	if len(body) == 0 {
		return nil, fmt.Errorf("%w: webhook body is empty", ErrResponseInvalid)
	}
	if now.IsZero() {
		now = time.Now()
	}
	if err := verifySignature(cfgCopy.WebhookSecret, headers.Get("Stripe-Signature"), body, now); err != nil {
		return nil, err
	}

	eventRaw, err := decodeRawMap(body)
	if err != nil {
		return nil, err
	}
	eventType := strings.TrimSpace(common.ReadString(eventRaw, "type"))
	data := common.ReadMap(eventRaw, "data")
	objectRaw := common.ReadMap(data, "object")
	if len(objectRaw) == 0 {
		return nil, fmt.Errorf("%w: webhook object is empty", ErrResponseInvalid)
	}

	result := &WebhookResult{
		EventID:   strings.TrimSpace(common.ReadString(eventRaw, "id")),
		EventType: eventType,
		Raw:       eventRaw,
	}
	if err := fillWebhookResult(result, eventType, objectRaw); err != nil {
		return nil, err
	}
	return result, nil
}

func fillWebhookResult(result *WebhookResult, eventType string, objectRaw map[string]interface{}) error {
	objectType := strings.TrimSpace(common.ReadString(objectRaw, "object"))
	metadata := common.ReadMap(objectRaw, "metadata")
	result.OrderNo = firstNonEmpty(
		common.ReadString(metadata, "trade_id"),
		common.ReadString(metadata, "order_no"),
		common.ReadString(objectRaw, "client_reference_id"),
	)
	result.MerchantOrderID = strings.TrimSpace(common.ReadString(metadata, "order_id"))

	switch objectType {
	case objectCheckoutSession:
		result.SessionID = strings.TrimSpace(common.ReadString(objectRaw, "id"))
		result.PaymentIntentID = strings.TrimSpace(readPaymentIntentID(objectRaw))
		result.ProviderRef = result.SessionID
		result.Currency = strings.ToUpper(strings.TrimSpace(common.ReadString(objectRaw, "currency")))
		amountMinor := readInt64(objectRaw, "amount_total")
		if amountMinor > 0 && result.Currency != "" {
			result.Amount = fromMinorAmount(amountMinor, result.Currency)
		}
	case objectPaymentIntent:
		result.PaymentIntentID = strings.TrimSpace(common.ReadString(objectRaw, "id"))
		result.ProviderRef = result.PaymentIntentID
		result.Currency = strings.ToUpper(strings.TrimSpace(common.ReadString(objectRaw, "currency")))
		amountMinor := readInt64(objectRaw, "amount_received")
		if amountMinor <= 0 {
			amountMinor = readInt64(objectRaw, "amount")
		}
		if amountMinor > 0 && result.Currency != "" {
			result.Amount = fromMinorAmount(amountMinor, result.Currency)
		}
	default:
		return fmt.Errorf("%w: unsupported webhook object %s", ErrResponseInvalid, objectType)
	}
	if created := readInt64(objectRaw, "created"); created > 0 {
		paidAt := time.Unix(created, 0)
		result.PaidAt = &paidAt
	}
	result.Status = mapEventStatus(eventType)
	if result.ProviderRef == "" {
		result.ProviderRef = firstNonEmpty(result.SessionID, result.PaymentIntentID)
	}
	if result.OrderNo == "" {
		return fmt.Errorf("%w: missing trade_id metadata", ErrResponseInvalid)
	}
	if result.ProviderRef == "" {
		return fmt.Errorf("%w: missing provider ref", ErrResponseInvalid)
	}
	return nil
}

func mapEventStatus(eventType string) string {
	switch strings.TrimSpace(eventType) {
	case eventCheckoutSessionCompleted, eventCheckoutSessionAsyncPaymentSuccess, eventPaymentIntentSucceeded:
		return StatusSuccess
	case eventCheckoutSessionExpired:
		return StatusExpired
	case eventCheckoutSessionAsyncPaymentFailed, eventPaymentIntentFailed, eventPaymentIntentCanceled:
		return StatusFailed
	case eventPaymentIntentProcessing:
		return StatusPending
	default:
		return StatusPending
	}
}

func doFormRequest(ctx context.Context, cfg *Config, method, path string, form url.Values) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, method, cfg.APIBaseURL+path, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, 0, fmt.Errorf("%w: build request failed", ErrRequestFailed)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(cfg.SecretKey, "")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("%w: %v", ErrRequestFailed, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("%w: read response failed", ErrRequestFailed)
	}
	return body, resp.StatusCode, nil
}

func verifySignature(secret, header string, body []byte, now time.Time) error {
	secret = strings.TrimSpace(secret)
	header = strings.TrimSpace(header)
	if secret == "" || header == "" {
		return fmt.Errorf("%w: missing secret or signature", ErrSignatureInvalid)
	}
	values := parseSignatureHeader(header)
	timestamp := values["t"]
	signatures := values["v1"]
	if timestamp == "" || signatures == "" {
		return fmt.Errorf("%w: malformed signature", ErrSignatureInvalid)
	}
	ts, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return fmt.Errorf("%w: invalid timestamp", ErrSignatureInvalid)
	}
	signedAt := time.Unix(ts, 0)
	if now.Sub(signedAt) > 5*time.Minute || signedAt.Sub(now) > 5*time.Minute {
		return fmt.Errorf("%w: timestamp outside tolerance", ErrSignatureInvalid)
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(timestamp))
	_, _ = mac.Write([]byte("."))
	_, _ = mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))
	for _, signature := range strings.Split(signatures, " ") {
		signature = strings.TrimSpace(signature)
		if signature == "" {
			continue
		}
		if subtle.ConstantTimeCompare([]byte(expected), []byte(signature)) == 1 {
			return nil
		}
	}
	return fmt.Errorf("%w: signature mismatch", ErrSignatureInvalid)
}

func parseSignatureHeader(header string) map[string]string {
	values := make(map[string]string)
	for _, part := range strings.Split(header, ",") {
		key, value, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			continue
		}
		if key == "v1" {
			if values[key] == "" {
				values[key] = value
			} else {
				values[key] += " " + value
			}
			continue
		}
		values[key] = value
	}
	return values
}

func decodeRawMap(body []byte) (map[string]interface{}, error) {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var raw map[string]interface{}
	if err := decoder.Decode(&raw); err != nil {
		return nil, fmt.Errorf("%w: decode json failed", ErrResponseInvalid)
	}
	if len(raw) == 0 {
		return nil, fmt.Errorf("%w: empty json", ErrResponseInvalid)
	}
	return raw, nil
}

func toMinorAmount(amountText, currency string) (int64, error) {
	amount, err := decimal.NewFromString(strings.TrimSpace(amountText))
	if err != nil || amount.LessThanOrEqual(decimal.Zero) {
		return 0, fmt.Errorf("%w: amount is invalid", ErrConfigInvalid)
	}
	scale := int32(2)
	if isZeroDecimalCurrency(currency) {
		scale = 0
	}
	minor := amount.Shift(scale)
	if !minor.Equal(minor.Truncate(0)) {
		return 0, fmt.Errorf("%w: amount precision is invalid", ErrConfigInvalid)
	}
	return minor.IntPart(), nil
}

func fromMinorAmount(amount int64, currency string) string {
	scale := int32(-2)
	if isZeroDecimalCurrency(currency) {
		scale = 0
	}
	return decimal.NewFromInt(amount).Shift(scale).String()
}

func isZeroDecimalCurrency(currency string) bool {
	switch strings.ToUpper(strings.TrimSpace(currency)) {
	case "BIF", "CLP", "DJF", "GNF", "JPY", "KMF", "KRW", "MGA", "PYG", "RWF", "UGX", "VND", "VUV", "XAF", "XOF", "XPF":
		return true
	default:
		return false
	}
}

func readPaymentIntentID(raw map[string]interface{}) string {
	value, ok := raw["payment_intent"]
	if !ok || value == nil {
		return ""
	}
	if s, ok := value.(string); ok {
		return strings.TrimSpace(s)
	}
	if m, ok := value.(map[string]interface{}); ok {
		return strings.TrimSpace(common.ReadString(m, "id"))
	}
	return ""
}

func readInt64(raw map[string]interface{}, key string) int64 {
	if raw == nil {
		return 0
	}
	value, ok := raw[key]
	if !ok || value == nil {
		return 0
	}
	switch v := value.(type) {
	case json.Number:
		n, _ := v.Int64()
		return n
	case float64:
		return int64(v)
	case int64:
		return v
	case int:
		return int64(v)
	case string:
		n, _ := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
		return n
	default:
		return 0
	}
}

func isSupportedMethod(method string) bool {
	switch strings.ToLower(strings.TrimSpace(method)) {
	case MethodAlipay, MethodWechatPay, MethodCard:
		return true
	default:
		return false
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
