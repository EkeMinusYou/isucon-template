# WAL付きオンメモリ状態と遅延DB投影

## 概要

高頻度のキー単位の可変状態更新を、リクエストごとのDBトランザクションから、ownerごとのオンメモリ状態と追記型の Write-Ahead Log（WAL）へ移す実装パターンである。DBを毎回更新してからレスポンスを返す代わりに、対象キーの状態遷移をアプリケーション内で直列化し、変更差分をWALへ記録してからメモリ状態へ適用する。DBへの反映は別のmaterializerが差分をまとめて行う。

ここでいう状態キーは、ユーザーに限定されない。ユーザーID、部屋ID、注文ID、カートID、テナントID、デバイスIDなど、同じキーに属する状態遷移を一つのownerへ集約できる単位を指す。ユーザーを例にする箇所は、このリポジトリの実装を説明するための固有例である。

```text
request
  -> owner routing
  -> per-key mailbox
  -> validate and build an after-image delta
  -> append the delta to WAL
  -> apply the delta to hot state
  -> response

WAL delta -> coalesce by key -> table-wise bulk upsert -> projection marker
```

この方式が有効なのは、同じ状態キーに属する状態を短時間に何度も読み書きし、各リクエストが次の状態をレスポンスへ返す必要がある一方、DBの永続化をリクエスト単位で完了させなくてもよい経路である。主に次の仕事を減らせる。

- 毎回のDB接続取得、行ロック、SQL往復、commitの待ち時間
- 同じキーへの同時更新で発生するDBロック競合
- 変更していない行を含む全状態の書き戻し
- 複数の小さなDMLを個別に実行するためのステートメント・トランザクションのオーバーヘッド

WALの追記、`fsync`、DB投影は別の処理である。追記はレコードをファイルへ渡す処理、`fsync` はOS・ファイルシステムのキャッシュをストレージへ反映する耐久化境界、DB投影はWALの内容をDBの正本またはread modelへ反映する処理である。性能上の要点は、通常リクエストをDB commitや個別の`fsync`へ直結させず、追記と耐久化をgroup commitし、DB投影もキー単位でcoalesceして複数行upsertすることである。

ただし、オンメモリ状態を正しく扱えることが前提であり、単にgoroutineへ処理を移す非同期化ではない。受理した更新の順序、ワンタイム処理、再起動後の復元、旧経路との切り替えを含めて一つの状態機械として設計する。

責務は3つに分かれる。キー単位の直列化を担うmailbox、追記とdurable sequenceを管理するWAL、差分をDBへ投影するmaterializerである。アプリ側では、この3つをそれぞれ独立したファイル・型として置くと、状態キーの選び方やDB投影の周期を後から変えやすい。定数、テーブル名、WAL配置、ホスト構成は対象ごとに決める値であり、他の実装からそのままコピーしない。

## 適用条件・制約

### 成立条件

- 状態を一つのキーへ分割できる。ユーザーID、部屋ID、注文IDなど、同じキーの更新を一つのownerへ送れる単位が候補になる。
- 同じキーの更新順序を保てる。各キーに一つのmailbox、actor、イベントループなどを割り当て、同じキーの処理を並行実行しない。
- 異なるキーは並列化できる。worker数、queue長、メモリ上限を設け、リクエスト数に比例してgoroutineや状態を無制限に増やさない。
- 更新を、変更された行のafter-image、追加、削除またはtombstoneを含む有限の差分として表現できる。差分からメモリ状態へ適用でき、DB投影とWAL replayの両方で同じ意味になる必要がある。
- リクエストが返す値を、DB投影完了前のhot stateから正しく構築できる。DBを読む別経路がある場合は、後述のflush・fence・cache失効を通して古いDB値を見せない。
- WALを再読込したとき、DBの最後の投影位置から未反映差分を順に適用できる。WALだけで復元するか、DBのcanonical stateとcheckpointを組み合わせるかを決める。

### 正本、鮮度、耐久性

hot stateは負荷の高い処理中の実行正本、WALは状態遷移の復旧用記録、DBは長期保存または外部参照用の投影先、という責務を明確に分ける。どれを最終的な業務正本と呼ぶかは対象の仕様に合わせるが、同じ更新を複数の独立した経路で自由に変更してはならない。

