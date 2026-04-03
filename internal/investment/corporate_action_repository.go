package investment

import (
	"database/sql"
	"fmt"

	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/dberrors"
	"github.com/haskovec/tmoney/internal/dbutil"
	"github.com/haskovec/tmoney/internal/types"
)

// CorporateActionRepository provides database operations for corporate actions.
type CorporateActionRepository struct {
	db *db.DB
}

// NewCorporateActionRepository creates a new CorporateActionRepository.
func NewCorporateActionRepository(database *db.DB) *CorporateActionRepository {
	return &CorporateActionRepository{db: database}
}

const corporateActionColumns = `id, action_type, security_id, target_security_id, action_date, parameters, created_at`

// scanCorporateAction scans a row into a CorporateAction.
func scanCorporateAction(row interface{ Scan(...any) error }) (*CorporateAction, error) {
	ca := &CorporateAction{}
	err := row.Scan(
		&ca.ID,
		&ca.ActionType,
		&ca.SecurityID,
		&ca.TargetSecurityID,
		&ca.ActionDate,
		&ca.Parameters,
		&ca.CreatedAt,
	)
	return ca, err
}

// Create inserts a new corporate action into the database.
func (r *CorporateActionRepository) Create(ca *CorporateAction) error {
	query := `
		INSERT INTO corporate_actions (
			id, action_type, security_id, target_security_id, action_date, parameters, created_at
		) VALUES (?, ?, CAST(? AS UUID), ` + dbutil.NullUUIDCast(ca.TargetSecurityID) + `, ?, ?, ?)
	`
	_, err := r.db.Conn().Exec(query,
		ca.ID,
		ca.ActionType.String(),
		ca.SecurityID.String(),
		dbutil.NullID(ca.TargetSecurityID),
		ca.ActionDate.Time(),
		ca.Parameters,
		ca.CreatedAt.Time(),
	)
	if err != nil {
		return fmt.Errorf("failed to create corporate action: %w", err)
	}
	return nil
}

// GetByID retrieves a corporate action by its ID.
func (r *CorporateActionRepository) GetByID(id types.ID) (*CorporateAction, error) {
	query := `SELECT ` + corporateActionColumns + ` FROM corporate_actions WHERE CAST(id AS VARCHAR) = ?`
	ca, err := scanCorporateAction(r.db.Conn().QueryRow(query, id.String()))
	if err == sql.ErrNoRows {
		return nil, &dberrors.NotFoundError{Entity: "corporate_action", ID: id.String()}
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get corporate action: %w", err)
	}
	return ca, nil
}

// ListBySecurity retrieves all corporate actions for a security, including actions
// where the security is the source or the target. Results are ordered by action_date ASC.
func (r *CorporateActionRepository) ListBySecurity(securityID types.ID) ([]*CorporateAction, error) {
	query := `
		SELECT ` + corporateActionColumns + `
		FROM corporate_actions
		WHERE CAST(security_id AS VARCHAR) = ? OR CAST(target_security_id AS VARCHAR) = ?
		ORDER BY action_date ASC, created_at ASC
	`
	rows, err := r.db.Conn().Query(query, securityID.String(), securityID.String())
	if err != nil {
		return nil, fmt.Errorf("failed to list corporate actions by security: %w", err)
	}
	defer rows.Close()

	actions := make([]*CorporateAction, 0)
	for rows.Next() {
		ca, err := scanCorporateAction(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan corporate action: %w", err)
		}
		actions = append(actions, ca)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating corporate actions: %w", err)
	}
	return actions, nil
}
