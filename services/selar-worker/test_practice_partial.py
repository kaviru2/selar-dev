import asyncio
import uuid
import asyncpg
from test_practice_service import DB, fixture, model, pytestmark
import practice_service as service


def test_weak_passage_does_not_discard_another_supported_question():
    async def run():
        conn = await asyncpg.connect(DB)
        owner, other, doc, chunk = await fixture(conn)
        try:
            await conn.execute("INSERT INTO chunks(id,user_id,document_id,chunk_index,content,locator) VALUES($1,$2,$3,1,'References only.','{\"page\":2}')", uuid.uuid4(), owner, doc)
            async def selective(task, data):
                return {} if data['passage'] == 'References only.' else await model(task, data)
            result = await service.generate(conn, owner, doc, selective)
            assert result['status'] == 'ready'
            assert len(result['items']) == 1
            assert result['items'][0]['question'] == 'What reduces error?'
        finally:
            await conn.execute('DELETE FROM users WHERE id=ANY($1::uuid[])', [owner, other])
            await conn.close()
    asyncio.run(run())
