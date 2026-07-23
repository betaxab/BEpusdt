package model

import "testing"

func TestResolveCallbackRoutes(t *testing.T) {
	tests := []struct {
		name     string
		payment  string
		duolabao string
		stripe   string
		want     CallbackRoutes
	}{
		{
			name: "defaults",
			want: CallbackRoutes{
				PaymentNotify:  DefaultPaymentNotifyPath,
				DuolabaoNotify: DefaultDuolabaoNotifyPath,
				StripeWebhook:  DefaultStripeWebhookPath,
			},
		},
		{
			name:     "custom routes",
			payment:  "/api/callback/payment-secret",
			duolabao: "/api/callback/duolabao-secret/",
			stripe:   "/api/callback/stripe-secret?ignored=1",
			want: CallbackRoutes{
				PaymentNotify:  "/api/callback/payment-secret",
				DuolabaoNotify: "/api/callback/duolabao-secret",
				StripeWebhook:  "/api/callback/stripe-secret",
			},
		},
		{
			name:     "invalid and reserved routes fall back",
			payment:  "callback/payment",
			duolabao: "/api/conf/sets",
			stripe:   "/api/v1/pay/info",
			want: CallbackRoutes{
				PaymentNotify:  DefaultPaymentNotifyPath,
				DuolabaoNotify: DefaultDuolabaoNotifyPath,
				StripeWebhook:  DefaultStripeWebhookPath,
			},
		},
		{
			name:     "similar non-reserved route remains valid",
			payment:  "/api/v1/pay/information",
			duolabao: "/api/v1/pay/duolabao-hook",
			stripe:   "/api/v1/pay/stripe-hook",
			want: CallbackRoutes{
				PaymentNotify:  "/api/v1/pay/information",
				DuolabaoNotify: "/api/v1/pay/duolabao-hook",
				StripeWebhook:  "/api/v1/pay/stripe-hook",
			},
		},
		{
			name:     "duplicate custom route falls back by priority",
			payment:  "/api/callback/shared",
			duolabao: "/api/callback/shared",
			stripe:   "/api/callback/stripe",
			want: CallbackRoutes{
				PaymentNotify:  "/api/callback/shared",
				DuolabaoNotify: DefaultDuolabaoNotifyPath,
				StripeWebhook:  "/api/callback/stripe",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveCallbackRoutes(tt.payment, tt.duolabao, tt.stripe)
			if got != tt.want {
				t.Fatalf("resolveCallbackRoutes() = %#v, want %#v", got, tt.want)
			}
		})
	}
}
