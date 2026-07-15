package billing

// CreatePaymentRequest is the input for creating a payment at the gateway.
type CreatePaymentRequest struct {
	OrderNo string
	Amount  float64
	Subject string // order description, e.g. "Pro Plan - Monthly"
}

// CreatePaymentResponse holds the result of creating a payment.
type CreatePaymentResponse struct {
	PayURL string // URL to redirect user to (epay, alipay redirect)
	QRCode string // QR code URL or content (alipay precreate, USDT)
}

// CallbackResult holds the verified callback data from a payment gateway.
type CallbackResult struct {
	TradeNo  string  // gateway's transaction ID
	OrderNo  string  // our order number
	Amount   float64 // actual paid amount
	Verified bool    // signature verification passed
}

// PaymentProvider is the interface that all payment gateway implementations satisfy.
type PaymentProvider interface {
	CreatePayment(req *CreatePaymentRequest) (*CreatePaymentResponse, error)
	VerifyCallback(params map[string]string) (*CallbackResult, error)
}