`Write`と`Sync`のどこをレスポンスの成功境界にするかを先に決める。

| 境界 | 意味 | 適用時の注意 |
| --- | --- | --- |
| WAL追記完了 | レコードがアプリのファイル書き込みへ渡った | OS・ファイルシステム障害時に失われる可能性がある。許容する損失範囲を仕様で確認する |
| group commit完了 | 対象sequenceまで`fsync`または同等の耐久化が完了した | 強い復旧要件に向く。複数リクエストを一回の同期へまとめる |
| DB transaction commit | DB上の投影が確定した | 他プロセスや旧経路がDBだけを読む場合の可視化境界になる |

公式仕様が更新の即時反映を要求する場合、DB投影を後回しにしても、hot stateを読む同一ownerの経路では更新後の値を返せなければならない。別owner、管理API、旧実装、追試用の読み取りがDBを参照するなら、対象キーのmaterializerを先にflushするか、同じWALを読む読取経路を用意する。

プロセス障害・OS障害・電源断で失われる範囲は異なる。レスポンスをWAL追記だけで成功にする設計は、`fsync`完了を待つ設計と同じ耐久性ではない。クラッシュ後にも受理済み更新を必ず保持する必要がある場合は、group commit後のdurable sequenceをレスポンスのbarrierにするか、外部の耐久キューを使う。競技や対象サービスが許す範囲で追記受理を先に返す場合でも、停止・owner切替・初期化・旧経路へのhandoffでは明示的なdurable fenceを置く。

### 整合性、認証認可、外部副作用

- 残高、在庫、予約、重複防止、ワンタイムトークン、受取済みフラグのような不変条件は、検証と状態遷移を同じキーのmailbox内で行う。検証だけをDBの外へ出してから更新すると、別リクエストとの間で二重消費が起こり得る。
- idempotency tokenやcredentialの発行、検証、消費を別のキャッシュとWALへ分割しない。種類、所有者、期限、消費時刻を同じ順序付き状態遷移として扱う。
- 認証、認可、所有権、テナント境界は、hot pathへ入る前または同じ正しい状態機械で確認する。認証前の`drain`や、別キーの状態を流用するseedを作らない。
- メール、決済、外部API、監査ログなどをWALのメモリ更新だけで完了扱いにしない。外部副作用が必須なら、WALまたはDBのoutboxへ冪等なイベントとして記録し、再送と重複実行の規則を持たせる。
- DB投影の失敗を成功値として捨てない。pendingを保持して再試行し、投影が必要な境界ではエラーを返すか旧経路へ安全に切り替える。

### 容量と適用しないケース

プロセス内に保持する行数、payload、WALサイズ、pending差分、ownerごとの状態キー数へ上限を置く。上限を超えた状態キーを部分的にhot化すると、hot stateとDBのどちらが正しいか判定できなくなるため、リクエスト単位で処理を中断して旧経路へhandoffする。

次の条件では、このパターンをそのまま適用しない。

- 複数キーを一つの原子的トランザクションで更新し、途中の状態を他のキーから見せてはならない。
- 書き込み直後に任意のホスト・任意のworkerからDBの値を読む必要があり、遅延投影やowner routingを許容できない。
- 状態が大きく、全件をメモリへ載せる上限・eviction・再構築方法を説明できない。
- 同じキーの書き込みが複数ホストへ無秩序に到達し、ownerを固定できない。
- 再起動後の復元に必要なデータがWAL、checkpoint、DBのいずれにも完全には残らない。
- 更新差分が非冪等な外部副作用を含み、sequenceやidempotency keyで再実行を抑止できない。

### 複数ホスト、WAL配置、世代

複数アプリホストで使う場合は、(1)キーからownerを決め、(2)通常リクエストをownerへ転送し、(3)ownerだけがhot stateとWALを書き、(4)owner切替時に旧ownerをfenceする、という境界をそろえる。ロードバランサーの振り分けだけでプロセス内状態を共有したことにしてはならない。

WALファイルは、対象ホストの再起動後にも読める永続ストレージへ置く。コンテナの一時領域、共有されないローカルディスク、複数ownerが同時に書ける単一ファイルを無条件に使わない。WALのgenerationまたはepochを初期化状態と結び付け、古い初期化のレコードを新しい状態へ混ぜない。

