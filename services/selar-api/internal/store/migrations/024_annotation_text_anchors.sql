-- Additive: NULL retains legacy rectangle-only annotations unchanged.
ALTER TABLE annotations ADD COLUMN IF NOT EXISTS anchor jsonb;
