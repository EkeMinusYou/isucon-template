# Score mechanics

公式仕様を正本に、Objectiveへ至る経路を分解する。

- validityやfail条件
- 成功した操作が生む得点
- 無効な操作、timeout、errorなどの損失
- シナリオ・ユーザー・配信のselectionとvalue差
- 処理量以外のpenalty、上限、終了時処理

技術指標が悪化しても最終Objectiveが改善する案を除外しない。負荷を選別する案は、得点への寄与、通常ユーザーへの影響、timeout、penaltyへの因果を一緒に評価する。
