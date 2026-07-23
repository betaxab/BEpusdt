package router

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/v03413/bepusdt/app/handler/epusdt"
	"github.com/v03413/bepusdt/app/model"
)

type callbackRouteKind uint8

const (
	callbackRouteNone callbackRouteKind = iota
	callbackRoutePaymentNotify
	callbackRouteDuolabaoNotify
	callbackRouteStripeWebhook
)

// callbackRouteMiddleware 将运行时配置的回调路径分发到对应处理器。
// 自定义路径启用后，原默认路径返回 404，避免同时暴露两套路由。
func callbackRouteMiddleware(handler *epusdt.Epusdt) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		path := strings.TrimRight(ctx.Request.URL.Path, "/")
		kind, blockDefault := matchCallbackRoute(path, ctx.Request.Method, model.GetCallbackRoutes())
		switch kind {
		case callbackRoutePaymentNotify:
			handler.Notify(ctx)
		case callbackRouteDuolabaoNotify:
			handler.DuolabaoNotify(ctx)
		case callbackRouteStripeWebhook:
			handler.StripeNotify(ctx)
		default:
			if blockDefault {
				ctx.AbortWithStatus(http.StatusNotFound)
				return
			}
			ctx.Next()
			return
		}
		ctx.Abort()
	}
}

func matchCallbackRoute(path, method string, routes model.CallbackRoutes) (callbackRouteKind, bool) {
	if method != http.MethodPost && method != http.MethodGet {
		return callbackRouteNone, false
	}

	if routes.PaymentNotify != model.DefaultPaymentNotifyPath && path == routes.PaymentNotify && method == http.MethodPost {
		return callbackRoutePaymentNotify, false
	}
	if routes.DuolabaoNotify != model.DefaultDuolabaoNotifyPath && path == routes.DuolabaoNotify &&
		(method == http.MethodPost || method == http.MethodGet) {
		return callbackRouteDuolabaoNotify, false
	}
	if routes.StripeWebhook != model.DefaultStripeWebhookPath && path == routes.StripeWebhook && method == http.MethodPost {
		return callbackRouteStripeWebhook, false
	}

	switch path {
	case model.DefaultPaymentNotifyPath:
		return callbackRouteNone, routes.PaymentNotify != model.DefaultPaymentNotifyPath
	case model.DefaultDuolabaoNotifyPath:
		return callbackRouteNone, routes.DuolabaoNotify != model.DefaultDuolabaoNotifyPath
	case model.DefaultStripeWebhookPath:
		return callbackRouteNone, routes.StripeWebhook != model.DefaultStripeWebhookPath
	default:
		return callbackRouteNone, false
	}
}
