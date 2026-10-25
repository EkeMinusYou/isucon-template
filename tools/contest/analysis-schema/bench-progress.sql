-- 参加者向けベンチ出力が負荷中に定期的に出す進捗行を解釈する、当日のベンチマーカー書式に依存するschema。
-- 負荷の増減とその理由、利用者の増減、不満などの推移から、得点を制約している要因を読む。
--
-- プレースホルダー。setupで当日の出力を見てから、下のビューの本体を書き換える。
-- 書き換えるまでは0行を返す。
create or replace view bench_progress as
select
    cast(null as varchar) as run_id,
    cast(null as bigint) as line_number,
    -- 負荷走行開始からの経過秒。
    cast(null as double) as elapsed_s,
    -- 進捗の種類（例: 'users_registered'、'load_increased'）。
    cast(null as varchar) as metric,
    cast(null as double) as value,
    -- value の意味（例: 'cumulative_count'、'event_count'、'reported_percent'）。
    cast(null as varchar) as semantics,
    cast(null as varchar) as source_artifact
where false;
