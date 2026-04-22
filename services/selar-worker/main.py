import os
import re
import json
import asyncio
from typing import List, Dict, Any, Optional
from pydantic import BaseModel
from fastapi import FastAPI, HTTPException, BackgroundTasks
import asyncpg
import pdfplumber
from google import genai
from dotenv import load_dotenv

# Load root .env.development
load_dotenv(os.path.join(os.path.dirname(os.path.dirname(os.path.dirname(__file__))), '.env.development'))

app = FastAPI(title="SELAR AI Ingestion Worker")

# Configure Gemini (new google.genai SDK)
api_key = os.getenv("GEMINI_API_KEY")
if not api_key:
    print("Warning: GEMINI_API_KEY is not set.")

client = genai.Client(api_key=api_key) if api_key else None

DATABASE_URL = os.getenv("DATABASE_URL", "postgres://selar:selar_dev@localhost:5432/selar?sslmode=disable")
EMBEDDING_MODEL = os.getenv("GEMINI_EMBEDDING_MODEL", "models/gemini-embedding-001")
TEXT_MODEL = os.getenv("GEMINI_TEXT_MODEL", "models/gemini-3-flash-preview")

class ProcessRequest(BaseModel):
    doc_id: str
    file_path: str


def safe_parse_json(text: str) -> dict:
    """Robustly extract JSON from LLM output that may be wrapped in markdown."""
    text = text.strip()
    # Strip markdown code fences
    if text.startswith("```json"):
        text = text[7:]
    elif text.startswith("```"):
        text = text[3:]
    if text.endswith("```"):
        text = text[:-3]
    text = text.strip()
    
    # Try direct parse first
    try:
        return json.loads(text)
    except json.JSONDecodeError:
        pass
    
    # Regex fallback: find the first complete JSON object
    match = re.search(r'\{.*\}', text, re.DOTALL)
    if match:
        try:
            return json.loads(match.group())
        except json.JSONDecodeError:
            pass
    
    return {}


