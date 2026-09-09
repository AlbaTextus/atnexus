package store

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Credits are whole numbers. Retain the historical storage scale of 100 to
// preserve existing balances; inputs and database constraints require multiples.
type Amount int64

const MaxWalletAmount Amount = 9_000_000_000_000

func ParseAmount(value string) (Amount, error) {
	if value == "" || len(value) > 16 {
		return 0, ErrWalletAmount
	}
	negative := strings.HasPrefix(value, "-")
	if negative {
		value = value[1:]
	}
	if value == "" || (len(value) > 1 && value[0] == '0') {
		return 0, ErrWalletAmount
	}
	for _, c := range value {
		if c < '0' || c > '9' {
			return 0, ErrWalletAmount
		}
	}
	n, err := strconv.ParseInt(value, 10, 64)
	if err != nil || n > int64(MaxWalletAmount)/100 {
		return 0, ErrWalletAmount
	}
	if negative {
		n = -n
	}
	return Amount(n * 100), nil
}

func (a Amount) String() string {
	return strconv.FormatInt(int64(a)/100, 10)
}
func (a Amount) MarshalJSON() ([]byte, error) {
	if a%100 != 0 {
		return nil, ErrWalletAmount
	}
	return json.Marshal(a.String())
}

var (
	ErrWalletQuery            = errors.New("invalid wallet query")
	ErrApplicationHasPayments = errors.New("application has payment records")
	ErrWalletAmount           = errors.New("invalid wallet amount")
	ErrWalletBalance          = errors.New("insufficient wallet balance")
	ErrWalletOrderClosed      = errors.New("payment order is closed")
	ErrWalletForbidden        = errors.New("wallet operation forbidden")
)

type Wallet struct {
	Balance        Amount `json:"balance"`
	PendingBalance Amount `json:"pending_balance"`
}

type WalletEntry struct {
	ID           uuid.UUID  `json:"id"`
	OrderID      *uuid.UUID `json:"order_id"`
	Amount       Amount     `json:"amount"`
	BalanceAfter Amount     `json:"balance_after"`
	Kind         string     `json:"kind"`
	Description  string     `json:"description"`
	CreatedAt    time.Time  `json:"created_at"`
}

type WalletEntrySearchStore interface {
	SearchWalletEntries(context.Context, uuid.UUID, string, int) ([]WalletEntry, error)
}

type PaymentOrder struct {
	ID         uuid.UUID  `json:"id"`
	ClientID   string     `json:"client_id"`
	ClientName string     `json:"client_name"`
	ExternalID string     `json:"external_order_id"`
	PayerID    uuid.UUID  `json:"payer_id"`
	PayeeID    uuid.UUID  `json:"payee_id"`
	Title      string     `json:"title"`
	Amount     Amount     `json:"amount"`
	Status     string     `json:"status"`
	CreatedAt  time.Time  `json:"created_at"`
	ExpiresAt  time.Time  `json:"expires_at"`
	PaidAt     *time.Time `json:"paid_at"`
}

type NewPaymentOrder struct {
	ClientID, SecretHash, ExternalID, Title string
	PayerID                                 uuid.UUID
	Amount                                  Amount
}

type WalletAdjustment struct {
	ActorID       uuid.UUID
	UserNumericID int
	Amount        Amount
	Reason, Key   string
}

type WalletStore interface {
	WalletUser(context.Context, int) (User, error)
	Wallet(context.Context, uuid.UUID) (Wallet, error)
	WalletEntries(context.Context, uuid.UUID, int) ([]WalletEntry, error)
	CreatePaymentOrder(context.Context, NewPaymentOrder) (PaymentOrder, bool, error)
	PaymentOrderForUser(context.Context, uuid.UUID, uuid.UUID) (PaymentOrder, error)
	PaymentOrderForClient(context.Context, uuid.UUID, string) (PaymentOrder, error)
	PaymentOrders(context.Context, uuid.UUID, int) ([]PaymentOrder, error)
	PayOrder(context.Context, uuid.UUID, uuid.UUID) (PaymentOrder, error)
	CancelPaymentOrder(context.Context, uuid.UUID, uuid.UUID) error
	AdjustWallet(context.Context, WalletAdjustment) (WalletEntry, error)
}
