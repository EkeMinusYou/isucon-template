---
name: isucon-investigate
description: ISUCON BacklogのINVESTIGATE Interventionを公式仕様、現行コード・設定、保存済みEvidenceから独立検証し、目的・採否境界・判定・安全を確定してREADY、BLOCKED、REJECTEDへ収束させる。Constraintの重複・relationも整合するが、実装、deploy、benchは行わない。
---

# ISUCON investigate

一つのINVESTIGATEを、実装可能な採否境界か根拠ある終端へ収束させる。READMEや起票時仮説を結論として扱わず、現在のEvidenceで独立確認する。

## 最初に読む

- `AGENTS.md`
- [Backlog workflow](../../../tools/backlog/backlog-workflow.md)：`Authority`・`Intervention`（`READY gate`・`BLOCKED`を含む）・`Relations`・`Evidence policy`・`Priority`・`Writer protocol`。`VALIDATED and REJECTED`は調査時の棄却規則のみ。
- [Evidence](../_shared/evidence.md)
- 対象カード、接続Objective・Constraint、依存、History
- 対象に関係する`docs/official/`

workflowは見出しを検索して必要な節だけ読む。Objective／Constraintを変更する場合は対応する節も読む。同じ内容を読了済みなら再読しない。

## 境界

- ローカルコード・設定・schema・保存済みRUNと必要最小限の読み取り専用SSHだけを使う。
- 実装、設定変更、deploy、service操作、bench、成果物再集計は行わない。
- READYへ進められる唯一の安全ゲートである。
- 情報不足や追加計測を新しいInterventionとして起票しない。
- 他actorがOwnerのカードを書き換えない。書込みはBacklog CLIとversion checkを使う。

## 手順

1. workflowのPriorityに従って対象を選び、version付きでclaimする。着手後にcard version、status、Ownerを再確認する。
2. Objectiveへの因果、Evidenceのsnapshot、現行で未解消か、仕様上許されるかを独立確認する。
3. 同じtarget・mechanism・採否境界のopen Interventionを確認し、重複なら統合する。
4. 一体で採用・適用・rollbackするChange boundaryを確定する。ファイル数やlayer数だけで分割しない。
5. workflowのREADY gateの4セクションを本文へ具体化し、条件を満たすか判定する。
6. 依存が本当に別の採否境界なら構造化dependencyにする。単なる実装順は同一カード内で扱う。
7. workflowのConstraint・Relationsに従って現在状態とrelationを整合させ、必要なassessmentをCLIへ渡す。無関係ならlinkしない。
8. 本文と状態を一つの`resolve`操作で確定し、Ownerを空にする。

## 完了条件

workflowの判定・History規則に従い、対象をREADY、BLOCKED、REJECTEDへ原子的に収束し、relation・依存を整合させる。`task backlog -- validate`を通し、カードID・判定根拠・残る不明点を報告する。
