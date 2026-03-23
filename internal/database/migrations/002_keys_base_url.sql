-- Add base_url column to keys table for self-hosted provider instances.
ALTER TABLE keys ADD COLUMN base_url TEXT NOT NULL DEFAULT '';
