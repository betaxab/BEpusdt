package router

import (
	"github.com/gin-gonic/gin"
	"github.com/v03413/bepusdt/app/handler/epusdt"
)

func epusdtInit(engine *gin.Engine) {
	epHdr := new(epusdt.Epusdt)
	engine.Use(callbackRouteMiddleware(epHdr))

	epGrp := engine.Group("/pay")
	{
		epGrp.GET("/checkout/:trade_id", epHdr.Checkout)
	}

	orderGrp := engine.Group("/api/v1/order")
	{
		orderGrp.Use(epHdr.SignVerify)
		orderGrp.POST("/create-transaction", epHdr.CreateTransaction)
		orderGrp.POST("/cancel-transaction", epHdr.CancelTransaction)
		orderGrp.POST("/create-order", epHdr.CreateOrder)
	}

	payGrp := engine.Group("/api/v1/pay")
	{
		payGrp.POST("/info", epHdr.Info)
		payGrp.POST("/notify", epHdr.Notify)
		payGrp.POST("/methods", epHdr.GetMethods)
		payGrp.POST("/update-order", epHdr.UpdateOrder)
		payGrp.POST("/duolabao/notify", epHdr.DuolabaoNotify)
		payGrp.GET("/duolabao/notify", epHdr.DuolabaoNotify)
		payGrp.GET("/duolabao/return/:trade_id", epHdr.DuolabaoReturn)
		payGrp.POST("/stripe/notify", epHdr.StripeNotify)
		payGrp.GET("/stripe/return/:trade_id", epHdr.StripeReturn)
		payGrp.GET("/stripe/cancel/:trade_id", epHdr.StripeCancel)
	}
}
