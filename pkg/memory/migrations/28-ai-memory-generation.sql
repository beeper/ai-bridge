-- v27 -> v28: add index_generation to ai_memory_meta
ALTER TABLE ai_memory_meta ADD COLUMN index_generation TEXT NOT NULL DEFAULT '';
