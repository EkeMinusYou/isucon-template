# 共有Skills

`.agents/skills/`がClaude Code / Codex共通のskill実体である。`.claude/skills`はこのディレクトリへのsymlink。

## 構成

- `isucon-setup` — 競技開始時の取得、生成、build、正規deploy・bench経路の準備
- `isucon-analyze` — Objective、Constraint、Intervention候補の発見
- `isucon-investigate` — INVESTIGATEの独立検証とREADY安全ゲート
- `isucon-worker` — READYの実装・適用、ベンチ後検証、rollback・復旧

発見観点やレポート種別ごとにskillを分割しない。性能、score mechanics、benchmark behavior、topology、既知solutionは`isucon-analyze/references/`から必要なものだけ読む。

## 共通規律

- `_shared/objective-constraint-intervention.md` — Backlog三層、relation、READY条件、優先順
- `_shared/evidence.md` — 公式仕様、コード、設定、保存済みRUNの扱い

計測はEvidence生成であり、skillやBacklogカードの種類にしない。標準計測基盤の設計変更が必要なら、ユーザー指定の別タスクとして扱う。

## 追加・変更

skillを追加する前に、既存4スキルの責務またはreferenceで表現できない独立した作業段階か確認する。追加・削除・改名時は、同じ変更でリポジトリルートの`AGENTS.md`も更新する。

各`SKILL.md`のfrontmatterは`name`と`description`だけを使う。詳細手順は、すべての実行に必要なものだけ本文へ置き、条件付き知識は`references/`へ分ける。
