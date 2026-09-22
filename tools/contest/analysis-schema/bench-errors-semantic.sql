-- bench.log のエラー行を意味ビューにする、当日のベンチマーカー書式に依存するschema。
-- 生テキストの取り込みは汎用側（tools/analysis/schema/bench-errors.sql が
-- bench_error_logs へ入れる）が担当し、ここは行の解釈だけを持つ。
--
-- プレースホルダー。setupで当日の出力を見てから、下の3ビューの本体を書き換える。
-- ビュー名と列はdashboard（tools/dashboard/server/bench_errors.go）とREADMEの契約なので
-- 変えない。書き換えるまでは、いずれも0行を返す。
-- 書式に依存するテストは tools/dashboard/server/<contest>_test.go へ置き、
-- tools/template/paths.txt の [contest] へ加える。
create table if not exists bench_error_logs (
    run_id varchar, content varchar, source_artifact varchar
);

-- 報告された1エラーにつき1行。発生した全エラーではない場合があるので、
-- time_semantics にその行の時刻の意味を入れる（例: 完了時のまとめなら 'completion_summary'）。
create or replace view bench_errors as
select
    cast(null as varchar) as run_id,
    cast(null as bigint) as line_number,
    cast(null as time) as reported_time,
    cast(null as bigint) as error_index,
    cast(null as varchar) as message,
    cast(null as varchar) as time_semantics,
    cast(null as varchar) as source_artifact
where false;

-- 発生時刻を持つ構造化ログ行だけをここへ入れる。まとめ出力の時刻はここへ入れない。
create or replace view bench_warning_events as
select
    cast(null as varchar) as run_id,
    cast(null as timestamptz) as occurred_at
where false;

-- 報告時点の累積エラー件数。個々のエラーの発生時刻ではない。
-- elapsed_s は負荷走行開始からの経過秒。
create or replace view bench_error_counts as
select
    cast(null as varchar) as run_id,
    cast(null as bigint) as line_number,
    cast(null as double) as elapsed_s,
    cast(null as bigint) as error_count
where false;