### 初期化、再起動、handoff

- 新規作成状態をseedする場合は、DB transactionのcommit後にのみ公開する。commit前にseedすると、失敗した登録がhot stateだけに残る。
- 通常再起動では、WALのヘッダ、version、長さ、checksum、generation、sequenceを検査し、壊れた末尾の扱いを明示する。途中の破損を無条件に無視しない。
- DBの`applied_seq`より後の差分だけをreplayし、同じafter-imageを再適用しても結果が変わらないようにする。projection markerの更新と投影行の更新は同じDB transactionに含める。
- `/initialize` 相当の処理では新世代を確定し、hot stateを停止またはpauseし、pendingをflushし、旧WAL・marker・キャッシュを新世代へ持ち越さない。初期化時間が負荷対象外でも、再現性と正当性は維持する。
- 認証、管理API、別サービス、owner切替など、hot経路以外が同じ状態を読む前に、対象キーだけをflush・durable fence・cache失効してからhandoffする。全状態キーを毎回drainする設計は、状態がないキーに余計なDBロードと待ち時間を発生させる。

## 探索方法

### まず仕様と構成を確定する

1. [`docs/official/`](../official/) の仕様から、更新の即時反映、レスポンスの成功条件、ワンタイム処理、初期化、再起動後の永続性、認証・管理APIの境界を確認する。仕様が不明なままDB投影を非同期化しない。
2. アプリホスト、DBホスト、ロードバランサー、shard、owner routingを対応づける。状態キーとアプリプロセスの所有関係がなければ、プロセス内状態ではなく共有ストアまたは別の設計を候補にする。
3. 直近の負荷上位経路をHTTP、DB、profile、ログの同じ窓で対応づける。候補は「DBの一文が遅い」だけでなく、同じ状態キーへの複数DML、接続プール待ち、commit、ロック待ち、書き込みバイト数、レスポンス後でも必須なprojection処理の合計で判断する。

### 書き込み経路を列挙する

次の検索語で、入口からDB・キャッシュ・副作用までたどる。

| 観点 | 検索語・確認箇所 | 適用候補の兆候 |
| --- | --- | --- |
| 取引境界 | `Begin`, `BeginTx`, `Beginx`, `Commit`, `Rollback`, `FOR UPDATE` | 一つの状態キー更新で複数の小さなtransactionや長いロック待ちがある |
| 更新 | `INSERT`, `UPDATE`, `DELETE`, `Exec`, `Upsert`, ORMのsave | 同じキーへの更新を行ごと・リクエストごとに発行している |
| 状態 | `cache`, `sync.Map`, `RWMutex`, `actor`, `mailbox`, `queue` | 既にキー単位で管理できる可変状態がある、または導入可能である |
| WAL | `WAL`, `wal`, `append`, `Write`, `Sync`, `fsync`, `sequence`, `checksum` | 追記記録と耐久化待ちを分離できる。順序と復旧位置を持てる |
| 投影 | `materialize`, `projection`, `applied_seq`, `checkpoint`, `replay` | DBへ送る行を差分化でき、複数行upsertと再試行が可能である |
| ライフサイクル | `initialize`, `startup`, `shutdown`, `close`, `restart`, `drain`, `fence` | hot stateと旧経路の切り替え点を一つに集約できる |
| 正当性 | `token`, `idempot`, `balance`, `stock`, `receive`, `auth`, `owner` | 同一キー内に順序・一意性・所有権の不変条件がある |

特に、ハンドラの一回の処理で「DBから状態を読む → 計算する → 複数テーブルを更新する → commitする」を繰り返している経路を、状態の読み取り・遷移・外部参照ごとに分解する。DB更新を非同期化するだけで、次のリクエストが古いDBを読むなら候補ではない。

### 候補の成立性を確認する

候補ごとに次を一枚の状態遷移として書く。

```text
input and auth
  -> key ownership
  -> current hot state or canonical load
  -> invariant check
  -> delta / tombstone creation
  -> WAL append barrier
  -> hot state apply
  -> response
  -> DB projection
  -> external read or fallback barrier
```

