---
name: isucon-worktree
description: ISUCONの並行改善で、wtで曜日略称のワークツリーと現在のHerdrワークスペース内のタブを作る。修正指示を各タブのCodexへisucon-agentスキル指定で渡し、実装をmainへ取り込んでからmainで計測・分析する。
---

# ISUCONの実装分担ワークツリー

`main`はRUNの横断分析、新しい仮説と優先順位の決定、実装の統合、デプロイ・計測の調整、計測結果の分析と採否判断を担当する。実装はmainで進めることも、ワークツリーへ分担することもある。ワークツリーにはユーザーが`docs/conversation/`の会話ログから選んだ改善仮説を一つずつ割り当て、担当Codexは調査・実装・ローカル確認まで行う。仮説の出典となる会話と検証内容を確認してから着手し、完了後は計測前にmainへ取り込む。仮説の出典は会話ログ、実装の履歴はコミットで追い、曜日名に意味を持たせない。

## 作成と作業

ワークツリーの作成指示には、現在のHerdrワークスペース内に新しいタブを作ることも含む。

1. `wt list --branches`でワークツリーと既存のローカルブランチを確認する。`mon`、`tue`、`wed`、`thu`、`fri`、`sat`、`sun`の順に循環し、使用中の名前を飛ばして次の名前を使う。すべて空なら`mon`から始める。空きがなければ既存の担当状況を報告し、作業中のワークツリーやブランチを上書きしない。
2. `main`を起点に`wt switch --create <weekday> --base main --format json`で作り、返されたJSONの`path`からワークツリーの絶対パスを取得する。分岐元のmainのコミットも確認する。ワークツリーの作成・管理には`wt`を使い、mainの未コミット変更は分岐に含まれないことに注意する。
3. 作成時のignoredファイルのコピーが終わったことを確認する。調査に使う`runs/analysis.duckdb`はワークツリー固有の実ファイルとし、共有DBへのシンボリックリンクにしない。コピーされていなければ、`.worktreeinclude`の対象を確認して`wt step copy-ignored --from main --to <weekday> --require-include`で補う。ルートの`raw`は`.config/wt.toml`のhookでprimary worktreeと共有するため、コピー対象に加えない。
4. Herdr操作前に`HERDR_ENV=1`を確認し、`herdr pane current --current`で呼び出し元ペインの所属ワークスペースを確認する。環境変数の`HERDR_WORKSPACE_ID`と食い違う場合は応答の所属先を使う。`herdr tab create --workspace <workspace-id> --cwd "<worktree-absolute-path>" --label "<weekday>" --no-focus`で、そのワークスペースにタブを開く。返されたJSONの`.result.tab`と`.result.root_pane`からタブとペインのIDを取得し、現在のタブへのフォーカスを維持する。Herdr内で動作していない場合や呼び出し元を特定できない場合は、タブ作成・Codex起動ができないことを報告し、フォーカス中の別セッションへ操作先を切り替えない。

## 修正指示をCodexへ渡す

ユーザーがワークツリーの修正・実装を指示したら、対象ごとにそのワークツリーのタブでCodexを起動し、`isucon-agent`スキルを明示して修正を開始させる。作成と修正を同時に指示された場合も、タブ作成後にこの手順を続ける。作成だけの指示では修正を開始しない。

1. [isucon-agent](../isucon-agent/SKILL.md)を確認し、対象ワークツリー、仮説の出典となる会話ログ、ユーザーが指示した修正範囲と評価条件を整理する。既存のワークツリーでも上の手順で呼び出し元の所属を確認し、`herdr tab list --workspace <workspace-id>`と`herdr pane list --workspace <workspace-id>`で対応するタブ・ペインと作業ディレクトリを確認する。タブがなければ上の作成手順に従って開く。
2. 対象ペインが対話シェルの入力待ちであることを確認し、`herdr agent start <agent-name> --kind codex --pane <pane-id>`で起動する。名前は`isucon-mon`など、対象が分かる一意な名前にする。対象ペインに既存のCodexがあれば、その状態と担当内容を確認して再利用し、重複して起動しない。
3. `herdr agent prompt <agent-name> '<修正指示>' --wait --until working --timeout 30000`で指示を送る。プロンプトにはリテラルの`$isucon-agent`と`.agents/skills/isucon-agent/SKILL.md`を含め、対象ワークツリー、会話ログ、修正内容、評価条件、既存の合意を伝える。ユーザーから修正を指示済みであることを明記し、その範囲のローカル実装を開始させる。担当は調査・実装・ローカル確認・コミットまでで、マージ後のデプロイ・計測・分析はmainが担当することも伝える。シェルから送る際は単一引用符などで`$isucon-agent`の変数展開を防ぐ。
4. 複数のワークツリーが指定された場合は、それぞれのCodexが`working`になったことを確認して次へ進み、各修正の完了を待たずに全対象の作業を開始する。既存のCodexが作業中の場合や送信が失敗した場合は、`herdr agent get <agent-name>`と`herdr agent read <agent-name>`で状況を確認し、同じ修正を重複して送らない。timeoutでも指示が届いている場合がある。`blocked`なら表示された質問・承認の内容を確認する。開始結果をワークツリー名とエージェント名とともに報告する。

