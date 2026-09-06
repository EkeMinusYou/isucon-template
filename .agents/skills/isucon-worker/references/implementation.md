# Implementation

以下はREADYの新規着手から適用までの手順である。自分のDOING・VERIFYを継続する場合は現在状態から進め、VERIFYで同じ変更境界の修正が必要ならDOINGへ戻す。APPLIEDの限定修正は[post-benchmark](post-benchmark.md)に従う。

1. card version、READY、Owner空、Objective、依存、Change boundaryを再確認する。
2. 現在diffと境界が競合しないことを確認し、Owner設定と`DOING`を原子的に行う。
3. 公式guardrailを保ちながら境界内だけを実装する。
4. formatter、unit test、構文検査、build、必要な統合検査を行う。
5. 完成snapshotを再確認して`VERIFY`へ進める。
6. 正規の`task deploy-*`で適用し、service、log、endpoint、設定hashを確認する。
7. `APPLIED`へ進め、次の手動ベンチで判定する内容をHistoryへ残す。

無関係なリファクタを混ぜない。一体でrollbackできない独立変更や、境界・因果の再調査が必要になった場合は実装を止め、Ownerを外してINVESTIGATEへ差し戻す。具体的な外部待ちは[workflowのBLOCKED条件](../../../../tools/backlog/backlog-workflow.md#blocked)に従って記録する。技術的反証や公式仕様・明示された要求への違反が判明した場合は、適用済み変更のrollbackを行ってから根拠を記録しREJECTEDにする。
