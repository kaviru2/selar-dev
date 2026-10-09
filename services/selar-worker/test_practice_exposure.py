import asyncio
import uuid
import asyncpg
from test_practice_service import DB,fixture,model,pytestmark

def test_recent_reading_cannot_be_reported_as_delayed_unassisted():
 async def run():
  import practice_service as p
  conn=await asyncpg.connect(DB);owner,other,doc,chunk=await fixture(conn)
  try:
   item=(await p.generate(conn,owner,doc,model))['items'][0]['id']
   req={'item_id':item,'request_key':str(uuid.uuid4()),'phase':'warmup','response':'Descent','exposed':False}
   await p.attempt(conn,owner,req,model)
   await conn.execute("UPDATE practice_schedule SET due_at=now()-interval '1 day',last_attempt_at=now()-interval '2 days' WHERE user_id=$1",owner)
   await conn.execute('INSERT INTO reading_sessions(user_id,document_id,started_at) VALUES($1,$2,now())',owner,doc)
   await p.attempt(conn,owner,{**req,'request_key':str(uuid.uuid4()),'phase':'review'},model)
   assert (await p.progress(conn,owner))['delayed_unassisted']==0
  finally:
   await conn.execute('DELETE FROM users WHERE id=ANY($1::uuid[])',[owner,other]);await conn.close()
 asyncio.run(run())