async def process_document_task(doc_id: str, file_path: str):
    """
    Background job: Parses PDF, chunks texts with bboxes, embeddings, semantic links,
    relation classification, and knowledge graph extraction.
    """
    conn = None
    try:
        if not os.path.exists(file_path):
            raise FileNotFoundError(f"File {file_path} not found.")

        # ── Phase 1: Parse PDF ──
        chunks_data = []

        with pdfplumber.open(file_path) as pdf:
            for page_num, page in enumerate(pdf.pages, start=1):
                words = page.extract_words()
                if not words:
                    continue

                width, height = page.width, page.height
                
                current_chunk_words = []
                x0, top, x1, bottom = float('inf'), float('inf'), 0.0, 0.0
                
                for word in words:
                    current_chunk_words.append(word['text'])
                    x0 = min(x0, word['x0'])
                    top = min(top, word['top'])
                    x1 = max(x1, word['x1'])
                    bottom = max(bottom, word['bottom'])

                    if len(current_chunk_words) > 200:
                        chunk_text = " ".join(current_chunk_words)
                        chunks_data.append({
                            "text": chunk_text,
                            "page": page_num,
                            "bbox": {
                                "x": x0 / width,
                                "y": top / height,
                                "w": (x1 - x0) / width,
                                "h": (bottom - top) / height
                            }
                        })
                        current_chunk_words = []
                        x0, top, x1, bottom = float('inf'), float('inf'), 0.0, 0.0

                if current_chunk_words:
                    chunks_data.append({
                        "text": " ".join(current_chunk_words),
                        "page": page_num,
                        "bbox": {
                                "x": x0 / width,
                                "y": top / height,
                                "w": (x1 - x0) / width,
                                "h": (bottom - top) / height
                        }
                    })

        # ── Phase 2: Embed chunks ──
        print(f"Extracted {len(chunks_data)} chunks. Generating embeddings...")
        
        contents = [chunk["text"] for chunk in chunks_data]
        
        batch_size = 100
        embeddings = []
        for i in range(0, len(contents), batch_size):
            batch = contents[i:i+batch_size]
            result = client.models.embed_content(
                model=EMBEDDING_MODEL,
                contents=batch,
                config={"task_type": "RETRIEVAL_DOCUMENT"}
            )
            for emb in result.embeddings:
                embeddings.append(emb.values)
            
        print(f"Generated {len(embeddings)} embeddings. Inserting into Postgres...")

        # ── Phase 3: Database Insertion ──
        conn = await asyncpg.connect(DATABASE_URL)
        
        row = await conn.fetchrow("SELECT user_id FROM documents WHERE id = $1", doc_id)
        if not row:
            raise Exception(f"Document {doc_id} not found in DB")
        user_id = row['user_id']
        
        for i, chunk in enumerate(chunks_data):
            bboxes_json = json.dumps([chunk['bbox']])
            vec = str(embeddings[i])
            
            await conn.execute("""
                INSERT INTO chunks (document_id, user_id, chunk_index, page_start, page_end, content, bboxes, embedding)
                VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
            """, doc_id, user_id, i, chunk['page'], chunk['page'], chunk['text'], bboxes_json, vec)
            
        # ── Phase 4: Semantic Link Generation (wider threshold + LIMIT) ──
        print(f"Generating Semantic Links for {doc_id}...")
        
        # New → Existing (limit 20 best matches)
        res1 = await conn.execute("""
            INSERT INTO link_suggestions (user_id, source_chunk_id, target_chunk_id, similarity, relation, status)
            SELECT $2, new_chunk.id, existing.id, 1 - (new_chunk.embedding <=> existing.embedding), 'related_to', 'pending'
            FROM chunks new_chunk
            CROSS JOIN chunks existing
            WHERE new_chunk.document_id = $1
              AND existing.document_id != $1
              AND existing.user_id = $2
              AND new_chunk.embedding <=> existing.embedding < 0.35
            ORDER BY new_chunk.embedding <=> existing.embedding ASC
            LIMIT 20
        """, doc_id, user_id)
        
        # Existing → New (limit 20 best matches)
        res2 = await conn.execute("""
            INSERT INTO link_suggestions (user_id, source_chunk_id, target_chunk_id, similarity, relation, status)
            SELECT $2, existing.id, new_chunk.id, 1 - (existing.embedding <=> new_chunk.embedding), 'related_to', 'pending'
            FROM chunks new_chunk
            CROSS JOIN chunks existing
            WHERE new_chunk.document_id = $1
              AND existing.document_id != $1
              AND existing.user_id = $2
              AND existing.embedding <=> new_chunk.embedding < 0.35
            ORDER BY existing.embedding <=> new_chunk.embedding ASC
            LIMIT 20
        """, doc_id, user_id)
        
        print(f"Link Generation results - Outbound: {res1}, Inbound: {res2}")

        # ── Phase 4b: Classify relations + generate summaries ──
        print(f"Classifying link relations for {doc_id}...")
        try:
            pending_links = await conn.fetch("""
                SELECT ls.id, sc.content AS src_text, tc.content AS tgt_text
                FROM link_suggestions ls
                JOIN chunks sc ON ls.source_chunk_id = sc.id
                JOIN chunks tc ON ls.target_chunk_id = tc.id
                WHERE (sc.document_id = $1 OR tc.document_id = $1)
                  AND ls.status = 'pending'
                  AND ls.summary = ''
                LIMIT 40
            """, doc_id)

            if pending_links:
                # Batch classify in groups of 10
                for batch_start in range(0, len(pending_links), 10):
                    batch = pending_links[batch_start:batch_start+10]
                    pairs_text = ""
                    for idx, link in enumerate(batch):
                        src_snip = link['src_text'][:300]
                        tgt_snip = link['tgt_text'][:300]
                        pairs_text += f"\nPAIR {idx+1}:\nA: {src_snip}\nB: {tgt_snip}\n"
                    
                    classify_prompt = f"""Classify each text pair's semantic relationship and write a one-sentence summary of how they connect.

Return a JSON array. Each element must have:
- "index": the pair number (1-based)
- "relation": one of 'prerequisite_of', 'related_to', 'sub_concept_of', 'contradicts', 'extends'
- "summary": one clear sentence explaining the connection (max 20 words)

{pairs_text}"""

                    response = client.models.generate_content(
                        model=TEXT_MODEL,
                        contents=classify_prompt,
                        config={"response_mime_type": "application/json"}
                    )
                    
                    classifications = safe_parse_json(response.text)
                    if isinstance(classifications, dict) and 'results' in classifications:
                        classifications = classifications['results']
                    if isinstance(classifications, dict) and not isinstance(classifications, list):
                        classifications = [classifications]
                    if not isinstance(classifications, list):
                        classifications = []

                    for cls in classifications:
                        try:
                            idx = int(cls.get('index', 0)) - 1
                            if 0 <= idx < len(batch):
                                rel = cls.get('relation', 'related_to')
                                if rel not in ['prerequisite_of', 'related_to', 'sub_concept_of', 'contradicts', 'extends']:
                                    rel = 'related_to'
                                summary = cls.get('summary', '')[:200]
                                
                                await conn.execute("""
                                    UPDATE link_suggestions SET relation = $1, summary = $2 WHERE id = $3
                                """, rel, summary, batch[idx]['id'])
                        except (ValueError, IndexError):
                            continue

                print(f"Classified {len(pending_links)} link relations")
        except Exception as e:
            print(f"Relation classification failed (non-fatal): {e}")

        # ── Phase 5: Knowledge Graph Extraction ──
        print(f"Generating Knowledge Graph Concepts for {doc_id}...")
        try:
            full_text = "\n\n".join(contents)
            if len(full_text) > 80000:
                full_text = full_text[:80000]
                
            prompt = """Analyze this academic text and extract the core concepts and their relationships for a knowledge graph.

Return ONLY valid JSON with this structure:
{
  "concepts": [
    { "name": "Concept Name", "description": "1-2 sentence description." }
  ],
  "edges": [
    { "source": "Source Concept Name", "target": "Target Concept Name", "relation": "related_to" }
  ]
}

Rules:
- Extract 5-8 specific, domain-relevant concepts (not generic terms like "methodology")
- Valid relations: prerequisite_of, related_to, sub_concept_of, contradicts, extends
- Edge source/target must exactly match a concept name"""
            
            response = client.models.generate_content(
                model=TEXT_MODEL,
                contents=f"{prompt}\n\nTEXT:\n{full_text}",
                config={"response_mime_type": "application/json"}
            )
            
            tg = safe_parse_json(response.text)
            concepts = tg.get('concepts', [])
            
            if concepts:
                concept_texts = [f"{c['name']}: {c.get('description', '')}" for c in concepts]
                c_result = client.models.embed_content(
                    model=EMBEDDING_MODEL,
                    contents=concept_texts,
                    config={"task_type": "RETRIEVAL_DOCUMENT"}
                )
                c_embeddings = [emb.values for emb in c_result.embeddings]
                
                name_to_uuid = {}
                for i, concept in enumerate(concepts):
                    c_vec = str(c_embeddings[i])
                    row = await conn.fetchrow("""
                        INSERT INTO concepts (user_id, name, description, embedding)
                        VALUES ($1, $2, $3, $4)
                        ON CONFLICT (user_id, name) DO UPDATE SET description = EXCLUDED.description
                        RETURNING id
                    """, user_id, concept['name'][:250], concept.get('description', ''), c_vec)
                    
                    if row:
                        name_to_uuid[concept['name']] = row['id']
                
                edge_count = 0
                for edge in tg.get('edges', []):
                    src_id = name_to_uuid.get(edge.get('source'))
                    tgt_id = name_to_uuid.get(edge.get('target'))
                    
                    if src_id and tgt_id and src_id != tgt_id:
                        rel = edge.get('relation', 'related_to')
                        if rel not in ['prerequisite_of', 'related_to', 'sub_concept_of', 'contradicts', 'extends']:
                            rel = 'related_to'
                            
                        await conn.execute("""
                            INSERT INTO concept_edges (user_id, source_concept_id, target_concept_id, relation, created_via)
                            VALUES ($1, $2, $3, $4, 'ai_suggested')
                            ON CONFLICT (source_concept_id, target_concept_id, relation) DO NOTHING
                        """, user_id, src_id, tgt_id, rel)
                        edge_count += 1
                        
                print(f"Taxonomy -> Nodes: {len(name_to_uuid)}, Edges: {edge_count}")
        except Exception as e:
            print(f"Failed to generate taxonomy for {doc_id}: {e}")

        # ── Phase 6: Cross-document concept linking ──
        try:
            existing_concepts = await conn.fetch("""
                SELECT id, name, embedding FROM concepts WHERE user_id = $1
            """, user_id)
            
            if len(existing_concepts) > 1:
                cross_edges = 0
                for i, c1 in enumerate(existing_concepts):
                    for c2 in existing_concepts[i+1:]:
                        if c1['embedding'] and c2['embedding']:
                            # Check if edge already exists
                            existing = await conn.fetchval("""
                                SELECT COUNT(*) FROM concept_edges 
                                WHERE (source_concept_id = $1 AND target_concept_id = $2)
                                   OR (source_concept_id = $2 AND target_concept_id = $1)
                            """, c1['id'], c2['id'])
                            
                            if existing == 0:
                                # Compute similarity via pgvector
                                sim = await conn.fetchval("""
                                    SELECT 1 - ($1::vector(3072) <=> $2::vector(3072))
                                """, str(c1['embedding']), str(c2['embedding']))
                                
                                if sim and sim > 0.75:
                                    await conn.execute("""
                                        INSERT INTO concept_edges (user_id, source_concept_id, target_concept_id, relation, created_via)
                                        VALUES ($1, $2, $3, 'related_to', 'ai_suggested')
                                        ON CONFLICT (source_concept_id, target_concept_id, relation) DO NOTHING
                                    """, user_id, c1['id'], c2['id'])
                                    cross_edges += 1
                
                if cross_edges > 0:
                    print(f"Cross-document concept links: {cross_edges}")
        except Exception as e:
            print(f"Cross-doc concept linking failed (non-fatal): {e}")

        # ── Finalize ──
        await conn.execute("""
            UPDATE documents SET status = 'ready', processed_at = NOW() WHERE id = $1
        """, doc_id)

        await conn.close()
        print(f"Successfully processed {doc_id}")

    except Exception as e:
        print(f"Failed to process {doc_id}: {e}")
        try:
            if conn:
                await conn.close()
            conn = await asyncpg.connect(DATABASE_URL)
            await conn.execute("UPDATE documents SET status = 'failed' WHERE id = $1", doc_id)
            await conn.close()
        except:
            pass

@app.post("/process")
async def process_document(req: ProcessRequest, background_tasks: BackgroundTasks):
    background_tasks.add_task(process_document_task, req.doc_id, req.file_path)
    return {"message": "Processing started in background", "doc_id": req.doc_id}

@app.get("/health")
def health():
    return {"status": "ok"}
