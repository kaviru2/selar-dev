import asyncio
import json
import time
import uuid
import asyncpg
import httpx
from test_practice_service import DB,fixture,model,pytestmark


def test_generation_total_budget_returns_truthful_unavailable_http(monkeypatch):
 import main,practice_routes,practice_service
 monkeypatch.setattr(main,'DATABASE_URL',DB)
 monkeypatch.setattr(practice_service,'GENERATION_BUDGET_SECONDS',.04,raising=False)
 async def slow(task,data):
  await asyncio.sleep(.1)
  return await model(task,data)
 monkeypatch.setattr(practice_routes,'model',slow)
 async def run():
  conn=await asyncpg.connect(DB);owner,other,doc,chunk=await fixture(conn)
  try:
   started=time.monotonic()
   async with httpx.AsyncClient(transport=httpx.ASGITransport(app=main.app),base_url='http://worker') as client:
    response=await client.post('/practice',json={'action':'generate','user_id':str(owner),'document_id':str(doc)})
   assert response.status_code==200 and response.json()['status']=='unavailable'
   assert time.monotonic()-started<.15
   assert await conn.fetchval('SELECT count(*) FROM practice_items WHERE user_id=$1',owner)==0
  finally:
   await conn.execute('DELETE FROM users WHERE id=ANY($1::uuid[])',[owner,other]);await conn.close()
 asyncio.run(run())
