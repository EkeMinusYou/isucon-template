-- 全ブロック共通の前提。analysisctl が必ず最初に読む。
--
-- schema/ は collector ごとに 1 ブロックへ分けてある。DuckDB はビューを作る
-- 時点で glob を検証し、1 件もマッチしないとエラーになるため、1 RUN だけを
-- 差分取り込みするときは「その RUN に実在する collector のブロックだけ」を
-- 連結して読み込む。どのブロックを読むかは sources.yaml の宣言が決める。

-- 読み取り対象の RUN。既定は全 RUN。差分取り込み時は analysisctl が 1 RUN に絞る。
-- getenv は未設定時に空文字を返すので nullif で潰す。
set variable run_glob = coalesce(nullif(getenv('ISUCON_RUN_GLOB'), ''), 'runs/*');

-- RUN index. scores.tsv owns the role history, while run.json owns the declared
-- adoption control. MySQL artifacts also use this view to resolve their host.
-- ここだけは常に全 RUN を読む (差分取り込みでも構成の参照先が要る)。
create or replace view runs as
with score_rows as (
    select *
    from read_csv('runs/scores.tsv', delim = '\t', header = true,
                  types = {'run_id': 'VARCHAR', 'score': 'BIGINT'})
), comparison_context as (
    select
        json_extract_string(content, '$.run_id') as run_id,
        json_extract_string(content, '$.comparison.run_id') as comparison_run_id,
        json_extract_string(content, '$.comparison.status') as comparison_status
    from read_text('runs/*/run.json')
)
select
    target.run_id,
    target.score,
    strptime(target.run_id, '%Y%m%d-%H%M%S') as started_at,
    string_split(target.app, ',')             as app_hosts,
    string_split(target.nginx, ',')           as nginx_hosts,
    target.mysql                              as mysql_host,
    string_split(target.app_traffic, ',')     as app_traffic_hosts,
    case when context.comparison_status = 'compatible'
         then target.score - control.score
         else null end                        as score_delta
from score_rows target
left join comparison_context context using (run_id)
left join score_rows control on control.run_id = context.comparison_run_id;
