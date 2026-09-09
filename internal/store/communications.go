package store

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type Communication struct {
	ID          uuid.UUID  `json:"id"`
	Kind        string     `json:"kind"`
	Title       string     `json:"title"`
	Content     string     `json:"content"`
	Audience    string     `json:"audience"`
	Status      string     `json:"status"`
	Revision    int        `json:"revision"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	PublishedAt *time.Time `json:"published_at"`
	Total       int        `json:"total"`
	Sent        int        `json:"sent"`
	Failed      int        `json:"failed"`
}

type CommunicationStore interface {
	ListCommunications(context.Context, bool) ([]Communication, error)
	PublicAnnouncement(context.Context, uuid.UUID) (Communication, error)
	SaveCommunication(context.Context, Communication, uuid.UUID) (Communication, error)
	QueueCommunication(context.Context, uuid.UUID, int, uuid.UUID) error
	ArchiveCommunication(context.Context, uuid.UUID, int) error
	CommunicationAudience(context.Context, uuid.UUID) (map[string]int, error)
}

const communicationColumns = `c.id,c.kind,c.title,c.content,c.audience,c.status,c.revision,c.created_at,c.updated_at,c.published_at,
 (SELECT COUNT(*) FROM mail_deliveries d WHERE d.communication_id=c.id),
 (SELECT COUNT(*) FROM mail_deliveries d WHERE d.communication_id=c.id AND d.status='sent'),
 (SELECT COUNT(*) FROM mail_deliveries d WHERE d.communication_id=c.id AND d.status='failed')`

func scanCommunication(row pgx.Row) (Communication, error) {
	var c Communication
	err := row.Scan(&c.ID, &c.Kind, &c.Title, &c.Content, &c.Audience, &c.Status, &c.Revision, &c.CreatedAt, &c.UpdatedAt, &c.PublishedAt, &c.Total, &c.Sent, &c.Failed)
	if errors.Is(err, pgx.ErrNoRows) {
		err = ErrNotFound
	}
	return c, err
}

func (p *Postgres) ListCommunications(ctx context.Context, public bool) ([]Communication, error) {
	rows, err := p.pool.Query(ctx, `SELECT `+communicationColumns+` FROM communications c WHERE NOT $1 OR (c.kind IN ('announcement','both') AND c.status IN ('published','queued')) ORDER BY c.created_at DESC,c.id DESC`, public)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]Communication, 0)
	for rows.Next() {
		c, err := scanCommunication(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, c)
	}
	return result, rows.Err()
}

func (p *Postgres) PublicAnnouncement(ctx context.Context, id uuid.UUID) (Communication, error) {
	return scanCommunication(p.pool.QueryRow(ctx, `SELECT `+communicationColumns+` FROM communications c WHERE id=$1 AND kind IN ('announcement','both') AND status IN ('published','queued')`, id))
}

func (p *Postgres) SaveCommunication(ctx context.Context, c Communication, actor uuid.UUID) (Communication, error) {
	if c.ID == uuid.Nil {
		c.ID = uuid.New()
		return scanCommunication(p.pool.QueryRow(ctx, `INSERT INTO communications AS c (id,kind,title,content,audience,status,created_by,published_at) VALUES ($1,$2,$3,$4,$5,$6,$7,CASE WHEN $6::varchar='published' THEN NOW() END) RETURNING `+communicationColumns, c.ID, c.Kind, c.Title, c.Content, c.Audience, c.Status, actor))
	}
	updated, err := scanCommunication(p.pool.QueryRow(ctx, `UPDATE communications c SET kind=$2,title=$3,content=$4,audience=$5,status=$6,revision=revision+1,updated_at=NOW(),published_at=CASE WHEN $6::varchar='published' THEN NOW() ELSE published_at END WHERE id=$1 AND revision=$7 AND status<>'queued' AND NOT EXISTS (SELECT 1 FROM mail_deliveries d WHERE d.communication_id=c.id) RETURNING `+communicationColumns, c.ID, c.Kind, c.Title, c.Content, c.Audience, c.Status, c.Revision))
	if errors.Is(err, ErrNotFound) {
		err = ErrConflict
	}
	return updated, err
}

const eligibleMailUser = `is_active AND email_verified AND email NOT LIKE '%.invalid'`

func (p *Postgres) CommunicationAudience(ctx context.Context, actor uuid.UUID) (map[string]int, error) {
	var all, admins, self int
	err := p.pool.QueryRow(ctx, `SELECT COUNT(*),COUNT(*) FILTER (WHERE role IN ('ADMIN','SUPER_ADMIN')),COUNT(*) FILTER (WHERE id=$1) FROM users WHERE `+eligibleMailUser, actor).Scan(&all, &admins, &self)
	return map[string]int{"all": all, "admins": admins, "self": self}, err
}

func (p *Postgres) QueueCommunication(ctx context.Context, id uuid.UUID, revision int, actor uuid.UUID) error {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var audience, status string
	var current int
	err = tx.QueryRow(ctx, `SELECT audience,status,revision FROM communications WHERE id=$1 AND kind IN ('email','both') FOR UPDATE`, id).Scan(&audience, &status, &current)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	// A repeated send request cannot enqueue the same campaign twice.
	if status == "queued" {
		return nil
	}
	if status != "draft" || current != revision {
		return ErrConflict
	}
	rows, err := tx.Query(ctx, `SELECT email FROM users WHERE `+eligibleMailUser+` AND ($1='all' OR ($1='admins' AND role IN ('ADMIN','SUPER_ADMIN')) OR ($1='self' AND id=$2))`, audience, actor)
	if err != nil {
		return err
	}
	recipients := []string{}
	for rows.Next() {
		var recipient string
		if err := rows.Scan(&recipient); err != nil {
			rows.Close()
			return err
		}
		recipients = append(recipients, recipient)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if len(recipients) == 0 {
		return ErrNotFound
	}
	batch := &pgx.Batch{}
	for _, recipient := range recipients {
		batch.Queue(`INSERT INTO mail_deliveries (id,communication_id,recipient) VALUES ($1,$2,$3)`, uuid.New(), id, recipient)
	}
	if err := tx.SendBatch(ctx, batch).Close(); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE communications SET status='queued',revision=revision+1,updated_at=NOW(),published_at=NOW() WHERE id=$1`, id)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Withdrawing the site announcement never changes the queued email or its content.
