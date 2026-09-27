-- Migration 035: Invert the stored merger exchange ratio.
--
-- Work item W3a in specs/work-two-ledgers.md. The CLI flag and the TUI field
-- have always described exchange_ratio as target shares received per source
-- share, but the service applied it the other way round: 100 source shares at
-- ratio 2 became 50 target shares. The service now uses the documented
-- meaning, so each merger row already on file is rewritten to the ratio that
-- describes, in the new meaning, the event that was actually applied.
--
-- Merger reversal is not supported, so the stored ratio feeds only the history
-- text (TUI and `tmoney investment actions`). A non-terminating inverse such
-- as 1/3 therefore costs no more than a rounded label. json_merge_patch
-- replaces exchange_ratio and keeps every other key (cash_per_share) as it is.
-- A ratio that is not positive cannot be inverted; Validate never stored one,
-- and such a row is left untouched rather than failing the migration.

UPDATE corporate_actions
SET parameters = CAST(json_merge_patch(
        parameters,
        json_object('exchange_ratio',
            1.0 / CAST(json_extract(parameters, '$.exchange_ratio') AS DOUBLE))
    ) AS VARCHAR)
WHERE action_type = 'merger'
  AND CAST(json_extract(parameters, '$.exchange_ratio') AS DOUBLE) > 0;