次の問いに答えられない候補は、オンメモリ化の対象から外すか、より狭いread modelに留める。

- 同じキーを二つのworkerが同時に更新しないか。
- どの値をレスポンスの正本として読むか。
- DB投影が遅れている間に、旧経路や別ホストが何を読むか。
- 差分に含める更新行、追加、削除、期限、idempotency keyやtokenの消費を全経路で列挙できるか。
- WALが途中まで書かれた、DB commitだけ成功した、プロセスが停止した、ownerが変わった場合に、どこから再開するか。
- 状態サイズ上限を超えたとき、重複実行や古いcacheを残さず旧経路へ戻せるか。

### 採用実装での確認箇所

当日の実装では、以下の責務がどこにあるかを`task setup-webapp`で取得したコードから特定する。

- WAL・mailbox・materializer本体 — 状態キーごとの直列化、投入、状態遷移、追記、durable sequence、復元、旧経路へのfallback、初期化時の準備を対応づける。
- handlerの入口 — 状態を作った直後のseed、認証・セッション経路でのdrain、更新系handlerのhot/legacy分岐を確認する。
- ルーティング境界 — 状態キーからownerを決め、非ownerのリクエストを転送する箇所を確認する。
- スキーマ定義 — runtime行、generation、投影位置（`applied_seq`相当）、主キー・一意キーを確認する。
- 初期化処理 — 初期化時にmarker、runtime、groupをどう再作成するかを確認する。

## 実装方法

### 1. 正本とownerを決める

対象キーごとに次を定義する。

- hot stateに保持するフィールドと、保持しない大きな履歴・masterデータ
- DBのcanonical row、WALのrecord、projection rowの対応
- ownerの選択方法と、非ownerからの転送方法
- 状態の最大件数、最大payload、queue長、WALの世代と配置
- hot pathで処理する操作、旧経路へhandoffする操作

ownerを固定できない場合は、最初から共有ストア・分散ログ・DB内actorなど、複数writerを前提にした方式を選ぶ。単に各ホストへ同じコードを配るだけでは、状態キー単位の直列性は成立しない。

### 2. commit後にboundedな状態をseedする

新規作成やcanonical loadの直後に、すでに確定した値をhot stateへ渡す。DB commit前には公開しない。状態が上限を超える、関連行のグループ化に失敗する、必須データが欠ける場合は、部分seedせず旧経路を使う。

```go
state, ok := buildBoundedState(stateKey, initialRows)
if err := tx.Commit(); err != nil {
	return err
}
if ok {
	engine.Seed(stateKey, state)
}
```

既存の状態キーの復旧では、DBのcanonical stateを読み、`applied_seq`より後のWAL差分だけを適用する。通常のリクエストで状態がないキーを暗黙に完全ロードすると、hot化のためのDB読み込みが元の処理を上回ることがある。coldキーは明示的にloadする経路か、旧経路へ戻す経路へ分ける。

### 3. 同じキーをmailboxで直列化する

各状態キーに常駐goroutineを作るのではなく、mailboxをbounded worker poolへ投入する。mailbox内では一つのstateを順に処理し、worker間では異なるキーを並列に扱う。

```go
func (e *Engine) submit(key int64, fn func(*State) (any, error)) (any, bool, error) {
	box := e.activeMailbox(key)
	if box == nil {
		return nil, false, nil
	}
	job := job{fn: fn, result: make(chan result, 1)}
	box.enqueue(job)
	e.schedule(box)
	r := <-job.result
	return r.value, r.handled, r.err
}
```

実際には、mailboxの取得・scheduleフラグ・停止・pause・fallbackを同じmutexまたはchannel規則で保護する。`false`は「業務エラー」ではなく、対象状態がhot経路で扱われず旧経路へhandoffすべき場合と区別する。

### 4. mutationから変更差分を作る

mutation関数は、入力検証と不変条件の判定を現在stateに対して行い、状態を直接変更せずに、結果とdeltaを返す。deltaはafter-imageと削除情報を持つ。

