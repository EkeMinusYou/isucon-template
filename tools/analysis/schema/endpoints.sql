-- alp の API 別集計 (alp.json)。パス単位の応答時間を RUN 横断で追える。
--
-- alp の json 出力は「1 行目が列名の配列、以降が値の配列」という形なので、
-- 各ファイルのヘッダ行から列位置を引いて取り出す。列の順序や増減を
-- alp.yml 側で変えても、この定義は追従する。
create or replace view endpoints as
with rws as (
    select
        regexp_extract(filename, 'runs/([^/]+)/', 1) as run_id,
        filename,
        json
    from read_json(getvariable('run_glob') || '/alp.json', filename = true)
),
hdr as (
    select
        filename,
        [json_extract_string(json, '$[' || (i - 1) || ']') for i in range(1, len(json) + 1)] as cols
    from rws
    where try_cast(json_extract_string(json, '$[0]') as bigint) is null
),
val as (
    select r.run_id, r.json, h.cols
    from rws r
    join hdr h using (filename)
    where try_cast(json_extract_string(r.json, '$[0]') as bigint) is not null
)
select
    run_id,
    json_extract_string(json, '$[' || (list_position(cols, 'method') - 1) || ']')          as method,
    json_extract_string(json, '$[' || (list_position(cols, 'uri')    - 1) || ']')          as uri,
    json_extract_string(json, '$[' || (list_position(cols, 'count')  - 1) || ']')::bigint  as count,
    json_extract_string(json, '$[' || (list_position(cols, 'sum')    - 1) || ']')::double  as sum_time_sec,
    json_extract_string(json, '$[' || (list_position(cols, 'avg')    - 1) || ']')::double  as avg_time_sec,
    json_extract_string(json, '$[' || (list_position(cols, 'max')    - 1) || ']')::double  as max_time_sec,
    json_extract_string(json, '$[' || (list_position(cols, 'p90')    - 1) || ']')::double  as p90_time_sec,
    json_extract_string(json, '$[' || (list_position(cols, 'p99')    - 1) || ']')::double  as p99_time_sec,
    json_extract_string(json, '$[' || (list_position(cols, 'sum_body') - 1) || ']')::double as sum_body_bytes,
    json_extract_string(json, '$[' || (list_position(cols, 'avg_body') - 1) || ']')::double as avg_body_bytes,
    json_extract_string(json, '$[' || (list_position(cols, '2xx')    - 1) || ']')::bigint  as status_2xx,
    json_extract_string(json, '$[' || (list_position(cols, '3xx')    - 1) || ']')::bigint  as status_3xx,
    json_extract_string(json, '$[' || (list_position(cols, '4xx')    - 1) || ']')::bigint  as status_4xx,
    json_extract_string(json, '$[' || (list_position(cols, '5xx')    - 1) || ']')::bigint  as status_5xx
from val;
