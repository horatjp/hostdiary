# hostdiary インストール手順

hostdiary は、1台の Linux サーバの CPU・メモリ・ディスク・ネットワークなどを1分ごとに記録し、
ブラウザで見られる画面と JSON を書き出すツールです。常駐するデーモン・データベース・外部への通信はありません。
詳しい仕様は同梱の `README.md`(英語)を見てください。

## 0. この配布物の中身

| ファイル | 内容 |
|---|---|
| `hostdiary` | 実行ファイル(1つで完結。ほかに入れるものはない) |
| `INSTALL.md` | 英語の手順書(正本) |
| `INSTALL.ja.md` | この手順書(英語版の日本語訳) |
| `config.example.json` | 設定ファイルの例 |
| `nginx-hostdiary.conf.example` | nginx の Basic 認証の内側で画面を見せる設定例 |
| `README.md`・`LICENSE` | 仕様の説明(英語)とライセンス(MIT) |
| `SHA256SUMS` | 上のファイルの SHA-256 |

## 1. 前提を確かめる

- OS: Debian 12 以降、または Ubuntu 22.04 以降
- CPU: 配布物の名前の `amd64` / `arm64` と合っていること
- systemd が PID 1 で動いていること、root になれること(sudo)

```sh
. /etc/os-release && echo "$PRETTY_NAME"
uname -m                    # x86_64: amd64 archive; aarch64: arm64 archive
ps -p 1 -o comm=            # Must print systemd
```

## 2. 展開して中身を確かめる

```sh
tar -xzf hostdiary-<version>-linux-<amd64|arm64>.tar.gz
cd hostdiary-<version>-linux-<amd64|arm64>
sha256sum -c SHA256SUMS     # All entries must report OK
./hostdiary --version       # Must print the version
```

配布物そのものの SHA-256 を受け取っている場合は、展開の前に `sha256sum <配布物>` で照らし合わせてください。

## 3. 設定する(任意)

設定ファイルが無くても動きます(装置は自動で見つけ、ホスト名はシステムのもの、アプリの記録はなし)。
アプリ(nginx・php-fpm など)のプロセス数とメモリも記録したいときは、install の前に置きます。

```sh
sudo install -d -o root -g root -m 0755 /etc/hostdiary
sudo install -o root -g root -m 0644 config.example.json /etc/hostdiary/config.json
sudoedit /etc/hostdiary/config.json
```

- `apps`: 表示名と、`/proc/<pid>/stat` のプロセス名(15バイトまで、完全一致)の並び。どれかに一致すれば数える
- `net`・`disk`・`diskio`: 記録するインターフェース・マウントポイント・ブロックデバイスを名前で絞る(空なら自動)
- `dirs.enabled`: 毎日のディレクトリ容量の測定。**ディレクトリ名(`/var/www/<サイト>`・`/home/<ユーザー>` など)が画面に出る**ので、出したくなければ `false` にする
- 知らない項目・型の違い・コメントはエラーになります(JSON にコメントは書けません)

## 4. インストールする

```sh
sudo ./hostdiary install
```

入るもの: `/usr/local/bin/hostdiary`、`/etc/systemd/system/hostdiary-{collect,dirs}.{service,timer}` の4つ、
`/etc/hostdiary/`(無ければ作る)。データは `/var/lib/hostdiary/` に溜まります。

install の中で試しの収集が1回走り、成功したときだけタイマーを有効にします。最後に次の2行が出れば成功です。

```
timer: enabled=enabled, active=active
dirs timer: enabled=enabled, active=active
```

失敗したときは、表示された内容と `journalctl -u hostdiary-collect.service` を見てください。
展開したディレクトリは、install が済んだら消してかまいません(実行ファイルは `/usr/local/bin` に入っています)。

## 5. 動作を確かめる

```sh
systemctl list-timers 'hostdiary-*.timer'     # collect: every minute; dirs: around 04:30 every morning
systemctl is-failed hostdiary-collect.service # inactive means it has not failed
ls /var/lib/hostdiary/public/                 # Contains index.html, index.json, data/, etc.
```

ディレクトリ容量を今すぐ測るときは `sudo systemctl start hostdiary-dirs.service`。

## 6. 画面を見る

画面は `/var/lib/hostdiary/public/index.html` です。サーバに通信せずに開ける作りなので、次のどちらかで見ます。

- **手元にコピーして見る**(設定の変更なし): `rsync -a <サーバ>:/var/lib/hostdiary/public/ ./hostdiary-view/` → `index.html` をブラウザで開く
- **nginx の認証の内側で見せる**: `nginx-hostdiary.conf.example` を既存の HTTPS の server に足し、パスワードファイルを作ってから `sudo nginx -t` → `sudo systemctl reload nginx`。
  すでに Basic 認証のかかった server に足すなら、`auth_basic` の2行を外してその認証を使ってもよい

外から見せたら、必ず次を確かめてください。

- 認証なしで `/hostdiary/`・`/hostdiary/index.json`・`/hostdiary/index.js`・`/hostdiary/summary.json`・`/hostdiary/llms.txt` が 401 になる(`/hostdiary` は 308 で `/hostdiary/` に送られる)
- 認証ありで画面が出る
- ほかのポート・IP 直打ち・別の Host 名の server から同じファイルが見えない(404 など)
- `public/` だけを出し、親の `/var/lib/hostdiary/`(state.json など)は出さない

画面の言語はブラウザの設定で、日本語ならば日本語、それ以外は英語になります。

AI に状態を確かめさせるときは、認証つきの `/hostdiary/` の URL を渡し、`llms.txt`(AI 向けの読み方)から読むように頼みます。
日ごとの要約は `summary.json` にあります(数字だけで、正常・異常の判定は含みません)。

## 7. 更新する

新しい配布物を展開し、手順 2 で確かめてから、もう一度 `sudo ./hostdiary install` を実行します。
データと設定はそのまま残ります。画面のファイルは次の収集(1分以内)で新しい版に入れ替わります。

## 8. 外す

```sh
sudo hostdiary uninstall
```

タイマーを止め、実行ファイルと unit を消します。**データ(`/var/lib/hostdiary/`)と設定(`/etc/hostdiary/`)は残ります。**
要らなければ、表示された案内に従って手で消してください。nginx に設定を足したときは、それも外して `nginx -t` → reload します。
