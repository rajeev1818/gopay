package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rajeev1818/gopay/internal/domain"
)

type TransactionRepository struct {
	db *pgxpool.Pool
}

func NewTransactionRepository(db *pgxpool.Pool) *TransactionRepository {
	return &TransactionRepository{db: db}
}

func (r *TransactionRepository) Create(ctx context.Context, tx pgx.Tx, txn domain.Transaction) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO transactions
			(id, idempotency_key, type, status, source_wallet_id, dest_wallet_id, amount, currency, description, partner_reference, failure_reason, created_at, updated_at)
		VALUES
			($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`,
		txn.ID,
		txn.IdempotencyKey,
		txn.Type,
		txn.Status,
		txn.SourceWalletID,
		txn.DestWalletID,
		txn.Amount,
		txn.Currency,
		txn.Description,
		txn.PartnerReference,
		txn.FailureReason,
		txn.CreatedAt,
		txn.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("inserting transaction: %w", err)
	}
	return nil
}