担当Codexは現行実装と保存済みRUNのEvidenceを調べ、選ばれた仮説の効果と正当性を評価できる変更を実装する。必要なbuild・checkを行って担当変更をコミットし、実装コミット、差分、確認結果、未確定点、比較候補の基準RUNと評価指標、想定する対象ホストと影響を担当ワークツリーの会話で報告する。main側の担当がこの報告を確認して統合・計測へ引き継ぐ。新しい作業記録ファイルの作成は求めず、記録の扱いは`AGENTS.md`に従う。

## mainへの取り込み

ユーザーがmainへのマージを指示したら、対象ワークツリーと実装コミットを特定して取り込む。指示はmain側・担当ワークツリー側のどちらの会話でもよい。マージは計測対象をmainへ統合する段階であり、スコア改善の採用判断は計測後に行う。

main側の担当がmainのタブから次の手順で取り込む。ワークツリーでのベンチRUNや効果確認をマージ条件にしない。

1. 担当Codexの実装と必要なローカル確認が完了し、対象の変更がコミットされ、候補とmainの作業ツリーがcleanであることを確認する。取り込む差分と、分岐後にmainへ追加された変更・RUNを確認して、担当仮説の範囲と比較条件を揃える。
2. `git -C "<worktree-absolute-path>" merge-base --is-ancestor main HEAD`でmainが候補の祖先か確認する。mainが先へ進んでいる場合は、`git -C "<worktree-absolute-path>" merge --no-ff --no-edit main`を行い、競合があれば解決してコミットする。実装の履歴を保つためrebaseやsquashは行わない。競合は変更の意図を確認して解決し、統合で影響した範囲のbuild・checkを行う。既存RUNの成果物や`runs/scores.tsv`はRUNごとの対応を保ち、空欄を0に変えない。
3. `wt -C "<worktree-absolute-path>" merge main --no-commit --no-rebase --no-ff`を実行する。祖先関係・作業ツリーの検査は`wt`に任せ、拒否された場合に原因を確認して対処する。成功時は`wt`の既定の削除処理でワークツリーを片付け、mainのマージコミットを確認する。mainが更新されて拒否された場合は、最新のmainとの差分を確認して取り込み直す。

新しい仮説を見つけた場合はmain側の会話に戻し、ユーザーが次の実装対象を選ぶ。

## mainでのデプロイ・計測・分析

mainではマージした仮説ごとに評価条件と基準RUNを確認し、原則として一つの仮説を計測・評価してから次の仮説を取り込む。mainで直接実装した変更も同じ単位で扱い、未計測の変更をまとめる場合は`AGENTS.md`の条件に従う。計測対象のコード・設定は計測前にコミットし、RUNとの対応を追えるようにする。

ローカル実装やマージだけを指定された場合は、その範囲に従う。mainのデプロイ指示、または`isucon-agent`による自律改善の依頼範囲に基づき、`AGENTS.md`とTaskfileに従い、対象ホストと影響を確認してmainから正規経路でデプロイする。DB初期化用のignored資材もmainに揃える。

main側の担当がmainから`task bench`を実行してRUN成果物を保存する。通常の`after-bench`が成功していればmainのDuckDBは同期済みとして分析へ進む。同期失敗や追加のRUN取り込みなど、同期が必要な場合だけ`task q-sync`を実行する。同条件の基準RUNとスコア・正当性・主要指標を比較し、結論と根拠、未確定点をmain側の会話で報告してユーザーと採否を判断する。効果が確認できない変更の修正・取り消しもmainで扱う。欠損成果物や失敗RUNの扱いは`AGENTS.md`に従う。
