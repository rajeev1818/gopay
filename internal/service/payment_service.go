package service

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rajeev1818/gopay/internal/domain"
	"github.com/rajeev1818/gopay/internal/repository"
)

type PaymentService struct {
	db      *pgxpool.Pool
	wallets *repository.WalletRepository
	txns    *repository.TransactionRepository
	ledger  *repository.LedgerRepository
}

type TransferRequest struct {
	FromWalletID   string
	ToWalletID     string
	Amount         int64
	Currency       domain.Currency
	Description    string
	IdempotencyKey string
}

func (s *PaymentService) Transfer(ctx context.Context, req TransferRequest) (domain.Transaction, error) {

	if req.Amount <= 0 {
		return domain.Transaction{}, fmt.Errorf("amount must be positive")
	}
	if req.FromWalletID == req.ToWalletID {
		return domain.Transaction{}, fmt.Errorf("cannot transfer to same wallet")
	}

	var txn domain.Transaction

	err := pgx.BeginTxFunc(ctx, s.db, pgx.TxOptions{
		IsoLevel: pgx.Serializable,
	}, func(tx pgx.Tx) error {
		firstId, secondId := req.FromWalletID, req.ToWalletID

		if firstId > secondId {
			firstId, secondId = secondId, firstId
		}

		first, err := s.wallets.GetByIDForUpdate(ctx, tx, firstId)
		if err != nil {
			return fmt.Errorf("locking first wallet: %w", err)
		}

		second, err := s.wallets.GetByIDForUpdate(ctx, tx, secondId)

		if err != nil {
			return fmt.Errorf("locking second wallet: %w", err)
		}

		var source, dest domain.Wallet
		if first.ID == req.FromWalletID {
			source, dest = first, second
		} else {
			source, dest = second, first
		}

		if source.Frozen || dest.Frozen {
			return fmt.Errorf("wallet is frozen")
		}
		if source.Currency != req.Currency || dest.Currency != req.Currency {
			return fmt.Errorf("currency mismatch")
		}
		if source.Balance < req.Amount {
			return fmt.Errorf("insufficient balance: have %d, need %d", source.Balance, req.Amount)
		}

		txn = domain.Transaction{
			ID:             uuid.New().String(),
			IdempotencyKey: req.IdempotencyKey,
			Type:           domain.TxnTransfer,
			Status:         domain.TxnCompleted,
			SourceWalletID: &req.FromWalletID,
			DestWalletID:   req.ToWalletID,
			Amount:         req.Amount,
			Currency:       req.Currency,
			Description:    req.Description,
			CreatedAt:      time.Now(),
		}

		if err := s.txns.Create(ctx, tx, txn); err != nil {
			return fmt.Errorf("creating transaction: %w", err)
		}
		newSourceBalance := source.Balance - req.Amount
		newDestBalance := dest.Balance + req.Amount

		if err := s.wallets.UpdateBalance(ctx, tx, source.ID, newSourceBalance); err != nil {
			return fmt.Errorf("debiting source: %w", err)
		}

		if err := s.wallets.UpdateBalance(ctx, tx, dest.ID, newDestBalance); err != nil {
			return fmt.Errorf("crediting dest: %w", err)
		}

		debitEntry := domain.LedgerEntry{
			ID:            uuid.New().String(),
			TransactionID: txn.ID,
			WalletID:      source.ID,
			EntryType:     domain.Debit,
			Amount:        req.Amount,
			BalanceAfter:  newSourceBalance,
		}

		creditEntry := domain.LedgerEntry{
			ID:            uuid.New().String(),
			TransactionID: txn.ID,
			WalletID:      dest.ID,
			EntryType:     domain.Credit,
			Amount:        req.Amount,
			BalanceAfter:  newDestBalance,
		}

		if err := s.ledger.Create(ctx, tx, debitEntry); err != nil {
			return fmt.Errorf("creating debit entry: %w", err)
		}
		if err := s.ledger.Create(ctx, tx, creditEntry); err != nil {
			return fmt.Errorf("creating credit entry: %w", err)
		}

		return nil
	})

	if err != nil {
		return domain.Transaction{}, err
	}

	return txn, nil
}
