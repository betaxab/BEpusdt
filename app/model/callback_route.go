package model

import (
	"net/url"
	pathpkg "path"
	"strings"
)

const (
	DefaultPaymentNotifyPath  = "/api/v1/pay/notify"
	DefaultDuolabaoNotifyPath = "/api/v1/pay/duolabao/notify"
	DefaultStripeWebhookPath  = "/api/v1/pay/stripe/notify"
)

// CallbackRoutes 保存当前生效的支付回调路由。
type CallbackRoutes struct {
	PaymentNotify  string
	DuolabaoNotify string
	StripeWebhook  string
}

var reservedCallbackRoutePrefixes = []string{
	"/api/auth/",
	"/api/conf/",
	"/api/wallet/",
	"/api/channel/",
	"/api/order/",
	"/api/rate/",
	"/api/dashboard/",
	"/api/v1/order/",
	"/api/v1/pay/duolabao/return/",
	"/api/v1/pay/stripe/return/",
	"/api/v1/pay/stripe/cancel/",
}

var reservedCallbackRoutes = map[string]struct{}{
	"/api/v1/pay/info":         {},
	"/api/v1/pay/methods":      {},
	"/api/v1/pay/update-order": {},
}

// GetCallbackRoutes 从配置缓存读取并解析当前回调路由。
// 配置为空、格式非法或发生冲突时，对应路由回退到内置默认路径。
func GetCallbackRoutes() CallbackRoutes {
	return resolveCallbackRoutes(
		GetC(PaymentNotifyRoute),
		GetC(DuolabaoNotifyRoute),
		GetC(StripeWebhookRoute),
	)
}

func resolveCallbackRoutes(paymentNotify, duolabaoNotify, stripeWebhook string) CallbackRoutes {
	routes := CallbackRoutes{
		PaymentNotify:  normalizeCallbackRoute(paymentNotify, DefaultPaymentNotifyPath, DefaultDuolabaoNotifyPath, DefaultStripeWebhookPath),
		DuolabaoNotify: normalizeCallbackRoute(duolabaoNotify, DefaultDuolabaoNotifyPath, DefaultPaymentNotifyPath, DefaultStripeWebhookPath),
		StripeWebhook:  normalizeCallbackRoute(stripeWebhook, DefaultStripeWebhookPath, DefaultPaymentNotifyPath, DefaultDuolabaoNotifyPath),
	}

	seen := make(map[string]struct{}, 3)
	values := []*string{&routes.PaymentNotify, &routes.DuolabaoNotify, &routes.StripeWebhook}
	defaults := []string{DefaultPaymentNotifyPath, DefaultDuolabaoNotifyPath, DefaultStripeWebhookPath}
	for i, value := range values {
		if _, ok := seen[*value]; ok {
			*value = defaults[i]
		}
		seen[*value] = struct{}{}
	}

	return routes
}

func normalizeCallbackRoute(value, fallback string, disallowed ...string) string {
	value = strings.TrimSpace(value)
	if index := strings.IndexAny(value, "?#"); index >= 0 {
		value = value[:index]
	}
	value = strings.TrimRight(value, "/")
	if value == "" {
		return fallback
	}
	if strings.Contains(value, "\\") || !strings.HasPrefix(value, "/api/") {
		return fallback
	}
	decoded, err := url.PathUnescape(value)
	if err != nil || decoded != value || pathpkg.Clean(value) != value {
		return fallback
	}
	for _, route := range disallowed {
		if value == route {
			return fallback
		}
	}
	if _, ok := reservedCallbackRoutes[value]; ok {
		return fallback
	}
	pathWithSlash := value + "/"
	for _, prefix := range reservedCallbackRoutePrefixes {
		if strings.HasPrefix(pathWithSlash, prefix) {
			return fallback
		}
	}
	return value
}
