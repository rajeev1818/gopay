package service

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rajeev1818/gopay/internal/domain"
	"github.com/rajeev1818/gopay/internal/integration"
	"github.com/rajeev1818/gopay/internal/repository"
)

type PaymentService struct {
	db             *pgxpool.Pool
	wallets        *repository.WalletRepository
	txns           *repository.TransactionRepository
	ledger         *repository.LedgerRepository
	circuitBreaker *integration.CircuitBreaker
	bankClient     *integration.MockBankClient
}

type TransferRequest struct {
	FromWalletID   string
	ToWalletID     string
	Amount         int64
	Currency       domain.Currency
	Description    string
	IdempotencyKey string
}

type TopUpRequest struct {
	WalletID       string
	Amount         int64
	Currency       domain.Currency
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

func (s *PaymentService) TopUp(ctx context.Context, req TopUpRequest) (domain.Transaction, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	var chargeResp integration.ChargeResponse

	err := s.circuitBreaker.Execute(ctx, func() error {
		var err error
		chargeResp, err = s.bankClient.Charge(ctx, integration.ChargeRequest{
			Amount:    req.Amount,
			Currency:  string(req.Currency),
			Reference: req.IdempotencyKey,
		})
		return err
	})

	if err != nil {
		failReason := err.Error()
		s.createFailedTransaction(req, &failReason)
		return domain.Transaction{}, fmt.Errorf("partner charge failed: %w", err)
	}

	// Step 2: Credit the wallet inside a DB transaction
	var txn domain.Transaction

	err = pgx.BeginTxFunc(ctx, s.db, pgx.TxOptions{IsoLevel: pgx.Serializable}, func(tx pgx.Tx) error {
		wallet, err := s.wallets.GetByIDForUpdate(ctx, tx, req.WalletID)
		if err != nil {
			return fmt.Errorf("locking wallet: %w", err)
		}
		if wallet.Frozen {
			return fmt.Errorf("wallet is frozen")
		}
		if wallet.Currency != req.Currency {
			return fmt.Errorf("currency mismatch: wallet is %s, request is %s", wallet.Currency, req.Currency)
		}

		partnerRef := chargeResp.PartnerTxnID
		txn = domain.Transaction{
			ID:               uuid.New().String(),
			IdempotencyKey:   req.IdempotencyKey,
			Type:             domain.TxnTopUp,
			Status:           domain.TxnCompleted,
			SourceWalletID:   nil,
			DestWalletID:     req.WalletID,
			Amount:           req.Amount,
			Currency:         req.Currency,
			PartnerReference: &partnerRef,
			CreatedAt:        time.Now(),
			UpdatedAt:        time.Now(),
		}

		if err := s.txns.Create(ctx, tx, txn); err != nil {
			return fmt.Errorf("creating transaction: %w", err)
		}

		newBalance := wallet.Balance + req.Amount
		if err := s.wallets.UpdateBalance(ctx, tx, wallet.ID, newBalance); err != nil {
			return fmt.Errorf("crediting wallet: %w", err)
		}

		creditEntry := domain.LedgerEntry{
			ID:            uuid.New().String(),
			TransactionID: txn.ID,
			WalletID:      wallet.ID,
			EntryType:     domain.Credit,
			Amount:        req.Amount,
			BalanceAfter:  newBalance,
			CreatedAt:     time.Now(),
		}
		if err := s.ledger.Create(ctx, tx, creditEntry); err != nil {
			return fmt.Errorf("creating ledger entry: %w", err)
		}

		return nil
	})

	if err != nil {
		return domain.Transaction{}, err
	}
	return txn, nil
}

func (s *PaymentService) createFailedTransaction(req TopUpRequest, failReason *string) {
	txn := domain.Transaction{
		ID:             uuid.New().String(),
		IdempotencyKey: req.IdempotencyKey,
		Type:           domain.TxnTopUp,
		Status:         domain.TxnFailed,
		DestWalletID:   req.WalletID,
		Amount:         req.Amount,
		Currency:       req.Currency,
		FailureReason:  failReason,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	// Best-effort: use a fresh background context so a cancelled caller ctx doesn't prevent the audit write
	bgCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_ = pgx.BeginTxFunc(bgCtx, s.db, pgx.TxOptions{}, func(tx pgx.Tx) error {
		return s.txns.Create(bgCtx, tx, txn)
	})
}
