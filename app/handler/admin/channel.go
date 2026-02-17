package admin

import (
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/v03413/bepusdt/app/handler/base"
	"github.com/v03413/bepusdt/app/model"
)

type Channel struct {
}

type cAddReq struct {
	Name        string `json:"name"`
	Remark      string `json:"remark"`
	Qrcode      string `json:"qrcode" binding:"required"`
	Config      string `json:"config"`
	TradeType   string `json:"trade_type" binding:"required"`
	OtherNotify uint8  `json:"other_notify"`
}

type cModReq struct {
	base.IDRequest
	Name        *string `json:"name"`
	Status      *uint8  `json:"status"`
	Qrcode      *string `json:"qrcode"`
	Config      *string `json:"config"`
	Remark      *string `json:"remark"`
	TradeType   *string `json:"trade_type"`
	OtherNotify *uint8  `json:"other_notify"`
}

type cListReq struct {
	base.ListRequest
	Name    string `json:"name"`
	Qrcode  string `json:"qrcode"`
	Config  string `json:"config"`
	Trade   string `json:"trade_type"`
}

func (Channel) Add(ctx *gin.Context) {
	var req cAddReq
	if err := ctx.ShouldBindJSON(&req); err != nil {
		base.BadRequest(ctx, err.Error())

		return
	}

	if !model.IsSupportedTradeType(model.TradeType(req.TradeType)) {
		base.BadRequest(ctx, fmt.Sprintf("不支持的交易类型: %s", req.TradeType))

		return
	}

	var channel = model.Channel{
		Name:        strings.TrimSpace(req.Name),
		Remark:      req.Remark,
		Qrcode:      strings.TrimSpace(req.Qrcode),
		Config:      req.Config,
		TradeType:   req.TradeType,
		Status:      model.WaStatusEnable,
		OtherNotify: req.OtherNotify,
	}

	if err := channel.Validate(); err != nil {
		base.BadRequest(ctx, err.Error())

		return
	}

	if err := model.Db.Create(&channel).Error; err != nil {
		base.Error(ctx, err)

		return
	}

	base.Response(ctx, 200, "success")
}

func (Channel) List(ctx *gin.Context) {
	var req cListReq
	if err := ctx.ShouldBind(&req); err != nil {
		base.Response(ctx, 400, err.Error())

		return
	}

	var data []model.Channel
	var db = model.Db

	if req.Name != "" {
		db = db.Where("name LIKE ?", "%"+req.Name+"%")
	}
	if req.Qrcode != "" {
		db = db.Where("qrcode LIKE ?", "%"+req.Qrcode+"%")
	}
	if req.Config != "" {
		db = db.Where("config LIKE ?", "%"+req.Config+"%")
	}
	if req.Trade != "" {
		db = db.Where("trade_type LIKE ?", "%"+req.Trade+"%")
	}

	var total int64

	db.Model(&model.Channel{}).Count(&total)

	err := db.Limit(req.Size).Offset((req.Page - 1) * req.Size).Order("id " + req.Sort).Find(&data).Error
	if err != nil {
		base.Response(ctx, 400, err.Error())

		return
	}

	base.Response(ctx, 200, data, total)
}

func (Channel) Mod(ctx *gin.Context) {
	var req cModReq
	if err := ctx.ShouldBindJSON(&req); err != nil {
		base.BadRequest(ctx, err.Error())

		return
	}

	var c model.Channel
	model.Db.Where("id = ?", req.ID).Find(&c)
	if c.ID == 0 {
		base.BadRequest(ctx, "通道不存在")

		return
	}

	if req.Name != nil {
		c.Name = strings.TrimSpace(*req.Name)
	}
	if req.Remark != nil {
		c.Remark = *req.Remark
	}
	if req.Qrcode != nil {
		c.Qrcode = strings.TrimSpace(*req.Qrcode)
	}
	if req.Config != nil {
		c.Config = *req.Config
	}
	if req.TradeType != nil {
		if !model.IsSupportedTradeType(model.TradeType(*req.TradeType)) {
			base.BadRequest(ctx, fmt.Sprintf("不支持的交易类型: %s", *req.TradeType))

			return
		}

		c.TradeType = *req.TradeType
	}
	if req.Status != nil {
		c.Status = *req.Status
	}
	if req.OtherNotify != nil {
		c.OtherNotify = *req.OtherNotify
	}

	if err := c.Validate(); err != nil {
		base.BadRequest(ctx, err.Error())

		return
	}

	if err := model.Db.Save(&c).Error; err != nil {
		base.Error(ctx, err)

		return
	}

	base.Response(ctx, 200, "修改成功")
}

func (Channel) Del(ctx *gin.Context) {
	var req base.IDRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		base.BadRequest(ctx, err.Error())

		return
	}

	model.Db.Where("id = ?", req.ID).Delete(&model.Channel{})

	base.Response(ctx, 200, "删除成功")
}
