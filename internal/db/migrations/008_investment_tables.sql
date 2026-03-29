-- Migration 008: Add investment transaction tables
-- Creates investment_transactions, replaces investment_lots with new schema,
-- adds investment_positions and investment_transaction_lots tables.

-- Step 1: Backup and drop existing investment_lots table
CREATE TEMPORARY TABLE investment_lots_backup_008 AS SELECT * FROM investment_lots;
DROP TABLE investment_lots;

-- Step 2: Create investment_transactions table
CREATE TABLE investment_transactions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id UUID NOT NULL REFERENCES accounts(id),
    date DATE NOT NULL,
    transaction_type TEXT NOT NULL CHECK (transaction_type IN (
        'buy', 'sell', 'dividend', 'reinvest_dividend',
        'fee', 'fee_liquidation', 'deposit', 'withdrawal',
        'interest', 'transfer_shares', 'transfer_cash', 'exchange'
    )),
    security_id UUID REFERENCES securities(id),
    shares DECIMAL(19, 8),
    price_per_share DECIMAL(19, 4),
    total_amount DECIMAL(19, 4) NOT NULL,
    commission DECIMAL(19, 4) DEFAULT 0,
    memo TEXT,
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN (
        'pending', 'cleared', 'reconciled'
    )),
    transfer_id UUID,
    transfer_account_id UUID REFERENCES accounts(id),
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_inv_tx_account ON investment_transactions(account_id);
CREATE INDEX idx_inv_tx_date ON investment_transactions(date);
CREATE INDEX idx_inv_tx_type ON investment_transactions(transaction_type);
CREATE INDEX idx_inv_tx_security ON investment_transactions(security_id);
CREATE INDEX idx_inv_tx_transfer ON investment_transactions(transfer_id);

-- Step 3: Create new investment_lots table with security_id reference
CREATE TABLE investment_lots (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id UUID NOT NULL REFERENCES accounts(id),
    security_id UUID NOT NULL REFERENCES securities(id),
    shares DECIMAL(19, 8) NOT NULL,
    original_shares DECIMAL(19, 8) NOT NULL,
    cost_per_share DECIMAL(19, 4) NOT NULL,
    purchase_date DATE NOT NULL,
    source_transaction_id UUID NOT NULL,
    closed BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_lots_account ON investment_lots(account_id);
CREATE INDEX idx_lots_security ON investment_lots(security_id);
CREATE INDEX idx_lots_closed ON investment_lots(closed);

-- Note: Old investment_lots data (symbol/name-based) is not migrated to the new schema
-- since there is no reliable mapping from symbol to security_id. The old data is dropped.
DROP TABLE investment_lots_backup_008;

-- Step 4: Create investment_positions table
CREATE TABLE investment_positions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id UUID NOT NULL REFERENCES accounts(id),
    security_id UUID NOT NULL REFERENCES securities(id),
    shares DECIMAL(19, 8) NOT NULL DEFAULT 0,
    average_cost_per_share DECIMAL(19, 4) NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (account_id, security_id)
);

CREATE INDEX idx_positions_account ON investment_positions(account_id);
CREATE INDEX idx_positions_security ON investment_positions(security_id);

-- Step 5: Create investment_transaction_lots junction table
CREATE TABLE investment_transaction_lots (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    transaction_id UUID NOT NULL REFERENCES investment_transactions(id),
    lot_id UUID NOT NULL REFERENCES investment_lots(id),
    shares DECIMAL(19, 8) NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_tx_lots_transaction ON investment_transaction_lots(transaction_id);
CREATE INDEX idx_tx_lots_lot ON investment_transaction_lots(lot_id);
