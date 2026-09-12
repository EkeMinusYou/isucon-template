---
name: isucon-special-sauce
description: docs/special-sourcesの指定資料、省略時は全件を現行と照合し、根拠のある適用候補をINVESTIGATEへ起票する。
---

# ISUCON 秘伝のタレ候補の調査

担当範囲は適用候補の調査・INVESTIGATE起票と報告。READY判定は`isucon-investigate`へ渡し、実装・設定変更・deploy・benchは行わない。

## 対象を選ぶ

- 指定があれば、`docs/special-sources/` 内のパス、ファイル名、カタログのリンク、一意な名称から対象を解決する。文書指定は文書全体、節・設定項目の指定はその範囲、複数指定は和集合だけを調べる。
- 指定がなければ `docs/special-sources/README.md` と実際のファイル一覧を照合し、ルート README 以外の全 Markdown 文書を調べる。カタログ未掲載の文書も含め、差分を報告する。
- 指定が曖昧または対象が存在しない場合は、対象を確認する。全件調査へ勝手に広げず、対象確定までは起票しない。

## 現行設定と照合して起票する

指定資料を現行設定・コード・公式仕様と照合する。具体的な差分と得点への改善方向を根拠付きで説明できる候補をINVESTIGATEへ起票する。効果量や詳細方式の確定は起票条件にしない。出典、現行値と提案値の差分、設定の生成元と反映先、未確定点を記録する。起票時に得点への寄与仮説を示し、リンクがあればTargetとObjectiveへの改善方向も示し、適用の必要性・因果の独立検証・変更範囲・採否の判断は `isucon-investigate` に委ねる。

[既知資料の候補評価手順](../isucon-analyze/references/known-solutions.md)に従い、適用済みの設定も現行の負荷・配置に対して追加改善できるか比較する。同一変更の重複・明確な適用不可は差分と理由を残す。重複の補足は既存カードの状態・Owner制約を守り、編集できなければ修正案を報告する。部分適用なら未反映部分も候補とし、一体で採否・適用すると分かる設定群は一枚にまとめる。

[Backlog workflow](../../../tools/backlog/backlog-workflow.md)のTarget・Relations・Writer protocolに従い、適合する既存ACTIVE Targetがあれば作成時に関連付け、主Targetを一つにする。Objective・Targetは変更しない。適合するTargetがなければリンクなしで起票し、対象・改善目標・得点への寄与仮説・baseline Evidence・評価条件と、既存Targetで扱えない理由を本文へ残す。Target不足だけで起票を保留しない。共通目標として管理する価値があれば[isucon-target](../isucon-target/SKILL.md)へ発見内容を報告するが、その整備を起票の前提にしない。

起票時の actor は `skill:isucon-special-sauce`。重複は対象・機構・Change boundaryで判定し、Interventionに独自Fingerprintを要求しない。

## 完了報告

指定対象／全件の別と調査文書、現行との差分、起票・補足したカード ID と Target・Objective、出典リンク、見送った理由、未確定点を簡潔に報告する。次の判断は `isucon-investigate` へ引き渡す。
