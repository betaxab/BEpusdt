package callback

import (
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"
)

var (
	stripeNotifyMu      sync.RWMutex
	stripeNotifyHandler gin.HandlerFunc
)

// RegisterStripeNotify registers the task-side Stripe notify processor.
func RegisterStripeNotify(handler gin.HandlerFunc) {
	stripeNotifyMu.Lock()
	defer stripeNotifyMu.Unlock()

	stripeNotifyHandler = handler
}

// HandleStripeNotify dispatches the Stripe notify request to the registered processor.
func HandleStripeNotify(ctx *gin.Context) {
	stripeNotifyMu.RLock()
	handler := stripeNotifyHandler
	stripeNotifyMu.RUnlock()

	if handler == nil {
		ctx.String(http.StatusOK, "fail")
		return
	}

	handler(ctx)
}
