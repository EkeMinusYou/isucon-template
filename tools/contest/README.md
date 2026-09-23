# Contest-specific assets

この競技だけで成立するものを集める。`tools/`の他のディレクトリは、ここを参照する側であっても
競技非依存に保つ。次の競技を始めるときは、このディレクトリの中身を入れ替えるか削除する。

ここへ置く基準は「前提が次の競技でも無条件に成り立つか」である。成り立たないもの、
採る解法次第でしか成り立たないものがここへ入る。固有名詞を含むかどうかでは判断しない。
汎用coreのアプリ固有adapterも、差し替え対象を一箇所へ集めるためここへ置く。

| ファイル | 読む側 | 内容 |
| --- | --- | --- |
| `bench-patterns.json` | `tools/bench-output` | ベンチ出力のscore・合否正規表現。coreは形式を持たない |
| `user-transition-routes.json` | `tools/user-transition-metrics` | Cookie列と正規化routeの宣言 |
| `analysis-schema/` | `tools/analysis/sources.yaml` | ベンチログの解釈をDuckDBのviewにするschema。エラー行の意味ビューとscore集計 |
| `smoke/` | `task setup-smoke` | ベンチを使わず計測経路を確認する、当日のセッションフローのGo実装 |
| `deployments.yaml` | `tools/deployctl` | 汎用graphをincludeし、解法固有のdeploymentとplanを置き換えるoverlay。追加seedもここに置く |

templateに置いてあるのは`bench-patterns.json`、`user-transition-routes.json`、
`analysis-schema/bench-errors-semantic.sql`、`smoke/`のプレースホルダーだけである。setupで当日の形式へ書き換え、
必要になった解法固有の宣言だけをこのディレクトリへ足す。`Taskfile.contest.yml`のhookタスク
（`prepare-db-assets`、`gen-contest-routing`）も同じ考え方で、該当する処理がなければ空のままにする。

`analysis-schema/`のviewは、dashboardとREADMEが名前と列で参照する契約である。書式に合わせて
本体を書き換えるときも、view名と列は変えない。書き換えなければ0行を返す。

## ここへ置けないもの

- Goのテストは対象パッケージと同じディレクトリに置く必要がある。競技固有のテストは
  `tools/analysisctl/<contest>_test.go`のようにファイル名で競技固有と分かるようにし、
  `tools/template/paths.txt`の`[contest]`へ加えて、adapterを差し替えるときに一緒に削除する。
  当日のベンチ出力を解析できたことの確認も、`tools/dashboard/server/<contest>_test.go`のように
  こちら側へ置く。汎用テストは、書式に依存しない経路（成果物の有無とJSON契約）だけを見る。
- `tools/analysis/sources.yaml`と`tools/deployctl/deployments.yaml`に残る参照・値 — 構造は汎用なので
  ファイルごとは分けられない。該当行に`# contest:`コメントを付け、
  `grep -rn '# contest:' tools/`で一覧できるようにする。
