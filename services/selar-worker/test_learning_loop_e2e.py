"""Real service/browser learning loop; only model responses are synthetic.

No production clock input: fabricated schedule rows alone are aged in this test.
"""
import asyncio
import json
import os
import uuid
from types import SimpleNamespace
import asyncpg
import httpx
import pytest
from test_synthetic_service_e2e import services, _request, _drain_job, _dismiss_optional_consent, PASSWORD, fabricated_pdf

TEXT='Gradient descent optimization reduces a fabricated error score in a toy observatory.'

def database_call(function):
 from concurrent.futures import ThreadPoolExecutor
 with ThreadPoolExecutor(max_workers=1) as executor:
  return executor.submit(lambda:asyncio.run(function())).result()


@pytest.mark.timeout(240)
def test_upload_warmup_reading_check_day_n_review_progress_and_private_boundaries(services, monkeypatch):
 import main
 from playwright.sync_api import sync_playwright, expect
 api,console=services
 monkeypatch.setattr(main,'DATABASE_URL',os.environ['TEST_DATABASE_URL'])
 monkeypatch.setattr(main,'embed_text_documents',lambda contents,title='':[[1.0]+[0.0]*3071 for _ in contents])
 class Models:
  def generate_content(self,**kwargs):
   system=getattr(kwargs.get('config'),'system_instruction','') or ''
   if 'practice question' in system:
    passage=json.loads(kwargs['contents'])['passage']
    return SimpleNamespace(text=json.dumps({'question':'What reduces the fabricated error score?','answer':'Gradient descent optimization','quote':passage}))
   if 'Independently check' in system:return SimpleNamespace(text=json.dumps({'supported':True,'reason':'Synthetic supported fixture'}))
   return SimpleNamespace(text=json.dumps({'main_claim':TEXT,'key_concepts':[{'name':'gradient descent optimization','description':'Synthetic','evidence_chunk_index':0}],'assumptions':[],'open_questions':[],'domain':'synthetic','concept_edges':[]}))
 monkeypatch.setattr(main,'client',SimpleNamespace(models=Models()))
 suffix=uuid.uuid4().hex
 with httpx.Client() as client:
  users=[_request(client,'POST',f'{api}/auth/register',expected=201,json={'email':f'loop-{kind}-{suffix}@example.invalid','password':PASSWORD}) for kind in ('owner','borrower')]
  owner,other=users; token=owner['token']; borrower=other['token']
  doc=_request(client,'POST',f'{api}/api/documents/upload',token,expected=202,files={'file':('synthetic-loop.pdf',fabricated_pdf(TEXT),'application/pdf')})['document']['id']
  asyncio.run(_drain_job(doc))
  peer=_request(client,'POST',f'{api}/api/documents/add',token,expected=202,json={'source_type':'text','title':'Synthetic peer','text':TEXT.replace('observatory','workshop')})['document']['id']
  asyncio.run(_drain_job(peer))
  links=_request(client,'GET',f'{api}/api/mental-model-links?document_id={doc}',token)
  assert links
  link=links[0]
  items=_request(client,'POST',f'{api}/api/practice',token,json={'action':'items','document_id':doc})['items']
  assert len(items)==1 and 'quote' not in items[0] and 'answer' not in items[0]
  assert _request(client,'POST',f'{api}/api/practice',borrower,json={'action':'items','document_id':doc})['items']==[]
  assert _request(client,'POST',f'{api}/api/practice',borrower,json={'action':'generate','document_id':doc})['status']=='not_found'
  with sync_playwright() as pw:
   browser=pw.chromium.launch(headless=True);context=browser.new_context();page=context.new_page()
   login=context.request.post(f'{console}/api/auth/login',data={'email':f'loop-owner-{suffix}@example.invalid','password':PASSWORD});assert login.status==200
   page.goto(f'{console}/reader?docId={doc}');_dismiss_optional_consent(page)
   page.get_by_role('heading',name='Before reading',exact=True).wait_for(timeout=20000)
   assert page.locator('.textLayer').count()==0
   async def session_count():
    conn=await asyncpg.connect(os.environ['TEST_DATABASE_URL'])
    try:return await conn.fetchval('SELECT count(*) FROM reading_sessions WHERE user_id=$1',owner['user']['id'])
    finally:await conn.close()
   assert database_call(session_count)==0, 'warm-up must not start formal reading anchors'
   page.get_by_role('textbox',name='Your recall').fill('Gradient descent optimization')
   page.get_by_role('button',name='Check my recall').click()
   page.get_by_text('Reference answer:',exact=False).wait_for(timeout=20000)
   expect(page.get_by_role('blockquote')).to_have_text(TEXT)
   page.get_by_role('button',name='Continue reading',exact=True).click()
   page.locator('.textLayer').get_by_text(TEXT).wait_for(timeout=20000)
   assert database_call(session_count)==1
   prompt=page.get_by_role('region',name='Connection reflection').first
   prompt.get_by_role('button',name='Compare passages',exact=True).click()
   assert prompt.locator('blockquote').count()==2
   with page.expect_response(lambda response:response.request.method=='POST' and '/flag' in response.url) as flagged:
    prompt.get_by_role('button',name='This link is wrong',exact=True).click()
   assert flagged.value.status==200
   assert json.loads(flagged.value.request.post_data)['revision']==link['revision']
   page.get_by_text('No suggested connections yet.',exact=False).wait_for(timeout=10000)
   assert not any(e.get('mental_link_id')==link['id'] for e in _request(client,'GET',f'{api}/api/graph',token)['edges'])
   page.get_by_role('button',name='End-reading check',exact=True).click()
   page.get_by_role('textbox',name='Your recall').fill('Gradient descent optimization reduces error')
   page.get_by_role('button',name='Check my recall').click();page.get_by_text('Reference answer:',exact=False).wait_for(timeout=20000)
   page.goto(f'{console}/review');page.get_by_text('Nothing due.',exact=False).wait_for(timeout=20000)
   async def day_n():
    conn=await asyncpg.connect(os.environ['TEST_DATABASE_URL'])
    try:
     assert 'selar_e2e' in os.environ['TEST_DATABASE_URL']
     await conn.execute("UPDATE practice_schedule SET last_attempt_at=now()-interval '2 days',due_at=now()-interval '1 day' WHERE user_id=$1",owner['user']['id'])
     await conn.execute("UPDATE reading_sessions SET started_at=started_at-interval '2 days',ended_at=ended_at-interval '2 days' WHERE user_id=$1",owner['user']['id'])
    finally:await conn.close()
   from concurrent.futures import ThreadPoolExecutor
   with ThreadPoolExecutor(max_workers=1) as executor:
    executor.submit(lambda: asyncio.run(day_n())).result()
   page.reload()
   page.get_by_role('textbox',name='Your recall').fill('Gradient descent optimization')
   page.get_by_role('button',name='Check my recall').click();page.get_by_text('Reference answer:',exact=False).wait_for(timeout=20000)
   page.goto(f'{console}/progress');page.get_by_text('1 delayed attempts',exact=False).wait_for(timeout=20000)
   assert 'not a retention probability' in page.inner_text('main')
   foreign=browser.new_context();foreign.request.post(f'{console}/api/auth/login',data={'email':f'loop-borrower-{suffix}@example.invalid','password':PASSWORD})
   foreign_page=foreign.new_page();foreign_page.goto(f'{console}/reader?docId={doc}')
   assert foreign_page.get_by_role('textbox',name='Your recall').count()==0
   assert foreign_page.evaluate("""async (document_id) => (await fetch('/api/practice',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({action:'items',document_id})})).json()""",doc)['items']==[]
   async def lock_cohort():
    conn=await asyncpg.connect(os.environ['TEST_DATABASE_URL'])
    try:
     await conn.execute("UPDATE users SET cohort='control' WHERE id=$1",owner['user']['id'])
     await conn.execute("INSERT INTO cohort_setting_locks(cohort,setting_key,value,reason) VALUES('control','suggestions.show_on_open','false','learning-loop-synthetic')")
    finally:await conn.close()
   database_call(lock_cohort)
   try:
    page.goto(f'{console}/reader?docId={doc}');page.get_by_role('button',name='Continue reading',exact=True).click()
    page.get_by_text('No suggested connections yet.',exact=False).wait_for(timeout=10000)
    assert page.get_by_role('region',name='Connection reflection').count()==0
    for suffix_path in ('preview','flag','respond'):
     status=page.evaluate("""async ({id,route})=>(await fetch(`/api/mental-model-links/${id}/${route}`,route==='preview'?{}:{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({revision:1,action:'retracted',reason:'synthetic'})})).status""",{'id':link['id'],'route':suffix_path})
     assert status==403,(suffix_path,status)
   finally:
    async def unlock_fixture():
     conn=await asyncpg.connect(os.environ['TEST_DATABASE_URL'])
     try:await conn.execute("DELETE FROM cohort_setting_locks WHERE reason='learning-loop-synthetic'")
     finally:await conn.close()
    database_call(unlock_fixture)
   browser.close()
  report=_request(client,'POST',f'{api}/api/practice',token,json={'action':'progress'})
  assert report['warmup']==1 and report['reading_check']==1 and report['delayed_unassisted']==1 and report['streak']==1
  async def persisted():
   conn=await asyncpg.connect(os.environ['TEST_DATABASE_URL'])
   try:
    assert await conn.fetchval('SELECT count(*) FROM practice_attempts WHERE user_id=$1',owner['user']['id'])==3
    assert await conn.fetchval('SELECT count(*) FROM quiz_attempts WHERE user_id=$1',owner['user']['id'])==0
    assert await conn.fetchval('SELECT count(*) FROM learner_concept_state WHERE user_id=$1 AND success_count>0',owner['user']['id'])==0
    await conn.execute("UPDATE documents SET content_hash='stale-loop-fixture' WHERE id=$1",doc)
   finally:await conn.close()
  asyncio.run(persisted())
  assert _request(client,'POST',f'{api}/api/practice',token,json={'action':'items','document_id':doc})['items']==[]
  assert _request(client,'POST',f'{api}/api/practice',token,json={'action':'progress'})['delayed_unassisted']==0


