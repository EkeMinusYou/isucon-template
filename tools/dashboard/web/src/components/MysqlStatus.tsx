import type { MysqlPoint, MysqlResponse } from '../api'
import { CategoryDivider } from './ChartCard'
import { EmptyState } from './EmptyState'
import { groupByCategory, TinyLineChart, type MetricDef } from './TinyLineChart'

const METRICS: MetricDef<MysqlPoint>[] = [
  { key: 'threads_connected', label: '接続中スレッド数', unit: '', category: '接続', decimals: 0 },
  { key: 'threads_running', label: '実行中スレッド数', unit: '', category: '接続', decimals: 0 },
  { key: 'threads_cached', label: 'キャッシュ済スレッド数', unit: '', category: '接続', decimals: 0 },
  { key: 'threads_created_per_sec', label: 'スレッド作成', unit: '/s', category: '接続', decimals: 2 },
  { key: 'connections_per_sec', label: '接続数', unit: '/s', category: '接続', decimals: 2 },
  { key: 'aborted_connects_per_sec', label: '接続失敗', unit: '/s', category: '接続', decimals: 3 },

  { key: 'questions_per_sec', label: 'クエリ数', unit: '/s', category: 'クエリ' },
  { key: 'com_select_per_sec', label: 'SELECT', unit: '/s', category: 'クエリ' },
  { key: 'com_insert_per_sec', label: 'INSERT', unit: '/s', category: 'クエリ' },
  { key: 'com_update_per_sec', label: 'UPDATE', unit: '/s', category: 'クエリ' },
  { key: 'com_delete_per_sec', label: 'DELETE', unit: '/s', category: 'クエリ' },
  { key: 'bytes_received_per_sec', label: '受信バイト数', unit: 'KB/s', category: 'クエリ', scale: 1 / 1000 },
  { key: 'bytes_sent_per_sec', label: '送信バイト数', unit: 'KB/s', category: 'クエリ', scale: 1 / 1000 },

  { key: 'buffer_pool_hit_pct', label: 'バッファプールヒット率', unit: '%', category: 'InnoDB', domain: [0, 100] },
  { key: 'buffer_pool_pages_total', label: 'バッファプール総ページ数', unit: '', category: 'InnoDB', decimals: 0 },
  { key: 'buffer_pool_pages_free', label: 'バッファプール空きページ数', unit: '', category: 'InnoDB', decimals: 0 },
  { key: 'buffer_pool_pages_dirty', label: 'バッファプールdirtyページ数', unit: '', category: 'InnoDB', decimals: 0 },
  { key: 'buffer_pool_read_requests_per_sec', label: 'バッファプール読み取り要求', unit: '/s', category: 'InnoDB' },
  { key: 'buffer_pool_reads_per_sec', label: 'バッファプールディスク読み取り', unit: '/s', category: 'InnoDB', decimals: 2 },
  { key: 'innodb_log_waits_per_sec', label: 'InnoDBログ待ち', unit: '/s', category: 'InnoDB', decimals: 3 },

  { key: 'row_lock_current_waits', label: '行ロック待ち中', unit: '', category: 'ロック', decimals: 0 },
  { key: 'row_lock_waits_per_sec', label: '行ロック待ち発生', unit: '/s', category: 'ロック', decimals: 3 },
  { key: 'row_lock_time_ms_per_sec', label: '行ロック待ち時間', unit: 'ms/s', category: 'ロック', decimals: 2 },

  { key: 'created_tmp_tables_per_sec', label: '一時テーブル作成', unit: '/s', category: '一時テーブル', decimals: 2 },
  { key: 'created_tmp_disk_tables_per_sec', label: 'ディスク一時テーブル作成', unit: '/s', category: '一時テーブル', decimals: 3 },
  { key: 'tmp_disk_ratio_pct', label: 'ディスク一時テーブル比率', unit: '%', category: '一時テーブル', domain: [0, 100] },
]

export function MysqlStatus({ data }: { data: MysqlResponse }) {
  if (!data.available || data.series.length === 0) {
    return (
      <EmptyState
        title="mysql-status.tsv がまだありません"
        detail="このRUNは計測パイプライン更新前のRUNの可能性があります"
      />
    )
  }

  return (
    <>
      {groupByCategory(METRICS).map(([category, defs]) => (
        <div key={category} className="mb-5">
          <CategoryDivider label={category} />
          <div className="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-3">
            {defs.map((m) => (
              <TinyLineChart key={String(m.key)} series={data.series} metric={m} />
            ))}
          </div>
        </div>
      ))}
    </>
  )
}
