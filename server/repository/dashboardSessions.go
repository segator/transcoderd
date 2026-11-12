package repository

import (
	"context"
	"fmt"
	"time"
)

func (s *SQLRepository) CreateOIDCDashboardSession(ctx context.Context, credentialID, hash [32]byte, expires time.Time, limit int) (bool, error) {
	var created bool
	err := s.WithTransaction(ctx, func(ctx context.Context, tx Repository) error {
		db, err := tx.getConnection()
		if err != nil {
			return err
		}
		// Serialize writers so concurrent logins cannot exceed the session cap.
		if _, err := db.ExecContext(ctx, "LOCK TABLE oidc_dashboard_sessions IN SHARE ROW EXCLUSIVE MODE"); err != nil {
			return fmt.Errorf("lock OIDC dashboard sessions: %w", err)
		}
		if _, err := db.ExecContext(ctx, "DELETE FROM oidc_dashboard_sessions WHERE expires_at <= CURRENT_TIMESTAMP"); err != nil {
			return fmt.Errorf("remove expired OIDC dashboard sessions: %w", err)
		}
		result, err := db.ExecContext(ctx, `INSERT INTO oidc_dashboard_sessions (credential_id, session_hash, expires_at)
			SELECT $1, $2, $3 WHERE (SELECT COUNT(*) FROM oidc_dashboard_sessions WHERE credential_id = $1) < $4`, credentialID[:], hash[:], expires, limit)
		if err != nil {
			return fmt.Errorf("insert OIDC dashboard session: %w", err)
		}
		rows, err := result.RowsAffected()
		created = rows > 0
		return err
	})
	return created, err
}

func (s *SQLRepository) HasOIDCDashboardSession(ctx context.Context, hash [32]byte) (bool, error) {
	db, err := s.getConnection()
	if err != nil {
		return false, err
	}
	var valid bool
	err = db.QueryRowContext(ctx, `SELECT EXISTS (
		SELECT 1 FROM oidc_dashboard_sessions WHERE session_hash = $1 AND expires_at > CURRENT_TIMESTAMP
	)`, hash[:]).Scan(&valid)
	return valid, err
}

func (s *SQLRepository) DeleteOIDCDashboardSession(ctx context.Context, hash [32]byte) error {
	db, err := s.getConnection()
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, "DELETE FROM oidc_dashboard_sessions WHERE session_hash = $1", hash[:])
	return err
}
