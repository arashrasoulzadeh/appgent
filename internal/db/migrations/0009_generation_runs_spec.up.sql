-- 0009_generation_runs_spec.sql
-- Persists the Plan spec (pages/components/dataModel) a run was generated
-- from, so RedeployWorkflow's rebuild-from-saved-source path can ask the
-- LLM to repair specific files on a build failure via a correctly-scoped
-- CodeActivity call, instead of giving up immediately with no chance to
-- self-heal (CodeActivity needs a Spec to know what a target's route path
-- or component name/type is).
ALTER TABLE generation_runs ADD COLUMN spec jsonb;
