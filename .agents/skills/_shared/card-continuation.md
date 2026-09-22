# カード処理の継続・終了

isucon-workerとisucon-investigateは、開始時・各カードの処理後・終了直前にこの手順に従う。カード単位の完了とスキル全体の終了を区別する。ユーザーが明示的に限定した場合を除き、workerは[workerの優先順](../isucon-worker/SKILL.md#優先順)に従うOwnerなしDOINGと全READY、investigateは[調査手順](../isucon-investigate/SKILL.md#手順)に従う全INVESTIGATEを対象とする。両者とも新着を含め、Priority・Evidenceは着手順の判断に使う。着手済み作業の完了・必要な修正を優先する。

isucon-workerでは、カードの実装レビュー後の継続判断は同じdeployment batchへの追加判断とする。区切り条件に達したら[実装手順](../isucon-worker/references/implementation.md)の統合検証・適用を先に行い、その後に継続・終了を判断する。適用可能な完成分を残したまま終了しない。ユーザーの停止指示は優先する。

## 継続の判断

Backlog CLIで最新のカード一覧を一度取得し、各Skillの対象・Priority・Owner・依存と各Skillの実行制約を再確認する。対象の中でOwner・依存・他作業との競合の条件を満たす対象があれば、各Skillの通常手順で取得・処理し、処理後に再びこの判断へ戻る。一件の処理完了や残件数の報告だけで終了しない。読み取り失敗を対象0件として扱わない。

新しいEvidenceは着手順・調査内容・採否判断へ反映し、低い優先度や効果量の未確定を理由に対象から外さない。取得できないカードは状態・Ownerを変更せず、理由と再検討条件を報告する。investigateでObjective／Target管理へ差し戻したカードは、同じ前提のまま再claimしない。

ユーザーによる対象・件数の制限と停止指示を優先する。指定件数の処理完了や停止指示があれば終了する。

## 取得範囲と再利用

一覧は`--format json --fields`で判断に必要な項目だけ取得する。例：`task --silent backlog -- list --format json --fields id,version,status,priority,owner,dependencies,target_ids`。対象状態のfilterは各Skillの範囲に合わせる。

詳細も`show --format json --fields`を使う。取得前に必要な項目を決め、カード本文は`sections`、履歴は`history`を必要時だけ含める。Target・Objectiveの`show`も同じ指定ができる。section名やHistoryのpositionによる絞り込みは返却JSONへ行い、必要なsectionと未読Historyだけ表示する。CLIの指定例と出力形式は[Backlog README](../../../tools/backlog/README.md#automation-without-wrapper-scripts)を参照する。

Backlogを正本として、現在処理中のopenカード本文と最新の引き渡しから読む。過去Historyは、そのカードの引き渡しが参照する判断・根拠や、現在の指示に不足・不一致がある点だけ追加で読む。最新のHistoryが取得・記録更新などの場合は、それを引き渡しとみなさず、直前の記録へさかのぼって引き渡しを確認する。履歴全文の通読から始めない。

参考コマンド（`B-003`とpositionの`13`は対象に置き換える）。

```sh
# Current card body
task --silent backlog -- show B-003 --format json --fields id,version,status,owner,sections

# Latest history entry
task --silent backlog -- show B-003 --format json --fields history | jq '.history[-1]'

# A specific older entry, only when needed
task --silent backlog -- show B-003 --format json --fields history | jq '.history[] | select(.position == 13)'
```

最新の記録が引き渡しでなければ、`[-2]`などで直前の記録を確認する。

取得済み内容はID・versionとともに再利用し、同じversionの読了部分を再読しない。変更や不足があれば関係する項目だけ確認する。更新成功時はCLIが返すversionを次の操作へ引き継ぐ。最新一覧の確認と更新時のversion・Owner条件は維持し、競合時は最新状態を読み直して判断する。

## 対象がない場合の終了

終了直前に最新一覧を再取得し、各Skillの対象に取得可能なカードがなければ終了する。investigateは新着INVESTIGATEも対象に含めて継続し、workerはOwnerなしDOINGと新着READYも対象に含めて継続する。Backlog全体が空である必要はない。

着手済みの処理や委任中の調査があれば、各Skillの手順に従って完了させてから継続を判断する。取得対象がないことを理由に、適用可能な完成分や委任結果の回収・反映を残したまま終了しない。

終了時は各Skillの検証・報告要件に従い、対象の処理結果と残る候補を区別し、取得可能な対象がないこと、または実行制約による終了理由を報告する。
