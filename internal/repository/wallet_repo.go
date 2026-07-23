package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rajeev1818/gopay/internal/domain"
)

type WalletRepository struct {
	db *pgxpool.Pool
}

func NewWalletRepository(db *pgxpool.Pool) *WalletRepository {
	return &WalletRepository{db: db}
}

func (r *WalletRepository) GetById(ctx context.Context, id string) (domain.Wallet, error) {
	var w domain.Wallet

	err := r.db.QueryRow(ctx, `SELECT id, user_id, currency, balance, frozen, created_at, updated_at
         FROM wallets WHERE id = $1`, id).Scan(&w.ID, &w.UserID, &w.Currency, &w.Balance, &w.Frozen, &w.CreatedAt, &w.UpdatedAt)

	if err == pgx.ErrNoRows {
		return domain.Wallet{}, fmt.Errorf("wallet not found: %s", id)
	}
	if err != nil {
		return domain.Wallet{}, fmt.Errorf("querying wallet: %w", err)
	}
	return w, nil
}

func (r *WalletRepository) GetByIDForUpdate(ctx context.Context, tx pgx.Tx, id string) (domain.Wallet, error) {
	var w domain.Wallet

	err := tx.QueryRow(ctx, `SELECT id, user_id, currency, balance, frozen, created_at, updated_at
         FROM wallets WHERE id = $1 FOR UPDATE`, id).Scan(&w.ID, &w.UserID, &w.Currency, &w.Balance, &w.Frozen, &w.CreatedAt, &w.UpdatedAt)

	if err != nil {
		return domain.Wallet{}, fmt.Errorf("locking wallet: %w", err)
	}
	return w, nil
}

func (r *WalletRepository) UpdateBalance(ctx context.Context, tx pgx.Tx, id string, newBalance int64) error {
	_, err := tx.Exec(ctx, `UPDATE wallets SET balance = $1, updated_at = NOW() WHERE id = $2`, newBalance, id)

	return err
}
