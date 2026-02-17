package model

import (
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/shopspring/decimal"
	"github.com/spf13/cast"
	"github.com/tidwall/gjson"
	"github.com/v03413/bepusdt/app/log"
	"github.com/v03413/bepusdt/app/utils"
	"gorm.io/gorm"
)

type Rate struct {
	Id
	Rate    string  `gorm:"column:rate;type:varchar(32);not null;comment:订单汇率" json:"rate"`
	Fiat    string  `gorm:"column:fiat;type:varchar(16);not null;comment:法币" json:"fiat"`
	Crypto  string  `gorm:"column:crypto;type:varchar(16);not null;comment:加密货币" json:"crypto"`
	RawRate float64 `gorm:"column:raw_rate;type:decimal(10,4);not null;comment:基准汇率" json:"raw_rate"`
	Syntax  string  `gorm:"column:syntax;type:varchar(32);not null;default:'';comment:浮动语法" json:"syntax"`
	AutoTimeAt
}

func (r *Rate) TableName() string {
	return "bep_rate"
}

func (r *Rate) BeforeCreate(*gorm.DB) error {
	var syntax = GetK(ConfKey(fmt.Sprintf("rate_float_%s_%s", r.Crypto, r.Fiat)))
	if syntax == "" {

		return nil
	}

	r.Syntax = syntax
	r.Rate = cast.ToString(ParseFloatRate(syntax, cast.ToFloat64(r.RawRate)))

	return nil
}

func CoingeckoRate() error {
	var fiats = make([]string, 0)
	for k := range supportFiat {
		fiats = append(fiats, string(k))
	}

	var ids = make([]string, 0)
	var tokens = make(map[CoinId]Crypto)
	for token, id := range supportCrypto {
		if token == CNYE {
			continue
		}
		ids = append(ids, string(id))
		tokens[id] = token
	}

	var url = fmt.Sprintf("%s/api/v3/simple/price?ids=%s&vs_currencies=%s", GetC(RateSyncCoingeckoApiUrl), strings.Join(ids, ","), strings.Join(fiats, ","))
	var client = &http.Client{Timeout: 15 * time.Second}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}

	req.Header.Set("x-cg-demo-api-key", GetC(RateSyncCoingeckoApiKey))

	resp, err := client.Do(req)
	if err != nil {

		return err
	}

	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {

		return err
	}

	if resp.StatusCode != http.StatusOK {

		return errors.New("CoingeckoRate: " + http.StatusText(resp.StatusCode))
	}

	var data = gjson.ParseBytes(body)
	if data.Get("status.error_code").Exists() {

		return errors.New("CoingeckoRate: " + data.Get("status.error_message").String())
	}

	var rows = make([]Rate, 0)
	for id, v := range data.Map() {
		var token, ok = tokens[CoinId(id)]
		if !ok {

			continue
		}

		for fiat, val := range v.Map() {
			rows = append(rows, Rate{
				Rate:    val.String(),
				Fiat:    strings.ToUpper(fiat),
				Crypto:  string(token),
				RawRate: val.Float(),
			})
		}
	}

	// Calculate CNYE rates
	var usdcRawRates = make(map[string]float64)
	for _, row := range rows {
		if row.Crypto == string(USDC) {
			usdcRawRates[row.Fiat] = row.RawRate
		}
	}

	for _, fiat := range fiats {
		upperFiat := strings.ToUpper(fiat)
		var rawRate float64

		if upperFiat == string(CNY) {
			rawRate = 1.0
		} else {
			usdcToCnyRaw, ok1 := usdcRawRates[string(CNY)]
			usdcToFiatRaw, ok2 := usdcRawRates[upperFiat]
			
			if !ok1 || !ok2 || usdcToCnyRaw == 0 {
				continue
			}

			// CNYE=(CNY到USDC的价格)/(订单法币到USDC的价格)
			// Apply syntax to USDC->CNY rate if exists
			syntax := GetK(ConfKey(fmt.Sprintf("rate_float_%s_%s", USDC, CNY)))
			usdcToCny := ParseFloatRate(syntax, usdcToCnyRaw)

			if usdcToCny == 0 {
				continue
			}
			
			// Use RAW rate for OrderFiat->USDC as syntax was not requested for this part
			rawRate = round(usdcToFiatRaw/usdcToCny, 6)
		}

		rows = append(rows, Rate{
			Rate:    cast.ToString(rawRate),
			Fiat:    upperFiat,
			Crypto:  string(CNYE),
			RawRate: rawRate,
		})
	}

	if len(rows) == 0 {

		return errors.New("CoingeckoRate: no data")
	}

	Db.Create(&rows)

	return nil
}

func ParseFloatRate(syntax string, rawVal float64) float64 {
	if syntax == "" {

		return rawVal
	}

	if utils.IsNumber(syntax) {

		return cast.ToFloat64(syntax)
	}

	match, err := regexp.MatchString(`^[~+-]\d+(\.\d+)?$`, syntax)
	if !match || err != nil {
		log.Error("浮动语法解析错误", err)

		return 0
	}

	var act = syntax[0:1]
	var raw = decimal.NewFromFloat(rawVal)
	var base = decimal.NewFromFloat(cast.ToFloat64(syntax[1:]))
	var result float64 = 0

	switch act {
	case "~":
		result = raw.Mul(base).InexactFloat64()
	case "+":
		result = raw.Add(base).InexactFloat64()
	case "-":
		result = raw.Sub(base).InexactFloat64()
	}

	return round(result, 4) // 保留4位小数
}

func round(val float64, precision int) float64 {
	// Round 四舍五入，ROUND_HALF_UP 模式实现
	// 返回将 val 根据指定精度 precision（十进制小数点后数字的数目）进行四舍五入的结果。precision 也可以是负数或零。

	if precision == 0 {
		return math.Round(val)
	}

	p := math.Pow10(precision)
	if precision < 0 {
		return math.Floor(val*p+0.5) * math.Pow10(-precision)
	}

	return math.Floor(val*p+0.5) / p
}

func GetOrderRate(token Crypto, fiat Fiat, syntax string) (decimal.Decimal, error) {
	var r Rate
	Db.Where("crypto = ? and fiat = ?", token, fiat).Order("created_at desc").Limit(1).Find(&r)
	if r.ID == 0 {

		return decimal.Decimal{}, fmt.Errorf("创建失败，请检查汇率同步是否正常：%s %s", token, fiat)
	}

	if syntax == "" {

		return decimal.NewFromString(r.Rate)
	}

	return decimal.NewFromFloat(ParseFloatRate(syntax, r.RawRate)), nil
}
