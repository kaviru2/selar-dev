"""Database boundary for private, source-bound practice; no formal instruments."""
import json
from practice import Unavailable, digest, generate_item, validated_feedback

async def attempt(conn, owner, request, model):
    fingerprint = digest(json.dumps(request, sort_keys=True))
    async with conn.transaction():
        await conn.execute('SELECT pg_advisory_xact_lock(hashtextextended($1,0))', f'practice-attempt:{owner}:{request["request_key"]}')
        prior = await conn.fetchrow('SELECT request_hash,feedback FROM practice_attempts WHERE user_id=$1 AND request_key=$2', owner, request['request_key'])
        if prior:
            return json.loads(prior['feedback']) if prior['request_hash'] == fingerprint else {'status': 'conflict'}
        row = await conn.fetchrow('SELECT p.payload ' + LIVE + ' AND p.id=$2 FOR UPDATE OF p,d,c', owner, request['item_id'])
        if not row:
            return {'status': 'not_found'}
        payload = json.loads(row['payload'])
        try:
            raw = await model('grade', {'question': payload['question'], 'answer': payload['answer'],
                                      'quote': payload['quote'], 'response': request['response']})
            feedback = validated_feedback(raw)
        except Exception:
            feedback = validated_feedback({'feedback': 'AI feedback unavailable; this attempt is unscored.'})
        result = {'status': 'recorded', 'feedback': feedback, 'quote': payload['quote'],
                  'answer': payload['answer'], 'locator': payload['locator']}
        await conn.execute('''INSERT INTO practice_attempts(user_id,item_id,request_key,request_hash,phase,response,exposed,feedback)
            VALUES($1,$2,$3,$4,$5,$6,$7,$8::jsonb)''', owner, request['item_id'], request['request_key'], fingerprint,
            request['phase'], request['response'], request.get('exposed', False), json.dumps(result))
        return result


LIVE = """FROM practice_items p JOIN documents d ON d.id=p.document_id AND d.user_id=p.user_id
 JOIN chunks c ON c.id=p.chunk_id AND c.user_id=p.user_id AND c.document_id=d.id
 WHERE p.user_id=$1 AND d.status='ready' AND d.content_hash=p.document_hash
 AND encode(sha256(convert_to(c.content,'UTF8')),'hex')=p.chunk_hash
 AND c.locator=p.payload->'locator' AND c.locator<>'{}'::jsonb
 AND position(p.payload->>'quote' in c.content)>0"""

async def items(conn, owner, document):
    rows = await conn.fetch("SELECT p.id,p.document_id,p.payload " + LIVE + ' AND p.document_id=$2 ORDER BY p.created_at,p.id', owner, document)
    return [{'id': str(r['id']), 'document_id': str(r['document_id']),
             **{k: json.loads(r['payload'])[k] for k in ('question','label','version')}} for r in rows]

async def generate(conn, owner, document, model):
    # Serialize retries without touching formal instruments; re-read source under
    # lock after model calls, protecting publish against deletion/re-ingestion.
    async with conn.transaction():
        await conn.execute('SELECT pg_advisory_xact_lock(hashtextextended($1,0))', f'practice:{owner}:{document}')
        doc = await conn.fetchrow("SELECT content_hash FROM documents WHERE id=$1 AND user_id=$2 AND status='ready' AND content_hash<>'' FOR UPDATE", document, owner)
        if not doc:
            return {'status': 'not_found', 'items': []}
        existing = await items(conn, owner, document)
        if existing:
            return {'status': 'ready', 'items': existing}
        sources = await conn.fetch("SELECT id,content,locator FROM chunks WHERE document_id=$1 AND user_id=$2 AND content<>'' AND locator<>'{}'::jsonb ORDER BY id LIMIT 3 FOR UPDATE", document, owner)
        if not sources:
            return {'status': 'unavailable', 'message': 'No located source passages; retry after ingestion', 'items': []}
        try:
            generated = [await generate_item(dict(source), model) for source in sources]
        except Exception:
            return {'status': 'unavailable', 'message': 'AI generation or separate grounding check unavailable/rejected; retry', 'items': []}
        for item in generated:
            await conn.execute("""INSERT INTO practice_items(user_id,document_id,chunk_id,document_hash,chunk_hash,payload)
                VALUES($1,$2,$3,$4,$5,$6::jsonb) ON CONFLICT(user_id,document_id,document_hash,chunk_id)
                DO UPDATE SET chunk_hash=EXCLUDED.chunk_hash,payload=EXCLUDED.payload""",
                owner, document, item['chunk_id'], doc['content_hash'], item['chunk_hash'], json.dumps(item))
        return {'status': 'ready', 'items': await items(conn, owner, document)}
