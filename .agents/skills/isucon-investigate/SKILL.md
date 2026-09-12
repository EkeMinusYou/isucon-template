---
name: isucon-investigate
description: 既存INVESTIGATEを独立検証し、実装範囲と評価条件を定めてREADY可否を判定する。実装は行わない。
---

# ISUCON investigate

担当範囲内のINVESTIGATEをクラスタ単位で調査し、各Interventionを実装可能な採否境界か根拠ある終端へ収束させる。カード一件は更新の単位であり、スキル一回の処理件数上限ではない。READMEや起票時仮説を結論として扱わず、現在のEvidenceで独立確認する。

## 最初に読む

- `AGENTS.md`
- [Backlog workflow](../../../tools/backlog/backlog-workflow.md)：`Authority`・`Intervention`直下・`READY gate`・`BLOCKED`・`Rejection during investigation`・`Relations`・`Evidence policy`・`Priority`・`Writer protocol`。子節は列挙したものだけ読む。
- [Evidence](../_shared/evidence.md)
- 対象カード、接続Objective・Target、依存、History
- 対象に関係する`docs/official/`

workflowは見出しを検索して必要な節だけ読む。Objective／Targetの妥当性確認には対応する節も読むが、内容・状態・TargetからObjectiveへのリンクは変更しない。同じ内容を読了済みなら再読しない。

## 境界

- ローカルコード・設定・schema・保存済みRUNと必要最小限の読み取り専用SSHだけを使う。保存済みEvidenceへの読み取り集計は可能で、既存DuckDB基盤を優先する。
- 実装、設定変更、deploy、service操作、bench、保存済みRUN成果物の再生成・上書き、計測基盤の変更は行わない。
- READYへの判定を担当する唯一のスキルである。
- 情報不足や追加計測を新しいInterventionとして起票しない。
- 他actorがOwnerのカードを書き換えない。書込みはBacklog CLIとversion checkを使う。

## 手順

