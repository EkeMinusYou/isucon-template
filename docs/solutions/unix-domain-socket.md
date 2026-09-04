# Unix Domain Socket で nginx と Go アプリを接続する

## 概要

nginx と Go アプリケーションが同じホストで動作する経路を、TCP の loopback 接続から Unix Domain
Socket（UDS）へ切り替える実装パターンを示す。ホスト内の接続で不要な TCP の処理を省ける可能性がある
一方、nginx の upstream、アプリの listener、systemd の権限、socket のライフサイクルを同時にそろえる
必要がある。

UDS はホスト間接続には使えない。nginx とアプリが別ホストにある構成では、対象経路だけ TCP のまま残す
か、役割を同居させて接続経路全体を設計し直す。

## 適用条件・制約

- nginx と対象アプリのプロセスが同じホストにあり、nginx から socket の親ディレクトリと socket へ
  接続できることが必要である。
- アプリを直接 `host:8080` で呼ぶベンチマーカー、別ホストのサービス、疎通確認、特殊 API がある場合は、
  TCP listener を残すか、それらの呼び出し側も UDS を利用できる構成へ変更する。UDS 化だけで TCP listener
  を削除しない。
- socket パスは nginx とアプリで完全に一致させる。古い socket を起動時に扱い、通常ファイルを誤って削除
  しない。親ディレクトリと socket の所有者・グループ・モードは、nginx の実ユーザーに必要最小限の権限を
  与える。
- systemd の `RuntimeDirectory` など、再起動時にも確実に作られる runtime directory を使う。`/tmp` に
  広い権限で置く構成や `0777` の socket は、所有権と不要な接続を管理しにくい。
- `/api/initialize` や `/api/register` のように別ホストへ固定された経路がある場合、通常 API だけを UDS
  にして他の upstream は TCP のままにできるかを確認する。
- 業務 listener と pprof などの計測用 listener は別物である。業務 listener を UDS 化しても、必要な計測用
  listener を一緒に削除しない。

このリポジトリでは、`Taskfile.yml` の `NGINX_HOSTS` と `APP_TRAFFIC_HOSTS` を比較し、同じホストに
割り当てられた経路だけを UDS の候補にする。役割を別ホストへ分離した場合、その経路は TCP のまま残す。
ベンチマーカーや内部の疎通確認がアプリの TCP listener を前提とする場合は、その経路を残すか呼び出し側も
合わせて変更する。

## 探索方法

1. nginx と各アプリの役割を IP・ホスト単位で対応づけ、同一ホストになる経路だけを候補にする。通常 API、
   初期化・登録、静的ファイル、ヘルスチェック、ベンチマーカーからの直接接続を分けて調べる。
2. nginx の `upstream`、`proxy_pass`、`proxy_http_version` と、アプリの `net.Listen`、`e.Start`、
   `listenPort` を検索する。`8080` が設定ファイル・Taskfile・サービス unit のどこで参照されるかも追う。
3. systemd unit の `User`、`Group`、`ExecStart`、`RuntimeDirectory`、`UMask` と、nginx の `user` 設定を
   両方確認する。親ディレクトリへ入る権限と socket へ接続する権限を別々に考える。
4. 設定の生成元と配布経路を特定する。このリポジトリの `nginx/conf.d/upstream.conf` は
   `task gen` が `Taskfile.yml` の役割・IP・portから生成するため、生成後のファイルを直接編集しない。
5. アプリの graceful shutdown、再起動、異常終了、複数 worker の起動時に、同じ socket パスを安全に再利用
   できるかをコードと unit の両方から確認する。

## 実装方法

### nginx の upstream

同一ホストの upstream だけを Unix socket へ向け、ホスト間の upstream は TCP のままにする。例では通常
API の upstream だけを変更している。

```nginx
upstream app {
  server unix:/run/app/app.sock;
  keepalive 128;
}

upstream remote_app {
  server <REMOTE_HOST>:<REMOTE_PORT>;
  keepalive 16;
}
```

`location` ごとの `proxy_pass` の振り分けや、必要な timeout・header は経路変更と同時に変えない。ホスト
ごとにローカル app が異なる場合は、同じ socket パスを全 nginx ホストへ配るのではなく、生成元がホストごと
の宛先を表現できるかを確認する。

### Go アプリの listener

`e.Start` が作る TCP listener をそのまま UDS に置き換えるのではなく、socket を検査してから listener を
作り、Echo の server へ渡す。次の関数は、既存パスが socket の場合だけ削除する例である。

```go
func listenUnix(socketPath string) (net.Listener, error) {
	info, err := os.Lstat(socketPath)
	switch {
	case err == nil:
		if info.Mode()&os.ModeSocket == 0 {
			return nil, fmt.Errorf("refusing to remove non-socket path: %s", socketPath)
		}
		if err := os.Remove(socketPath); err != nil {
			return nil, fmt.Errorf("remove stale socket: %w", err)
		}
	case !errors.Is(err, os.ErrNotExist):
		return nil, fmt.Errorf("inspect socket: %w", err)
	}

	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return nil, fmt.Errorf("listen on unix socket: %w", err)
	}
	if err := os.Chmod(socketPath, 0o660); err != nil {
		listener.Close()
		return nil, fmt.Errorf("chmod unix socket: %w", err)
	}
	return listener, nil
}
```

アプリケーション側では、作成した listener を server に設定して起動する。`errors`、`fmt`、`net`、`os`
の import と、graceful shutdown 時に listener を閉じる処理を既存の起動構造へ合わせる。

```go
listener, err := listenUnix("/run/app/app.sock")
if err != nil {
	e.Logger.Errorf("failed to create unix listener: %v", err)
	os.Exit(1)
}
e.Listener = listener

server := &http.Server{}
if err := e.StartServer(server); err != nil {
	e.Logger.Errorf("failed to start HTTP server: %v", err)
	os.Exit(1)
}
```

### systemd の runtime directory と権限

nginx の実ユーザーが socket を開けるよう、専用グループまたは既存のサービスグループを明示する。設定例は
概念例であり、既存の `ExecStart`、`WorkingDirectory`、`EnvironmentFile`、restart 設定に合わせて組み込む。

```ini
[Service]
RuntimeDirectory=app
RuntimeDirectoryMode=0770
UMask=0007
User=isucon
Group=www-data
```

このリポジトリの unit は現状 `Group=isucon`、nginx は `www-data` であるため、この例をそのまま適用できない。
専用グループを作るのか unit のグループを変更するのかを、他のファイル権限やサービスとの共有範囲を含めて
決める。`RuntimeDirectory` で作る親ディレクトリと、アプリが `chmod` する socket の両方が nginx から到達
可能でなければならない。

生成設定を使うリポジトリでは、nginx の生成元、systemd unit、アプリの socket パスを一つの変更として管理
し、生成物やサーバー上の `/etc/nginx` を直接編集しない。
