# アプリケーション処理の非同期化

## 概要

Go アプリケーションの一つのリクエスト内で、互いに独立した処理を goroutine へ分けて並列に実行する実装パターンである。外部 API の取得、独立した読み取り、複数の CPU 処理など、直列に待つ必要のない処理を同時に開始することで、リクエスト中の待ち時間を重ね合わせる。

このパターンには、次の二つの形がある。

~~~text
リクエスト内の並列化:
request -> goroutine A / goroutine B -> Wait -> response

レスポンス後の実行:
request -> goroutine を起動 -> response
                    └-> best-effort な処理
~~~

前者はレスポンス返却前に Wait するため、API の成功条件とエラー処理を保ちやすい。後者はレスポンスを速く返せる一方、プロセス終了時に処理が失われ、エラーを呼び出し元へ返せない。そのため、後者は失ってもよいログ・メトリクス・通知などに限る。

プロセス外へジョブを永続化する方式は、このsolutionの対象外とする。再起動後も必ず実行する必要がある処理は、アプリケーションコードだけの goroutine では保証できないため、このパターンを適用しない。

## 適用条件・制約

### 適用できる条件

- 並列に開始する処理が互いの結果を必要としない。一方の結果を使って次の処理を決める場合は、その依存関係に沿って直列に実行する。
- 各 goroutine が別々の入力・結果領域を扱い、同じ map、slice、Rows、トランザクション、可変な状態を無同期で共有しない。
- リクエスト内の並列化では、すべての goroutine の完了を待ち、エラーをレスポンスへ反映できる。エラーを無視したまま成功を返してはならない。
- レスポンス後の goroutine では、処理が失われても仕様を壊さない。失敗時の再試行や完了保証が必要なら対象外である。
- 同時実行数を固定値、semaphore、errgroup.SetLimit などで制限する。リクエスト数を掛け合わせた総並列数が、DB接続数や外部 API の rate limit を超えないようにする。

### 同期に残す境界

次の処理を、結果を待たずに goroutine へ移してはならない。

- 認証・認可、対象ユーザーやテナントの所有権確認
- 在庫、座席、残高、予約枠、重複登録防止キーなどの一意性・原子性を決める更新
- レスポンスに返す値を確定する正本更新
- 仕様が read-after-write、処理完了、厳密な順序を要求する副作用

同じトランザクションや行ロックを複数 goroutine から操作してはならない。独立した読み取りに分ける場合も、同じスナップショットが必要なら一つのトランザクションや一括 SQL を優先する。長い外部 API 呼び出しをロック保持中に並列化して、ロック競合を増やしてはならない。

### DB・外部 API・CPU の制約

- N+1 クエリを goroutine で並列化しても、クエリ数は減らない。関連データは JOIN、IN、bulk、preload を優先し、個別 I/O の並列化はそれらでまとめられない場合に限る。
- 外部 API の同時実行数・要求頻度が API キー、テナント、送信元などの単位で制限されている場合、リクエストごとの goroutine 数制限だけでは不十分である。アプリケーション全体で共有する上限が必要な処理は対象外にするか、既存の制御機構を使う。
- DB が CPU、接続プール、ディスク I/O、ロックで律速している場合、並列化は待ち時間を減らさず競合を増やす。sql.Tx、同じ Rows、同じロック対象を並列に使わない。
- CPU-bound な処理は、goroutine 数を増やせば必ず速くなるわけではない。利用可能な CPU コア数、GC、メモリ使用量を超える無制限な並列化をしない。

### goroutine のライフサイクル

- リクエスト内 goroutine にはリクエストの context.Context を渡し、タイムアウトやクライアント切断で外部処理も停止できるようにする。
- レスポンス後も続ける best-effort goroutine には、リクエスト終了と同時にキャンセルされる context を無条件に渡さない。ただしアプリケーション停止時には新規処理を止め、プロセス終了まで無制限に残さない。
- go func の起動数に上限を設け、上限到達時に破棄してよいのか、同期処理へ戻すのかを処理種別ごとに定める。破棄できない処理にはこの方式を使わない。
- goroutine 間で共有する状態は不変値として渡すか、mutex、atomic、channel などで同期する。データ競合を「処理が短いから」とみなして許容してはならない。

## 探索方法

1. docs/official/ の当日マニュアル、アプリケーションマニュアル、API 定義を読み、レスポンスの成功条件、処理完了の要否、外部 API の rate limit、反映期限、厳密な順序を確認する。レスポンス後に処理が失われても仕様を満たす経路だけを best-effort 候補にする。
2. route/controller/handler から呼び出し先をたどり、直列に並んだ http.Client.Do、gRPC client、独立した SELECT、ファイル読み込み、JSON変換、画像・文書変換などを探す。go 、errgroup、sync.WaitGroup、channel、semaphore、SetLimit を検索語にする。
3. 処理間の依存関係を図にする。一方の結果を必要とする経路は直列、独立して開始できる経路は並列候補、正本更新・ロック・認証を含む経路は同期境界として残す。関数単位で分けられていても、DBトランザクションや共有状態を隠し持っていないか確認する。
4. DBアクセスは、使用する接続プール、トランザクション、FOR UPDATE、更新対象キーを確認する。同時に実行して安全なのは別々の読み取りか、互いにロックを取得しない処理に限る。関連一覧の取得が目的なら、goroutineより先にN+1解消の候補として分類する。
5. 外部 API は、同じ API キー・ユーザー・テナントに対する並列数、呼び出し順、429・タイムアウト時の扱いを確認する。独立した呼び出しでも全リクエスト合計の制限を守れない場合は、単純な goroutine 化の候補から外す。
6. このリポジトリ固有では、アプリケーション本体の webapp/go で handler から model・外部クライアントへの経路を確認する。今回のパターンでは新しいサービスや外部キューを追加せず、アプリケーションコード内の変更だけを対象にする。

