# 共有Skills

`.agents/skills/`がClaude Code / Codex共通のskill実体である。`.claude/skills`はこのディレクトリへのsymlink。

## 構成

- `isucon-setup` — 競技開始時の取得、生成、build、正規deploy・bench経路の準備
- `isucon-analyze` — Objective、Constraint、Intervention候補の発見
- `isucon-investigate` — INVESTIGATEの独立検証とREADY安全ゲート
- `isucon-worker` — READYの実装・適用、ベンチ後検証、rollback・復旧

発見観点やレポート種別ごとにskillを分割しない。性能、score mechanics、benchmark behavior、topology、既知solutionは`isucon-analyze/references/`から必要なものだけ読む。

## 共通規律

- [Backlog workflow](../../tools/backlog/backlog-workflow.md) — 三層モデル、状態遷移、READY・採用条件、優先順、書込み規則の正本
- [Backlog README](../../tools/backlog/README.md) — CLI操作、機械検査、保存形式
- [Evidence](_shared/evidence.md) — 根拠の選択・比較・欠損・因果の扱い

各Skillには担当範囲と実行手順を置き、共通ルールの詳細は再定義しない。workerのAPPLIED上限など、担当固有の運用規則はそのSkillを正本とする。`_shared/objective-constraint-intervention.md`はworkflowへの案内として残す。

## 追加・変更

skillを追加する前に、既存4スキルの責務またはreferenceで表現できない独立した作業段階か確認する。追加・削除・改名時は、この一覧と参照元を同じ変更で更新する。全体方針や案内先が変わる場合は`AGENTS.md`も更新する。

各`SKILL.md`のfrontmatterは`name`と`description`だけを使う。詳細手順は、すべての実行に必要なものだけ本文へ置き、条件付き知識は`references/`へ分ける。
