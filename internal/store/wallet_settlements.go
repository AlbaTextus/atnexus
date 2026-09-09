package store

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// All settlement paths lock the wallet before the queue rows. This also runs
// before payments/adjustments, so due funds are spendable at the 24-hour boundary
// even when the background worker has not ticked or has been offline.
func settleLockedWallet(ctx context.Context, tx pgx.Tx, userID uuid.UUID, wallet *lockedWallet) error {
	rows, err := tx.Query(ctx, `SELECT s.order_id,s.amount,s.settles_at,o.title
 FROM wallet_settlements s JOIN payment_orders o ON o.id=s.order_id
 WHERE s.user_id=$1 AND s.settled_at IS NULL AND s.settles_at<=clock_timestamp()
 ORDER BY s.settles_at,s.order_id FOR UPDATE OF s`, userID)
	if err != nil {
		return err
	}
	type settlement struct {
		id     uuid.UUID
		amount Amount
		at     time.Time
		title  string
	}
	var due []settlement
	for rows.Next() {
		var item settlement
		if err = rows.Scan(&item.id, &item.amount, &item.at, &item.title); err != nil {
			rows.Close()
			return err
		}
		due = append(due, item)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if len(due) == 0 {
		return nil
	}
	for _, item := range due {
		wallet.balance += item.amount
		wallet.pendingBalance -= item.amount
		// Settlement changes availability, not the total. Never spend pending credits.
		if wallet.pendingBalance < 0 || wallet.balance > MaxWalletAmount {
			return ErrWalletAmount
		}
		if _, err = tx.Exec(ctx, `INSERT INTO wallet_entries(id,user_id,order_id,amount,balance_after,kind,description,created_at)
   VALUES($1,$2,$3,$4,$5,'receipt',$6,$7)`, uuid.New(), userID, item.id, item.amount, wallet.balance, item.title, item.at); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `UPDATE wallet_settlements SET settled_at=clock_timestamp() WHERE order_id=$1`, item.id); err != nil {
			return err
		}
	}
	_, err = tx.Exec(ctx, `UPDATE wallets SET balance=$2,pending_balance=$3,updated_at=NOW() WHERE user_id=$1`, userID, wallet.balance, wallet.pendingBalance)
	return err
}

// Multiple server instances may scan the same batch; wallet locks and the unique
// order/user ledger constraint serialize them without duplicating any credits.
func (p *Postgres) SettleDueWallets(ctx context.Context, limit int) (int, error) {
	if limit < 1 || limit > 1000 {
		limit = 100
	}
	rows, err := p.pool.Query(ctx, `SELECT user_id FROM wallet_settlements
 WHERE settled_at IS NULL AND settles_at<=clock_timestamp() GROUP BY user_id ORDER BY user_id LIMIT $1`, limit)
	if err != nil {
		return 0, err
	}
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return 0, err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return 0, err
	}
	for i, id := range ids {
		if _, err = p.Wallet(ctx, id); err != nil {
			return i, err
		}
	}
	return len(ids), nil
}
