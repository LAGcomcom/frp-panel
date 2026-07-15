package billing

import (
	"encoding/json"
	"fmt"

	"github.com/frp-panel/frp-panel/internal/model"
)

// NewProvider creates a PaymentProvider from a PaymentConfig.
func NewProvider(config *model.PaymentConfig) (PaymentProvider, error) {
	var cfg map[string]string
	if err := json.Unmarshal([]byte(config.Config), &cfg); err != nil {
		return nil, fmt.Errorf("invalid payment config JSON: %w", err)
	}

	switch config.Type {
	case "epay":
		return &EPayProvider{
			APIURL:    cfg["api_url"],
			PID:       cfg["pid"],
			Key:       cfg["key"],
			NotifyURL: cfg["notify_url"],
		}, nil
	case "alipay":
		return &AlipayProvider{
			AppID:           cfg["app_id"],
			PrivateKey:      cfg["private_key"],
			AlipayPublicKey: cfg["alipay_public_key"],
			NotifyURL:       cfg["notify_url"],
		}, nil
	case "usdt":
		return &USDTProvider{
			APIURL:        cfg["api_url"],
			WalletAddress: cfg["wallet_address"],
			APIKey:        cfg["api_key"],
			NotifyURL:     cfg["notify_url"],
		}, nil
	default:
		return nil, fmt.Errorf("unsupported payment type: %s", config.Type)
	}
}

// SafePaymentMethod is the public-safe representation of a payment config (no credentials).
type SafePaymentMethod struct {
	ID        uint   `json:"id"`
	Name      string `json:"name"`
	Type      string `json:"type"`
	SortOrder int    `json:"sort_order"`
}
