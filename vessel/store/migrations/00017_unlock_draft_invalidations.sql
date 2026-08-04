-- +goose Up

-- Cascade revalidation used to invalidate reports that were still being
-- drafted (2026-08-01). A draft is incomplete by definition, so the
-- moment its ROB or time figures didn't yet reconcile with the previous
-- report — which is most of the time you are typing one — pressing Save
-- draft or Check report flipped it to `invalidated`, and every save
-- after that came back 409 "report is invalidated and locked; start a
-- correction to edit it". The officer was locked out of their own
-- unfinished report, and the only escape (a correction) minted a new
-- version of something that had never been submitted.
--
-- Cascade now runs over committed versions only (domain.State.InChain),
-- and continuity breaks reach a draft through the health check instead.
-- This releases the reports that were already caught: invalidated_from
-- records the state the report held before cascade took it, so a draft
-- or ready one goes straight back where it was, with no trace of the
-- invalidation left to keep locking it.
--
-- Rows with an empty invalidated_from predate that column being
-- populated and are left alone — there is no way to know what state to
-- return them to, and guessing would be worse than leaving an old
-- report visibly flagged.
UPDATE reports
SET state             = invalidated_from,
    invalidated_from  = '',
    invalidated_rules = 'null'
WHERE state = 'invalidated'
  AND invalidated_from IN ('draft', 'ready');

-- +goose Down

-- Irreversible by design: the pre-repair rows recorded a state
-- ('invalidated') that the application no longer produces for a draft,
-- so restoring them would re-lock reports the officer can now edit. The
-- audit trail keeps the original `invalidated` events either way, so
-- nothing about what happened is lost.
SELECT 1;
