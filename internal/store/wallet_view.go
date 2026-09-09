package store

import (
	"context"
	"math/big"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Read models retain the wallet UI contract without exposing store credentials.
type WalletActivity struct {
	ID              uuid.UUID `json:"id"`
	OrderNo         string    `json:"order_no"`
	OrderName       string    `json:"order_name"`
	MerchantOrderNo string    `json:"merchant_order_no"`
	Amount          Amount    `json:"amount"`
	Type            string    `json:"type"`
	Status          string    `json:"status"`
	PayerID         string    `json:"payer_user_id"`
	PayeeID         string    `json:"payee_user_id"`
	PayerName       string    `json:"payer_username"`
	PayeeName       string    `json:"payee_username"`
	PayerAvatar     string    `json:"payer_avatar_url"`
	PayeeAvatar     string    `json:"payee_avatar_url"`
	ClientID        string    `json:"client_id"`
	AppName         string    `json:"app_name"`
	AppHomepageURL  string    `json:"app_homepage_url"`
	Remark          string    `json:"remark"`
	CreatedAt       string    `json:"created_at"`
	UpdatedAt       string    `json:"updated_at"`
	TradeTime       string    `json:"trade_time"`
	ExpiresAt       string    `json:"expires_at"`
	PaymentType     string    `json:"payment_type"`
	TransferStatus  string    `json:"payee_transfer_status"`
	TransferAt      string    `json:"payee_transfer_at"`
}

type WalletActivityQuery struct {
	Page, PageSize  int
	Types, Statuses []string
	Start, End      time.Time // End is exclusive.
}
type WalletActivityPage struct {
	Orders   []WalletActivity `json:"orders"`
	Total    int64            `json:"total"`
	Page     int              `json:"page"`
	PageSize int              `json:"page_size"`
}
type WalletDailyStat struct {
	Date    string `json:"date"`
	Income  string `json:"income"`
	Expense string `json:"expense"`
}
type WalletStats struct {
	Days         []WalletDailyStat `json:"days"`
	TotalIncome  string            `json:"total_income"`
	TotalExpense string            `json:"total_expense"`
}
type WalletViewStore interface {
	Wallet(context.Context, uuid.UUID) (Wallet, error)
	WalletActivity(context.Context, uuid.UUID, WalletActivityQuery) (WalletActivityPage, error)
	WalletDailyStats(context.Context, uuid.UUID, time.Time) (WalletStats, error)
}

// Payers see their order lifecycle; recipients see only confirmed receipts.
// Adjustment rows belong to their wallet owner, never the administrator's wallet.
// Paid orders occur once here even after their receipt ledger entry is posted.
const walletActivityCTE = `WITH activity AS (
 SELECT o.id,o.title,o.amount,o.payer_id,o.payee_id,o.client_id,o.client_name,o.external_order_id,
 o.created_at,o.expires_at,o.paid_at AS trade_at,
 CASE WHEN o.payer_id=$1 THEN 'payment' ELSE 'receive' END AS type,
 CASE WHEN o.status='paid' THEN 'success' WHEN o.status='cancelled' THEN 'failed'
      WHEN o.expires_at<=transaction_timestamp() THEN 'expired' ELSE 'pending' END AS status,
 CASE WHEN o.status='cancelled' THEN '订单已取消' ELSE '' END AS remark,
 COALESCE(s.settles_at,o.paid_at) AS transfer_at,
 (o.status='paid' AND s.order_id IS NOT NULL AND s.settled_at IS NULL) AS awaiting
 FROM payment_orders o LEFT JOIN wallet_settlements s ON s.order_id=o.id
 WHERE o.payer_id=$1 OR (o.payee_id=$1 AND o.status='paid')
 UNION ALL
 SELECT e.id,e.description,abs(e.amount),CASE WHEN e.amount<0 THEN e.user_id ELSE NULL::uuid END,
 CASE WHEN e.amount>0 THEN e.user_id ELSE NULL::uuid END,NULL::varchar,'ALBATEXTUS','',
 e.created_at,NULL::timestamptz,e.created_at,'community','success',e.description,e.created_at,false
 FROM wallet_entries e WHERE e.user_id=$1 AND e.kind='adjustment'
), filtered AS (
 SELECT * FROM activity WHERE ($2::text[] IS NULL OR type=ANY($2))
 AND ($3::text[] IS NULL OR status=ANY($3)) AND created_at >= $4 AND created_at < $5
) `

func (p *Postgres) WalletActivity(ctx context.Context, userID uuid.UUID, q WalletActivityQuery) (WalletActivityPage, error) {
	result := WalletActivityPage{Orders: []WalletActivity{}, Page: q.Page, PageSize: q.PageSize}
	if q.Page < 1 || q.Page > 100001 || q.PageSize < 1 || q.PageSize > 100 || int64(q.Page-1)*int64(q.PageSize) > 100000 || !q.End.After(q.Start) {
		return result, ErrWalletQuery
	}
	if _, err := p.Wallet(ctx, userID); err != nil {
		return result, err
	}
	tx, err := p.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return result, err
	}
	defer tx.Rollback(ctx)
	args := []any{userID, q.Types, q.Statuses, q.Start, q.End}
	if err = tx.QueryRow(ctx, walletActivityCTE+`SELECT count(*) FROM filtered`, args...).Scan(&result.Total); err != nil {
		return result, err
	}
	rows, err := tx.Query(ctx, walletActivityCTE+`SELECT f.id,f.title,f.amount,f.type,f.status,
 COALESCE(f.payer_id::text,'platform'),COALESCE(f.payee_id::text,'platform'),
 COALESCE(NULLIF(payer.display_name,''),'用户 '||payer.numeric_id::text,'ALBATEXTUS'),
 COALESCE(NULLIF(payee.display_name,''),'用户 '||payee.numeric_id::text,'ALBATEXTUS'),
 CASE WHEN payer.avatar_source='custom' THEN payer.custom_avatar_url ELSE COALESCE((SELECT ei.avatar_url FROM external_identities ei
  WHERE ei.user_id=payer.id AND ei.avatar_url<>'' AND (payer.avatar_source='auto' OR ei.provider=payer.avatar_source) ORDER BY ei.created_at,ei.id LIMIT 1),'') END,
 CASE WHEN payee.avatar_source='custom' THEN payee.custom_avatar_url ELSE COALESCE((SELECT ei.avatar_url FROM external_identities ei
  WHERE ei.user_id=payee.id AND ei.avatar_url<>'' AND (payee.avatar_source='auto' OR ei.provider=payee.avatar_source) ORDER BY ei.created_at,ei.id LIMIT 1),'') END,
 COALESCE(f.client_id,''),f.client_name,COALESCE(c.homepage_url,''),f.external_order_id,f.remark,
 f.created_at,f.expires_at,f.trade_at,f.transfer_at,f.awaiting
 FROM filtered f LEFT JOIN users payer ON payer.id=f.payer_id LEFT JOIN users payee ON payee.id=f.payee_id
 LEFT JOIN oauth_clients c ON c.id=f.client_id ORDER BY f.created_at DESC,f.id DESC LIMIT $6 OFFSET $7`, append(args, q.PageSize, (q.Page-1)*q.PageSize)...)
	if err != nil {
		return result, err
	}
	for rows.Next() {
		var item WalletActivity
		var created time.Time
		var expires, trade, transfer *time.Time
		var awaiting bool
		if err = rows.Scan(&item.ID, &item.OrderName, &item.Amount, &item.Type, &item.Status, &item.PayerID, &item.PayeeID,
			&item.PayerName, &item.PayeeName, &item.PayerAvatar, &item.PayeeAvatar, &item.ClientID, &item.AppName, &item.AppHomepageURL,
			&item.MerchantOrderNo, &item.Remark, &created, &expires, &trade, &transfer, &awaiting); err != nil {
			rows.Close()
			return result, err
		}
		item.OrderNo = item.ID.String()
		item.CreatedAt = created.UTC().Format(time.RFC3339Nano)
		item.UpdatedAt = item.CreatedAt
		item.ExpiresAt = "—"
		if expires != nil {
			item.ExpiresAt = expires.UTC().Format(time.RFC3339Nano)
		}
		if trade != nil {
			item.TradeTime = trade.UTC().Format(time.RFC3339Nano)
			item.UpdatedAt = item.TradeTime
		}
		if transfer != nil {
			item.TransferAt = transfer.UTC().Format(time.RFC3339Nano)
		}
		item.PaymentType = "credits"
		item.TransferStatus = "completed"
		if awaiting {
			item.TransferStatus = "pending"
		}
		result.Orders = append(result.Orders, item)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return result, err
	}
	return result, tx.Commit(ctx)
}

