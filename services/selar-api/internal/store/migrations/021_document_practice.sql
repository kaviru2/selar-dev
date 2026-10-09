-- Practice deliberately has no foreign key or query into formal quiz items.
CREATE TABLE practice_items (
 id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
 user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 document_id UUID NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
 chunk_id UUID NOT NULL REFERENCES chunks(id) ON DELETE CASCADE,
 document_hash TEXT NOT NULL CHECK(document_hash <> ''),
 chunk_hash TEXT NOT NULL,
 payload JSONB NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 UNIQUE(user_id,document_id,document_hash,chunk_id)
);
CREATE INDEX practice_owner ON practice_items(user_id,document_id);
CREATE TABLE practice_attempts (
 id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
 user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 item_id UUID NOT NULL REFERENCES practice_items(id) ON DELETE CASCADE,
 request_key UUID NOT NULL,
 request_hash TEXT NOT NULL,
 phase TEXT NOT NULL CHECK(phase IN ('warmup','reading_check','review')),
 response TEXT NOT NULL,
 exposed BOOLEAN NOT NULL DEFAULT false,
 feedback JSONB NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 UNIQUE(user_id,request_key)
);
