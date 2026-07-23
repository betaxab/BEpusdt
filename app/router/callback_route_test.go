package router

import (
	"net/http"
	"testing"

	"github.com/v03413/bepusdt/app/model"
)

func TestMatchCallbackRoute(t *testing.T) {
	routes := model.CallbackRoutes{
		PaymentNotify:  "/api/callback/payment",
		DuolabaoNotify: "/api/callback/duolabao",
		StripeWebhook:  "/api/callback/stripe",
	}

	tests := []struct {
		name        string
		path        string
		method      string
		wantKind    callbackRouteKind
		wantBlocked bool
	}{
		{name: "payment custom post", path: routes.PaymentNotify, method: http.MethodPost, wantKind: callbackRoutePaymentNotify},
		{name: "payment custom get rejected", path: routes.PaymentNotify, method: http.MethodGet},
		{name: "duolabao custom post", path: routes.DuolabaoNotify, method: http.MethodPost, wantKind: callbackRouteDuolabaoNotify},
		{name: "duolabao custom get", path: routes.DuolabaoNotify, method: http.MethodGet, wantKind: callbackRouteDuolabaoNotify},
		{name: "stripe custom post", path: routes.StripeWebhook, method: http.MethodPost, wantKind: callbackRouteStripeWebhook},
		{name: "stripe custom get rejected", path: routes.StripeWebhook, method: http.MethodGet},
		{name: "payment default blocked", path: model.DefaultPaymentNotifyPath, method: http.MethodPost, wantBlocked: true},
		{name: "duolabao default blocked", path: model.DefaultDuolabaoNotifyPath, method: http.MethodGet, wantBlocked: true},
		{name: "stripe default blocked", path: model.DefaultStripeWebhookPath, method: http.MethodPost, wantBlocked: true},
		{name: "unrelated route passes", path: "/api/v1/pay/info", method: http.MethodPost},
		{name: "unsupported method passes", path: model.DefaultPaymentNotifyPath, method: http.MethodPut},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kind, blocked := matchCallbackRoute(tt.path, tt.method, routes)
			if kind != tt.wantKind || blocked != tt.wantBlocked {
				t.Fatalf("matchCallbackRoute() = (%d, %t), want (%d, %t)", kind, blocked, tt.wantKind, tt.wantBlocked)
			}
		})
	}
}

func TestMatchCallbackRouteDefaultsPassThrough(t *testing.T) {
	routes := model.CallbackRoutes{
		PaymentNotify:  model.DefaultPaymentNotifyPath,
		DuolabaoNotify: model.DefaultDuolabaoNotifyPath,
		StripeWebhook:  model.DefaultStripeWebhookPath,
	}

	for _, path := range []string{
		model.DefaultPaymentNotifyPath,
		model.DefaultDuolabaoNotifyPath,
		model.DefaultStripeWebhookPath,
	} {
		kind, blocked := matchCallbackRoute(path, http.MethodPost, routes)
		if kind != callbackRouteNone || blocked {
			t.Fatalf("default route %s should pass through, got (%d, %t)", path, kind, blocked)
		}
	}
}
