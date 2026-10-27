# 共有Skills

`.agents/skills/`がClaude Code / Codex共通のskill実体である。`.claude/skills`はこのディレクトリへのsymlink。

## 改善の3段階

| 段階 | Skill | 担当と完了条件 |
| --- | --- | --- |
| 1. 環境・標準計測 | [isucon-setup](isucon-setup/SKILL.md) | 環境、正規deploy、ポータル、標準計測を整え、初回ベンチで回収・分析を確認する。WAL移行は含めない |
| 2. 初期改善 | [isucon-bootstrap](isucon-bootstrap/SKILL.md) | 秘伝のタレ、適用可能なN+1解消・索引追加、nginx静的配信、全機能のメモリ＋WAL移行と計測整備を行う。正当性・復元・計測を確認し、移行後の基準RUNを引き継ぐ |
| 3. 自律改善 | [isucon-agent](isucon-agent/SKILL.md) | 得点への効果から改善対象を選び、実装・deploy・ベンチ・採否判断を指定目標まで繰り返す。計測も維持する |

対象や順序の明示指定を優先する。初期対応だけなら第2段階で終了し、目標までの継続も依頼されていれば同じスレッドで第3段階へ進む。完了済みの段階は繰り返さない。既存構成への改善だけの依頼や相談・調査のみの依頼は、その範囲で対応する。

段階ごとに依頼する例:

```text
$isucon-setup 環境と標準計測を整えてください。
$isucon-bootstrap 初期改善を行い、WAL構成の計測まで整えてください。
$isucon-agent task benchでPASS・100万点を達成するまで改善してください。
```

一度に依頼する場合は、3スキルの順序と最終目標を指定する。中間段階の完了だけで停止しない。スレッドを分ける場合は、コード・設定・RUNを根拠に再開する。引き継ぎ項目は[bootstrapの完了条件](isucon-bootstrap/SKILL.md#完了と引き継ぎ)を参照する。

原因の説明や候補選定が進まない場合は、メインが[isucon-agentの診断工程](isucon-agent/SKILL.md#診断工程)を担当し、必要に応じてサブエージェントへ独立診断を依頼する。

## 個別の資料作業

- `isucon-special-sauce` — 設定資料の値を現行構成に合わせて適用し、正規deploy
- `isucon-use-solution` — 指定された一つの`docs/solutions/`文書の適用条件を現行環境と照合して報告
- `isucon-create-solution` — 再利用できる実装パターンを`docs/solutions/`に新規作成・更新

個別に使う場合は明示的に呼び出す。例: `$isucon-special-sauce`、`$isucon-use-solution docs/solutions/n-plus-one.md`、`$isucon-create-solution <文書化するテーマ>`。bootstrap内の秘伝のタレの工程も`isucon-special-sauce`を使う。

## 共通規律

初期改善と自律改善は、依頼範囲に必要なdeploy・サービス操作・競技DB再作成等を含み、改善案ごとの確認を待たずに進める。相談・調査のみなどの明示的な範囲指定は優先する。

標準計測の手順は`isucon-setup`を正本とする。[初回構築](isucon-setup/references/initial-setup.md)と[部分補修](isucon-setup/SKILL.md#既存環境の計測補修)を使い分ける。bootstrapとagentも自分の変更に伴う計測を整え、補修のためだけに初回構築を繰り返さない。計測のみの依頼もsetupで対応できる。

- [Evidence](_shared/evidence.md) — 根拠の選択・比較・欠損・因果の扱い
- [既知資料ガイド](_shared/known-solutions.md) — solutionsの適用条件・追加改善の比較

各Skillには担当範囲と実行手順を置き、共通ルールの詳細は再定義しない。担当固有の運用規則はそのSkillを正本とする。

## 追加・変更

skillを追加する前に、既存スキルの責務またはreferenceで表現できるか、独立した作業段階またはユーザーが指定して使う入口が必要か確認する。共通規律は複製しない。追加・削除・改名時は、この一覧と参照元を同じ変更で更新する。全体方針や案内先が変わる場合は`AGENTS.md`も更新する。

各`SKILL.md`のfrontmatterは`name`と`description`だけを使う。詳細手順は、すべての実行に必要なものだけ本文へ置き、条件付き知識は`references/`へ分ける。
