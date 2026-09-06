# 分析レポート

新しいレポートは、実行したスキル名のディレクトリ`docs/reports/<SKILL_NAME>/<RUN_ID>.md`へ保存します。
たとえば、`isucon-special-sauce`の調査結果は`docs/reports/isucon-special-sauce/`配下に保存します。
同じスキルで同じRUNを再調査する場合は、2回目を`<RUN_ID>-r2.md`、3回目を`<RUN_ID>-r3.md`のように
連番で保存します。`rN`はレポートの置き換え版ではなく、そのスキルでそのRUNを調査した回数を表します。

解析日時はファイル名へ付けず、レポート本文にRFC 3339形式で記録します。既存ファイルは上書きせず、
次の連番を使います。

レポートでは、参照したObjectiveと公式仕様、RUN・コード・設定から確認した事実、Constraint、
Intervention候補、推論・代替仮説・未確定点、更新したBacklog IDを分けて記録します。
