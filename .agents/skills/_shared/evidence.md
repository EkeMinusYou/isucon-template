# Evidence

Evidenceは判断根拠であり、Backlogの作業種類ではない。

## 優先するEvidence

1. `docs/official/`の当日マニュアル、アプリケーション仕様、API定義
2. 現行コード、schema、設定、Taskfileの役割定義
3. 対象RUNの`run.json`と保存済み成果物
4. 比較可能なRUN横断値
5. 必要な範囲の読み取り専用実効環境

公式仕様と他の資料が矛盾したら公式仕様を優先する。競技中は外部記事を公式仕様の代わりにせず、
許可された範囲と必要性を確認してから参照する。

## RUNの扱い

- `run.json`のphase、役割、source、APPLIED snapshot、artifact statusを先に確認する。
- 採否では`passed=true`と既知のscoreを確認し、宣言済みcontrolがある場合は`comparison.status=compatible`と
  そのRUNとの差分を使う。`scores.tsv`の単なる直前行をcontrolにしない。
- これらの採否条件を例外的に上書きする`task pass FORCE=true`は、理由を明示できる場合だけ使う。
  finalized状態、APPLIED snapshot、カード定義一致はforceでも必須とする。
- 採用判断はBacklog SQLiteのadoption eventを正本とし、そこに固定されたscore・control・manifest hashを使う。
  `outcomes.tsv`は派生出力として扱う。
- 欠損成果物を0として扱わない。
- 比較RUNは役割・source・計測窓・負荷条件の互換性を確認する。
- 時間、仕事量、待ち、成功数、失敗数、得点を混同しない。単位と母数を併記する。
- 相関と因果を分け、コードまたは仕様から効果の経路を説明する。
- correctness、5xx、OOM、panic、service再起動の確認には、同じload windowのapp journal、nginx error、
  kernel/OOM成果物も使う。

既存Evidenceで方向と安全性を説明できるなら、追加計測をREADYの前提にしない。判断不能なら不足をConstraintまたはInterventionのHistoryへ残す。標準計測基盤の新設・変更が本当に必要な場合は、Backlogカードへ偽装せずユーザー指定の別タスクとして扱う。
