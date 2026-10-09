"""Synthetic real API/queue/worker/PG acceptance. No provider/model calls."""
import asyncio
import hashlib
import json
import os
from pathlib import Path
import subprocess
import uuid
import asyncpg
import httpx
import pytest
from test_synthetic_service_e2e import _port, _ready, _request, fabricated_pdf, PASSWORD
from test_file_formats import docx_bytes


@pytest.mark.timeout(120)
def test_cross_format_source_snapshot_lifecycle(tmp_path, monkeypatch):
    db = os.getenv('TEST_DATABASE_URL', '')
    binary = os.getenv('TEST_API_BINARY', '')
    if not db or not binary:
        pytest.skip('requires isolated TEST_DATABASE_URL and built TEST_API_BINARY')
    assert 'selar_e2e' in db and ':55439/' not in db
    import main
    import ingestion
    from storage import LocalStorage
    monkeypatch.setattr(main, 'DATABASE_URL', db)
    objects = LocalStorage(str(tmp_path))
    monkeypatch.setattr(main, 'get_storage', lambda: objects)
    monkeypatch.setattr(ingestion, 'get_storage', lambda: objects)
    monkeypatch.setattr(main, 'embed_text_documents', lambda contents, title='': [[1.0]+[0.0]*3071 for _ in contents])
    class Models:
        def generate_content(self, **kwargs):
            return type('Result', (), {'text':json.dumps({'main_claim':'Synthetic evidence only.', 'key_concepts':[{'name':'fabricated evidence','description':'Synthetic test phrase','evidence_chunk_index':0}], 'assumptions':[], 'open_questions':[], 'domain':'synthetic', 'concept_edges':[]})})()
    monkeypatch.setattr(main, 'client', type('Client', (), {'models':Models()})())
    port = _port(); api = f'http://127.0.0.1:{port}'
    process = subprocess.Popen([binary], cwd=tmp_path, env={**os.environ, 'DATABASE_URL':db,'PORT':str(port),'LOCAL_STORAGE_DIR':str(tmp_path),'ANALYTICS_ENABLED':'false','STORAGE_BACKEND':'local','JWT_SECRET':'synthetic-format-test-secret','APP_ENV':'development','WORKER_URL':'http://127.0.0.1:1'})
    owners = []
    async def drain(doc_id, digest):
        job = await main.claim_ingestion_job()
        assert job and str(job['document_id']) == doc_id
        await main.run_claimed_ingestion_job(job)
        conn = await asyncpg.connect(db)
        try:
            row = await conn.fetchrow('SELECT status,content_hash,metadata FROM documents WHERE id=$1',doc_id)
            assert row['status'] == 'ready', dict(row)
            assert row['content_hash'] == digest
            assert await conn.fetchval('SELECT status FROM ingestion_jobs WHERE document_id=$1',doc_id) == 'completed'
            chunks = await conn.fetch('SELECT id,content,locator FROM chunks WHERE document_id=$1 ORDER BY chunk_index',doc_id)
            blocks = await conn.fetch('SELECT block_index,text FROM content_blocks WHERE document_id=$1',doc_id)
            by_index = {b['block_index']:b['text'] for b in blocks}
            for chunk in chunks:
                locator = json.loads(chunk['locator'])
                # Exact normalized whitespace witness; no invented lexical joins.
                assert chunk['content'] in ' '.join(by_index[locator['block_index']].split())
            assert chunks
            return str(chunks[0]['id'])
        finally:
            await conn.close()
    try:
        _ready(api+'/healthz',process)
        with httpx.Client() as client:
            for label in ['owner','foreign']:
                owners.append(_request(client,'POST',api+'/auth/register',expected=201,json={'email':f'{label}-{uuid.uuid4().hex}@example.invalid','password':PASSWORD}))
            token, foreign = [o['token'] for o in owners]
            line = 'co- operation remains distinct in fabricated evidence.'
            fixtures = [('notes.md', ('# Heading\n\n'+line).encode()),('notes.txt',line.encode()),('notes.docx',docx_bytes('<w:p><w:r><w:t>'+line+'</w:t></w:r></w:p>')),('paper.pdf',fabricated_pdf(line))]
            prior_data = b'Prior fabricated evidence remains an exact source witness.'
            prior = _request(client,'POST',api+'/api/documents/upload',token,expected=202,files={'file':('prior.txt',prior_data)})['document']['id']
            asyncio.run(drain(prior,hashlib.sha256(prior_data).hexdigest()))
            async def contract(doc):
                from candidate_contract import persist_grounded_overlap
                conn = await asyncpg.connect(db)
                try:
                    async def chunk(document):
                        return dict(await conn.fetchrow('SELECT c.*,d.title,d.content_hash FROM chunks c JOIN documents d ON d.id=c.document_id WHERE c.document_id=$1 ORDER BY c.chunk_index LIMIT 1',document))
                    source,target = await chunk(doc),await chunk(prior)
                    source_model = await conn.fetchval('SELECT id FROM document_mental_models WHERE document_id=$1 ORDER BY version DESC LIMIT 1',doc)
                    target_model = await conn.fetchval('SELECT id FROM document_mental_models WHERE document_id=$1 ORDER BY version DESC LIMIT 1',prior)
                    await persist_grounded_overlap(conn,{'key_concepts':[{'name':'fabricated evidence'}]},source,target,source['user_id'],doc,prior,source_model,target_model,.9)
                    link = await conn.fetchrow('SELECT ml.*,valid_grounded_mental_link(ml) AS valid FROM mental_model_links ml WHERE source_model_id=$1 AND target_model_id=$2',source_model,target_model)
                    assert link['valid']
                    witness = json.loads(link['source_evidence'])
                    assert witness['source_snapshot_hash'] == source['content_hash']
                    assert witness['locator'] == json.loads(source['locator'])
                    assert witness['quote'] in source['content']
                    with pytest.raises(asyncpg.PostgresError):
                        async with conn.transaction():
                            await conn.execute('UPDATE mental_model_links SET source_evidence=$2 WHERE id=$1',link['id'],json.dumps({**witness,'quote':'fabricated invented quote'}))
                    await conn.execute("UPDATE documents SET content_hash='changed-snapshot' WHERE id=$1",doc)
                    assert not await conn.fetchval('SELECT valid_grounded_mental_link(ml) FROM mental_model_links ml WHERE id=$1',link['id'])
                    await conn.execute('UPDATE documents SET content_hash=$2 WHERE id=$1',doc,source['content_hash'])
                finally: await conn.close()
            for name,data in fixtures:
                result = _request(client,'POST',api+'/api/documents/upload',token,expected=202,files={'file':(name,data)})
                doc = result['document']['id']
                async def seed_snapshot_metadata():
                    conn = await asyncpg.connect(db)
                    try:
                        await conn.execute("UPDATE documents SET metadata=$2 WHERE id=$1",doc,json.dumps({'snapshot':True,'original_sha256':hashlib.sha256(data).hexdigest(),'provider_file_id':'synthetic-provider-id'}))
                    finally: await conn.close()
                asyncio.run(seed_snapshot_metadata())
                old_chunk = asyncio.run(drain(doc,hashlib.sha256(data).hexdigest()))
                asyncio.run(contract(doc))
                content = _request(client,'GET',api+f'/api/documents/{doc}/content',token)
                assert content['document']['metadata']['provider_file_id'] == 'synthetic-provider-id'
                assert content['document']['content_hash'] == hashlib.sha256(data).hexdigest()
                _request(client,'GET',api+f'/api/documents/{doc}/content',foreign,expected=404)
                _request(client,'DELETE',api+f'/api/documents/{doc}',foreign,expected=404)
                async def reingest():
                    conn = await asyncpg.connect(db)
                    try:
                        job = await conn.fetchrow('SELECT * FROM ingestion_jobs WHERE document_id=$1',doc)
                    finally: await conn.close()
                    await main.process_document_task(doc, file_path=job['file_path'],source_id=str(job['source_id']),run_id=str(job['run_id']),source_type=job['source_type'],title=job['title'])
                    conn = await asyncpg.connect(db)
                    try:
                        assert not await conn.fetchval('SELECT EXISTS(SELECT 1 FROM chunks WHERE id=$1)',old_chunk)
                        assert await conn.fetchval('SELECT content_hash FROM documents WHERE id=$1',doc) == hashlib.sha256(data).hexdigest()
                    finally: await conn.close()
                asyncio.run(reingest())
                _request(client,'DELETE',api+f'/api/documents/{doc}',token)
                async def absent():
                    conn = await asyncpg.connect(db)
                    try:
                        assert not await conn.fetchval('SELECT EXISTS(SELECT 1 FROM chunks WHERE id=$1)',old_chunk)
                        assert not await conn.fetchval('SELECT EXISTS(SELECT 1 FROM ingestion_jobs WHERE document_id=$1)',doc)
                    finally: await conn.close()
                asyncio.run(absent())
            result = _request(client,'POST',api+'/api/documents/upload',token,expected=202,files={'file':('tampered.txt',b'Original immutable witness.')})
            tampered_doc = result['document']['id']
            async def tampered():
                conn = await asyncpg.connect(db)
                try:
                    job = await conn.fetchrow('SELECT * FROM ingestion_jobs WHERE document_id=$1',tampered_doc)
                    Path(job['file_path']).write_bytes(b'Changed witness must never replace the original.')
                    await conn.execute('UPDATE documents SET metadata=$2 WHERE id=$1',tampered_doc,json.dumps({'original_sha256':hashlib.sha256(b'Original immutable witness.').hexdigest()}))
                    await conn.execute('UPDATE ingestion_jobs SET max_attempts=1 WHERE document_id=$1',tampered_doc)
                finally: await conn.close()
                job = await main.claim_ingestion_job()
                await main.run_claimed_ingestion_job(job)
                conn = await asyncpg.connect(db)
                try:
                    assert await conn.fetchval('SELECT status FROM ingestion_jobs WHERE document_id=$1',tampered_doc) == 'failed'
                    assert not await conn.fetchval('SELECT EXISTS(SELECT 1 FROM chunks WHERE document_id=$1)',tampered_doc)
                finally: await conn.close()
            asyncio.run(tampered())
    finally:
        process.terminate(); process.wait(timeout=10)
        async def cleanup():
            conn = await asyncpg.connect(db)
            try:
                for owner in owners: await conn.execute('DELETE FROM users WHERE id=$1',owner['user']['id'])
            finally: await conn.close()
        asyncio.run(cleanup())