```go
type Delta struct {
	Key                 int64
	Sequence            uint64
	KeySequence         uint64
	Runtime             *StateRow
	Idempotency          *IdempotencyRecord
	IdempotencyChanged   bool
	Rows                []RowAfterImage
	Deletes             []DeleteMark
}

func (e *Engine) mutate(key int64, fn func(*State) (any, *Delta, error)) (any, bool, error) {
	return e.submit(key, func(state *State) (any, error) {
		value, delta, err := fn(state)
		if err != nil || delta == nil {
			return value, err
		}
		delta.Key = key
		delta.KeySequence = state.KeySequence + 1
		seq, err := e.wal.Append(delta)
		if err != nil {
			return nil, err
		}
		delta.Sequence = seq
		ApplyDelta(state, delta)
		e.materializer.Enqueue(delta)
		return value, nil
	})
}
```

`Append`が失敗した場合にメモリ状態だけが進まない順序にする。slice、map、pointer、payloadはWAL、hot state、materializerで共有せず、コピーするか不変値として扱う。行の削除は、投影側で確実に適用できるtombstoneまたはdelete listとしてdeltaに含める。

通常のmutationで完全な状態をcloneしてJSON化しない。完全snapshotが必要ならcheckpoint・明示的なhandoff・復旧用として独立させ、WALのdelta形式とDB投影形式を混同しない。checkpointを導入する場合は、checkpoint sequenceより前のdeltaを安全に捨てられる条件も定義する。

### 5. WALを追記、同期、replay可能にする

WALレコードには少なくとも、version、generation、record kind、global sequence、key、key内sequence、payload length、checksumを持たせる。JSONは実装しやすいが、頻度とpayloadサイズによっては固定長binaryやcompact encodingを検討する。値をSQL文字列へ埋め込まず、WALのpayloadも入力サイズ上限を検査する。

```go
func (w *WAL) Append(delta *Delta) (uint64, error) {
	w.writeMu.Lock()
	defer w.writeMu.Unlock()

	seq := w.nextSequence()
	record := encodeRecord(w.version, w.generation, seq, delta)
	if _, err := w.file.Write(record); err != nil {
		w.markFailed(err)
		return 0, err
	}
	w.markWritten(seq, uint64(len(record)))
	w.notifySync()
	return seq, nil
}

func (w *WAL) SyncUntil(seq uint64) error {
	return w.waitDurable(seq)
}
```

writerのmutexは物理的なレコード順序だけを守り、`file.Sync()`中に通常のappendを同じmutexで待たせない。同期workerは時間窓またはバイト閾値で`writtenSeq`を取り込み、一回の`Sync`で複数レコードをdurableにし、完了したwaiterへ`durableSeq`を通知する。

通常リクエストの完了境界を追記受理にする場合でも、`prepareLegacyFallback`、owner移管、停止、初期化のような境界では`SyncUntil`を使う。強い耐久性が必要な対象では、通常リクエストもgroup commitのdurable sequenceを待つ。

WALのreplayでは、generationが現在の世代と一致するか、sequenceが単調か、checksumとpayloadの長さが正しいかを検証する。壊れた末尾を切り詰める仕様を採る場合も、途中の破損や異なるversionを無視して処理を続けない。

### 6. DB materializerを差分・batch・冪等にする

materializerはキーごとの最新deltaをpending mapへcoalesceする。after-imageを持つ行は同じ主キーの最新値を残し、削除はtombstoneを失わない。一定時間またはキー数でbatchを取り出し、テーブル単位の複数行upsertを一つのDB transactionで実行する。

```sql
INSERT INTO state_runtime
  (state_key, generation, state_version, updated_at)
VALUES
  (?, ?, ?, ?), (?, ?, ?, ?)
ON DUPLICATE KEY UPDATE
  generation = VALUES(generation),
  state_version = VALUES(state_version),
  updated_at = VALUES(updated_at);

INSERT INTO state_projection_marker
  (state_key, generation, applied_seq, updated_at)
VALUES (?, ?, ?, ?)
ON DUPLICATE KEY UPDATE
  generation = VALUES(generation),
  applied_seq = GREATEST(applied_seq, VALUES(applied_seq)),
  updated_at = VALUES(updated_at);
```

これは説明用のSQLであり、table名、列、upsert構文、generation型は対象DBとschemaへ置き換える。marker更新を業務行と同じtransactionへ含めることで、DB反映済みsequenceと実データの境界をそろえる。batch失敗時はdeltaを再queueし、同じbatchを再実行してもafter-imageが壊れないようにする。

