---
name: isucon-investigate
description: ISUCON BacklogのINVESTIGATE Interventionを公式仕様、現行コード・設定、保存済みEvidenceから独立検証し、目的・採否境界・判定・安全を確定してREADY、BLOCKED、REJECTEDへ収束させる。Constraintの重複・relationも整合するが、実装、deploy、benchは行わない。
---

# ISUCON investigate

一つのINVESTIGATEを、実装可能な採否境界か根拠ある終端へ収束させる。READMEや起票時仮説を結論として扱わず、現在のEvidenceで独立確認する。

## 最初に読む

- `AGENTS.md`
- [Objective / Constraint / Intervention](../_shared/objective-constraint-intervention.md)
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

共有規律の順に、既存claim、validity、ユーザー指定、RESOLVES、Objective直結、MITIGATESを扱う。ACTIVE Constraintに候補がなくても他の安全なInterventionを阻害しない。

## 手順

1. 対象カードをversion付きでclaimする。着手後にcard version、status、Ownerを再確認する。
2. Objectiveへの因果、Evidenceのsnapshot、現行で未解消か、仕様上許されるかを独立確認する。
3. 同じtarget・mechanism・採否境界のopen Interventionを確認し、重複なら統合する。
4. 一体で採用・適用・rollbackするChange boundaryを確定する。ファイル数やlayer数だけで分割しない。
5. 次の4点を本文へ明記する。
   - 目的: Objectiveと因果
   - 境界: 実装対象とrollback単位
   - 判定: 採用・修正・棄却条件
   - 安全: 公式guardrail、停止条件、rollback
6. 依存が本当に別の採否境界なら構造化dependencyにする。単なる実装順は同一カード内で扱う。
7. Constraintとの関係を確認する。解消条件を満たす場合は`RESOLVES`、部分改善なら`MITIGATES`、無関係ならlinkしない。
8. 本文と状態を一つの`resolve`操作で確定し、Ownerを空にする。

## 判定

### READY

- 効果の方向とObjectiveへの因果を説明できる。
- Change boundaryを一体で実装・適用・rollbackできる。
- 結果別の判定を標準Evidenceまたはcorrectness checkで行える。
- 公式仕様のguardrailと停止条件がある。
- 未充足のBLOCKING dependencyがない。

効果量が未知、現在の主要Constraintでない、計測指標が直接ないことだけでは拒否しない。「変更してスコアだけを見る」しか説明がない場合はREADYにしない。

### BLOCKED

現在のリポジトリ・保存済みEvidence・許可された読み取りで得られない、具体的な外部状態または権限待ちだけに使う。問い、確認済み事項、欠けた最小情報、取得経路、再開条件をHistoryへ残す。

### REJECTED

現行で解消済み、仕様違反、因果方向を説明不能、安全な境界を作れない、完全重複、技術的反証のいずれかをEvidence付きで示す。工数、規模、単一RUNのスコアだけを理由にしない。

## Constraint整合

- 同じidentity・snapshotのACTIVE Constraintは一つへ統合する。
- 観測事実が解消したら`RESOLVED`、帰属が誤りなら`INVALIDATED`、重複なら`MERGED`。
- 候補がないだけならACTIVEを維持し、探索済み範囲とreconsider conditionをHistoryへ残す。
- 性能`RESOLVES`だけstructured residual assessmentを要求する。`MITIGATES`へは解消数式を強制しない。

## 完了条件

対象をREADY、BLOCKED、REJECTEDのいずれかへ原子的に収束し、Objective relation、必要なConstraint relation、依存、Historyが整合している。最後に`task backlog -- validate`を通し、実装・deploy・bench・追加計測カードを行っていないことを報告する。
