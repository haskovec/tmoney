-- Migration 011: Create portfolio_holdings database view
-- Provides aggregated holdings per account+security, supporting both
-- lot-tracking accounts (summing from investment_lots) and non-lot-tracking
-- accounts (reading from investment_positions).

CREATE VIEW portfolio_holdings AS
SELECT
    a.id AS account_id,
    a.name AS account_name,
    s.id AS security_id,
    s.ticker,
    s.name AS security_name,
    CASE
        WHEN a.track_lots THEN
            (SELECT COALESCE(SUM(l.shares), 0) FROM investment_lots l
             WHERE l.account_id = a.id AND l.security_id = s.id AND NOT l.closed)
        ELSE
            (SELECT COALESCE(p.shares, 0) FROM investment_positions p
             WHERE p.account_id = a.id AND p.security_id = s.id)
    END AS total_shares,
    CASE
        WHEN a.track_lots THEN
            (SELECT COALESCE(SUM(l.shares * l.cost_per_share), 0) FROM investment_lots l
             WHERE l.account_id = a.id AND l.security_id = s.id AND NOT l.closed)
        ELSE
            (SELECT COALESCE(p.shares * p.average_cost_per_share, 0) FROM investment_positions p
             WHERE p.account_id = a.id AND p.security_id = s.id)
    END AS total_cost_basis
FROM accounts a
CROSS JOIN securities s
WHERE a.type = 'investment' AND a.active = TRUE;
