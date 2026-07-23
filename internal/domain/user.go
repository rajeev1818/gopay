package domain

import (
	"time"
)

type KYCStatus string

const (
	KYCPending  KYCStatus = "pending"
	KYCApproved KYCStatus = "approved"
	KYCRejected KYCStatus = "rejected"
)

type User struct {
	ID           string    `json:"id"`
	Email        string    `json:"email"`
	Name         string    `json:"name"`
	PasswordHash string    `json:"-"`
	KYCStatus    KYCStatus `json:"kyc_status"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}
