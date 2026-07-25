package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rajeev1818/gopay/internal/domain"
)

type LedgerRepository struct {
	db *pgxpool.Pool
}

func NewLedgerRepository(db *pgxpool.Pool) *LedgerRepository {
	return &LedgerRepository{
		db: db,
	}
}

func (r *LedgerRepository) Create(ctx context.Context, tx pgx.Tx, entry domain.LedgerEntry) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO ledger_entries
			(id, transaction_id, wallet_id, entry_type, amount, balance_after, created_at)
		VALUES
			($1, $2, $3, $4, $5, $6, $7)`,
		entry.ID,
		entry.TransactionID,
		entry.WalletID,
		entry.EntryType,
		entry.Amount,
		entry.BalanceAfter,
		entry.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("inserting ledger entry: %w", err)
	}
	return nil
}