@pytest.mark.timeout(80)
def test_six_eleven_second_provider_calls_fit_real_http_deadlines(services):
 import time
 api,_=services
 with httpx.Client() as client:
  account=_request(client,'POST',f'{api}/auth/register',expected=201,json={'email':f'deadline-{uuid.uuid4().hex}@example.invalid','password':PASSWORD})
  async def seed():
   conn=await asyncpg.connect(os.environ['TEST_DATABASE_URL'])
   try:
    doc=await conn.fetchval("INSERT INTO documents(user_id,title,status,content_hash) VALUES($1,'Synthetic deadline','ready','deadline-snapshot') RETURNING id",account['user']['id'])
    for i in range(3):
     await conn.execute("INSERT INTO chunks(user_id,document_id,chunk_index,content,locator) VALUES($1,$2,$3,$4,'{\"page\":1}')",account['user']['id'],doc,i,TEXT+' __synthetic_11_second_provider__ '+str(i))
    return str(doc)
   finally:await conn.close()
  doc=asyncio.run(seed());started=time.monotonic()
  result=_request(client,'POST',f'{api}/api/practice',account['token'],json={'action':'generate','document_id':doc})
  duration=time.monotonic()-started
  assert result['status']=='ready' and len(result['items'])==3,result
  assert 20<=duration<40,duration
  print(f'Actual API/worker HTTP generation: six 11-second provider calls, three items, {duration:.2f}s; server deadline unchanged')
