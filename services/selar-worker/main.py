import os
import sys
import json
import asyncio
from typing import List, Dict, Any, Optional
from pydantic import BaseModel
from fastapi import FastAPI, HTTPException, BackgroundTasks
import asyncpg
import pdfplumber
import google.generativeai as genai
from dotenv import load_dotenv

# Load root .env.development
load_dotenv(os.path.join(os.path.dirname(os.path.dirname(os.path.dirname(__file__))), '.env.development'))

app = FastAPI(title="SELAR AI Ingestion Worker")

# Configure Gemini
api_key = os.getenv("GEMINI_API_KEY")
if not api_key:
    print("Warning: GEMINI_API_KEY is not set.")
else:
    genai.configure(api_key=api_key)

DATABASE_URL = os.getenv("DATABASE_URL", "postgres://selar:selar_dev@localhost:5432/selar?sslmode=disable")

class ProcessRequest(BaseModel):
    doc_id: str
    file_path: str

async def process_document_task(doc_id: str, file_path: str):
    """
    Background job: Parses PDF, chunks texts with bboxes, embeddings, and database inserts.
    """
    try:
        if not os.path.exists(file_path):
            raise FileNotFoundError(f"File {file_path} not found.")

        # 1. Parse PDF using pdfplumber to get text and proportional bboxes
        chunks_data = []

        with pdfplumber.open(file_path) as pdf:
            for page_num, page in enumerate(pdf.pages, start=1):
                # We'll do a simple paragraph grouping based on words spacing
                words = page.extract_words()
                if not words:
                    continue

                width, height = page.width, page.height
                
                # Heuristic: Group words into lines and lines into paragraphs. 
                # For Phase 3 MVP, we simply clump words roughly into ~300 max token chunks per page.
                
                current_chunk_words = []
                # Initialize bbox
                x0, top, x1, bottom = float('inf'), float('inf'), 0.0, 0.0
                
                for word in words:
                    current_chunk_words.append(word['text'])
                    x0 = min(x0, word['x0'])
                    top = min(top, word['top'])
                    x1 = max(x1, word['x1'])
                    bottom = max(bottom, word['bottom'])

                    if len(current_chunk_words) > 200:
                        # Finalize chunk
                        chunk_text = " ".join(current_chunk_words)
                        chunks_data.append({
                            "text": chunk_text,
                            "page": page_num,
                            # Store relative percentages, e.g., 0.15 instead of 115px
                            "bbox": {
                                "x": x0 / width,
                                "y": top / height,
                                "w": (x1 - x0) / width,
                                "h": (bottom - top) / height
                            }
                        })
                        current_chunk_words = []
                        x0, top, x1, bottom = float('inf'), float('inf'), 0.0, 0.0

                # Append any remaining words on the page as the last chunk
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

        # 2. Embed chunks using Gemini API
        print(f"Extracted {len(chunks_data)} chunks. Generating embeddings...")
        
        # Batch embedding is faster
        model_name = os.getenv("GEMINI_EMBEDDING_MODEL", "models/gemini-embedding-001")
        contents = [chunk["text"] for chunk in chunks_data]
        
        # Safety limit for Google API is typically 100 per request, we'll slice it into batches of 100
        batch_size = 100
        embeddings = []
        for i in range(0, len(contents), batch_size):
            batch = contents[i:i+batch_size]
            result = genai.embed_content(
                model=model_name,
                content=batch,
                task_type="retrieval_document"
            )
            embeddings.extend(result['embedding'])
            
        print(f"Generated {len(embeddings)} embeddings. Inserting into Postgres...")

        # 3. Database Insertion via asyncpg
        conn = await asyncpg.connect(DATABASE_URL)
        
        # Fetch the user_id that owns this document
        row = await conn.fetchrow("SELECT user_id FROM documents WHERE id = $1", doc_id)
        if not row:
            raise Exception(f"Document {doc_id} not found in DB")
        user_id = row['user_id']
        
        for i, chunk in enumerate(chunks_data):
            # Schema expects a JSON array of bboxes
            bboxes_json = json.dumps([chunk['bbox']])
            vec = str(embeddings[i]) # pgvector expects specific format, driver converts "[1,2,3]"
            
            await conn.execute("""
                INSERT INTO chunks (document_id, user_id, chunk_index, page_start, page_end, content, bboxes, embedding)
                VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
            """, doc_id, user_id, i, chunk['page'], chunk['page'], chunk['text'], bboxes_json, vec)
            
        # Phase 4 Semantic Link Generation
        # We tighten the threshold to < 0.15 for higher quality matches (similarity > 0.85)
        # And we perform BI-DIRECTIONAL inserts so that BOTH documents receive highlights natively!
        print(f"Generating Semantic Links for {doc_id}...")
        
        # Insert New -> Existing
        res1 = await conn.execute("""
            INSERT INTO link_suggestions (user_id, source_chunk_id, target_chunk_id, similarity, relation, status)
            SELECT $2, new_chunk.id, existing.id, 1 - (new_chunk.embedding <=> existing.embedding), 'related_to', 'pending'
            FROM chunks new_chunk
            CROSS JOIN chunks existing
            WHERE new_chunk.document_id = $1
              AND existing.document_id != $1
              AND existing.user_id = $2
              AND new_chunk.embedding <=> existing.embedding < 0.20
        """, doc_id, user_id)
        
        # Insert Existing -> New
        res2 = await conn.execute("""
            INSERT INTO link_suggestions (user_id, source_chunk_id, target_chunk_id, similarity, relation, status)
            SELECT $2, existing.id, new_chunk.id, 1 - (existing.embedding <=> new_chunk.embedding), 'related_to', 'pending'
            FROM chunks new_chunk
            CROSS JOIN chunks existing
            WHERE new_chunk.document_id = $1
              AND existing.document_id != $1
              AND existing.user_id = $2
              AND existing.embedding <=> new_chunk.embedding < 0.20
        """, doc_id, user_id)
        
        print(f"Link Generation results - Outbound: {res1}, Inbound: {res2}")

        # Phase 5 Taxonomy / Knowledge Graph Generation
        print(f"Generating Knowledge Graph Concepts for {doc_id}...")
        try:
            full_text = "\n\n".join(contents)
            if len(full_text) > 100000:
                full_text = full_text[:100000] # Fit into window if massive
                
            prompt = """
            Analyze the following academic/technical text and extract the core taxonomic concepts and their relationships to build a knowledge graph.
            Return ONLY a strict JSON object with this exact structure:
            {
              "concepts": [
                { "name": "Concept Name", "description": "Brief description of the concept." }
              ],
              "edges": [
                { "source": "Source Concept Name", "target": "Target Concept Name", "relation": "related_to" }
              ]
            }
            Valid relations are strictly: 'prerequisite_of', 'related_to', 'sub_concept_of', 'contradicts', 'extends'.
            Extract exactly 5-8 of the most salient concepts. Do not use generic names.
            """
            
            text_model = os.getenv("GEMINI_TEXT_MODEL", "models/gemini-3-flash-preview")
            response = genai.GenerativeModel(text_model).generate_content(
                f"{prompt}\n\nTEXT:\n{full_text}",
                generation_config=genai.GenerationConfig(response_mime_type="application/json")
            )
            raw_text = response.text.strip()
            if raw_text.startswith("```json"): raw_text = raw_text[7:]
            elif raw_text.startswith("```"): raw_text = raw_text[3:]
            if raw_text.endswith("```"): raw_text = raw_text[:-3]
            raw_text = raw_text.strip()
            
            tg = json.loads(raw_text)
            concept_names = [c['name'] + ": " + c['description'] for c in tg.get('concepts', [])]
            
            if concept_names:
                c_embeddings = genai.embed_content(
                    model=model_name,
                    content=concept_names,
                    task_type="retrieval_document"
                )['embedding']
                
                name_to_uuid = {}
                for i, concept in enumerate(tg.get('concepts', [])):
                    c_vec = str(c_embeddings[i])
                    row = await conn.fetchrow("""
                        INSERT INTO concepts (user_id, name, description, embedding)
                        VALUES ($1, $2, $3, $4)
                        ON CONFLICT (user_id, name) DO UPDATE SET description = EXCLUDED.description
                        RETURNING id
                    """, user_id, concept['name'][:250], concept['description'], c_vec)
                    
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
                        """, user_id, src_id, tgt_id, rel)
                        edge_count += 1
                        
                print(f"Taxonomy Extractions -> Nodes: {len(name_to_uuid)}, Edges: {edge_count}")
        except Exception as e:
            print(f"Failed to generate taxonomy for {doc_id}: {e}")


        # Update Document status
        await conn.execute("""
            UPDATE documents SET status = 'ready', processed_at = NOW() WHERE id = $1
        """, doc_id)

        await conn.close()
        print(f"Successfully processed {doc_id}")

    except Exception as e:
        print(f"Failed to process {doc_id}: {e}")
        try:
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
