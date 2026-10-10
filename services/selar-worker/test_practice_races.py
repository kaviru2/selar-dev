import asyncio
import json
import uuid
import asyncpg
from test_practice_service import DB, fixture, model, pytestmark


def test_changed_extraction_cannot_rebind_old_attempts_to_new_item():
 async def run():
  import practice_service as p
  conn=await asyncpg.connect(DB);owner,other,doc,chunk=await fixture(conn)
  try:
   item=(await p.generate(conn,owner,doc,model))['items'][0]['id']
   await p.attempt(conn,owner,{'item_id':item,'request_key':str(uuid.uuid4()),'phase':'warmup','response':'A gradient method'},model)
   await conn.execute("UPDATE chunks SET content='Descent reduces error. Changed extraction.' WHERE id=$1",chunk)
   await p.generate(conn,owner,doc,model)
   assert (await p.progress(conn,owner))['warmup']==0
  finally:
   await conn.execute('DELETE FROM users WHERE id=ANY($1::uuid[])',[owner,other]);await conn.close()
 asyncio.run(run())


def test_source_delete_waits_for_generation_then_removes_all_derived_practice():
 async def run():
  import practice_service as p
  conn=await asyncpg.connect(DB);owner,other,doc,chunk=await fixture(conn)
  started=asyncio.Event();release=asyncio.Event()
  async def held(task,data):
   if task=='generate':started.set();await release.wait()
   return await model(task,data)
  async def delete():
   c=await asyncpg.connect(DB)
   try:await c.execute('DELETE FROM documents WHERE id=$1 AND user_id=$2',doc,owner)
   finally:await c.close()
  try:
   generation=asyncio.create_task(p.generate(conn,owner,doc,held));await started.wait()
   deletion=asyncio.create_task(delete());await asyncio.sleep(.03);assert not deletion.done()
   release.set();await generation;await deletion
   assert await p.items(conn,owner,doc)==[]
   assert await conn.fetchval('SELECT count(*) FROM practice_items WHERE user_id=$1',owner)==0
  finally:
   await conn.execute('DELETE FROM users WHERE id=ANY($1::uuid[])',[owner,other]);await conn.close()
 asyncio.run(run())
