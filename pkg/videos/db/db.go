// Code generated by sqlc. DO NOT EDIT.
// versions:
//   sqlc v1.26.0

package db

import (
	"context"
	"database/sql"
	"fmt"
)

type DBTX interface {
	ExecContext(context.Context, string, ...interface{}) (sql.Result, error)
	PrepareContext(context.Context, string) (*sql.Stmt, error)
	QueryContext(context.Context, string, ...interface{}) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...interface{}) *sql.Row
}

func New(db DBTX) *Queries {
	return &Queries{db: db}
}

func Prepare(ctx context.Context, db DBTX) (*Queries, error) {
	q := Queries{db: db}
	var err error
	if q.createVideoStmt, err = db.PrepareContext(ctx, createVideo); err != nil {
		return nil, fmt.Errorf("error preparing query CreateVideo: %w", err)
	}
	if q.deleteVideoByIdStmt, err = db.PrepareContext(ctx, deleteVideoById); err != nil {
		return nil, fmt.Errorf("error preparing query DeleteVideoById: %w", err)
	}
	if q.getAllVideosStmt, err = db.PrepareContext(ctx, getAllVideos); err != nil {
		return nil, fmt.Errorf("error preparing query GetAllVideos: %w", err)
	}
	if q.getVideoByIdStmt, err = db.PrepareContext(ctx, getVideoById); err != nil {
		return nil, fmt.Errorf("error preparing query GetVideoById: %w", err)
	}
	if q.updateVideoStmt, err = db.PrepareContext(ctx, updateVideo); err != nil {
		return nil, fmt.Errorf("error preparing query UpdateVideo: %w", err)
	}
	return &q, nil
}

func (q *Queries) Close() error {
	var err error
	if q.createVideoStmt != nil {
		if cerr := q.createVideoStmt.Close(); cerr != nil {
			err = fmt.Errorf("error closing createVideoStmt: %w", cerr)
		}
	}
	if q.deleteVideoByIdStmt != nil {
		if cerr := q.deleteVideoByIdStmt.Close(); cerr != nil {
			err = fmt.Errorf("error closing deleteVideoByIdStmt: %w", cerr)
		}
	}
	if q.getAllVideosStmt != nil {
		if cerr := q.getAllVideosStmt.Close(); cerr != nil {
			err = fmt.Errorf("error closing getAllVideosStmt: %w", cerr)
		}
	}
	if q.getVideoByIdStmt != nil {
		if cerr := q.getVideoByIdStmt.Close(); cerr != nil {
			err = fmt.Errorf("error closing getVideoByIdStmt: %w", cerr)
		}
	}
	if q.updateVideoStmt != nil {
		if cerr := q.updateVideoStmt.Close(); cerr != nil {
			err = fmt.Errorf("error closing updateVideoStmt: %w", cerr)
		}
	}
	return err
}

func (q *Queries) exec(ctx context.Context, stmt *sql.Stmt, query string, args ...interface{}) (sql.Result, error) {
	switch {
	case stmt != nil && q.tx != nil:
		return q.tx.StmtContext(ctx, stmt).ExecContext(ctx, args...)
	case stmt != nil:
		return stmt.ExecContext(ctx, args...)
	default:
		return q.db.ExecContext(ctx, query, args...)
	}
}

func (q *Queries) query(ctx context.Context, stmt *sql.Stmt, query string, args ...interface{}) (*sql.Rows, error) {
	switch {
	case stmt != nil && q.tx != nil:
		return q.tx.StmtContext(ctx, stmt).QueryContext(ctx, args...)
	case stmt != nil:
		return stmt.QueryContext(ctx, args...)
	default:
		return q.db.QueryContext(ctx, query, args...)
	}
}

func (q *Queries) queryRow(ctx context.Context, stmt *sql.Stmt, query string, args ...interface{}) *sql.Row {
	switch {
	case stmt != nil && q.tx != nil:
		return q.tx.StmtContext(ctx, stmt).QueryRowContext(ctx, args...)
	case stmt != nil:
		return stmt.QueryRowContext(ctx, args...)
	default:
		return q.db.QueryRowContext(ctx, query, args...)
	}
}

type Queries struct {
	db                  DBTX
	tx                  *sql.Tx
	createVideoStmt     *sql.Stmt
	deleteVideoByIdStmt *sql.Stmt
	getAllVideosStmt    *sql.Stmt
	getVideoByIdStmt    *sql.Stmt
	updateVideoStmt     *sql.Stmt
}

func (q *Queries) WithTx(tx *sql.Tx) *Queries {
	return &Queries{
		db:                  tx,
		tx:                  tx,
		createVideoStmt:     q.createVideoStmt,
		deleteVideoByIdStmt: q.deleteVideoByIdStmt,
		getAllVideosStmt:    q.getAllVideosStmt,
		getVideoByIdStmt:    q.getVideoByIdStmt,
		updateVideoStmt:     q.updateVideoStmt,
	}
}