func (p *Postgres) WalletDailyStats(ctx context.Context, userID uuid.UUID, now time.Time) (WalletStats, error) {
	result := WalletStats{Days: make([]WalletDailyStat, 7), TotalIncome: "0", TotalExpense: "0"}
	if _, err := p.Wallet(ctx, userID); err != nil {
		return result, err
	}
	local := now.In(time.FixedZone("Asia/Shanghai", 8*60*60))
	today := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, local.Location())
	for i := range result.Days {
		result.Days[i] = WalletDailyStat{Date: today.AddDate(0, 0, -i).Format("2006-01-02"), Income: "0", Expense: "0"}
	}
	rows, err := p.pool.Query(ctx, `SELECT to_char(created_at AT TIME ZONE 'Asia/Shanghai','YYYY-MM-DD'),
 trunc(COALESCE(sum(amount) FILTER (WHERE amount>0),0)/100)::text,
 trunc(COALESCE(sum(-amount) FILTER (WHERE amount<0),0)/100)::text
 FROM wallet_entries WHERE user_id=$1 AND created_at >= $2 AND created_at <= $3
 GROUP BY 1`, userID, today.AddDate(0, 0, -6), now)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	income, expense := new(big.Int), new(big.Int)
	for rows.Next() {
		var day WalletDailyStat
		if err = rows.Scan(&day.Date, &day.Income, &day.Expense); err != nil {
			return result, err
		}
		for i := range result.Days {
			if result.Days[i].Date == day.Date {
				result.Days[i] = day
			}
		}
		in, okIn := new(big.Int).SetString(day.Income, 10)
		out, okOut := new(big.Int).SetString(day.Expense, 10)
		if !okIn || !okOut {
			return result, ErrWalletAmount
		}
		income.Add(income, in)
		expense.Add(expense, out)
	}
	result.TotalIncome = income.String()
	result.TotalExpense = expense.String()
	return result, rows.Err()
}

var _ WalletViewStore = (*Postgres)(nil)
