-- Migration 010: Remove FK constraints on security_id columns
-- DuckDB implements UPDATE as DELETE+INSERT internally, which fails when
-- child rows reference the parent via FK constraints. This prevents updating
-- any column on a securities row that has prices, lots, positions, or transactions.
-- Application-level validation already enforces referential integrity.

-- Step 1: Recreate security_prices without FK on security_id
CREATE TEMPORARY TABLE security_prices_backup AS SELECT * FROM security_prices;
DROP TABLE security_prices;

CREATE TABLE security_prices (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    security_id UUID NOT NULL,
    date DATE NOT NULL,
    price DECIMAL(19, 4) NOT NULL CHECK (price > 0),
    source TEXT NOT NULL CHECK (source IN (
        'manual', 'transaction', 'import', 'api'
    )),
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO security_prices SELECT * FROM security_prices_backup;
DROP TABLE security_prices_backup;

CREATE UNIQUE INDEX idx_security_prices_security_date ON security_prices(security_id, date);
CREATE INDEX idx_security_prices_security ON security_prices(security_id);
CREATE INDEX idx_security_prices_date ON security_prices(date);

-- Step 2: Recreate investment_transactions without FK on security_id
CREATE TEMPORARY TABLE investment_transactions_backup AS SELECT * FROM investment_transactions;
DROP TABLE investment_transaction_lots;
DROP TABLE investment_transactions;

CREATE TABLE investment_transactions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id UUID NOT NULL REFERENCES accounts(id),
    date DATE NOT NULL,
    transaction_type TEXT NOT NULL CHECK (transaction_type IN (
        'buy', 'sell', 'dividend', 'reinvest_dividend',
        'fee', 'fee_liquidation', 'deposit', 'withdrawal',
        'interest', 'transfer_shares', 'transfer_cash', 'exchange'
    )),
    security_id UUID,
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

INSERT INTO investment_transactions SELECT * FROM investment_transactions_backup;
DROP TABLE investment_transactions_backup;

CREATE INDEX idx_inv_tx_account ON investment_transactions(account_id);
CREATE INDEX idx_inv_tx_date ON investment_transactions(date);
CREATE INDEX idx_inv_tx_type ON investment_transactions(transaction_type);
CREATE INDEX idx_inv_tx_security ON investment_transactions(security_id);
CREATE INDEX idx_inv_tx_transfer ON investment_transactions(transfer_id);

-- Step 3: Recreate investment_lots without FK on security_id
CREATE TEMPORARY TABLE investment_lots_backup AS SELECT * FROM investment_lots;
DROP TABLE investment_lots;

CREATE TABLE investment_lots (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id UUID NOT NULL REFERENCES accounts(id),
    security_id UUID NOT NULL,
    shares DECIMAL(19, 8) NOT NULL,
    original_shares DECIMAL(19, 8) NOT NULL,
    cost_per_share DECIMAL(19, 4) NOT NULL,
    purchase_date DATE NOT NULL,
    source_transaction_id UUID NOT NULL,
    closed BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO investment_lots SELECT * FROM investment_lots_backup;
DROP TABLE investment_lots_backup;

CREATE INDEX idx_lots_account ON investment_lots(account_id);
CREATE INDEX idx_lots_security ON investment_lots(security_id);
CREATE INDEX idx_lots_closed ON investment_lots(closed);

-- Step 4: Recreate investment_transaction_lots (depends on investment_transactions and investment_lots)
CREATE TABLE investment_transaction_lots (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    transaction_id UUID NOT NULL REFERENCES investment_transactions(id),
    lot_id UUID NOT NULL REFERENCES investment_lots(id),
    shares DECIMAL(19, 8) NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_tx_lots_transaction ON investment_transaction_lots(transaction_id);
CREATE INDEX idx_tx_lots_lot ON investment_transaction_lots(lot_id);

-- Step 5: Recreate investment_positions without FK on security_id
CREATE TEMPORARY TABLE investment_positions_backup AS SELECT * FROM investment_positions;
DROP TABLE investment_positions;

CREATE TABLE investment_positions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id UUID NOT NULL REFERENCES accounts(id),
    security_id UUID NOT NULL,
    shares DECIMAL(19, 8) NOT NULL DEFAULT 0,
    average_cost_per_share DECIMAL(19, 4) NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (account_id, security_id)
);

INSERT INTO investment_positions SELECT * FROM investment_positions_backup;
DROP TABLE investment_positions_backup;

CREATE INDEX idx_positions_account ON investment_positions(account_id);
CREATE INDEX idx_positions_security ON investment_positions(security_id);

-- Step 6: Recreate corporate_actions without FK on security_id/target_security_id
CREATE TEMPORARY TABLE corporate_actions_backup AS SELECT * FROM corporate_actions;
DROP TABLE corporate_actions;

CREATE TABLE corporate_actions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    action_type TEXT NOT NULL CHECK (action_type IN (
        'split', 'reverse_split', 'merger', 'spin_off'
    )),
    security_id UUID NOT NULL,
    target_security_id UUID,
    action_date DATE NOT NULL,
    parameters TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO corporate_actions SELECT * FROM corporate_actions_backup;
DROP TABLE corporate_actions_backup;

CREATE INDEX idx_corporate_actions_security ON corporate_actions(security_id);
CREATE INDEX idx_corporate_actions_target ON corporate_actions(target_security_id);
CREATE INDEX idx_corporate_actions_date ON corporate_actions(action_date);
