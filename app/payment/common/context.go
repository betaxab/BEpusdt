package common

import (
	"context"
	"time"
)

// DefaultTimeout 支付请求默认超时。
const DefaultTimeout = 12 * time.Second

// WithDefaultTimeout 在没有 deadline 时添加默认超时。
func WithDefaultTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	if _, ok := ctx.Deadline(); ok {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, DefaultTimeout)
}
