# 分析レポート

新しいレポートは、実行したスキル名のディレクトリ`docs/reports/<SKILL_NAME>/<RUN_ID>.md`へ保存します。
たとえば、`isucon-special-sauce`の調査結果は`docs/reports/isucon-special-sauce/`配下に保存します。
同じスキルで同じRUNを再調査する場合は、2回目を`<RUN_ID>-r2.md`、3回目を`<RUN_ID>-r3.md`のように
連番で保存します。`rN`はレポートの置き換え版ではなく、そのスキルでそのRUNを調査した回数を表します。

解析日時はファイル名へ付けず、レポート本文にRFC 3339形式で記録します。既存ファイルは上書きせず、
次の連番を使います。

レポートでは、参照したObjectiveと公式仕様、RUN・コード・設定から確認した事実、Target、
Intervention候補、推論・代替仮説・未確定点、更新したBacklog IDを分けて記録します。

`isucon-objective`は改善方針ごとの寄与仮説・整理判断・Target側の調整事項、`isucon-target`はObjectiveごとの既存Target検査・不足対象の探索範囲・目標の根拠・達成判定を記録します。RUNがない場合は架空のRUN_IDを作らず、BacklogのHistoryと完了報告に残します。

`isucon-analyze`ではTargetごとの最新の観測、アクティブなIntervention、既存変更が残した仕事、調べた補完策・代替策、未解決の問いを分けて記録します。クラスタと担当・探索範囲、対象Objective・Target、確認RUNとコード・設定の時点、結果（新規起票／既存カード修正／修正案の報告／重複／見送り／未確定）、Targetへの追記、終了理由、未探索部分と今回扱わなかった理由、再探索条件を残します。次回は差分と未探索部分から範囲を選び、同じ候補を重複起票しません。RUNがない場合は`docs/reports/isucon-analyze/no-run.md`、2回目以降は`no-run-r2.md`のように新規保存し、本文にRUNなしと明記します。これはRUN_IDではなく、既存カードを変更せず修正案を残す場合にも使える探索記録です。

`isucon-rethink`では疑った構造上の前提、要件から描いた構成と難点による組み替え、現行との比較、探索した案と寄与仮説、事実・推論・未確定点、起票・修正・重複・見送り、優先度の判断理由、終了理由と再探索条件を記録します。RUNがない場合は`docs/reports/isucon-rethink/no-run.md`、以降は`no-run-r2.md`のように新規保存し、本文にRUNなしと明記します。
