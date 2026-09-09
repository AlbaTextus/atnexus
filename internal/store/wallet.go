package store

import (
	"context"
	"errors"
	"sort"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const orderColumns = `id,client_id,client_name,external_order_id,payer_id,payee_id,title,amount,
 CASE WHEN status='pending' AND expires_at<=clock_timestamp() THEN 'expired' ELSE status END,created_at,expires_at,paid_at`
const entryColumns = `id,order_id,amount,balance_after,kind,description,created_at`

func (p *Postgres) WalletUser(ctx context.Context, numericID int) (User, error) {
	user, err := scanUser(p.pool.QueryRow(ctx, `SELECT `+userColumns+` FROM users WHERE numeric_id=$1`, numericID))
	if errors.Is(err, pgx.ErrNoRows) {
		err = ErrNotFound
	}
	return user, err
}

func scanOrder(row pgx.Row) (PaymentOrder, error) {
	var o PaymentOrder
	err := row.Scan(&o.ID, &o.ClientID, &o.ClientName, &o.ExternalID, &o.PayerID, &o.PayeeID, &o.Title, &o.Amount, &o.Status, &o.CreatedAt, &o.ExpiresAt, &o.PaidAt)
	if errors.Is(err, pgx.ErrNoRows) {
		err = ErrNotFound
	}
	return o, err
}

func scanEntry(row pgx.Row) (WalletEntry, error) {
	var e WalletEntry
	err := row.Scan(&e.ID, &e.OrderID, &e.Amount, &e.BalanceAfter, &e.Kind, &e.Description, &e.CreatedAt)
	return e, err
}

func (p *Postgres) Wallet(ctx context.Context, userID uuid.UUID) (Wallet, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return Wallet{}, err
	}
	defer tx.Rollback(ctx)
	if _, err = lockWallets(ctx, tx, userID); err != nil {
		return Wallet{}, err
	}
	var wallet Wallet
	err = tx.QueryRow(ctx, `SELECT balance,pending_balance FROM wallets WHERE user_id=$1`, userID).
		Scan(&wallet.Balance, &wallet.PendingBalance)
	if err != nil {
		return Wallet{}, err
	}
	return wallet, tx.Commit(ctx)
}

