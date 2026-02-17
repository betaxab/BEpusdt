package task

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/shopspring/decimal"
	"github.com/v03413/bepusdt/app/log"
	"github.com/v03413/bepusdt/app/model"
)

type AlipayV3Client struct {
	AppId      string
	PrivateKey string
	PublicKey  string
	Host       string
}

func NewAlipayV3Client(appId, privateKey, publicKey string) *AlipayV3Client {
	return &AlipayV3Client{
		AppId:      appId,
		PrivateKey: formatKey(privateKey, "PRIVATE"),
		PublicKey:  formatKey(publicKey, "PUBLIC"),
		Host:       "openapi.alipay.com",
	}
}

func formatKey(key, keyType string) string {
	key = strings.ReplaceAll(key, "-----BEGIN RSA PRIVATE KEY-----", "")
	key = strings.ReplaceAll(key, "-----END RSA PRIVATE KEY-----", "")
	key = strings.ReplaceAll(key, "-----BEGIN PUBLIC KEY-----", "")
	key = strings.ReplaceAll(key, "-----END PUBLIC KEY-----", "")
	key = strings.ReplaceAll(key, "-----BEGIN RSA PUBLIC KEY-----", "")
	key = strings.ReplaceAll(key, "-----END RSA PUBLIC KEY-----", "")
	key = strings.ReplaceAll(key, "\r", "")
	key = strings.ReplaceAll(key, "\n", "")
	key = strings.ReplaceAll(key, " ", "")

	var header, footer string
	if keyType == "PUBLIC" {
		header = "-----BEGIN PUBLIC KEY-----"
		footer = "-----END PUBLIC KEY-----"
	} else {
		header = "-----BEGIN RSA PRIVATE KEY-----"
		footer = "-----END RSA PRIVATE KEY-----"
	}

	// Add proper line breaks every 64 chars
	var sb strings.Builder
	sb.WriteString(header + "\n")
	for i := 0; i < len(key); i += 64 {
		end := i + 64
		if end > len(key) {
			end = len(key)
		}
		sb.WriteString(key[i:end] + "\n")
	}
	sb.WriteString(footer)
	return sb.String()
}

func (c *AlipayV3Client) generateSign(authString, httpMethod, httpUri, httpBody string) (string, error) {
	content := fmt.Sprintf("%s\n%s\n%s\n%s\n", authString, httpMethod, httpUri, httpBody)

	block, _ := pem.Decode([]byte(c.PrivateKey))
	if block == nil {
		return "", errors.New("failed to parse private key PEM")
	}

	var priv *rsa.PrivateKey
	var err error

	// Try PKCS1 first
	priv, err = x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		// Try PKCS8
		key, err2 := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err2 != nil {
			return "", fmt.Errorf("parse private key error: %v (PKCS1), %v (PKCS8)", err, err2)
		}
		var ok bool
		priv, ok = key.(*rsa.PrivateKey)
		if !ok {
			return "", errors.New("private key is not an RSA key")
		}
	}

	hashed := sha256.Sum256([]byte(content))
	signature, err := rsa.SignPKCS1v15(rand.Reader, priv, crypto.SHA256, hashed[:])
	if err != nil {
		return "", fmt.Errorf("sign error: %w", err)
	}

	return base64.StdEncoding.EncodeToString(signature), nil
}

func (c *AlipayV3Client) verify(respBody, sign, timestamp, nonce string) bool {
	if sign == "" || respBody == "" {
		return false
	}

	content := fmt.Sprintf("%s\n%s\n%s\n", timestamp, nonce, respBody)

	block, _ := pem.Decode([]byte(c.PublicKey))
	if block == nil {
		return false
	}
	pubInterface, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return false
	}
	pub := pubInterface.(*rsa.PublicKey)

	hashed := sha256.Sum256([]byte(content))
	decodedSign, _ := base64.StdEncoding.DecodeString(sign)

	err = rsa.VerifyPKCS1v15(pub, crypto.SHA256, hashed[:], decodedSign)
	return err == nil
}

func (c *AlipayV3Client) Request(path string, queryParams map[string]string) (map[string]interface{}, error) {
	httpMethod := "GET"

	q := url.Values{}
	for k, v := range queryParams {
		q.Set(k, v)
	}
	queryString := q.Encode()
	httpUri := path + "?" + queryString
	requestUrl := "https://" + c.Host + httpUri

	nonce := fmt.Sprintf("%d", time.Now().UnixNano()) // Simple nonce
	timestamp := fmt.Sprintf("%d", time.Now().UnixNano()/1e6)

	authString := fmt.Sprintf("app_id=%s,nonce=%s,timestamp=%s", c.AppId, nonce, timestamp)
	httpBody := "{}" // GET request body is empty JSON for signature consistency

	sign, err := c.generateSign(authString, httpMethod, httpUri, httpBody)
	if err != nil {
		return nil, err
	}

	authHeader := fmt.Sprintf("ALIPAY-SHA256withRSA %s,sign=%s", authString, sign)

	req, err := http.NewRequest(httpMethod, requestUrl, strings.NewReader(httpBody))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", authHeader)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	respBody := string(bodyBytes)

	// Verify logic
	respHeaders := resp.Header
	if !c.verify(respBody, respHeaders.Get("alipay-signature"), respHeaders.Get("alipay-timestamp"), respHeaders.Get("alipay-nonce")) {
		signHeader := respHeaders.Get("alipay-signature")
		if signHeader != "" {
			 return nil, errors.New("Alipay Verify Failed! [sign=" + signHeader + "]")
		}
	}

	var result map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &result); err != nil {
		return nil, err
	}

	return result, nil
}

