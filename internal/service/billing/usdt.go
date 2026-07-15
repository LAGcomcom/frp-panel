package billing

import (
	"crypto/md5"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
)

// USDTProvider implements PaymentProvider for USDT/TRC20 via a third-party gateway.
// Many USDT gateways use an ePay-compatible API, so this shares the signing logic.
type USDTProvider struct {
	APIURL    string // gateway API URL
	WalletAddress string // TRC20 wallet address
	APIKey    string
	NotifyURL string
}

type usdtCreateResponse struct {
	Code    int    `json:"code"`
	Msg     string `json:"msg"`
	OrderNo string `json:"trade_id"`
	PayURL  string `json:"pay_url"`
	QRCode  string `json:"qrcode"`
}

func (p *USDTProvider) CreatePayment(req *CreatePaymentRequest) (*CreatePaymentResponse, error) {
	// Many USDT gateways support ePay-compatible API
	// Try the standard ePay submit.php style first
	params := map[string]string{
		"pid":          p.APIKey,
		"type":         "usdt",
		"out_trade_no": req.OrderNo,
		"notify_url":   p.NotifyURL,
		"return_url":   p.NotifyURL,
		"name":         req.Subject,
		"money":        fmt.Sprintf("%.2f", req.Amount),
	}
	params["sign"] = p.signMD5(params)
	params["sign_type"] = "MD5"

	// If gateway supports submit.php style
	submitURL := fmt.Sprintf("%s/submit.php?%s", strings.TrimRight(p.APIURL, "/"), buildQuery(params))

	return &CreatePaymentResponse{
		PayURL: submitURL,
	}, nil
}

func (p *USDTProvider) VerifyCallback(params map[string]string) (*CallbackResult, error) {
	sign := params["sign"]
	signType := params["sign_type"]
	if signType != "MD5" {
		return &CallbackResult{}, nil
	}

	expected := p.signMD5(params)
	if sign != expected {
		return &CallbackResult{}, nil
	}

	tradeStatus := params["trade_status"]
	if tradeStatus != "TRADE_SUCCESS" {
		return &CallbackResult{Verified: true}, nil
	}

	orderNo := params["out_trade_no"]
	tradeNo := params["trade_no"]
	var amount float64
	fmt.Sscanf(params["money"], "%f", &amount)

	return &CallbackResult{
		TradeNo:  tradeNo,
		OrderNo:  orderNo,
		Amount:   amount,
		Verified: true,
	}, nil
}

func (p *USDTProvider) signMD5(params map[string]string) string {
	keys := make([]string, 0, len(params))
	for k := range params {
		if k == "sign" || k == "sign_type" {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		if params[k] != "" {
			parts = append(parts, fmt.Sprintf("%s=%s", k, params[k]))
		}
	}
	raw := strings.Join(parts, "&") + p.APIKey

	hash := md5.Sum([]byte(raw))
	return fmt.Sprintf("%x", hash)
}

// QueryOrderViaAPI queries order status via the gateway API (optional fallback).
func (p *USDTProvider) QueryOrderViaAPI(orderNo string) (bool, string, error) {
	params := map[string]string{
		"act":    "order",
		"pid":    p.APIKey,
		"out_trade_no": orderNo,
	}
	params["sign"] = p.signMD5(params)
	params["sign_type"] = "MD5"

	u := fmt.Sprintf("%s/api.php?%s", strings.TrimRight(p.APIURL, "/"), buildQuery(params))
	resp, err := http.Get(u)
	if err != nil {
		return false, "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var result struct {
		TradeStatus string `json:"trade_status"`
		TradeNo     string `json:"trade_no"`
	}
	json.Unmarshal(body, &result)

	return result.TradeStatus == "TRADE_SUCCESS", result.TradeNo, nil
}
