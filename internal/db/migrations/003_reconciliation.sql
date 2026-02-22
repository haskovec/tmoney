-- Migration 003: Reconciliation sessions
-- Adds reconciliation_sessions table for tracking account reconciliation.

CREATE TABLE reconciliation_sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id UUID NOT NULL REFERENCES accounts(id),
    statement_date DATE NOT NULL,
    statement_balance DECIMAL(19, 4) NOT NULL,
    status TEXT NOT NULL DEFAULT 'in_progress' CHECK (status IN (
        'in_progress', 'completed'
    )),
    completed_at TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_reconciliation_sessions_account ON reconciliation_sessions(account_id);
CREATE INDEX idx_reconciliation_sessions_status ON reconciliation_sessions(status);
