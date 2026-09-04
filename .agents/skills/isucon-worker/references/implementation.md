# Implementation

1. card version、READY、Owner空、Objective、依存、Change boundary、rollbackを再確認する。
2. 現在diffと境界が競合しないことを確認し、Owner設定と`DOING`を原子的に行う。
3. 公式guardrailを保ちながら境界内だけを実装する。
4. formatter、unit test、構文検査、build、必要な統合検査を行う。
5. 完成snapshotを再確認して`VERIFY`へ進める。
6. 正規の`task deploy-*`で適用し、service、log、endpoint、設定hashを確認する。
7. `APPLIED`へ進め、次の手動ベンチで判定する内容をHistoryへ残す。

一体でrollbackできない独立変更が見つかった場合は実装を止め、INVESTIGATEへ差し戻す。無関係なリファクタを混ぜない。
