// measurectl は計測サイクルの実行と記録をまとめる CLI。
//
// Taskfile が持っていた「1 走行で何が起きたか」の情報は、scores.tsv の 1 行と
// 0 バイトの .stderr が残っているかどうかに散っていた。ここではそれを
// runs/<RUN_ID>/run.json 1 枚へ集約する。
//
//	measurectl manifest begin ...       走行開始時点の manifest を固定する
//	measurectl manifest finalize ...    回収結果で manifest を完成させる
package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "run":
		err = runLifecycle(os.Args[2:])
	case "manifest":
		err = runManifest(os.Args[2:])
	case "collect":
		err = runCollect(os.Args[2:])
	case "digest":
		err = runDigest(os.Args[2:])
	case "artifacts":
		err = runArtifacts(os.Args[2:])
	case "-h", "--help", "help":
		usage()
		return
	default:
		fmt.Fprintf(os.Stderr, "unknown subcommand: %s\n\n", os.Args[1])
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "measurectl: %v\n", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `measurectl — ISUCON 計測サイクルの実行と記録

usage:
  measurectl run begin [flags]                 RUN作成、manifest、prepare、collector起動を一括実行する
  measurectl run finalize [flags]              collector停止、digest、manifest確定を一括実行する
  measurectl collect prepare [flags]           走行前の下ごしらえ (ログのローテートなど) を流す
  measurectl collect start  [flags]            collectors.yaml の collector を全ホストで起動する
  measurectl collect stop   [flags]            collector を止めて runs/<RUN_ID>/ へ回収する
  measurectl collect oneshot [flags]           走行中に 1 回だけ実行するもの (fgprofなど) を回収する
  measurectl collect check-clean [flags]       全ホストの collector プロセスと残骸がゼロか確認する
  measurectl collect sweep  [flags]            残った collector と作業ディレクトリを掃除する
  measurectl digest         [flags]            生ログを回収し、digesters.yaml の集計をかけて畳む
  measurectl manifest begin [flags]            走行開始時点の run.json を書く
  measurectl manifest finalize [flags]         scoreと成果物を追記してrun.jsonを完成させる
  measurectl artifacts [flags]                 宣言が出す成果物を一覧する
                                             -run-dir <dir> 実走行に揃っているか照合
                                             -check         読み手とのズレを検出

-dry-run を付けると、実際に流す ssh / rsync のコマンドだけを表示する。
既定無効の collector は collect start / stop に -include <name> を付けて有効化する。
-no-collectors は prepare とdigestを維持したまま常駐collectorだけを無効にする。

`)
}
