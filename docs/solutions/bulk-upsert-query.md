# Bulk upsert の query を安全に組み立てる

## 概要

複数行を一度に `INSERT` し、既存キーと衝突した行を更新する bulk upsert の実装パターンを示す。
行ごとに `INSERT` や `UPDATE` を発行する処理を、パラメータ化した一つ以上の SQL 文へまとめることで、
アプリケーションと DB の往復回数、ステートメントの準備・解析回数を減らせる。

この文書のSQLとGoの型は説明用の例であり、このテンプレートは `user_items` テーブルの存在を前提としない。
対象アプリのテーブル・列・型へ置き換えて考える。

## 適用条件・制約

- 対象テーブルに、upsert の衝突判定へ使う PRIMARY KEY または UNIQUE KEY が必要である。意図しない
  キーが衝突して更新されると、通常の insert と異なる結果になる。
- 新規行と既存行でどの列を更新するかを決める。たとえば `amount` と `updated_at` だけを更新し、
  `created_at` は保持するようにする。
- `id` をアプリケーションで採番するのか、AUTO_INCREMENT に任せるのかを既存の書き込み経路とそろえる。
  1バッチ内で同じ一意キーが複数回現れたときの最終値も定義する。
- 値はプレースホルダーの引数として渡し、値を SQL 文字列へ連結しない。テーブル名や列名を動的にする
  場合も、ユーザー入力を使わず、許可した識別子の map から選ぶ。
- 行数が多い場合は、プレースホルダー数と `max_allowed_packet` の制約を超えないように分割する。
  分割しても、親子関係や同じトランザクションで扱うべき更新の境界を壊さない。
- MySQL のバージョンによって `VALUES(column)` の互換性や警告が異なる。接続先が使える row alias
  構文を確認してから選ぶ。
- `RowsAffected` は insert、値が変わる update、値が変わらない重複で見え方が異なる。挿入件数や更新件数
  の業務上の意味を、ドライバと DB の仕様に依存した値だけで判定しない。

## 探索方法

1. スキーマ定義と既存の一意キーを調べ、upsert の衝突条件と更新対象列を確定する。SQL では
   `SHOW CREATE TABLE` と `SHOW INDEX` が手掛かりになる。
2. 書き込み経路で、ループ内の `INSERT`、`UPDATE`、`Exec`、`NamedExec`、`sqlx.In`、または同じ
   SQL を繰り返す ORM 呼び出しを探す。
3. そのループが一つのリクエストだけで完結するのか、複数の関連テーブル・外部 API・副作用を含むのかを
   確認する。bulk upsert の前後を同じトランザクションに置く必要があるかもここで判断する。
4. 既存行の更新時に維持すべき値、insert 時だけ設定する値、更新時刻、監査列、イベント発行などを
   呼び出し元とスキーマの両方から洗い出す。

## 実装方法

行ごとに値の組を作り、値の個数に応じたプレースホルダーだけを組み立てる。SQL の構文要素は固定し、
値は `args` に残す。

```go
import (
	"context"
	"fmt"
	"strings"

	"github.com/jmoiron/sqlx"
)

func BulkUpsertUserItems(ctx context.Context, tx *sqlx.Tx, userItems []*UserItem) error {
	if len(userItems) == 0 {
		return nil
	}

	values := make([]string, 0, len(userItems))
	args := make([]any, 0, len(userItems)*7)
	for i, item := range userItems {
		if item == nil {
			return fmt.Errorf("userItems[%d] is nil", i)
		}

		values = append(values, "(?, ?, ?, ?, ?, ?, ?)")
		args = append(args,
			item.ID,
			item.UserID,
			item.ItemID,
			item.ItemType,
			item.Amount,
			item.CreatedAt,
			item.UpdatedAt,
		)
	}

	query := "INSERT INTO user_items " +
		"(id, user_id, item_id, item_type, amount, created_at, updated_at) VALUES " +
		strings.Join(values, ", ") +
		" ON DUPLICATE KEY UPDATE amount = VALUES(amount), updated_at = VALUES(updated_at)"

	_, err := tx.ExecContext(ctx, query, args...)
	return err
}
```

呼び出し側では、bulk upsert と、それに伴う関連更新を同じトランザクションの境界へ置く。トランザク
ション開始後に失敗した場合は変更を確定せず、成功した場合だけ commit する。

```go
tx, err := db.BeginTxx(ctx, nil)
if err != nil {
	return err
}
defer tx.Rollback()

if err := BulkUpsertUserItems(ctx, tx, userItems); err != nil {
	return err
}
return tx.Commit()
```

バッチを分割する場合は、各チャンクの一意キーが重複する可能性、チャンク間で見える順序、トランザク
ションの原子性を設計する。必要な列だけを insert/update 対象にし、更新しない列を重複時に上書きしない。
