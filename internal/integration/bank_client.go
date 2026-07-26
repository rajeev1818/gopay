package integration

import (
	"context"
	"fmt"
	"math/rand"
	"time"
)

type MockBankClient struct {
	failRate float64
	latency  time.Duration
}

func NewMockBankClient(failRate float64, latency time.Duration) *MockBankClient {
	return &MockBankClient{failRate: failRate, latency: latency}
}

func (c *MockBankClient) Charge(ctx context.Context, req ChargeRequest) (ChargeResponse, error) {
	select {
	case <-time.After(c.latency):
	case <-ctx.Done():
		return ChargeResponse{}, ctx.Err()
	}

	// Simulate failures
	if rand.Float64() < c.failRate {
		return ChargeResponse{}, fmt.Errorf("bank unavailable: connection timeout")
	}

	return ChargeResponse{
		PartnerTxnID: fmt.Sprintf("BANK-%d", time.Now().UnixNano()),
		Status:       "success",
	}, nil
}
