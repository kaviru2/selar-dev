-- Migration 002: Add summary column to link_suggestions for AI-generated explanations
ALTER TABLE link_suggestions ADD COLUMN IF NOT EXISTS summary TEXT NOT NULL DEFAULT '';
