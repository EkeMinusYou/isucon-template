-- nginx worker の on-CPU profile (<host>-nginx-oncpu.folded)。1 行は
-- "comm;root;...;leaf <samples>"。leaf が自己時間、frames に含まれる関数が包含時間になる。
-- TLS ハンドシェイクなどの割合は list_has_any(frames, [...]) で数える。
create or replace view nginx_oncpu_stacks as
with lines as (
    select
        regexp_extract(filename, 'runs/([^/]+)/', 1) as run_id,
        regexp_extract(filename, '/([^/]+)-nginx-oncpu\.folded$', 1) as host,
        unnest(string_split(content, chr(10))) as line
    from read_text(getvariable('run_glob') || '/*-nginx-oncpu.folded')
), stacks as (
    select
        run_id,
        host,
        regexp_extract(line, '^(.*) ([0-9]+)$', 1) as stack,
        try_cast(regexp_extract(line, '^(.*) ([0-9]+)$', 2) as bigint) as samples
    from lines
    where line <> ''
), framed as (
    select
        run_id,
        host,
        stack,
        samples,
        -- perf script のシンボルは "func+0x1a" なので関数名だけを残す。
        list_transform(string_split(stack, ';'), lambda f: regexp_replace(f, '\+0x[0-9a-fA-F]+$', ''))
            as frames
    from stacks
    where samples is not null
)
select
    run_id,
    host,
    stack,
    frames,
    frames[-1] as leaf_function,
    samples,
    samples / sum(samples) over (partition by run_id, host) as sample_share
from framed;
