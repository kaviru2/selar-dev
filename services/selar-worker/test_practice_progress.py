import asyncio
import importlib
import uuid
import asyncpg
from test_practice_service import DB, fixture, model, pytestmark


def test_progress_separates_exposed_warmup_and_delayed_unassisted():
 async def run():
  p=importlib.import_module('practice_service');conn=await asyncpg.connect(DB)
  owner,other,doc,chunk=await fixture(conn)
  try:
   item=(await p.generate(conn,owner,doc,model))['items'][0]['id']
   for phase in ('warmup','reading_check'):
    await p.attempt(conn,owner,{'item_id':item,'request_key':str(uuid.uuid4()),'phase':phase,'response':'A gradient method','exposed':False},model)
   await conn.execute("UPDATE practice_schedule SET due_at=now()-interval '1 day',last_attempt_at=now()-interval '2 days' WHERE user_id=$1",owner)
   await p.attempt(conn,owner,{'item_id':item,'request_key':str(uuid.uuid4()),'phase':'review','response':'A gradient method','exposed':False},model)
   report=await p.progress(conn,owner)
   assert report['warmup']==1 and report['reading_check']==1
   assert report['delayed_unassisted']==1 and report['delayed_scored']==1 and report['delayed_mean_score']==.5
   assert report['estimated_recall'] is None
   assert (await p.progress(conn,other))['delayed_unassisted']==0
   await conn.execute("UPDATE documents SET content_hash='new snapshot' WHERE id=$1",doc)
   assert (await p.progress(conn,owner))['delayed_unassisted']==0
  finally:
   await conn.execute('DELETE FROM users WHERE id=ANY($1::uuid[])',[owner,other]);await conn.close()
 asyncio.run(run())