func (p *Postgres) WalletEntries(ctx context.Context, userID uuid.UUID, offset int) ([]WalletEntry, error) {
	if _, err := p.Wallet(ctx, userID); err != nil {
		return nil, err
	}
	rows, err := p.pool.Query(ctx, `SELECT `+entryColumns+` FROM wallet_entries WHERE user_id=$1 ORDER BY created_at DESC,id DESC LIMIT 21 OFFSET $2`, userID, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	entries := make([]WalletEntry, 0)
	for rows.Next() {
		entry, err := scanEntry(rows)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return entries, rows.Err()
}

// Search the owner's complete ledger before pagination. STRPOS treats the query
// literally, so wildcard characters cannot broaden an identifier search.
func (p *Postgres) SearchWalletEntries(ctx context.Context, userID uuid.UUID, query string, offset int) ([]WalletEntry, error) {
	if len(query) > 100 || offset < 0 || offset > 100000 {
		return nil, ErrWalletQuery
	}
	rows, err := p.pool.Query(ctx, `SELECT `+entryColumns+` FROM wallet_entries
	 WHERE user_id=$1 AND (strpos(lower(id::text),lower($2))>0 OR strpos(lower(COALESCE(order_id::text,'')),lower($2))>0)
	 ORDER BY created_at DESC,id DESC LIMIT 21 OFFSET $3`, userID, query, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	entries := make([]WalletEntry, 0)
	for rows.Next() {
		entry, err := scanEntry(rows)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return entries, rows.Err()
}

// All wallet writers lock users before wallets. Sorting both sets also permits
// simultaneous A→B and B→A payments without acquiring wallet locks in reverse.
func lockWalletUsers(ctx context.Context, tx pgx.Tx, ids ...uuid.UUID) (map[uuid.UUID]User, error) {
	rows, err := tx.Query(ctx, `SELECT id,role,is_active FROM users WHERE id=ANY($1::uuid[]) ORDER BY id FOR SHARE`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	users := make(map[uuid.UUID]User)
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Role, &u.IsActive); err != nil {
			return nil, err
		}
		users[u.ID] = u
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, id := range ids {
		if !users[id].IsActive {
			return nil, ErrWalletForbidden
		}
	}
	return users, nil
}

type lockedWallet struct {
	balance        Amount
	pendingBalance Amount
}

func lockWallets(ctx context.Context, tx pgx.Tx, ids ...uuid.UUID) (map[uuid.UUID]lockedWallet, error) {
	sort.Slice(ids, func(i, j int) bool { return ids[i].String() < ids[j].String() })
	wallets := make(map[uuid.UUID]lockedWallet)
	for _, id := range ids {
		if _, found := wallets[id]; found {
			continue
		}
		if _, err := tx.Exec(ctx, `INSERT INTO wallets(user_id) VALUES($1) ON CONFLICT DO NOTHING`, id); err != nil {
			return nil, err
		}
		var wallet lockedWallet
		err := tx.QueryRow(ctx, `SELECT balance,pending_balance FROM wallets WHERE user_id=$1 FOR UPDATE`, id).
			Scan(&wallet.balance, &wallet.pendingBalance)
		if err != nil {
			return nil, err
		}
		if err := settleLockedWallet(ctx, tx, id, &wallet); err != nil {
			return nil, err
		}
		wallets[id] = wallet
	}
	return wallets, nil
}

func (p *Postgres) CreatePaymentOrder(ctx context.Context, in NewPaymentOrder) (PaymentOrder, bool, error) {
	if in.Amount <= 0 || in.Amount > MaxWalletAmount || in.Amount%100 != 0 {
		return PaymentOrder{}, false, ErrWalletAmount
	}
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return PaymentOrder{}, false, err
	}
	defer tx.Rollback(ctx)
	var owner *uuid.UUID
	var name, minimum, hash string
	var active bool
	err = tx.QueryRow(ctx, `SELECT owner_id,name,minimum_role,COALESCE(secret_hash,''),is_active FROM oauth_clients WHERE id=$1 FOR SHARE`, in.ClientID).
		Scan(&owner, &name, &minimum, &hash, &active)
	if errors.Is(err, pgx.ErrNoRows) {
		return PaymentOrder{}, false, ErrWalletForbidden
	}
	if err != nil {
		return PaymentOrder{}, false, err
	}
	if !active || owner == nil || hash == "" || hash != in.SecretHash || *owner == in.PayerID {
		return PaymentOrder{}, false, ErrWalletForbidden
	}
	users, err := lockWalletUsers(ctx, tx, in.PayerID, *owner)
	if err != nil {
		return PaymentOrder{}, false, err
	}
	if !MeetsMinimumRole(users[in.PayerID].Role, minimum) {
		return PaymentOrder{}, false, ErrWalletForbidden
	}
	order, err := scanOrder(tx.QueryRow(ctx, `INSERT INTO payment_orders
 (id,client_id,client_name,external_order_id,payer_id,payee_id,title,amount,expires_at)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8,clock_timestamp()+INTERVAL '15 minutes')
 ON CONFLICT(client_id,external_order_id) DO NOTHING RETURNING `+orderColumns,
		uuid.New(), in.ClientID, name, in.ExternalID, in.PayerID, *owner, in.Title, in.Amount))
	created := err == nil
	if errors.Is(err, ErrNotFound) {
		order, err = scanOrder(tx.QueryRow(ctx, `SELECT `+orderColumns+` FROM payment_orders WHERE client_id=$1 AND external_order_id=$2`, in.ClientID, in.ExternalID))
		if err == nil && (order.PayerID != in.PayerID || order.PayeeID != *owner || order.Amount != in.Amount || order.Title != in.Title) {
			err = ErrConflict
		}
	}
	if err != nil {
		return PaymentOrder{}, false, err
	}
	return order, created, tx.Commit(ctx)
}

func (p *Postgres) PaymentOrderForUser(ctx context.Context, orderID, userID uuid.UUID) (PaymentOrder, error) {
	return scanOrder(p.pool.QueryRow(ctx, `SELECT `+orderColumns+` FROM payment_orders WHERE id=$1 AND payer_id=$2`, orderID, userID))
}

func (p *Postgres) PaymentOrderForClient(ctx context.Context, orderID uuid.UUID, clientID string) (PaymentOrder, error) {
	return scanOrder(p.pool.QueryRow(ctx, `SELECT `+orderColumns+` FROM payment_orders WHERE id=$1 AND client_id=$2`, orderID, clientID))
}

func (p *Postgres) PaymentOrders(ctx context.Context, userID uuid.UUID, offset int) ([]PaymentOrder, error) {
	rows, err := p.pool.Query(ctx, `SELECT `+orderColumns+` FROM payment_orders WHERE payer_id=$1 ORDER BY created_at DESC,id DESC LIMIT 21 OFFSET $2`, userID, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	orders := make([]PaymentOrder, 0)
	for rows.Next() {
		order, err := scanOrder(rows)
		if err != nil {
			return nil, err
		}
		orders = append(orders, order)
	}
	return orders, rows.Err()
}

func (p *Postgres) PayOrder(ctx context.Context, orderID, userID uuid.UUID) (PaymentOrder, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return PaymentOrder{}, err
	}
	defer tx.Rollback(ctx)
	order, err := scanOrder(tx.QueryRow(ctx, `SELECT `+orderColumns+` FROM payment_orders WHERE id=$1 AND payer_id=$2 FOR UPDATE`, orderID, userID))
	if err != nil {
		return PaymentOrder{}, err
	}
	if order.Status == "paid" {
		return order, nil
	}
	if order.Status != "pending" {
		return PaymentOrder{}, ErrWalletOrderClosed
	}
	var owner *uuid.UUID
	var active bool
	var minimum string
	err = tx.QueryRow(ctx, `SELECT owner_id,is_active,minimum_role FROM oauth_clients WHERE id=$1 FOR SHARE`, order.ClientID).Scan(&owner, &active, &minimum)
	if err != nil {
		return PaymentOrder{}, err
	}
	if !active || owner == nil || *owner != order.PayeeID {
		return PaymentOrder{}, ErrWalletForbidden
	}
	users, err := lockWalletUsers(ctx, tx, userID, order.PayeeID)
	if err != nil {
		return PaymentOrder{}, err
	}
	if !MeetsMinimumRole(users[userID].Role, minimum) {
		return PaymentOrder{}, ErrWalletForbidden
	}
	wallets, err := lockWallets(ctx, tx, userID, order.PayeeID)
	if err != nil {
		return PaymentOrder{}, err
	}
	payer, payee := wallets[userID], wallets[order.PayeeID]
	if payer.balance < order.Amount {
		return PaymentOrder{}, ErrWalletBalance
	}
	if payee.balance+payee.pendingBalance > MaxWalletAmount-order.Amount {
		return PaymentOrder{}, ErrWalletAmount
	}
	if _, err = tx.Exec(ctx, `UPDATE wallets SET balance=balance-$2,updated_at=NOW() WHERE user_id=$1`, userID, order.Amount); err != nil {
		return PaymentOrder{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE wallets SET pending_balance=pending_balance+$2,updated_at=NOW() WHERE user_id=$1`, order.PayeeID, order.Amount); err != nil {
		return PaymentOrder{}, err
	}
	order, err = scanOrder(tx.QueryRow(ctx, `UPDATE payment_orders SET status='paid',paid_at=clock_timestamp() WHERE id=$1 AND expires_at>clock_timestamp() RETURNING `+orderColumns, order.ID))
	if errors.Is(err, ErrNotFound) {
		return PaymentOrder{}, ErrWalletOrderClosed
	}
	if err != nil {
		return PaymentOrder{}, err
	}
	// The debit and pending credit use the same successful payment timestamp.
	if _, err = tx.Exec(ctx, `INSERT INTO wallet_entries(id,user_id,order_id,amount,balance_after,kind,description,created_at)
 VALUES($1,$2,$3,$4,$5,'payment',$6,$7)`, uuid.New(), userID, order.ID, -order.Amount, payer.balance-order.Amount, order.Title, order.PaidAt); err != nil {
		return PaymentOrder{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO wallet_settlements(order_id,user_id,amount,settles_at)
 VALUES($1,$2,$3,$4::timestamptz + INTERVAL '24 hours')`, order.ID, order.PayeeID, order.Amount, order.PaidAt); err != nil {
		return PaymentOrder{}, err
	}
	return order, tx.Commit(ctx)
}

func (p *Postgres) CancelPaymentOrder(ctx context.Context, orderID, userID uuid.UUID) error {
	result, err := p.pool.Exec(ctx, `UPDATE payment_orders SET status='cancelled' WHERE id=$1 AND payer_id=$2 AND status='pending' AND expires_at>clock_timestamp()`, orderID, userID)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		order, err := p.PaymentOrderForUser(ctx, orderID, userID)
		if err != nil {
			return err
		}
		if order.Status == "cancelled" {
			return nil
		}
		return ErrWalletOrderClosed
	}
	return nil
}

func (p *Postgres) AdjustWallet(ctx context.Context, in WalletAdjustment) (WalletEntry, error) {
	if in.Amount == 0 || in.Amount > MaxWalletAmount || in.Amount < -MaxWalletAmount || in.Amount%100 != 0 {
		return WalletEntry{}, ErrWalletAmount
	}
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return WalletEntry{}, err
	}
	defer tx.Rollback(ctx)
	// Serialize retries of the same key, even if a caller changes the target user.
	_, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "wallet-adjust:"+in.ActorID.String()+":"+in.Key)
	if err != nil {
		return WalletEntry{}, err
	}
	var userID uuid.UUID
	err = tx.QueryRow(ctx, `SELECT id FROM users WHERE numeric_id=$1`, in.UserNumericID).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return WalletEntry{}, ErrNotFound
	}
	if err != nil {
		return WalletEntry{}, err
	}
	users, err := lockWalletUsers(ctx, tx, in.ActorID, userID)
	if err != nil {
		return WalletEntry{}, err
	}
	if users[in.ActorID].Role != "SUPER_ADMIN" {
		return WalletEntry{}, ErrWalletForbidden
	}
	var existingUser uuid.UUID
	err = tx.QueryRow(ctx, `SELECT user_id FROM wallet_entries WHERE actor_id=$1 AND idempotency_key=$2`, in.ActorID, in.Key).Scan(&existingUser)
	if err == nil {
		entry, err := scanEntry(tx.QueryRow(ctx, `SELECT `+entryColumns+` FROM wallet_entries WHERE actor_id=$1 AND idempotency_key=$2`, in.ActorID, in.Key))
		if err == nil && (existingUser != userID || entry.Amount != in.Amount || entry.Description != in.Reason) {
			err = ErrConflict
		}
		return entry, err
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return WalletEntry{}, err
	}
	wallets, err := lockWallets(ctx, tx, userID)
	if err != nil {
		return WalletEntry{}, err
	}
	balance := wallets[userID].balance + in.Amount
	if balance < 0 {
		return WalletEntry{}, ErrWalletBalance
	}
	if balance+wallets[userID].pendingBalance > MaxWalletAmount {
		return WalletEntry{}, ErrWalletAmount
	}
	_, err = tx.Exec(ctx, `UPDATE wallets SET balance=$2,updated_at=NOW() WHERE user_id=$1`, userID, balance)
	if err != nil {
		return WalletEntry{}, err
	}
	entry, err := scanEntry(tx.QueryRow(ctx, `INSERT INTO wallet_entries(id,user_id,amount,balance_after,kind,description,actor_id,idempotency_key)
 VALUES($1,$2,$3,$4,'adjustment',$5,$6,$7) RETURNING `+entryColumns, uuid.New(), userID, in.Amount, balance, in.Reason, in.ActorID, in.Key))
	if err != nil {
		return WalletEntry{}, err
	}
	return entry, tx.Commit(ctx)
}

var _ WalletStore = (*Postgres)(nil)
