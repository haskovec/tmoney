-- Migration 009: Add corporate actions table
-- Tracks stock splits, reverse splits, mergers, and spin-offs for audit
-- and price/lot adjustment purposes. Parameters are stored as JSON.

CREATE TABLE corporate_actions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    action_type TEXT NOT NULL CHECK (action_type IN (
        'split', 'reverse_split', 'merger', 'spin_off'
    )),
    security_id UUID NOT NULL REFERENCES securities(id),
    target_security_id UUID REFERENCES securities(id),
    action_date DATE NOT NULL,
    parameters TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_corp_actions_security ON corporate_actions(security_id);
CREATE INDEX idx_corp_actions_date ON corporate_actions(action_date);
