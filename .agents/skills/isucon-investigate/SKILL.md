---
name: isucon-investigate
description: ISUCON BacklogのINVESTIGATE Interventionを公式仕様、現行コード・設定、保存済みEvidenceから独立検証し、目的・採否境界・判定を確定してREADY、BLOCKED、REJECTEDへ収束させる。Constraintの重複・relationも整合するが、実装、deploy、benchは行わない。
---

# ISUCON investigate

担当範囲内のINVESTIGATEをクラスタ単位で調査し、各Interventionを実装可能な採否境界か根拠ある終端へ収束させる。カード一件は更新の単位であり、スキル一回の処理件数上限ではない。READMEや起票時仮説を結論として扱わず、現在のEvidenceで独立確認する。

## 最初に読む

- `AGENTS.md`
- [Backlog workflow](../../../tools/backlog/backlog-workflow.md)：`Authority`・`Intervention`直下・`READY gate`・`BLOCKED`・`Rejection during investigation`・`Relations`・`Evidence policy`・`Priority`・`Writer protocol`。子節は列挙したものだけ読む。
- [Evidence](../_shared/evidence.md)
- 対象カード、接続Objective・Constraint、依存、History
- 対象に関係する`docs/official/`

workflowは見出しを検索して必要な節だけ読む。Objective／Constraintを変更する場合は対応する節も読む。同じ内容を読了済みなら再読しない。

## 境界

- ローカルコード・設定・schema・保存済みRUNと必要最小限の読み取り専用SSHだけを使う。
- 実装、設定変更、deploy、service操作、bench、成果物再集計は行わない。
- READYへの判定を担当する唯一のスキルである。
- 情報不足や追加計測を新しいInterventionとして起票しない。
- 他actorがOwnerのカードを書き換えない。書込みはBacklog CLIとversion checkを使う。

## 手順

1. workflowのPriorityに従って対象を選び、下記の規則でクラスタにまとめる。親が着手・委任前にクラスタ内の全カードをversion付きでclaimする。着手後にcard version、status、Ownerを再確認する。
2. Objectiveへの因果、Evidenceのsnapshot、現行で未解消か、仕様上許されるかを独立確認する。
3. 同じtarget・mechanism・採否境界のopen Interventionを確認し、重複なら統合する。
4. 一体で採用・適用・rollbackするChange boundaryを確定する。ファイル数やlayer数だけで分割しない。
5. workflowのREADY gateの3セクションを本文へ具体化し、条件を満たすか判定する。
6. 依存が本当に別の採否境界なら構造化dependencyにする。単なる実装順は同一カード内で扱う。
7. workflowのConstraint・Relationsに従って現在状態とrelationを整合させ、必要なassessmentをCLIへ渡す。無関係ならlinkしない。
8. 親が調査結果と根拠を確認して最終判断し、更新直前にcard version、status、Ownerを再確認する。カードごとに本文と状態を一つの`resolve`操作で確定し、Ownerを空にする。

## クラスタと並列調査

委任の単位はカードではなくクラスタとする。同じtarget・mechanism・Change boundary本文の変更対象を共有するカードはまとめ、統合・分割の判断に必要な相互の文脈を同じ担当へ渡す。同じテーブル・SQL形状・endpoint群・設定、同じ機構の別インスタンス、一方の変更で他方の採否境界が消える場合を確認する。クラスタは調査の単位であり、カードの統合や構造化dependencyを自動的に意味しない。

- サブエージェントが利用可能で、独立した読み取り専用調査を並列化する価値がある場合は、独立クラスタをサブエージェントへ委任する。利用できない場合や並列化の価値がない場合は親が調査を続ける。
- 同時調査は親が直接担当するものを含め最大4クラスタとし、利用可能な実行枠が少なければその範囲で行う。超過分は待機させ、空いた枠へworkflowのPriorityに従って投入する。
- 委任可否はクラスタ間の独立性で判断する。Constraintへのlinkの有無や`RESOLVES` / `MITIGATES`の違いで親専任にしない。
- 子は証拠と判断案だけを返し、ファイル、バックログ、リモート状態を変更しない。claim、本文・relation・dependencyの更新、状態遷移を含む書込みと最終判断は親だけが行う。共通のSSH確認は親が一度だけ行い、結果を共有する。
- 委任時は全カードのID・本文、接続Objective・Constraint・依存、まとめた理由、Evidenceのパスとsnapshot、確認済み事項、残る問い、調査範囲と制約を渡す。子にはカードごとの根拠、目的・境界・判定の案、統合・分割の可否と理由、残る不明点を返させる。
- 実行中に対象範囲内の新着カードを取り込む場合は、既存クラスタとの共有を確認する。関連するなら親がclaimして同じ担当へ追加依頼し、確認済みEvidenceを再利用する。独立なら待機クラスタへ入れる。snapshotや前提が変わった部分は再確認する。

親は子の判断案をそのまま確定せず、Evidenceとworkflowの判定条件を照合し、クラスタをまたぐ重複とrelationの整合も確認する。

## 継続・待機・終了

開始時・各カードの処理後は、共通の[継続・待機・終了手順](../_shared/card-wait.md)に従う。対象は、ユーザーが指定した範囲内のOwner・依存・他作業との競合を確認してclaimできるINVESTIGATEに限る。親が委任中のクラスタも処理中として管理し、結果待ちを対象なしとして追加待機・終了へ進まない。

## 完了条件

各カードはworkflowの判定・History規則に従い、READY、BLOCKED、REJECTEDへ原子的に収束し、relation・依存を整合させる。

終了時は`task backlog -- validate`を通し、処理したカードID・判定根拠・残る不明点を報告する。
並列調査した場合は、委任したクラスタとカードID、統合・分割の判断、親が確定した結果を報告する。委任分を含む対象カードの結果を回収・反映し、claimを残したまま完了しない。