## 実装方法

### リクエスト内の独立処理を errgroup で並列化する

goroutine を使っても、レスポンス返却前に Wait するなら、これはバックグラウンド化ではなく request 内の fan-out/fan-in である。独立した I/O の待ち時間を重ね合わせられるため、互いに結果を必要としない外部 API の取得や読み取りが候補になる。

Go では errgroup.WithContext を使うと、最初のエラーを返し、関連する context をキャンセルし、Wait で goroutine の完了を待てる。SetLimit で一つの group 内の同時実行数も制限する。次のコードは説明用であり、関数、型、同時実行数は対象アプリへ置き換える。

~~~go
import (
    "context"
    "database/sql"

    "golang.org/x/sync/errgroup"
)

func LoadPage(ctx context.Context, db *sql.DB, userID int64) (Page, error) {
    g, ctx := errgroup.WithContext(ctx)
    g.SetLimit(2)

    var profile Profile
    var settings Settings

    g.Go(func() error {
        var err error
        profile, err = loadProfile(ctx, db, userID)
        return err
    })
    g.Go(func() error {
        var err error
        settings, err = loadSettings(ctx, db)
        return err
    })

    if err := g.Wait(); err != nil {
        return Page{}, err
    }
    return Page{Profile: profile, Settings: settings}, nil
}
~~~

この例では、各 goroutine が別の変数へ書き込み、g.Wait の後でだけ読み取っている。共有 map や slice へ同時に append する場合は、結果用の添字を分ける、mutex を使う、channel で集約するなど、データ競合を避ける形へ置き換える。

### エラーとキャンセルを伝播する

並列化した処理の一つが失敗した場合に、残りの外部呼び出しを続ける必要がなければ、共有 context をキャンセルする。各 HTTP、gRPC、DB 呼び出しへその context を渡し、キャンセルを無視する関数を goroutine の中へ置かない。

sync.WaitGroup を使う場合も、エラー用 channel、context のキャンセル、全 goroutine の完了待ちを別途実装する。go func の中でエラーを捨て、handler は常に成功を返す実装にしてはならない。

### レスポンス後の best-effort 処理を bounded にする

ログやメトリクスなど、失われても正しいレスポンスや正本を壊さない処理だけは、レスポンス返却前に待たず goroutine へ渡せる。ただし無制限に起動せず、アプリケーション内の semaphore で上限を設ける。

~~~go
var bestEffortSlots = make(chan struct{}, 32)

func SendBestEffort(serviceCtx context.Context, event Event) bool {
    select {
    case bestEffortSlots <- struct{}{}:
    default:
        return false
    }

    go func(event Event) {
        defer func() { <-bestEffortSlots }()
        _ = sendLog(serviceCtx, event)
    }(event)
    return true
}
~~~

上限到達時に false を返してよいのは、呼び出し元がその処理の欠落を許容する場合だけである。通知、決済、監査ログ、データ更新などをこの形で捨ててはならない。再試行、完了保証、プロセス再起動後の復旧が必要な処理は、このsolutionの対象外とする。

serviceCtx はアプリケーションのライフサイクルに属する context とし、リクエストの context をそのまま渡すかどうかは仕様で決める。プロセス停止時は新しい goroutine の起動を止め、処理中の goroutine が利用する接続や外部 API の timeout を解放できるようにする。

### 並列化しない処理を明確にする

- N+1 は JOIN、IN、bulk、preload でクエリ数を減らす。goroutine は最後の手段であり、最初の解決策にしない。
- 同じ DB トランザクション、同じ行ロック、同じ外部 API キーの順序依存処理は直列に保つ。
- 在庫、残高、座席、予約、決済、認証など、結果が正本の状態遷移を決める処理は同期境界から切り離さない。
- CPU-bound 処理は、固定数の goroutine と入力分割を使い、共有データを更新する順序を決める。goroutine 数を入力件数と同じにしない。
- goroutine 化でリクエストが速くなっても、外部 API の制限、DBの競合、CPU、メモリを別の律速へ移すだけの場合がある。処理の独立性とリソース上限を説明できない変更は適用しない。

このリポジトリ固有の実装では、Goの変更を webapp/go に置く。etc/env.sh、nginx/conf.d/upstream.conf、新しいサービス定義や外部キューを変更対象にせず、既存アプリケーションのライフサイクル内で goroutine を開始・終了できる形にする。

## 参考資料

- [ISUCON8 本選問題の解説と講評](https://isucon.net/archives/52598691.html) — POST /orders から runTrade を分離し、レスポンスが必要とする範囲を確認したうえで処理を非同期実行する例。
- [golang.org/x/sync/errgroup package documentation](https://pkg.go.dev/golang.org/x/sync/errgroup) — goroutine のエラー伝播、context キャンセル、同時実行数制限、完了待ちの仕様。
