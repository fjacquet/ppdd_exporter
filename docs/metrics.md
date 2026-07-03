# Metrics reference

All metrics are gauges. Per-appliance metrics carry a `system` label;
`ppdd_collector_up{collector}` is 1 when a module collected cleanly, 0 otherwise. The
exporter-level metrics below are the exception — they describe the exporter process, not
a DD, so they have no `system` label.

## exporter
- `ppdd_exporter_build_info{version}` (constant 1; the running exporter version is in the
  `version` label — scrape it to confirm which build is deployed)

## capacity
- `ppdd_filesystem_total_bytes` / `ppdd_filesystem_used_bytes` / `ppdd_filesystem_available_bytes` (from `/system`)
- `ppdd_compression_factor` (from `/system`)
- `ppdd_filesystem_cleaning_running` (1 while GC runs; from `/file-systems` `fs_clean_status`)

## mtrees (labels: mtree)
- `ppdd_mtree_logical_used_bytes` / `ppdd_mtree_compression_factor` (per-MTree v2.0 stats, latest epoch)
- `ppdd_mtree_degraded` (1 if degraded) / `ppdd_mtree_retention_lock_enabled` (1 if retention lock active)
- `ppdd_mtree_physical_used_bytes` (post-comp written), `ppdd_mtree_quota_soft_limit_bytes` / `ppdd_mtree_quota_hard_limit_bytes` (from `quota_config`)

## mtree_replication (labels: source, destination; +state on the state metric)
- `ppdd_mtree_replication_state{state}` (1 for the current state; `state` ∈ CONNECTING|UNINITIALIZED|INITIALIZING|NORMAL|RESYNCING|RECOVERING)
- `ppdd_mtree_replication_connected` (1 if connected)
- `ppdd_mtree_replication_need_resync` (1 if a resync is required)
- `ppdd_mtree_replication_enabled` (1 if the context is enabled)

## file_replication (labels: context; +status on the status metric)
- `ppdd_file_replication_network_bytes`
- `ppdd_file_replication_logical_replicated_bytes`
- `ppdd_file_replication_active_files`
- `ppdd_file_replication_status{status}` (1 for the current status; `status` ∈ completed|error|warning|unknown)

## health
- `ppdd_disk_failed{disk}` (1 if failed; `disk` is the `enclosure.slot` device path, e.g. `1.1`, which is unique across shelves — the DD `id` is not)
- `ppdd_alerts_active{severity, class}` (active alerts only, `is_active=true`; `severity` is the DD enum casing, e.g. `CRITICAL`)
- `ppdd_system_cpu_percent`
- `ppdd_system_read_bytes_per_second` / `ppdd_system_write_bytes_per_second`
