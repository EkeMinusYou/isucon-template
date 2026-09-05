---
name: isucon-investigate
description: ISUCON BacklogのINVESTIGATE Interventionを公式仕様、現行コード・設定、保存済みEvidenceから独立検証し、目的・採否境界・判定・安全を確定してREADY、BLOCKED、REJECTEDへ収束させる。Constraintの重複・relationも整合するが、実装、deploy、benchは行わない。
---

# ISUCON investigate

一つのINVESTIGATEを、実装可能な採否境界か根拠ある終端へ収束させる。READMEや起票時仮説を結論として扱わず、現在のEvidenceで独立確認する。

## 最初に読む

- `AGENTS.md`
- [Backlog workflow](../../../tools/backlog/backlog-workflow.md)
- [Evidence](../_shared/evidence.md)
- 対象カード、接続Objective・Constraint、依存、History
- 対象に関係する`docs/official/`

## 境界

- ローカルコード・設定・schema・保存済みRUNと必要最小限の読み取り専用SSHだけを使う。
- 実装、設定変更、deploy、service操作、bench、成果物再集計は行わない。
- READYへ進められる唯一の安全ゲートである。
- 情報不足や追加計測を新しいInterventionとして起票しない。
- 他actorがOwnerのカードを書き換えない。書込みはBacklog CLIとversion checkを使う。

## 優先順

[workflowの優先順](../../../tools/backlog/backlog-workflow.md#priority)に従い、INVESTIGATEの対象を選ぶ。

## 手順

1. 対象カードをversion付きでclaimする。着手後にcard version、status、Ownerを再確認する。
2. Objectiveへの因果、Evidenceのsnapshot、現行で未解消か、仕様上許されるかを独立確認する。
3. 同じtarget・mechanism・採否境界のopen Interventionを確認し、重複なら統合する。
4. 一体で採用・適用・rollbackするChange boundaryを確定する。ファイル数やlayer数だけで分割しない。
5. [READY gate](../../../tools/backlog/backlog-workflow.md#ready-gate)の4セクションを本文へ具体化し、条件を満たすか判定する。
6. 依存が本当に別の採否境界なら構造化dependencyにする。単なる実装順は同一カード内で扱う。
7. [Constraint](../../../tools/backlog/backlog-workflow.md#constraint)の現在状態と[relation](../../../tools/backlog/backlog-workflow.md#relations)を整合させ、必要なassessmentをCLIへ渡す。無関係ならlinkしない。
8. 本文と状態を一つの`resolve`操作で確定し、Ownerを空にする。

判定とHistoryの必須内容はworkflowの[READY](../../../tools/backlog/backlog-workflow.md#ready-gate)、[BLOCKED](../../../tools/backlog/backlog-workflow.md#blocked)、[REJECTED](../../../tools/backlog/backlog-workflow.md#validated-and-rejected)に従う。

## 完了条件

対象をREADY、BLOCKED、REJECTEDのいずれかへ原子的に収束し、Objective relation、必要なConstraint relation、依存、Historyが整合している。最後に`task backlog -- validate`を通し、実装・deploy・bench・追加計測カードを行っていないことを報告する。
