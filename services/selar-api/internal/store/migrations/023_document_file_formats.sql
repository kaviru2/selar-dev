-- 021/022 are reserved by the concurrent learning-loop PR stack.
ALTER TABLE documents DROP CONSTRAINT documents_source_type_check;
ALTER TABLE documents ADD CONSTRAINT documents_source_type_check CHECK (source_type IN ('pdf','web','text','markdown','txt','docx'));
ALTER TABLE content_sources DROP CONSTRAINT content_sources_kind_check;
ALTER TABLE content_sources ADD CONSTRAINT content_sources_kind_check CHECK (kind IN ('pdf','web','text','markdown','txt','docx'));
ALTER TABLE ingestion_jobs DROP CONSTRAINT ingestion_jobs_source_type_check;
ALTER TABLE ingestion_jobs ADD CONSTRAINT ingestion_jobs_source_type_check CHECK (source_type IN ('pdf','web','text','markdown','txt','docx'));
