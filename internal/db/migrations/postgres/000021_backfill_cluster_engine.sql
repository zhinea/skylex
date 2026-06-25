-- Backfill clusters.engine corrupted by an earlier CreateCluster bug that stored
-- the proto enum *name* (e.g. 'ENGINE_POSTGRESQL', or 'ENGINE_UNSPECIFIED' when
-- the client omitted the field) instead of the canonical engine value
-- ('postgresql'). The engine registry is keyed by the canonical value, so these
-- rows failed engine.For() lookups with "no provider registered for ...".
-- PostgreSQL is the only supported engine today, so any non-canonical value is
-- normalized to 'postgresql'.
UPDATE clusters SET engine = 'postgresql' WHERE engine <> 'postgresql';
