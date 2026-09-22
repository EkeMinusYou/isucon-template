-- 全ブロック共通の前提。analysisctl が必ず最初に読む。
--
-- schema/ は collector ごとに 1 ブロックへ分けてある。DuckDB はビューを作る
-- 時点で glob を検証し、1 件もマッチしないとエラーになるため、1 RUN だけを
-- 差分取り込みするときは「その RUN に実在する collector のブロックだけ」を
-- 連結して読み込む。どのブロックを読むかは sources.yaml の宣言が決める。

-- 読み取り対象の RUN。既定は全 RUN。差分取り込み時は analysisctl が 1 RUN に絞る。
-- getenv は未設定時に空文字を返すので nullif で潰す。
set variable run_glob = coalesce(nullif(getenv('ISUCON_RUN_GLOB'), ''), 'runs/*');

-- RUN index. scores.tsv owns the role history, while run.json owns the run
-- metadata. MySQL artifacts also use this view to resolve their host.
-- ここだけは常に全 RUN を読む (差分取り込みでも構成の参照先が要る)。
create or replace view runs as
with score_rows as (
    select *
    from read_csv('runs/scores.tsv', delim = '\t', header = true,
                  types = {'run_id': 'VARCHAR', 'score': 'BIGINT'})
)
select
    target.run_id,
    target.score,
    strptime(target.run_id, '%Y%m%d-%H%M%S') as started_at,
    string_split(target.app, ',')             as app_hosts,
    string_split(target.nginx, ',')           as nginx_hosts,
    target.mysql                              as mysql_host,
    string_split(target.app_traffic, ',')     as app_traffic_hosts
from score_rows target;

-- slp と pt-query-digest はリテラルを N へ正規化するが VALUES・IN・CASE WHEN の反復は
-- 畳まないので 1 文が 37 万文字まで伸びる。DuckDB はそこまで大きい文字列の圧縮を
-- あきらめるため、畳まないと queries だけで DB の半分 (3.3 GiB) を占める。
create or replace macro fold_repeated_sql(sql) as
    regexp_replace(
        regexp_replace(
            regexp_replace(sql, '(VALUES\s*\([^)]*\))(\s*,\s*\([^)]*\))+', '\1 /*...*/', 'gi'),
            'IN\s*\(\s*(N|\?)(\s*,\s*(N|\?))+\s*\)', 'IN (/*...*/)', 'gi'),
        '(WHEN\s+\S+\s+THEN\s+\S+)(\s+WHEN\s+\S+\s+THEN\s+\S+)+', '\1 /*...*/', 'gi');