type alipayMck struct {
	client *AlipayV3Client
}

var ali alipayMck

func alipayMckInit() {
	// ali = newAlipayMck() // We instantiate client per channel config in syncBill loop
	ali = alipayMck{} // Singleton holder
	Register(Task{
		Duration: 30 * time.Second, // Poll every 30s
		Callback: ali.syncBill,
	})
	Register(Task{
		Duration: 5 * time.Second,
		Callback: ali.tradeConfirmHandle,
	})
}


func (a *alipayMck) syncBill(ctx context.Context) {
	// Find pending AlipayMck orders
	var orders []model.Order
	model.Db.Where("trade_type = ? AND status = ?", model.AlipayMck, model.OrderStatusWaiting).Find(&orders)

	if len(orders) == 0 {
		return
	}

	// Group by Config to avoid too many requests
	// Current logic iterates orders and requests for EACH order.
	// Optimization: Group by Channel/Config.

	processedChannels := make(map[string]bool)

	for _, order := range orders {
		// Check for expiration
		if time.Now().After(order.ExpiredAt) {
			log.Info(fmt.Sprintf("AlipayMck: Order %s expired", order.OrderId))
			order.SetExpired()
			continue
		}

		var channel model.Channel
		result := model.Db.Where("match_qr = ?", order.Address).First(&channel)
		if result.Error != nil {
			log.Error(fmt.Sprintf("AlipayMck task: Channel not found for order %s address %s", order.OrderId, order.Address))
			continue
		}

		// Avoid duplicate scanning for the same channel in one tick if possible,
		// BUT alipay query is time-based. Sharing the response for multiple orders of same channel is better.
		// For now, let's keep it simple or strictly follow previous logic but structured.

		if processedChannels[channel.Name] {
			continue
		}

		var config model.AlipayMckConfig
		if err := json.Unmarshal([]byte(channel.Config), &config); err != nil {
			log.Error(fmt.Sprintf("AlipayMck task: Invalid config for channel %s", channel.Name))
			continue
		}

		client := NewAlipayV3Client(config.AppId, config.PrivateKey, config.PublicKey)

		// Create validation task
		// Time range: now-5m to now
		now := time.Now()
		startTime := now.Add(-5 * time.Minute)

		queryParams := map[string]string{
			"start_time": startTime.Format("2006-01-02 15:04:05"),
			"end_time":   now.Format("2006-01-02 15:04:05"),
			"page_no":    "1",
			"page_size":  "2000",
		}

		res, err := client.Request("/v3/alipay/data/bill/sell/query", queryParams)
		if err != nil {
			log.Error(fmt.Sprintf("AlipayMck request error for order %s: %v", order.OrderId, err))
			continue
		}

		processedChannels[channel.Name] = true

		// Parse result and push to queue
		if detailListInterface, ok := res["detail_list"]; ok {
			detailList, ok := detailListInterface.([]interface{})
			if !ok {
				 continue
			}

			var transfers []transfer
			for _, itemInterface := range detailList {
				 item, ok := itemInterface.(map[string]interface{})
				 if !ok { continue }

				 t := a.parseTransfer(item)
				 if t.TxHash != "" {
					 transfers = append(transfers, t)
				 }
			}

			if len(transfers) > 0 {
				transferQueue.In <- transfers
			}
		}
	}
}

func (a *alipayMck) parseTransfer(item map[string]interface{}) transfer {
	tradeStatus, _ := item["trade_status"].(string)
	if tradeStatus != "成功" {
		return transfer{}
	}

	totalAmountStr, _ := item["total_amount"].(string)
	totalAmount, _ := decimal.NewFromString(totalAmountStr)
	alipayOrderNo, _ := item["alipay_order_no"].(string)
	merchantOrderNo, _ := item["merchant_order_no"].(string)
	otherAccount, _ := item["other_account"].(string)

	// Parse time
	tradeTimeStr, _ := item["gmt_payment"].(string) // Try payment time first
	if tradeTimeStr == "" {
		tradeTimeStr, _ = item["gmt_create"].(string) // Fallback to create time
	}

	var tradeTime time.Time
	if tradeTimeStr != "" {
		var err error
		tradeTime, err = time.ParseInLocation("2006-01-02 15:04:05", tradeTimeStr, time.Local)
		if err != nil {
			 log.Error(fmt.Sprintf("AlipayMck: Failed to parse time %s: %v", tradeTimeStr, err))
			 tradeTime = time.Now()
		}
	} else {
		tradeTime = time.Now()
	}

	log.Task.Info(fmt.Sprintf("AlipayMck Scan: order_no=%s account=%s amount=%s status=%s time=%s", alipayOrderNo, otherAccount, totalAmountStr, tradeStatus, tradeTimeStr))

	return transfer{
		Network:     "AlipayMck", // Internal use
		TxHash:      alipayOrderNo,
		Amount:      totalAmount,
		FromAddress: otherAccount,
		RecvAddress: merchantOrderNo, // Storing merchant Order No specifically here to be saved as ref_orderno
		Timestamp:   tradeTime,
		TradeType:   model.AlipayMck,
		BlockNum:    0, // irrelevant
	}
}

// tradeConfirmHandle handles the final confirmation of AlipayMck orders
func (a *alipayMck) tradeConfirmHandle(ctx context.Context) {
	orders := getConfirmingOrders([]model.TradeType{model.AlipayMck})
	for _, o := range orders {
		markFinalConfirmed(o)
		log.Info(fmt.Sprintf("AlipayMck: Order %s finalized", o.OrderId))
	}
}
