package integration

import "context"

type ChargeRequest struct {
	Amount    int64
	Currency  string
	Reference string
	Metadata  map[string]string
}

type ChargeResponse struct {
	PartnerTxnID string
	Status       string
	Message      string
}

type PaymentGateway interface {
	Charge(ctx context.Context, req ChargeRequest) (ChargeResponse, error)
	Refund(ctx context.Context, txnId string, amount int64) error
	QueryStatus(ctx context.Context, txnID string) (string, error)
}
