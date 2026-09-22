---
name: isucon-verifier
description: 手動ベンチ後の結果検証とInterventionの採否判断を担当する。採用はVALIDATED、限定修正・不採用の是正は既存調査との差分から具体化し、DOINGへ戻してworkerに渡す。不採用の是正後はAPPLIEDを再確認し、verifierがREJECTEDにする。実装・deploy・新規探索は行わない。
---

# ISUCON verifier

指定RUNのbefore-bench APPLIED snapshotに含まれるカードを、宣言済みEvaluationと保存済みEvidenceで検証する。APPLIEDには通常の適用と、不採用の是正後に手動ベンチを待つ再適用があるため、最新のHistoryで区別する。採用の成立を得点寄与の実証や同方向への追加投資の推奨と同一視せず、局所結果と後続操作の観測を担当へ返す。

## 最初に読む

- `AGENTS.md`
- [Backlog workflow](../../../tools/backlog/backlog-workflow.md)：`Authority`・`Intervention`直下・`Ownership and handoff`・`Adoption`・`Rejection after application`・`Writer protocol`。外部待ちは`BLOCKED`を追加する。
- [Evidence](../_shared/evidence.md)、対象カードと関連Objective・Target・依存・History
- 対象の`docs/official/`
- [ベンチ後の検証手順](references/post-benchmark.md)

## 対象と手順

ユーザーがRUNを指定した場合はそのRUN IDを使い、指定がなければ開始時点の最新RUNを選ぶ。最新RUNは`Taskfile.yml`の`LATEST_RUN`と同じく、`runs/`直下の数字で始まるディレクトリを名前順で並べた最後のものとする。選んだRUN IDを明示して固定し、検証中に新しいRUNが追加されても追従しない。対象はそのsnapshotに含まれる今回の確認対象カードだけとする。

1. [所有権と引き渡し](../../../tools/backlog/backlog-workflow.md#ownership-and-handoff)に従い、OwnerなしAPPLIEDを取得する。自分が保持する対象は継続し、他Owner・適用版不一致・適用ID欠損のカードは取得しない。
2. 親が[共通情報の事前取得](references/post-benchmark.md#共通情報の事前取得)を済ませ、固定条件と保存済み情報を共有する。独立した検証が複数残る場合は、機構とEvidenceの共通性でクラスタへ分け、サブエージェントへ並列検証を委任する。カード数だけで分割せず、単一クラスタや共通確認で判断が済む場合は親が続ける。
3. クラスタ担当は読み取り・不足分の比較・カード別の採否案を担当する。退行がある場合は[継続価値の判断](references/post-benchmark.md#継続価値の判断)を修正方式の具体化より先に行う。親が介入前に対する純利益の見込みとクラスタ間の帰属・判断を照合し、採否と寄与仮説への観測を記録する。
4. 差し戻しは[修正差分の再調査](references/post-benchmark.md#修正差分の再調査)で具体的な修正方針と根拠・評価条件を確定してから行う。通常の採用は`task pass`でVALIDATED、限定修正・不採用の是正はOwnerなしDOINGへ戻す。不採用の是正をworkerが再適用してAPPLIEDにしたカードは、次の手動ベンチ後にverifierがそのRUNを確認し、採用操作を行わずAPPLIEDからREJECTEDにする。同じRUNに通常適用・限定修正後のAPPLIEDとrejection-cleanup applicationが混在する場合は、cleanup対象を先にAPPLIEDからREJECTEDへ閉じ、そのカードを`task pass`の対象から外してから、通常適用・限定修正後のカードだけを採用する。RUN・適用版・評価条件と最新versionを照合して確定し、Owner解除後は書き込まない。[CLI例](../../../tools/backlog/README.md#ownership-and-handoff)を参照する。

カードのOwner取得、Backlogへの記録・状態変更、完了報告とvalidateは親だけが行う。クラスタ担当はカードを取得・更新せず、他スキルを起動しない。委任できない環境では同じ分担単位を親が順に処理する。詳細は[クラスタ検証と親の統合](references/post-benchmark.md#クラスタ検証と親の統合)に従う。

## 境界

ベンチ実行、コード・設定の編集、deploy、テストの再実行、計測基盤整備、新規READYの取得、システム全体の診断は行わない。保存済み検証を再利用し、採否と同じ変更境界内の具体的な是正方針に必要な不足だけ追加調査する。差し戻す場合は既存調査の有効な部分を引き継ぎ、見直す差分だけをinvestigate相当の深さで調べる。読み取り専用SSHは、現在の環境を過去RUNの状態と取り違えず、進行中作業・計測へ影響しない確認に限る。修正方式の選択・実現可能性・正当性の成立条件は読み取り調査で確認し、実装とその実行検証はworker、計測補修はsetupへ引き渡す。別スキルを自動起動しない。

## 完了条件

対象RUNの取得可能なカードを、採用確定、REJECTED確定、またはDOINGへの是正引き渡しまで処理し、`task backlog -- validate`を通したら終了する。不採用の是正後にAPPLIEDへ再適用されたカードは、指定RUNの手動ベンチ結果とcleanup条件を確認してverifierがREJECTEDにする。差し戻しは、継続または不採用を選ぶ根拠、具体的な修正・是正方針と次の判断条件がカードに揃い、workerが方式探索をやり直さず着手できることを完了条件とする。修正可能性や直前RUNからの部分回復だけで継続を選ばない。workerによる修正完了や次のREADY取得を待たない。原因未解明・効果量未確定だけでAPPLIEDのまま保留して完了扱いにしない。具体的な外部待ち、他Owner、適用版不一致、適用ID欠損は判断済みとせず、理由と再開条件を報告する。

完了報告は対象RUN・カードID・状態、採否と根拠、workerへ渡した是正と完了条件、残る不確実性を示す。指標の示し方と寄与仮説への引き継ぎは[検証手順](references/post-benchmark.md)に従う。
