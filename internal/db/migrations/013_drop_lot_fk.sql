-- Migration 013: Drop FK from investment_transaction_lots.lot_id to investment_lots.id
--
-- DuckDB enforces foreign keys on UPDATE of the parent row even when the
-- referenced (primary-key) column is not being modified. That blocks the
-- legitimate "reduce a lot's remaining shares" update done by Sell — and
-- in particular blocks reversing a sell during an Edit. Application-level
-- code already maintains the relationship (junction rows are inserted and
-- deleted together with their parent transaction), so we drop the FK.

CREATE TEMPORARY TABLE investment_transaction_lots_backup AS SELECT * FROM investment_transaction_lots;
DROP TABLE investment_transaction_lots;

CREATE TABLE investment_transaction_lots (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    transaction_id UUID NOT NULL,
    lot_id UUID NOT NULL,
    shares DECIMAL(19, 8) NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO investment_transaction_lots SELECT * FROM investment_transaction_lots_backup;
DROP TABLE investment_transaction_lots_backup;

CREATE INDEX idx_tx_lots_transaction ON investment_transaction_lots(transaction_id);
CREATE INDEX idx_tx_lots_lot ON investment_transaction_lots(lot_id);
