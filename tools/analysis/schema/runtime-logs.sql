-- Raw fault logs keep their originating host and artifact for drill-down.
create or replace view runtime_logs_raw as
select regexp_extract(filename, 'runs/([^/]+)/', 1) as run_id,
       regexp_extract(filename, '/([^/]+)-(?:app-journal|service-journal|kernel|oom)\.log$', 1) as host,
       regexp_extract(filename, '/([^/]+)$', 1) as source_artifact, content
-- Read only declared diagnostic logs.
from read_text([
    getvariable('run_glob') || '/*-app-journal.log',
    getvariable('run_glob') || '/*-service-journal.log',
    getvariable('run_glob') || '/*-kernel.log',
    getvariable('run_glob') || '/*-oom.log'
]);
