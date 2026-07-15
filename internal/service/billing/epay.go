package billing

import (
	"crypto/md5"
	"fmt"
	"net/url"
	"sort"
	"strings"
)

// EPayProvider implements PaymentProvider for ePay (易支付) gateways.
type EPayProvider struct {
	APIURL    string // e.g. https://pay.example.com
	PID       string
	Key       string
	NotifyURL string
}

func (p *EPayProvider) CreatePayment(req *CreatePaymentRequest) (*CreatePaymentResponse, error) {
	params := map[string]string{
		"pid":          p.PID,
		"type":         "alipay",
		"out_trade_no": req.OrderNo,
		"notify_url":   p.NotifyURL,
		"return_url":   p.NotifyURL, // user redirect back
		"name":         req.Subject,
		"money":        fmt.Sprintf("%.2f", req.Amount),
	}
	params["sign"] = p.signMD5(params)
	params["sign_type"] = "MD5"

	u := fmt.Sprintf("%s/submit.php?%s", strings.TrimRight(p.APIURL, "/"), buildQuery(params))
	return &CreatePaymentResponse{PayURL: u}, nil
}

func (p *EPayProvider) VerifyCallback(params map[string]string) (*CallbackResult, error) {
	// Extract and remove sign/sign_type before verification
	sign := params["sign"]
	signType := params["sign_type"]
	if signType != "MD5" {
		return &CallbackResult{}, nil
	}

	// Recalculate signature
	expected := p.signMD5(params)
	if sign != expected {
		return &CallbackResult{}, nil
	}

	// Check trade status
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

// signMD5 generates an MD5 signature for ePay.
// Sorted key=value pairs (excluding sign, sign_type) concatenated with & + key at the end.
func (p *EPayProvider) signMD5(params map[string]string) string {
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
	raw := strings.Join(parts, "&") + p.Key

	hash := md5.Sum([]byte(raw))
	return fmt.Sprintf("%x", hash)
}

func buildQuery(params map[string]string) string {
	v := url.Values{}
	for k, val := range params {
		v.Set(k, val)
	}
	return v.Encode()
}
