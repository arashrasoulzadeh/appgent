-- 0008_generation_runs_source_path.sql
-- Raw generated source (before build) is now always persisted, separately
-- from bundle_path (the BUILT static output) — bundle_path stays NULL when
-- a build fails, but the source must still be recoverable so Deploy can
-- retry the build without needing a full regenerate.
ALTER TABLE generation_runs ADD COLUMN source_path text;
