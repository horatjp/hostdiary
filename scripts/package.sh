#!/bin/bash
# package.sh — 配布物(tar.gz)を作る
#
# 何を: linux/amd64 と linux/arm64 を別々にビルドし、CPU の種類ごとに
#       dist/hostdiary-<版>-linux-<arch>.tar.gz を作る。中身は実行ファイル・日本語の手順書・
#       設定例・nginx 例・README・LICENSE・SHA256SUMS。dist/SHA256SUMS に配布物の SHA-256 を書く。
# なぜ: ほかのサーバに入れるとき、展開して手順書どおりに入れれば済むようにする
#       (change release-package)。
# 使い方: リポジトリのどこからでも `scripts/package.sh`。Go 1.26 が要る。Bash 3.2 で動く。
set -euo pipefail

repo=$(cd "$(dirname "$0")/.." && pwd)
cd "$repo"

# 版は collect.Version(--version と画面の VERSION に出るもの)にそろえる
version=$(sed -n 's/^const Version = "\(.*\)"$/\1/p' internal/collect/collect.go)
[ -n "$version" ] || { echo "package: Version を読めません" >&2; exit 1; }

# sha256sum(Linux)が無ければ shasum -a 256(macOS)。出力の形は同じ
if command -v sha256sum >/dev/null 2>&1; then sha() { sha256sum "$@"; }
else sha() { shasum -a 256 "$@"; }
fi

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
mkdir -p dist
rm -f dist/hostdiary-"$version"-linux-*.tar.gz dist/SHA256SUMS

for arch in amd64 arm64; do
  name="hostdiary-$version-linux-$arch"
  dir="$work/$name"
  mkdir -p "$dir"
  CGO_ENABLED=0 GOOS=linux GOARCH="$arch" go build -trimpath -o "$dir/hostdiary" ./cmd/hostdiary
  chmod 0755 "$dir/hostdiary"
  cp packaging/INSTALL.ja.md packaging/config.example.json packaging/nginx-hostdiary.conf.example \
    README.md LICENSE "$dir/"
  chmod 0644 "$dir"/INSTALL.ja.md "$dir"/config.example.json "$dir"/nginx-hostdiary.conf.example \
    "$dir"/README.md "$dir"/LICENSE
  (cd "$dir" && sha hostdiary INSTALL.ja.md config.example.json nginx-hostdiary.conf.example \
    README.md LICENSE > SHA256SUMS && chmod 0644 SHA256SUMS)
  # 所有者は 0:0 にそろえ、作った人のユーザー名を配布物に残さない。
  # COPYFILE_DISABLE は macOS の tar が ._* の拡張属性ファイルを入れないようにする
  if tar --version 2>/dev/null | grep -q 'GNU tar'; then
    tar -C "$work" --owner=0 --group=0 --numeric-owner -czf "dist/$name.tar.gz" "$name"
  else
    COPYFILE_DISABLE=1 tar -C "$work" --uid 0 --gid 0 --uname root --gname root \
      --no-xattrs -czf "dist/$name.tar.gz" "$name"
  fi
done

(cd dist && sha hostdiary-"$version"-linux-*.tar.gz > SHA256SUMS)
echo "package: $(git rev-parse --short HEAD 2>/dev/null || echo unknown) の版 $version"
cat dist/SHA256SUMS
