package model

import (
	"encoding/json"
	"fmt"

	"strings"

	"github.com/v03413/bepusdt/app/utils"
	"gorm.io/gorm"
)

const (
	ClStatusEnable  uint8 = 1
	ClStatusDisable uint8 = 0
	ClOtherEnable   uint8 = 1
	ClOtherDisable  uint8 = 0
)

type Channel struct {
	Id
	Name        string `gorm:"column:name;type:varchar(32);not null;default:-';comment:名称" json:"name"`
	Status      uint8  `gorm:"column:status;type:tinyint(1);not null;default:1;comment:地址状态" json:"status"`
	Qrcode      string `gorm:"column:qrcode;type:varchar(128);not null;index;comment:二维码链接" json:"qrcode"`
	MatchQr     string `gorm:"column:match_qr;type:varchar(128);not null;uniqueIndex:idx_qrcode;comment:匹配地址" json:"match_qr"`
	Config      string `gorm:"column:config;type:text;not null;comment:通道配置" json:"config"`
	TradeType   string `gorm:"column:trade_type;type:varchar(20);not null;uniqueIndex:idx_qrcode;comment:交易类型" json:"trade_type"`
	OtherNotify uint8  `gorm:"column:other_notify;type:tinyint(1);not null;default:0;comment:其它通知" json:"other_notify"`
	Remark      string `gorm:"column:remark;type:varchar(255);not null;default:'';comment:备注" json:"remark"`
	AutoTimeAt
}

func (cl *Channel) TableName() string {

	return "bep_channel"
}

func (cl *Channel) SetStatus(status uint8) {
	cl.Status = status
	Db.Save(cl)
}

func (cl *Channel) Validate() error {
	tradeType := TradeType(cl.TradeType)

	// 二维码格式验证
	if tradeType == AlipayMck {
		// 二维码格式：https://qr.alipay.com/tsx10738hvgx4upcpnel5da
		if !utils.IsValidAlipayQR(cl.Qrcode) {
			return fmt.Errorf("支付宝二维码格式错误")
		}

		// 验证配置
		var config AlipayMckConfig
		if err := json.Unmarshal([]byte(cl.Config), &config); err != nil {
			return fmt.Errorf("配置格式错误: %v", err)
		}

		// Alipay AppId
		if !utils.IsValidAlipayAppId(config.AppId) {
			return fmt.Errorf("支付宝 AppId 格式错误")
		}
		// Alipay 公钥
		if !utils.IsValidAlipayPublicKey(config.PublicKey) {
			return fmt.Errorf("支付宝公钥格式错误")
		}
		// Alipay 私钥
		if !utils.IsValidAlipayPrivateKey(config.PrivateKey) {
			return fmt.Errorf("支付宝私钥格式错误")
		}
	}

	// 默认不验证
	return nil
}

func (cl *Channel) BeforeSave(tx *gorm.DB) (err error) {
	cl.MatchQr = cl.Qrcode

	// 非大小写敏感的地址，统一转为小写存储
	if !AddrCaseSens(TradeType(cl.TradeType)) {
		cl.MatchQr = strings.ToLower(cl.MatchQr)
	}
	return
}

type AlipayMckConfig struct {
	AppId      string `json:"appid"`
	PublicKey  string `json:"publickey"`
	PrivateKey string `json:"privatekey"`
}

func (cl *Channel) SetOtherNotify(notify uint8) {
	cl.OtherNotify = notify

	Db.Save(cl)
}

func (cl *Channel) Delete() {
	Db.Delete(cl)
}

func (cl *Channel) GetTokenDecimals() int32 {
	if c, ok := registry[TradeType(cl.TradeType)]; ok {

		return c.Decimal
	}

	return -18
}

func GetAvailableChannel(t TradeType) []string {
	var rows []Channel
	Db.Where("trade_type = ? and status = ?", t, ClStatusEnable).Find(&rows)

	Channels := make([]string, 0, len(rows))
	for _, w := range rows {
		Channels = append(Channels, w.MatchQr)
	}

	return Channels
}
