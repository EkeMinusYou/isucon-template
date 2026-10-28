# 共有Skills

`.agents/skills/`がClaude Code / Codex共通のskill実体である。`.claude/skills`はこのディレクトリへのsymlink。
このテンプレートは全面再設計を試すためのもの。初期実装は外部動作の参照に使い、内部構成は新しく設計する。

## 3段階

| 段階 | Skill | 担当と完了条件 |
|---|---|---|
| 1. 準備 | [isucon-setup](isucon-setup/SKILL.md) | 仕様・初期実装・資源・接続先・ポータル操作を準備する。旧実装の計測導入・再デプロイは必須にしない |
| 2. 全面再設計 | [isucon-bootstrap](isucon-bootstrap/SKILL.md) | 振る舞いを抽出して新構成を設計・全機能を実装する。正当性・復元・標準計測・初回PASSを確認する |
| 3. 自律改善 | [isucon-agent](isucon-agent/SKILL.md) | 新実装の設計予測と実測を照合し、得点への効果から構成・実装を改善し続ける |

```text
$isucon-setup HARを参考に、全面再設計に必要な準備をしてください。
$isucon-bootstrap 初期実装の振る舞いを維持し、内部構成をゼロから設計・実装してPASSまで検証してください。
$isucon-agent task benchでPASS・100万点を達成するまで改善してください。
```

設計・実装までならbootstrap完了で終了する。目標までの継続も依頼されていれば、同じスレッドでagentへ進む。完了済み段階は繰り返さない。専用の引き継ぎ資料は要求しない。

## 共通規律と資料

操作許可・正規経路は`AGENTS.md`、仮説・比較・欠損の扱いは[Evidence](_shared/evidence.md)を参照する。原因を絞れない場合は[診断工程](isucon-agent/SKILL.md#診断工程)を使う。
計測の技術的な確認手順は[標準計測](isucon-setup/references/measurement.md)にまとめ、bootstrapで新実装に組み込み、agentで維持する。

[既知資料](_shared/known-solutions.md)は構成候補の参考であり、適用順序ではない。`isucon-special-sauce`、`isucon-use-solution`、`isucon-create-solution`は個別に必要な場合だけ使う。

追加・変更時は、既存の責務やreferenceで表現できるか確認する。段階固有の詳細をSKILL.mdへ重複させず、方針・案内先が変わる場合は`AGENTS.md`も更新する。
