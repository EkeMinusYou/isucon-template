-- measurectl manifest が書く run.json。走行 1 回分の記録をまとめたもので、
-- scores.tsv には無い「アプリの commit」「回収物の状態」まで持つ。
-- scores.tsv 由来の runs と違い、ベンチ失敗で記録されなかった走行も載る。
create or replace view manifests as
select
    json_extract_string(content, '$.run_id') as run_id,
    try_cast(json_extract_string(content, '$.score') as bigint) as score,
    try_cast(json_extract_string(content, '$.passed') as boolean) as passed,
    try_cast(json_extract_string(content, '$.started_at') as timestamp) as started_at,
    json_extract_string(content, '$.source.commit') as commit,
    try_cast(json_extract_string(content, '$.source.dirty') as boolean) as dirty,
    try_cast(json_extract(content, '$.roles.app') as varchar[]) as app_hosts,
    try_cast(json_extract(content, '$.roles.app_traffic') as varchar[]) as app_traffic_hosts,
    try_cast(json_extract(content, '$.roles.nginx') as varchar[]) as nginx_hosts,
    json_extract_string(content, '$.roles.entry') as entry_host,
    json_extract_string(content, '$.roles.mysql') as mysql_host,
    json_extract(content, '$.roles.additional') as additional_roles,
    try_cast(json_extract_string(content, '$.preflight.collector_clean') as boolean) as collector_clean,
    json_extract_string(content, '$.comparison.run_id') as compare_run_id,
    json_extract_string(content, '$.comparison.status') as comparison_status,
    try_cast(json_extract(content, '$.comparison.reasons') as varchar[]) as comparison_reasons,
    try_cast(json_extract(content, '$.comparison.card_delta') as varchar[]) as comparison_card_delta,
    try_cast(json_extract(content, '$.comparison.role_delta') as varchar[]) as comparison_role_delta,
    try_cast(json_extract_string(content, '$.load_window.started_at') as timestamptz) as load_started_at,
    try_cast(json_extract_string(content, '$.load_window.ended_at') as timestamptz) as load_ended_at,
    try_cast(json_extract_string(content, '$.load_window.duration_ms') as bigint) as load_duration_ms,
    json_extract_string(content, '$.load_window.source') as load_window_source,
    json_extract_string(content, '$.load_window.status') as load_window_status,
    json_extract_string(content, '$.load_window.reason') as load_window_reason,
    try_cast(json_extract_string(content, '$.raw_bytes') as bigint) as raw_bytes
from read_text(getvariable('run_glob') || '/run.json')
where try_cast(json_extract_string(content, '$.schema_version') as integer) = 4;

-- 回収物の 1 行 1 件。status が ok 以外の RUN を拾えば、
-- どの計測が欠けたまま解析していたかが分かる。
create or replace view artifacts as
select
    json_extract_string(r.content, '$.run_id') as run_id,
    json_extract_string(a.value, '$.name') as name,
    try_cast(json_extract_string(a.value, '$.bytes') as bigint) as bytes,
    json_extract_string(a.value, '$.status') as status,
    json_extract_string(a.value, '$.reason') as reason,
    try_cast(json_extract_string(a.value, '$.quality.expected') as boolean) as quality_expected,
    json_extract_string(a.value, '$.quality.status') as quality_status,
    try_cast(json_extract_string(a.value, '$.quality.rows') as bigint) as quality_rows,
    try_cast(json_extract_string(a.value, '$.quality.in_window_samples') as bigint) as quality_in_window_samples,
    try_cast(json_extract_string(a.value, '$.quality.expected_samples') as bigint) as quality_expected_samples,
    try_cast(json_extract_string(a.value, '$.quality.window_coverage_pct') as double) as quality_window_coverage_pct,
    try_cast(json_extract_string(a.value, '$.quality.max_gap_ms') as bigint) as quality_max_gap_ms,
    try_cast(json_extract_string(a.value, '$.quality.monotonic') as boolean) as quality_monotonic,
    try_cast(json_extract_string(a.value, '$.quality.finite') as boolean) as quality_finite,
    json_extract_string(a.value, '$.quality.reason') as quality_reason
from read_text(getvariable('run_glob') || '/run.json') r,
     json_each(coalesce(json_extract(r.content, '$.artifacts'), json('[]'))) a
where try_cast(json_extract_string(r.content, '$.schema_version') as integer) = 4;

-- 現行run.jsonのbefore-bench時点でAPPLIEDだったカード。
create or replace view run_applied_cards as
select
    json_extract_string(r.content, '$.run_id')                           as run_id,
    try_cast(json_extract_string(r.content, '$.backlog_snapshot.captured_at') as timestamp) as captured_at,
    try_cast(json_extract_string(r.content, '$.backlog_snapshot.revision') as bigint)       as backlog_revision,
    json_extract_string(c.value, '$.id')                                 as card_id,
    json_extract_string(c.value, '$.status')                             as status,
    try_cast(json_extract_string(c.value, '$.version') as integer)       as card_version,
    json_extract_string(c.value, '$.title')                              as title,
    json_extract_string(c.value, '$.change_boundary_hash')               as change_boundary_hash,
    json_extract_string(c.value, '$.decision_hash')                      as decision_hash
from read_text(getvariable('run_glob') || '/run.json') r,
     json_each(coalesce(json_extract(r.content, '$.backlog_snapshot.cards'), json('[]'))) c
where try_cast(json_extract_string(r.content, '$.schema_version') as integer) = 4
  and try_cast(json_extract_string(r.content, '$.backlog_snapshot.schema_version') as integer) = 3;
