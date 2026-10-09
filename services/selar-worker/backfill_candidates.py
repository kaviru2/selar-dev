"""Insert-only grounded-candidate backfill for ONE owner (issue #117).

Usage: DATABASE_URL=... python backfill_candidates.py <owner-uuid> [--dry-run]

Runs the same exact two-sided witness contract as ingestion over every pair of
the owner's latest ready reading models, in both directions. It makes no model
calls, never updates or deletes a link, adds at most MAX_LINKS_PER_PAIR links per
pair, never re-suggests a concept already linked (including a learner rejection), and every insert still passes the database
``grounded_mental_link_write`` trigger. ``--dry-run`` rolls the transaction back.
"""
import asyncio
import os
import sys
import uuid

import asyncpg

from candidate_generation import backfill_owner


async def run(owner: str, dry_run: bool) -> list:
    conn = await asyncpg.connect(os.environ["DATABASE_URL"])
    try:
        tx = conn.transaction()
        await tx.start()
        try:
            created = await backfill_owner(conn, uuid.UUID(owner))
        except BaseException:
            await tx.rollback()
            raise
        if dry_run:
            await tx.rollback()
        else:
            await tx.commit()
        return created
    finally:
        await conn.close()


def main(argv) -> int:
    if len(argv) < 2:
        print(__doc__)
        return 2
    owner, dry_run = argv[1], "--dry-run" in argv[2:]
    uuid.UUID(owner)  # refuse anything but an explicit owner id
    created = asyncio.run(run(owner, dry_run))
    for source_document, concept in created:
        print(f"{'would insert' if dry_run else 'inserted'} candidate: source={source_document} concept={concept!r}")
    print(f"{len(created)} candidate(s) {'(dry run, rolled back)' if dry_run else 'committed'}")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
