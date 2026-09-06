package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

type LegalDocument struct {
	Slug        string    `json:"slug"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Content     string    `json:"content"`
	Revision    int       `json:"revision"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type LegalStore interface {
	LegalDocument(context.Context, string) (LegalDocument, error)
	UpdateLegalDocument(context.Context, LegalDocument) (LegalDocument, error)
}

func scanLegalDocument(row pgx.Row) (LegalDocument, error) {
	var document LegalDocument
	err := row.Scan(&document.Slug, &document.Title, &document.Description, &document.Content, &document.Revision, &document.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return document, ErrNotFound
	}
	return document, err
}

func (p *Postgres) LegalDocument(ctx context.Context, slug string) (LegalDocument, error) {
	return scanLegalDocument(p.pool.QueryRow(ctx, `SELECT slug,title,description,content,revision,updated_at FROM legal_documents WHERE slug=$1`, slug))
}

func (p *Postgres) UpdateLegalDocument(ctx context.Context, document LegalDocument) (LegalDocument, error) {
	updated, err := scanLegalDocument(p.pool.QueryRow(ctx, `UPDATE legal_documents SET title=$2,description=$3,content=$4,revision=revision+1,updated_at=NOW()
	 WHERE slug=$1 AND revision=$5 RETURNING slug,title,description,content,revision,updated_at`, document.Slug, document.Title, document.Description, document.Content, document.Revision))
	if errors.Is(err, ErrNotFound) {
		return LegalDocument{}, ErrConflict
	}
	return updated, err
}