func (p *Postgres) ArchiveCommunication(ctx context.Context, id uuid.UUID, revision int) error {
	result, err := p.pool.Exec(ctx, `UPDATE communications SET status='archived',revision=revision+1,updated_at=NOW() WHERE id=$1 AND revision=$2 AND kind IN ('announcement','both') AND status IN ('published','queued')`, id, revision)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return ErrConflict
	}
	return nil
}

type MailDelivery struct {
	ID                        uuid.UUID
	Recipient, Title, Content string
}

func (p *Postgres) ClaimMailDelivery(ctx context.Context) (MailDelivery, error) {
	var d MailDelivery
	// Interrupted SMTP deliveries are uncertain: never automatically resend them.
	_, err := p.pool.Exec(ctx, `UPDATE mail_deliveries SET status='failed',updated_at=NOW() WHERE status='processing' AND updated_at<NOW()-INTERVAL '2 minutes'`)
	if err != nil {
		return d, err
	}
	err = p.pool.QueryRow(ctx, `WITH next AS (SELECT id FROM mail_deliveries WHERE status='pending' ORDER BY updated_at,id FOR UPDATE SKIP LOCKED LIMIT 1), claimed AS (UPDATE mail_deliveries d SET status='processing',updated_at=NOW() FROM next WHERE d.id=next.id RETURNING d.id,d.recipient,d.communication_id) SELECT d.id,d.recipient,c.title,c.content FROM claimed d JOIN communications c ON c.id=d.communication_id`).Scan(&d.ID, &d.Recipient, &d.Title, &d.Content)
	if errors.Is(err, pgx.ErrNoRows) {
		err = ErrNotFound
	}
	return d, err
}

func (p *Postgres) FinishMailDelivery(ctx context.Context, id uuid.UUID, sent bool) error {
	status := "failed"
	if sent {
		status = "sent"
	}
	_, err := p.pool.Exec(ctx, `UPDATE mail_deliveries SET status=$2,updated_at=NOW() WHERE id=$1 AND status='processing'`, id, status)
	return err
}
