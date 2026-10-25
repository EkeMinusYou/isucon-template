-- Interpret participant-visible progress when available.
-- No rows means no interpreted observations, not zero activity.
create or replace view bench_progress as
select
    cast(null as varchar) as run_id,
    cast(null as bigint) as line_number,
    -- Seconds since the load phase began.
    cast(null as double) as elapsed_s,
    cast(null as varchar) as metric,
    cast(null as double) as value,
    -- Distinguish cumulative counts, events, and reported percentages.
    cast(null as varchar) as semantics,
    cast(null as varchar) as source_artifact
where false;
