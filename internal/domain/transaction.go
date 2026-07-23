package domain

import "time"

type TxnType string
type TxnStatus string

const (
	TxnTopUp    TxnType = "topup"
	TxnTransfer TxnType = "transfer"
	TxnPayment  TxnType = "payment"
	TxnRefund   TxnType = "refund"
)

const (
	TxnPending   TxnStatus = "pending"
	TxnCompleted TxnStatus = "completed"
	TxnFailed    TxnStatus = "failed"
	TxnReversed  TxnStatus = "reversed"
)

type Transaction struct {
	ID               string    `json:"id"`
	IdempotencyKey   string    `json:"idempotency_key"`
	Type             TxnType   `json:"type"`
	Status           TxnStatus `json:"status"`
	SourceWalletID   *string   `json:"source_wallet_id"` // nil for top-ups
	DestWalletID     string    `json:"dest_wallet_id"`
	Amount           int64     `json:"amount"`
	Currency         Currency  `json:"currency"`
	Description      string    `json:"description"`
	PartnerReference *string   `json:"partner_reference"` // external txn ID
	FailureReason    *string   `json:"failure_reason"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}
