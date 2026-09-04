-- 全ブロック共通の前提。analysisctl が必ず最初に読む。
--
-- schema/ は collector ごとに 1 ブロックへ分けてある。DuckDB はビューを作る
-- 時点で glob を検証し、1 件もマッチしないとエラーになるため、1 RUN だけを
-- 差分取り込みするときは「その RUN に実在する collector のブロックだけ」を
-- 連結して読み込む。どのブロックを読むかは sources.yaml の宣言が決める。

-- 読み取り対象の RUN。既定は全 RUN。差分取り込み時は analysisctl が 1 RUN に絞る。
-- getenv は未設定時に空文字を返すので nullif で潰す。
set variable run_glob = coalesce(nullif(getenv('ISUCON_RUN_GLOB'), ''), 'runs/*');

-- RUN の索引。役割構成は scores.tsv が唯一の記録なので、ホスト名が
-- ファイル名に出ない MySQL collector の host 解決にも使う。
-- ここだけは常に全 RUN を読む (差分取り込みでも構成の参照先が要る)。
create or replace view runs as
select
    run_id,
    score,
    strptime(run_id, '%Y%m%d-%H%M%S')      as started_at,
    string_split(app, ',')                 as app_hosts,
    string_split(nginx, ',')               as nginx_hosts,
    mysql                                  as mysql_host,
    string_split(app_traffic, ',')         as app_traffic_hosts,
    score - lag(score) over (order by run_id) as score_delta
from read_csv('runs/scores.tsv', delim = '\t', header = true,
              types = {'run_id': 'VARCHAR', 'score': 'BIGINT'});
