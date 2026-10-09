# Backfilling grounded concept-link candidates for one owner

`services/selar-worker/backfill_candidates.py` re-runs the ingestion linking logic
(`candidate_generation.backfill_owner`) over every pair of one owner's latest ready
reading models. It is insert-only and idempotent:

- adds up to `MAX_LINKS_PER_PAIR` (5) grounded `concept_overlap` candidates per pair,
  counting existing active links (candidate/confirmed/relabeled) toward the cap;
- never re-suggests a concept that already has a link of any status for that pair
  (including rejected); never updates or deletes rows; makes no model calls;
- every row still passes the `grounded_mental_link_write` trigger (migration 021).

Run (requires migration 021 applied; always dry-run first):

```bash
cd services/selar-worker
DATABASE_URL=postgres://... python backfill_candidates.py <owner-uuid> --dry-run
DATABASE_URL=postgres://... python backfill_candidates.py <owner-uuid>
```

`--dry-run` prints the candidates it would insert and rolls the transaction back.
Do not run against production without the owner's explicit approval.
