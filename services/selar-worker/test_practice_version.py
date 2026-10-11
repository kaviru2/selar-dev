"""Uses only TEST_DATABASE_URL: isolated, synthetic PostgreSQL fixtures."""
import asyncio
import json
from test_practice_service import DB, fixture, model, pytestmark
import asyncpg
import practice_service as service
from practice import VERSION


def test_old_version_is_hidden_and_unattempted_item_can_be_regenerated():
    async def run():
        conn = await asyncpg.connect(DB)
        owner, other, doc, chunk = await fixture(conn)
        try:
            item = (await service.generate(conn, owner, doc, model))['items'][0]['id']
            await conn.execute("UPDATE practice_items SET payload=jsonb_set(payload,'{version}','\"old-version\"') WHERE id=$1", item)
            assert await service.items(conn, owner, doc) == []
            refreshed = await service.generate(conn, owner, doc, model)
            assert refreshed['status'] == 'ready'
            assert refreshed['items'][0]['version'] == VERSION
            assert await conn.fetchval('SELECT count(*) FROM practice_items WHERE user_id=$1', owner) == 1
        finally:
            await conn.execute('DELETE FROM users WHERE id=ANY($1::uuid[])', [owner, other])
            await conn.close()
    asyncio.run(run())


def test_retired_attempted_item_does_not_rewrite_history_or_schedule():
    async def run():
        conn = await asyncpg.connect(DB)
        owner, other, doc, chunk = await fixture(conn)
        try:
            item = (await service.generate(conn, owner, doc, model))['items'][0]['id']
            await service.attempt(conn, owner, {'item_id':item,'request_key':'00000000-0000-0000-0000-000000000001','phase':'reading_check','response':'Descent'}, model)
            await conn.execute("UPDATE practice_items SET payload=jsonb_set(payload,'{version}','\"old-version\"') WHERE id=$1", item)
            before = await conn.fetchrow('SELECT payload FROM practice_items WHERE id=$1', item)
            await conn.execute("UPDATE practice_schedule SET due_at=now()-interval '1 day' WHERE item_id=$1", item)
            assert (await service.daily(conn, owner))['items'] == []
            assert (await service.progress(conn, owner))['reading_check'] == 0
            result = await service.generate(conn, owner, doc, model)
            assert result['status'] == 'unavailable' and result['items'] == []
            assert await conn.fetchrow('SELECT payload FROM practice_items WHERE id=$1', item) == before
            assert await conn.fetchval('SELECT count(*) FROM practice_attempts WHERE item_id=$1', item) == 1
            assert await conn.fetchval('SELECT count(*) FROM practice_schedule WHERE item_id=$1', item) == 1
        finally:
            await conn.execute('DELETE FROM users WHERE id=ANY($1::uuid[])', [owner, other])
            await conn.close()
    asyncio.run(run())
