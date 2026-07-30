#!/bin/sh
set -eu

# Materialize the pinned GNU/Linux AMD64 sysroot from Zig's libc cache. This is
# a maintainer-only helper: normal TinyGo builds consume lib/sysroots directly
# and never invoke Zig.

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
zig_command=${TINYGO_ZIG:-zig}
if ! zig=$(command -v "$zig_command"); then
	printf 'Zig not found in PATH: %s; install zig or set TINYGO_ZIG\n' "$zig_command" >&2
	exit 1
fi
out="$root/lib/sysroots/linux-amd64-gnu"
target=x86_64-linux-gnu.2.25
cache=$(mktemp -d "${TMPDIR:-/tmp}/tinygo-glibc-cache.XXXXXX")

cleanup() {
	rm -rf "$cache"
}
trap cleanup EXIT HUP INT TERM

case "$zig" in
	/*) ;;
	*) zig=$(CDPATH= cd -- "$(dirname -- "$zig")" && pwd)/$(basename -- "$zig") ;;
esac

probe="$cache/probe.c"
printf 'int main(void) { return 0; }\n' >"$probe"
ZIG_GLOBAL_CACHE_DIR="$cache" ZIG_LOCAL_CACHE_DIR="$cache" \
	"$zig" cc -target "$target" "$probe" -o "$cache/probe"

rm -rf "$out"
mkdir -p "$out/usr/lib" "$out/usr/include"

asset() {
	path=$(find "$cache" -type f -name "$1" -print | sort | tail -n 1)
	if [ -z "$path" ]; then
		printf 'Zig did not materialize %s\n' "$1" >&2
		exit 1
	fi
	cp "$path" "$out/usr/lib/$1"
}

for name in \
	crt1.o libubsan_rt.a libc_nonshared.a libcompiler_rt.a \
	libc.so.6 libm.so.6 libld.so.2 libresolv.so.2 libpthread.so.0 \
	libdl.so.2 librt.so.1 libutil.so.1
do
	asset "$name"
done

zigdir=$(dirname "$zig")
headers="$cache/headers.d"
header_flags="-nostdlibinc -isystem $zigdir/lib/include -isystem $zigdir/lib/libc/include/x86-linux-gnu -isystem $zigdir/lib/libc/include/generic-glibc -isystem $zigdir/lib/libc/include/x86-linux-any -isystem $zigdir/lib/libc/include/any-linux-any"

# Keep only the transitive header closure required by C code that is compiled
# for this target. Copying Zig's full multi-architecture header tree would add
# tens of megabytes of unrelated headers to the checked-in sysroot.
for source in \
	src/internal/futex/futex_linux.c \
	src/internal/task/task_threads.c \
	src/runtime/runtime_unix.c \
	src/runtime/signal.c
do
	# 构建开启优化后会激活 bits/stdio.h 等 glibc 条件包含，依赖扫描也必须
	# 开启优化，才能将优化 C 编译所需的完整头文件闭包复制到 sysroot。
	# shellcheck disable=SC2086
	"$zig" cc -target "$target" $header_flags -O2 -D_GNU_SOURCE -D_XOPEN_SOURCE \
		-M "$root/$source" >>"$headers"
done

copy_header() {
	header=$1
	case "$header" in
		/*) ;;
		*) header="$root/$header" ;;
	esac
	case "$header" in
		"$zigdir/lib/include/"*)
			destination="$out/usr/include/zig/${header#"$zigdir/lib/include/"}"
			;;
		"$zigdir/lib/libc/include/x86-linux-gnu/"*)
			destination="$out/usr/include/x86-linux-gnu/${header#"$zigdir/lib/libc/include/x86-linux-gnu/"}"
			;;
		"$zigdir/lib/libc/include/generic-glibc/"*)
			destination="$out/usr/include/generic-glibc/${header#"$zigdir/lib/libc/include/generic-glibc/"}"
			;;
		"$zigdir/lib/libc/include/x86-linux-any/"*)
			destination="$out/usr/include/x86-linux-any/${header#"$zigdir/lib/libc/include/x86-linux-any/"}"
			;;
		"$zigdir/lib/libc/include/any-linux-any/"*)
			destination="$out/usr/include/any-linux-any/${header#"$zigdir/lib/libc/include/any-linux-any/"}"
			;;
		*)
			return
			;;
	esac
	mkdir -p "$(dirname "$destination")"
	cp "$header" "$destination"
}

sed 's/^[^:]*: //' "$headers" | tr ' \\' '\n' | sort -u | while IFS= read -r header
do
	[ -n "$header" ] && copy_header "$header"
done

printf 'Generated %s for %s\n' "$out" "$target"
