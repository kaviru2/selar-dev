"""Database boundary for private, source-bound practice; no formal instruments."""
import asyncio
import json
GENERATION_BUDGET_SECONDS = 40  # below Go server WriteTimeout=60s and Modal=120s
from practice import Unavailable, digest, exact_recall_feedback, generate_item, validated_feedback
from practice_schedule import next_interval, streak

async def progress(conn, owner):
    rows = await conn.fetch('''SELECT phase,exposed,delayed_unassisted,feedback FROM practice_attempts a
        WHERE a.user_id=$1 AND a.created_at<=now() AND a.item_id IN (SELECT p.id '''+LIVE+')',owner)
    report={'status':'ready','warmup':0,'reading_check':0,'review':0,'exposed':0,
            'delayed_unassisted':0,'delayed_scored':0,'unscored':0,'delayed_mean_score':None,
            'estimated_recall':None,'estimate_note':'No calibrated recall estimate is available. Scheduling intervals are a heuristic, not efficacy evidence.'}
    scores=[]
    for row in rows:
        report[row['phase']]+=1
        report['exposed']+=int(row['exposed'])
        report['delayed_unassisted']+=int(row['delayed_unassisted'])
        feedback=json.loads(row['feedback'])['feedback']
        if feedback['score'] is None: report['unscored']+=1
        elif row['delayed_unassisted']: scores.append(feedback['score'])
    report['delayed_scored']=len(scores)
    if scores: report['delayed_mean_score']=sum(scores)/len(scores)
    queue=await daily(conn,owner)
    report['streak']=queue['streak'];report['due']=await conn.fetchval('SELECT count(*) '+LIVE+''' AND EXISTS(SELECT 1 FROM practice_schedule s WHERE s.item_id=p.id AND s.user_id=$1 AND s.due_at<=now())''',owner);report['timezone']='UTC'
    return report

async def daily(conn, owner):
    rows = await conn.fetch('SELECT p.id,p.document_id,p.payload ' + LIVE + ''' AND EXISTS(
        SELECT 1 FROM practice_schedule s WHERE s.item_id=p.id AND s.user_id=$1 AND s.due_at<=now())
        ORDER BY p.created_at,p.id LIMIT 20''', owner)
    timestamps = await conn.fetch("SELECT created_at FROM practice_attempts WHERE user_id=$1 AND phase='review' AND created_at<=now()", owner)
    now = await conn.fetchval('SELECT now()')
    return {'status':'ready', 'items':[{'id':str(r['id']),'document_id':str(r['document_id']),
        **{k:json.loads(r['payload'])[k] for k in ('question','label')}} for r in rows],
        'streak':streak([r['created_at'] for r in timestamps],now), 'timezone':'UTC',
        'policy':'doubling-interval-v1 (heuristic, not a recall estimate)'}


async def attempt(conn, owner, request, model):
    fingerprint = digest(json.dumps(request, sort_keys=True))
    async with conn.transaction():
        await conn.execute("SET LOCAL lock_timeout='3s'")
        await conn.execute('SELECT pg_advisory_xact_lock(hashtextextended($1,0))', f'practice-attempt:{owner}:{request["request_key"]}')
        prior = await conn.fetchrow('SELECT request_hash,feedback FROM practice_attempts WHERE user_id=$1 AND request_key=$2', owner, request['request_key'])
        row = await conn.fetchrow('SELECT p.payload ' + LIVE + ' AND p.id=$2 FOR UPDATE OF p,d,c', owner, request['item_id'])
        if not row:
            return {'status': 'not_found'}
        if prior:
            return json.loads(prior['feedback']) if prior['request_hash'] == fingerprint else {'status': 'conflict'}
        schedule = await conn.fetchrow('SELECT *,due_at<=now() AS due,last_attempt_at<=now()-interval \'1 day\' AS delayed FROM practice_schedule WHERE item_id=$1 AND user_id=$2 FOR UPDATE', request['item_id'],owner)
        if request['phase']=='review' and (not schedule or not schedule['due']):
            return {'status':'not_due'}
        payload = json.loads(row['payload'])
        try:
            # Exact restatements are graded deterministically; only genuine
            # free recall (paraphrase, partial, negated) reaches the model.
            raw = exact_recall_feedback(payload, request['response']) or await model('grade', {
                'question': payload['question'], 'answer': payload['answer'],
                'quote': payload['quote'], 'response': request['response']})
            feedback = validated_feedback(raw)
        except Exception:
            feedback = validated_feedback({'feedback': 'AI feedback unavailable; this attempt is unscored.'})
        result = {'status': 'recorded', 'feedback': feedback, 'quote': payload['quote'],
                  'answer': payload['answer'], 'locator': payload['locator']}
        exposed = request.get('exposed', False) or request['phase']=='reading_check'
        recent_read = await conn.fetchval('''SELECT EXISTS(SELECT 1 FROM reading_sessions rs JOIN practice_items pi ON pi.document_id=rs.document_id AND pi.user_id=rs.user_id
            WHERE pi.id=$1 AND rs.user_id=$2 AND (rs.ended_at IS NULL OR greatest(rs.started_at,rs.ended_at)>now()-interval '1 day'))''',request['item_id'],owner)
        exposed = exposed or bool(recent_read)
        delayed = bool(request['phase']=='review' and schedule and schedule['delayed'] and not exposed)
        await conn.execute('''INSERT INTO practice_attempts(user_id,item_id,request_key,request_hash,phase,response,exposed,feedback,delayed_unassisted)
            VALUES($1,$2,$3,$4,$5,$6,$7,$8::jsonb,$9)''', owner, request['item_id'], request['request_key'], fingerprint,
            request['phase'], request['response'], exposed, json.dumps(result),delayed)
        interval = next_interval(schedule['interval_days'] if schedule else 1,feedback['score'],not delayed)
        await conn.execute('''INSERT INTO practice_schedule(item_id,user_id,interval_days,due_at,last_attempt_at)
            VALUES($1,$2,$3,now()+$3::integer*interval '1 day',now()) ON CONFLICT(item_id) DO UPDATE
            SET interval_days=EXCLUDED.interval_days,due_at=EXCLUDED.due_at,last_attempt_at=EXCLUDED.last_attempt_at''',request['item_id'],owner,interval)
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
        await conn.execute("SET LOCAL lock_timeout='3s'")
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
            generated = await asyncio.wait_for(asyncio.gather(*(generate_item(dict(source), model) for source in sources)), timeout=GENERATION_BUDGET_SECONDS)
        except Exception:
            return {'status': 'unavailable', 'message': 'AI generation or separate grounding check unavailable/rejected; retry', 'items': []}
        for item in generated:
            await conn.execute("""INSERT INTO practice_items(user_id,document_id,chunk_id,document_hash,chunk_hash,payload)
                VALUES($1,$2,$3,$4,$5,$6::jsonb) ON CONFLICT(user_id,document_id,document_hash,chunk_id)
                DO NOTHING""",
                owner, document, item['chunk_id'], doc['content_hash'], item['chunk_hash'], json.dumps(item))
        published = await items(conn, owner, document)
        return {'status': 'ready' if published else 'unavailable', 'items': published,
                'message': '' if published else 'Source extraction changed; re-ingest before generating new practice.'}