DB接続プール、`max_allowed_packet`、placeholder数、行ロック、commit時間に合わせてbatchを分割する。materializerの遅延を無制限に増やさず、pending件数・WAL backlog・projection lagが容量上限を超えたときの停止または同期flushを定義する。

### 7. hot pathとlegacy pathのhandoffを一つにする

hot操作が対象外・容量超過・復旧失敗になったとき、各handlerが個別にlegacyへ進むのではなく、共通handoffを呼ぶ。

```text
disable new jobs for the key
  -> drain the mailbox
  -> flush projection through the latest sequence
  -> durable fence the WAL when required
  -> invalidate legacy caches
  -> transfer still-valid idempotency ownership
  -> remove or fence hot state
  -> execute legacy path
```

hot側で確定した業務エラーを、容量超過と混同して旧経路へ再実行しない。逆に、旧経路へ移るのにhot state、idempotency key、DB、cacheを残すと、古い状態を使った二重処理や不正レスポンスが発生する。

認証、admin、外部参照、cache missの読み取りなど、hot stateを直接読まない経路にも同じ対象キーのflush barrierを用意する。状態がないキーに対して毎回load・fenceするのではなく、active mailboxまたは未反映WALがある場合だけ実行する。

### 8. 再起動と初期化を実装する

再起動時の基本経路は次のとおりである。

```text
open owner-local WAL
  -> validate and replay records
  -> load canonical DB state for a key when needed
  -> read projection marker
  -> apply WAL records after applied_seq
  -> publish mailbox only after recovery succeeds
```

DB側のmarkerがない場合、DBがどのsequenceまで反映済みかを推測しない。初期化時にmarkerをtruncateする、世代を更新する、または正本から再構築する。WALを消す前には、必要な差分がDBへ投影され、対象sequenceがdurableであることを確認する。

実装例としては、状態キー単位のseed後に投影位置テーブル（`applied_seq`相当の列）でどこまでDBへ反映済みかを持ち、初期化エンドポイントが通る経路でmarkerとruntimeを作り直す。既存のmigration・初期化スクリプトを正本として、対象の状態キーに対応する同じライフサイクルを組み込む。

### 9. 設定と置換箇所を対象環境へ合わせる

次の値は候補値であり、無条件にコピーしない。

- syncの時間窓とバイト閾値: ストレージの同期遅延、許容するdurability window、リクエスト量で決める。
- materializerのキー数・行数・時間窓: DBのcommit、packet、lock、connection poolの容量で決める。
- hot stateの行数・payload上限: 代表的な状態キーの状態量、heap、GC、fallbackの許容コストで決める。
- worker数: CPU、キーの並列度、DB投影の容量に合わせる。`GOMAXPROCS`へ機械的に比例させない。
- WALディレクトリとservice権限: 再起動後に残る専用ディレクトリ、所有者、mode、ディスク容量、rotation方針を既存のservice配置へ合わせる。
- owner数・キー分割・DB shard: アプリホストとDBの配置、転送遅延、owner障害時の再配置方法へ合わせる。

生成されたnginx設定や稼働中ホスト上のファイルを直接編集せず、対象プロジェクトの設定正本、初期化DDL、service定義、アプリの起動処理から変更する。WALを導入するためだけに、既存のDB正本・認証・公式の初期化処理を削除しない。

## 参考資料

- [`docs/official/`](../official/) の当日マニュアル — 更新の即時反映、初期化・負荷走行の失敗、HTTPエラーとタイムアウトの扱い、追試で再起動後に負荷走行中の書き込みを取得できることを確認する根拠。
- [`docs/official/`](../official/) のアプリケーションマニュアル — ワンタイムトークンなど、同じリクエストの重複実行を防ぐ状態遷移を同期境界に残す根拠。
- [オンメモリ化](in-memory.md) — cache、read model、プロセス再起動、複数ホスト、鮮度を検討する一般的な前提。
- [アプリケーション処理の非同期化](asynchronous-processing.md) — レスポンス後に失われてもよい処理と、完了保証が必要な処理を分ける前提。
