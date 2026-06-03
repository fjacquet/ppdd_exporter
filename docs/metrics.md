# Metrics reference

All metrics are gauges and carry a `system` label. `ppdd_collector_up{collector}` is 1
when a module collected cleanly, 0 otherwise.

## capacity
- `ppdd_filesystem_total_bytes` / `ppdd_filesystem_used_bytes` / `ppdd_filesystem_available_bytes` (from `/system`)
- `ppdd_compression_factor` (from `/system`)
- _Provisional (no source in the 7.3 API guide; best-effort from `/file-system`):_
  `ppdd_compression_global_factor` / `ppdd_compression_local_factor` / `ppdd_compression_total_factor`,
  `ppdd_filesystem_cleaning_running` (1 while GC runs)

## mtrees (labels: mtree)
- `ppdd_mtree_logical_used_bytes` / `ppdd_mtree_compression_factor` (per-MTree v2.0 stats, latest epoch)
- `ppdd_mtree_degraded` (1 if degraded) / `ppdd_mtree_retention_lock_enabled` (1 if retention lock active)
- _Provisional:_ `ppdd_mtree_physical_used_bytes` (mapped to `post_comp_written`),
  `ppdd_mtree_quota_soft_limit_bytes` / `ppdd_mtree_quota_hard_limit_bytes`

## replication (labels: source, destination; +state on the state metric)
- `ppdd_replication_state{state}` (1 for the active state)
- `ppdd_replication_sync_lag_seconds`
- `ppdd_replication_precomp_bytes_remaining`
- `ppdd_replication_throughput_bytes_per_second`

## health
- `ppdd_disk_failed{disk}` (1 if failed)
- `ppdd_alerts_active{severity, class}` (active alerts only; fetched with `is_active=true`)
- `ppdd_system_cpu_percent`
- `ppdd_system_read_bytes_per_second` / `ppdd_system_write_bytes_per_second`
