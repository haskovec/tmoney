-- Migration 024: Backfill original_shares on forward-split-mangled lots.
--
-- Older binaries' stock-split engine scaled investment_lots.shares (and
-- cost_per_share) by the split ratio but left original_shares untouched. For a
-- forward split that leaves an open lot with shares > original_shares — a state
-- that can never arise normally, since shares only ever decrease from
-- original_shares as sells consume the lot. The drift has two bad effects:
--
--   1. The buy/reinvest edit guard (shares != original_shares => "lot has
--      already been sold against") mis-fires on these lots, wrongly blocking
--      edits of an unsold lot.
--   2. It breaks the invariant remaining = original_shares - consumed that the
--      lot rebuild relies on.
--
-- The current engine scales original_shares in lock-step, so new splits are
-- clean; this restores the post-split original on lots mangled by the old one.
-- The correct post-split original is current shares plus the shares already
-- consumed by later sells (junction records), both in post-split terms.
--
-- Targeting is deliberately narrow: only lots with shares > original_shares.
-- That is the unique signature of the forward-split bug and never matches a
-- normal lot or an unrelated closed-lot anomaly (e.g. a fully-closed lot whose
-- original_shares was zeroed by some other path), so this migration cannot
-- corrupt original_shares on lots it does not understand.

-- Forward-split-mangled lots that have been partially sold: original is the
-- still-open shares plus everything consumed by junctions.
UPDATE investment_lots AS l
SET original_shares = l.shares + j.consumed
FROM (
    SELECT lot_id, SUM(shares) AS consumed
    FROM investment_transaction_lots
    GROUP BY lot_id
) j
WHERE l.id = j.lot_id
  AND l.shares > l.original_shares;

-- Forward-split-mangled lots with no sells yet (no junction rows): original is
-- simply the current (split-scaled) shares.
UPDATE investment_lots
SET original_shares = shares
WHERE shares > original_shares
  AND id NOT IN (SELECT lot_id FROM investment_transaction_lots);
