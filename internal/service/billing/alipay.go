package billing

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// AlipayProvider implements PaymentProvider for Alipay face-to-face (当面付).
type AlipayProvider struct {
	AppID           string
	PrivateKey      string // PEM format
	AlipayPublicKey string // PEM format
	NotifyURL       string
}

type alipayBizContent struct {
	OutTradeNo string `json:"out_trade_no"`
	TotalAmount string `json:"total_amount"`
	Subject     string `json:"subject"`
}

type alipayPrecreateResponse struct {
	Code    string `json:"code"`
	Msg     string `json:"msg"`
	SubCode string `json:"sub_code"`
	QRCode  string `json:"qr_code"`
}

type alipayResponse struct {
	AlipayTradePrecreateResponse alipayPrecreateResponse `json:"alipay_trade_precreate_response"`
	Sign                         string                  `json:"sign"`
}

func (p *AlipayProvider) CreatePayment(req *CreatePaymentRequest) (*CreatePaymentResponse, error) {
	bizContent, _ := json.Marshal(alipayBizContent{
		OutTradeNo:  req.OrderNo,
		TotalAmount: fmt.Sprintf("%.2f", req.Amount),
		Subject:     req.Subject,
	})

	params := map[string]string{
		"app_id":      p.AppID,
		"method":      "alipay.trade.precreate",
		"format":      "JSON",
		"charset":     "utf-8",
		"sign_type":   "RSA2",
		"timestamp":   time.Now().Format("2006-01-02 15:04:05"),
		"version":     "1.0",
		"notify_url":  p.NotifyURL,
		"biz_content": string(bizContent),
	}

	sign, err := p.signRSA2(params)
	if err != nil {
		return nil, fmt.Errorf("sign failed: %w", err)
	}
	params["sign"] = sign

	// Call Alipay gateway
	apiURL := "https://openapi.alipay.com/gateway.do"
	formData := url.Values{}
	for k, v := range params {
		formData.Set(k, v)
	}

	resp, err := http.PostForm(apiURL, formData)
	if err != nil {
		return nil, fmt.Errorf("alipay request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var result alipayResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse alipay response failed: %w", err)
	}

	if result.AlipayTradePrecreateResponse.Code != "10000" {
		return nil, fmt.Errorf("alipay error: %s - %s",
			result.AlipayTradePrecreateResponse.SubCode,
			result.AlipayTradePrecreateResponse.Msg)
	}

	return &CreatePaymentResponse{
		QRCode: result.AlipayTradePrecreateResponse.QRCode,
	}, nil
}

func (p *AlipayProvider) VerifyCallback(params map[string]string) (*CallbackResult, error) {
	sign := params["sign"]
	signType := params["sign_type"]
	if signType != "RSA2" {
		return &CallbackResult{}, nil
	}

	// Verify RSA2 signature with Alipay public key
	ok, err := p.verifyRSA2(params, sign)
	if err != nil || !ok {
		return &CallbackResult{}, nil
	}

	// Check trade status
	tradeStatus := params["trade_status"]
	if tradeStatus != "TRADE_SUCCESS" && tradeStatus != "TRADE_FINISHED" {
		return &CallbackResult{Verified: true}, nil
	}

	orderNo := params["out_trade_no"]
	tradeNo := params["trade_no"]
	var amount float64
	fmt.Sscanf(params["total_amount"], "%f", &amount)

	return &CallbackResult{
		TradeNo:  tradeNo,
		OrderNo:  orderNo,
		Amount:   amount,
		Verified: true,
	}, nil
}

// signRSA2 signs params with RSA2 (SHA256WithRSA).
func (p *AlipayProvider) signRSA2(params map[string]string) (string, error) {
	block, _ := pem.Decode([]byte(p.PrivateKey))
	if block == nil {
		return "", fmt.Errorf("failed to decode private key PEM")
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		key, err = x509.ParsePKCS1PrivateKey(block.Bytes)
		if err != nil {
			return "", fmt.Errorf("failed to parse private key: %w", err)
		}
	}

	raw := p.buildSignString(params)
	hash := sha256.Sum256([]byte(raw))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key.(*rsa.PrivateKey), crypto.SHA256, hash[:])
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(sig), nil
}

// verifyRSA2 verifies an RSA2 signature with the Alipay public key.
func (p *AlipayProvider) verifyRSA2(params map[string]string, sign string) (bool, error) {
	block, _ := pem.Decode([]byte(p.AlipayPublicKey))
	if block == nil {
		return false, fmt.Errorf("failed to decode alipay public key PEM")
	}
	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return false, fmt.Errorf("failed to parse alipay public key: %w", err)
	}

	sig, err := base64.StdEncoding.DecodeString(sign)
	if err != nil {
		return false, err
	}

	raw := p.buildSignString(params)
	hash := sha256.Sum256([]byte(raw))
	err = rsa.VerifyPKCS1v15(pub.(*rsa.PublicKey), crypto.SHA256, hash[:], sig)
	return err == nil, nil
}

// buildSignString builds the sorted sign string from params (excluding sign and sign_type).
func (p *AlipayProvider) buildSignString(params map[string]string) string {
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
			parts = append(parts, k+"="+params[k])
		}
	}
	return strings.Join(parts, "&")
}
