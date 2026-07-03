# SELAR Architecture

## System Architecture Diagram

```mermaid
graph TB
    User["👤 User<br/>Browser"]
    
    subgraph "Frontend Layer"
        Console["selar-console<br/>Next.js 16<br/>Port 3000<br/>TypeScript/React"]
    end
    
    subgraph "API Layer"
        API["selar-api<br/>Go 1.22<br/>Port 8080<br/>Chi Router"]
    end
    
    subgraph "Data Layer"
        PG["PostgreSQL 16<br/>+ pgvector<br/>Port 5432<br/>Vector DB"]
        Tables["📊 Tables:<br/>users<br/>documents<br/>chunks<br/>concepts<br/>link_suggestions<br/>annotations"]
    end
    
    subgraph "AI/Processing Layer"
        Worker["selar-worker<br/>Python 3.11+<br/>Port 8000<br/>FastAPI"]
        Gemini["🤖 Google Gemini API<br/>embedding-001<br/>3-flash-preview"]
    end
    
    User -->|Browser| Console
    Console -->|HTTP/REST| API
    API -->|SQL| PG
    PG --> Tables
    
    API -->|JSON Task Queue| Worker
    Worker -->|HTTP| Gemini
    Worker -->|SQL| PG
    
    Console -.->|Optional Direct| Worker
    
    style Console fill:#61dafb,stroke:#333,color:#000
    style API fill:#00add8,stroke:#333,color:#fff
    style PG fill:#336791,stroke:#333,color:#fff
    style Worker fill:#3776ab,stroke:#333,color:#fff
    style Gemini fill:#e37400,stroke:#333,color:#fff
    style User fill:#90ee90,stroke:#333,color:#000
```

## Data Flow Diagram

```mermaid
graph LR
    User["User"]
    
    subgraph "1. Registration & Auth"
        Auth["1️⃣ Register/Login<br/>JWT issued"]
    end
    
    subgraph "2. Document Upload"
        Upload["2️⃣ Upload PDF<br/>API stores metadata<br/>Document status: pending"]
    end
    
    subgraph "3. Document Ingestion"
        Process["3️⃣ Worker processes:<br/>• Chunk PDF<br/>• Extract text<br/>• Generate embeddings<br/>• Find semantic links"]
    end
    
    subgraph "4. Link Suggestions"
        Suggest["4️⃣ Display to user:<br/>AI-generated matches<br/>Confirm/Reject"]
    end
    
    subgraph "5. Knowledge Graph"
        Graph["5️⃣ Build concept graph:<br/>Extract concepts<br/>Show relationships"]
    end
    
    User -->|selar-console| Auth
    Auth -->|selar-api| Upload
    Upload -->|Trigger| Process
    Process -->|Gemini API| Suggest
    Suggest -->|User Feedback| Graph
    Graph -->|Query| User
```

## Component Interactions

```mermaid
graph TB
    subgraph "Client"
        Browser["Browser<br/>selar-console"]
    end
    
    subgraph "Backend Services"
        Go["Go API<br/>REST Endpoints"]
        Py["Python Worker<br/>Async Tasks"]
    end
    
    subgraph "Database"
        DB["PostgreSQL<br/>+ pgvector"]
    end
    
    subgraph "External"
        Gemini["Gemini API"]
    end
    
    Browser -->|GET/POST /api/...<br/>JWT Auth| Go
    Go -->|Validate<br/>Query/Mutate| DB
    Go -->|Enqueue Task<br/>Job Status| Py
    Py -->|Read/Write<br/>Embeddings<br/>Chunks| DB
    Py -->|embeddings<br/>generateText| Gemini
    Browser -->|Fetch Status<br/>Get Results| Go
    
    style Browser fill:#61dafb,stroke:#333,color:#000
    style Go fill:#00add8,stroke:#333,color:#fff
    style Py fill:#3776ab,stroke:#333,color:#fff
    style DB fill:#336791,stroke:#333,color:#fff
    style Gemini fill:#e37400,stroke:#333,color:#fff
```

## Technology Stack Summary

| Layer | Service | Tech | Port | Purpose |
|-------|---------|------|------|---------|
| **Frontend** | selar-console | Next.js 16, TypeScript, React | 3000 | PDF reader, matches UI, graph visualization |
| **API** | selar-api | Go 1.22, Chi router | 8080 | REST API, JWT auth, CRUD, business logic |
| **Worker** | selar-worker | Python 3.11+, FastAPI | 8000 | Document ingestion, chunking, embeddings, link generation |
| **Database** | PostgreSQL | PostgreSQL 16 + pgvector | 5432 | User data, documents, embeddings (3072-dim), concept graph |
| **External** | Gemini API | Google AI Studio | — | Text embeddings & generation |

## Key Features & Flow

1. **User Registration**: JWT-based auth in selar-api
2. **PDF Upload**: Document stored in DB with `status: pending`
3. **Async Processing**: Worker consumes PDF, chunks text, generates embeddings
4. **Semantic Linking**: pgvector similarity search finds related passages
5. **User Feedback**: Confirm/reject suggestions → retrieval-practice event
6. **Knowledge Graph**: Extract & visualize concepts and relationships
