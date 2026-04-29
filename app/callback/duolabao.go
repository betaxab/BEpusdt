package callback

import (
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"
)

var (
	duolabaoNotifyMu      sync.RWMutex
	duolabaoNotifyHandler gin.HandlerFunc
)

// RegisterDuolabaoNotify registers the task-side Duolabao notify processor.
func RegisterDuolabaoNotify(handler gin.HandlerFunc) {
	duolabaoNotifyMu.Lock()
	defer duolabaoNotifyMu.Unlock()

	duolabaoNotifyHandler = handler
}

// HandleDuolabaoNotify dispatches the Duolabao notify request to the registered processor.
func HandleDuolabaoNotify(ctx *gin.Context) {
	duolabaoNotifyMu.RLock()
	handler := duolabaoNotifyHandler
	duolabaoNotifyMu.RUnlock()

	if handler == nil {
		ctx.String(http.StatusOK, "fail")
		return
	}

	handler(ctx)
}
