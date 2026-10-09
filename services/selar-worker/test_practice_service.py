import asyncio
import importlib
import json
import os
import uuid
import asyncpg
import pytest

DB = os.getenv('TEST_DATABASE_URL')
pytestmark = pytest.mark.skipif(not DB, reason='requires isolated migrated pgvector')

async def fixture(conn):
    owner, other, doc, chunk = [uuid.uuid4() for _ in range(4)]
    for u in (owner, other):
        await conn.execute("INSERT INTO users(id,email,password_hash) VALUES($1,$2,'synthetic')", u, f'{u}@example.invalid')
    await conn.execute("INSERT INTO documents(id,user_id,title,status,content_hash) VALUES($1,$2,'Synthetic','ready','snapshot')", doc, owner)
    await conn.execute("INSERT INTO chunks(id,user_id,document_id,chunk_index,content,locator) VALUES($1,$2,$3,0,'Descent reduces error.','{\"page\":1}')", chunk, owner, doc)
    return owner, other, doc, chunk

async def model(task, data):
    if task == 'generate':
        return {'question': 'What reduces error?', 'answer': 'Descent', 'quote': 'Descent reduces error.'}
    if task == 'verify':
        return {'supported': True}
    return {'score': .5, 'confident': True, 'feedback': 'Partially correct'}


def test_owner_private_idempotent_generation_and_live_source_validation():
    async def run():
        p = importlib.import_module('practice_service')
        conn = await asyncpg.connect(DB)
        owner = other = None
        try:
            owner, other, doc, chunk = await fixture(conn)
            assert (await p.generate(conn, other, doc, model))['status'] == 'not_found'
            assert (await p.generate(conn, owner, doc, model))['status'] == 'ready'
            assert (await p.generate(conn, owner, doc, model))['status'] == 'ready'
            items = await p.items(conn, owner, doc)
            assert len(items) == 1 and 'answer' not in items[0] and 'quote' not in items[0]
            assert await p.items(conn, other, doc) == []
            await conn.execute("UPDATE chunks SET content='Changed source' WHERE id=$1", chunk)
            assert await p.items(conn, owner, doc) == []
        finally:
            if owner: await conn.execute('DELETE FROM users WHERE id=ANY($1::uuid[])', [owner, other])
            await conn.close()
    asyncio.run(run())


def test_attempt_is_private_atomic_idempotent_and_unavailable_unscored():
    async def run():
        p = importlib.import_module('practice_service')
        conn = await asyncpg.connect(DB)
        owner, other, doc, chunk = await fixture(conn)
        try:
            generated = await p.generate(conn, owner, doc, model)
            item = generated['items'][0]['id']
            request = {'item_id': item, 'request_key': str(uuid.uuid4()), 'response': 'Descent', 'phase': 'warmup', 'exposed': False}
            assert (await p.attempt(conn, other, request, model))['status'] == 'not_found'
            result = await p.attempt(conn, owner, request, model)
            assert result['feedback']['score'] == .5 and result['quote'] == 'Descent reduces error.'
            assert await p.attempt(conn, owner, request, model) == result
            assert (await p.attempt(conn, owner, {**request, 'response': 'changed'}, model))['status'] == 'conflict'
            assert await conn.fetchval('SELECT count(*) FROM practice_attempts WHERE user_id=$1', owner) == 1
        finally:
            await conn.execute('DELETE FROM users WHERE id=ANY($1::uuid[])', [owner, other])
            await conn.close()
    asyncio.run(run())