親は[Work selection](../../../tools/backlog/backlog-workflow.md#work-selection)で選ばれた今回の調査範囲と根拠を引き継ぎ、単独呼び出しでは保存済みEvidenceとカードから選ぶ。READYへ進める最終的なカード内容に対して、[workflowのPriority](../../../tools/backlog/backlog-workflow.md#priority)に従って必ず優先度を設定する。分割・統合・タイトル変更を経たカードも、`resolve --status READY`に`--priority`を指定し、本文・状態と同時に確定する。

1. 今回の範囲内でworkflowのPriorityに従って対象を選び、下記の規則でクラスタにまとめる。親が着手・委任前にクラスタ内の全カードをversion付きでclaimする。着手後にcard version、status、Ownerを再確認する。
2. Interventionの改善対象・目標・得点への寄与仮説、リンクがあればTargetの改善目標とObjectiveへの因果、共通の有効性条件、Evidenceのsnapshot、現行で未解消か、仕様上許されるかを独立確認する。[共通の評価基準](../_shared/evidence.md#改善方向と次の投資判断)に沿って局所案と構造案を評価し、主要な追加費用が適用範囲や代替構成の選択を変える場合は、その比較も確認する。正当性の検証だけでこの判断を代用しない。
3. 同じtarget・mechanism・採否境界のopen Interventionを確認し、重複なら統合する。
4. 一体で採用・適用するChange boundaryを確定する。ファイル数やlayer数だけで分割しない。
5. workflowのREADY gateの3セクションを本文へ具体化し、条件を満たすか判定する。`Evaluation`の確定時は[Evaluationの指定方法](../_shared/evaluation.md)を読み、改善対象の機構・悪化し得る関連処理・正常性について、読む成果物と行・指標、採否条件を対応付ける。既存の抽出機能で表現できる部分はJSONで指定し、表現できない条件は具体的な確認手順として残す。
6. 依存が本当に別の採否境界なら構造化dependencyにする。単なる実装順は同一カード内で扱う。
7. workflowのTarget・Relationsに従って対象・目標・評価条件を確認する。適合する既存ACTIVE Targetがあれば関連付け、主Targetを一つにする。リンクなしでもIntervention自身の改善対象・目標・得点への寄与仮説・baseline Evidence・採否の判断条件を独立検証し、条件を満たせばREADYへ進める。Target不足だけで差し戻さず、意味のあるリンクをゲート回避のために外さない。既存リンクの問題はisucon-targetへ根拠付きで報告し、READY条件を満たさなければINVESTIGATEに留める。Objective自体の問題はisucon-objectiveへ報告する。管理スキルの作業待ちだけでBLOCKEDにしない。
8. 親が調査結果と根拠を確認して最終判断し、更新直前にcard version、status、Ownerを再確認する。READY・BLOCKED・REJECTEDへ進めるカードは本文と状態を一つの`resolve`操作で確定し、Ownerを空にする。Objective／Target管理へ差し戻すカードは、根拠・必要な修正・再検討条件をHistoryへ残し、version付き`update`でOwnerを空にしてINVESTIGATEに留める。

## クラスタと並列調査

委任の単位はカードではなくクラスタとする。同じtarget・mechanism・Change boundary本文の変更対象を共有するカードはまとめ、統合・分割の判断に必要な相互の文脈を同じ担当へ渡す。同じテーブル・SQL形状・endpoint群・設定、同じ機構の別インスタンス、一方の変更で他方の採否境界が消える場合を確認する。クラスタは調査の単位であり、カードの統合や構造化dependencyを自動的に意味しない。

- サブエージェントが利用可能で、独立した読み取り専用調査を並列化する価値がある場合は、独立クラスタをサブエージェントへ委任する。利用できない場合や並列化の価値がない場合は親が調査を続ける。
- 同時調査は親が直接担当するものを含め最大4クラスタとし、利用可能な実行枠が少なければその範囲で行う。超過分は待機させ、空いた枠へworkflowのPriorityに従って投入する。
- 委任可否はクラスタ間の独立性で判断する。同じTargetへのlinkだけで、独立した調査を親専任にしない。
- 子は証拠と判断案だけを返し、ファイル、バックログ、リモート状態を変更しない。claim、本文・relation・dependencyの更新、状態遷移を含む書込みと最終判断は親だけが行う。共通のSSH確認は親が一度だけ行い、結果を共有する。
- 委任時は全カードのID・本文、接続Objective・Target・依存、まとめた理由、Evidenceのパスとsnapshot、確認済み事項、残る問い、調査範囲と制約を渡す。子にはカードごとの根拠、目的・境界・判定の案、統合・分割の可否と理由、残る不明点を返させる。
- 実行中に対象範囲内の新着カードを取り込む場合は、既存クラスタとの共有を確認する。関連するなら親がclaimして同じ担当へ追加依頼し、確認済みEvidenceを再利用する。独立なら待機クラスタへ入れる。snapshotや前提が変わった部分は再確認する。

親は子の判断案をそのまま確定せず、Evidenceとworkflowの判定条件を照合し、クラスタをまたぐ重複とrelationの整合も確認する。

## 継続・終了

開始時・各カードの処理後は、共通の[継続・終了手順](../_shared/card-continuation.md)に従う。対象は、ユーザーが指定した範囲内のOwner・依存・他作業との競合を確認してclaimできるINVESTIGATEに限る。親が委任中のクラスタも処理中として管理し、結果待ちを対象なしとして終了へ進まない。

Objective／Target管理へ差し戻したカードは、同じ前提のまま今回の実行で再claimしない。再検討条件に変化があれば対象へ戻す。

## 完了条件

各カードはworkflowの判定・History規則に従い、READY、BLOCKED、REJECTEDへ原子的に収束し、relation・依存を整合させる。Objective／Target管理への差し戻しはINVESTIGATEのまま理由と再検討条件を報告し、READY判定の完了とは扱わない。

終了時は`task backlog -- validate`を通し、処理したカードID・判定根拠・残る不明点を報告する。READY判定と今回の実装候補の選択を区別し、workerへ渡す範囲と理由に独立確認の結果を反映する。
並列調査した場合は、委任したクラスタとカードID、統合・分割の判断、親が確定した結果を報告する。委任分を含む対象カードの結果を回収・反映し、claimを残したまま完了しない。
