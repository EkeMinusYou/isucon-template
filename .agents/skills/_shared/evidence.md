# Evidence

Evidenceの選択・比較・欠損・因果の扱いを定める。Backlog上の判断条件は[workflow](../../../tools/backlog/backlog-workflow.md)を参照する。

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
- RUN間の比較には対象manifestが宣言した`compatible`なcontrolを使う。`scores.tsv`の単なる直前行をcontrolにしない。比較不能理由を残し、対象RUN単独で確認できる事実と分ける。
- 欠損成果物を0として扱わない。
- 比較RUNは役割・source・計測窓・負荷条件の互換性を確認する。
- 時間、仕事量、待ち、成功数、失敗数、得点を混同しない。単位と母数を併記する。
- 相関と因果を分け、コードまたは仕様から効果の経路を説明する。
- correctness、5xx、OOM、panic、service再起動の確認には、同じload windowのapp journal、nginx error、
  kernel/OOM成果物も使う。

READY前の追加計測、不足情報の記録、標準計測基盤の変更範囲は[workflowのEvidence policy](../../../tools/backlog/backlog-workflow.md#evidence-policy)に従う。採用条件は[VALIDATED and REJECTED](../../../tools/backlog/backlog-workflow.md#validated-and-rejected)、CLIのRUN選択とhash仕様は[Backlog README](../../../tools/backlog/README.md#evidence-and-snapshots)を参照する。
