-- Migration 036: Delete reconciliation sessions on investment accounts.
--
-- Work item W6 in specs/work-two-ledgers.md. Reconcile compares a statement
-- with the register ledger, and an investment account keeps its cash and
-- shares on the investment ledger, so a session on one never matched anything.
-- From this version StartReconciliation refuses such accounts. An in-progress
-- session left on one would then have no way to be cancelled (cancel lives in
-- the reconcile view, which Start opens) and would block the account's delete.
-- Completed sessions there describe nothing and go too.

DELETE FROM reconciliation_sessions
WHERE account_id IN (
    SELECT id FROM accounts WHERE type IN ('investment', 'hsa_investment')
);
