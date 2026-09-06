-- Normalized pprof protobuf emitted by analysisctl/internal/pprofimport. The .rows files
-- are transient index-build inputs, not declared RUN artifacts. All numeric
-- time columns in the legacy profile_* views are goroutine wall-clock seconds.
-- The pprof_* views below retain every sample type with explicit units.
create or replace view profile_metadata_raw as
select * from read_csv(
    getenv('ISUCON_PROFILE_DIR') || '/profile-metadata.rows',
    delim = '\t', header = true,
    columns = {
        'run_id':'VARCHAR', 'host':'VARCHAR', 'source':'VARCHAR',
        'profile_sha256':'VARCHAR', 'sample_type':'VARCHAR', 'sample_unit':'VARCHAR',
        'duration_seconds':'DOUBLE', 'total_wall_seconds':'DOUBLE', 'period_seconds':'DOUBLE',
        'sample_count':'BIGINT', 'incomplete_samples':'BIGINT', 'embedded_function_names':'BIGINT',
        'external_binary_used':'BOOLEAN', 'binary_path':'VARCHAR', 'binary_sha256':'VARCHAR',
        'binary_matches_run':'VARCHAR'
    }
);

create or replace view profile_samples_raw as
select * from read_csv(
    getenv('ISUCON_PROFILE_DIR') || '/profile-samples.rows',
    delim = '\t', header = true,
    columns = {'run_id':'VARCHAR', 'host':'VARCHAR', 'sample_id':'BIGINT', 'wall_seconds':'DOUBLE', 'stack_depth':'BIGINT'}
);

create or replace view profile_frames_raw as
select * from read_csv(
    getenv('ISUCON_PROFILE_DIR') || '/profile-frames.rows',
    delim = '\t', header = true,
    columns = {'run_id':'VARCHAR', 'host':'VARCHAR', 'sample_id':'BIGINT', 'depth':'BIGINT', 'function':'VARCHAR', 'file':'VARCHAR', 'line':'BIGINT'}
);

create or replace view profile_functions_raw as
select * from read_csv(
    getenv('ISUCON_PROFILE_DIR') || '/profile-functions.rows',
    delim = '\t', header = true,
    columns = {'run_id':'VARCHAR', 'host':'VARCHAR', 'function':'VARCHAR', 'file':'VARCHAR', 'line':'BIGINT', 'flat_wall_seconds':'DOUBLE', 'cumulative_wall_seconds':'DOUBLE'}
);

create or replace view profile_edges_raw as
select * from read_csv(
    getenv('ISUCON_PROFILE_DIR') || '/profile-edges.rows',
    delim = '\t', header = true,
    columns = {'run_id':'VARCHAR', 'host':'VARCHAR', 'caller':'VARCHAR', 'callee':'VARCHAR', 'wall_seconds':'DOUBLE', 'sample_occurrences':'BIGINT'}
);

-- Each capture is identified by (run_id, host, source); each metric additionally
-- requires sample_type. Zero-sample profiles still have metadata rows.
create or replace view pprof_metadata_raw as
select * from read_csv(getenv('ISUCON_PROFILE_DIR') || '/pprof-metadata.rows',
    delim = '\t', header = true, columns = {
        'run_id':'VARCHAR', 'host':'VARCHAR', 'source':'VARCHAR', 'profile_type':'VARCHAR',
        'sample_type':'VARCHAR', 'sample_unit':'VARCHAR', 'value_unit':'VARCHAR',
        'profile_sha256':'VARCHAR', 'time_unix_nano':'BIGINT', 'duration_seconds':'DOUBLE',
        'total_value':'DOUBLE', 'period_seconds':'DOUBLE', 'sample_count':'BIGINT',
        'incomplete_samples':'BIGINT', 'embedded_function_names':'BIGINT', 'period':'BIGINT', 'period_unit':'VARCHAR'
    });

create or replace view pprof_samples_raw as
select * from read_csv(getenv('ISUCON_PROFILE_DIR') || '/pprof-samples.rows',
    delim = '\t', header = true, columns = {
        'run_id':'VARCHAR', 'host':'VARCHAR', 'source':'VARCHAR', 'profile_type':'VARCHAR',
        'sample_type':'VARCHAR', 'sample_unit':'VARCHAR', 'value_unit':'VARCHAR',
        'sample_id':'BIGINT', 'value':'DOUBLE', 'raw_value':'BIGINT', 'stack_depth':'BIGINT'
    });

create or replace view pprof_frames_raw as
select * from read_csv(getenv('ISUCON_PROFILE_DIR') || '/pprof-frames.rows',
    delim = '\t', header = true, columns = {
        'run_id':'VARCHAR', 'host':'VARCHAR', 'source':'VARCHAR', 'profile_type':'VARCHAR',
        'sample_type':'VARCHAR', 'sample_unit':'VARCHAR', 'value_unit':'VARCHAR',
        'sample_id':'BIGINT', 'depth':'BIGINT', 'function':'VARCHAR', 'file':'VARCHAR', 'line':'BIGINT'
    });

create or replace view pprof_functions_raw as
select * from read_csv(getenv('ISUCON_PROFILE_DIR') || '/pprof-functions.rows',
    delim = '\t', header = true, columns = {
        'run_id':'VARCHAR', 'host':'VARCHAR', 'source':'VARCHAR', 'profile_type':'VARCHAR',
        'sample_type':'VARCHAR', 'sample_unit':'VARCHAR', 'value_unit':'VARCHAR',
        'function':'VARCHAR', 'file':'VARCHAR', 'line':'BIGINT',
        'flat_value':'DOUBLE', 'cumulative_value':'DOUBLE'
    });

create or replace view pprof_edges_raw as
select * from read_csv(getenv('ISUCON_PROFILE_DIR') || '/pprof-edges.rows',
    delim = '\t', header = true, columns = {
        'run_id':'VARCHAR', 'host':'VARCHAR', 'source':'VARCHAR', 'profile_type':'VARCHAR',
        'sample_type':'VARCHAR', 'sample_unit':'VARCHAR', 'value_unit':'VARCHAR',
        'caller':'VARCHAR', 'callee':'VARCHAR', 'value':'DOUBLE', 'sample_occurrences':'BIGINT'
    });
