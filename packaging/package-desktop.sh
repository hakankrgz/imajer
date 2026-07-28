#!/bin/sh
set -eu

PROJECT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
GO_BIN=${GO:-go}
VERSION=${VERSION:-0.6.2}
DIST_DIR="$PROJECT_DIR/dist"
BUILD_DIR="$DIST_DIR/package-build"
PACKAGE_DIR="$DIST_DIR/packages"
AGENT_DIR="$BUILD_DIR/common/agents"
KEY_DIR=${IMAJER_RELEASE_KEY_DIR:-"$PROJECT_DIR/.release-keys"}
SIGN_TOOL="$BUILD_DIR/sign-tool"
BASE_LDFLAGS="-s -w -X main.version=$VERSION"
DESKTOP_LDFLAGS="$BASE_LDFLAGS -X main.desktopMode=true -X main.desktopWindowMode=true"

if [ "$(uname -s)" != Darwin ]; then
  echo "Tam masaüstü paketleme macOS üzerinde çalıştırılmalıdır." >&2
  exit 2
fi

case "$VERSION" in
  *[!0-9A-Za-z.-]*|"")
    echo "Geçersiz VERSION: $VERSION" >&2
    exit 2
    ;;
esac

rm -rf "$BUILD_DIR" "$PACKAGE_DIR" "$DIST_DIR/IMAJER.app"
mkdir -p "$BUILD_DIR/common" "$AGENT_DIR" "$PACKAGE_DIR" "$KEY_DIR"
chmod 700 "$KEY_DIR"

build_controller() {
  target_os=$1
  target_arch=$2
  output=$3
  mode=$4
  ldflags=$BASE_LDFLAGS
  if [ "$mode" = desktop ]; then
    ldflags=$DESKTOP_LDFLAGS
    if [ "$target_os" = windows ]; then
      ldflags="$ldflags -H=windowsgui"
    fi
  fi
  echo "Controller: $target_os/$target_arch ($mode)"
  CGO_ENABLED=0 GOOS="$target_os" GOARCH="$target_arch" "$GO_BIN" build \
    -buildvcs=false -trimpath -ldflags "$ldflags" -o "$output" "$PROJECT_DIR/cmd/imajer"
}

build_agent() {
  target_os=$1
  target_arch=$2
  output=$3
  echo "Agent: $target_os/$target_arch"
  CGO_ENABLED=0 GOOS="$target_os" GOARCH="$target_arch" "$GO_BIN" build \
    -buildvcs=false -trimpath -ldflags "$BASE_LDFLAGS" -o "$output" "$PROJECT_DIR/cmd/imajer-agent"
}

build_macos_shell() {
  target_arch=$1
  output=$2
  swift_arch=$target_arch
  if [ "$target_arch" = amd64 ]; then
    swift_arch=x86_64
  fi
  sdk_path=${IMAJER_MACOS_SDK:-$(xcrun --sdk macosx --show-sdk-path)}
  compatible_clt_sdk=/Library/Developer/CommandLineTools/SDKs/MacOSX15.4.sdk
  if [ -z "${IMAJER_MACOS_SDK:-}" ] && [ -d "$compatible_clt_sdk" ]; then
    sdk_path=$compatible_clt_sdk
  fi
  module_cache="$BUILD_DIR/swift-module-cache"
  mkdir -p "$module_cache"
  echo "Native macOS window: $target_arch"
  CLANG_MODULE_CACHE_PATH="$module_cache" SWIFT_MODULECACHE_PATH="$module_cache" \
    xcrun swiftc -O -swift-version 5 \
    -target "$swift_arch-apple-macos11.0" \
    -sdk "$sdk_path" \
    -framework Cocoa -framework WebKit \
    -o "$output" "$PROJECT_DIR/packaging/macos/IMAJERApp.swift"
}

echo "IMAJER Desktop $VERSION paketleri hazırlanıyor..."
CGO_ENABLED=0 "$GO_BIN" build -buildvcs=false -trimpath -ldflags "$BASE_LDFLAGS" \
  -o "$SIGN_TOOL" "$PROJECT_DIR/cmd/imajer"

build_agent darwin amd64 "$AGENT_DIR/imajer-agent-darwin-amd64"
build_agent darwin arm64 "$AGENT_DIR/imajer-agent-darwin-arm64"
build_agent linux amd64 "$AGENT_DIR/imajer-agent-linux-amd64"
build_agent linux arm64 "$AGENT_DIR/imajer-agent-linux-arm64"
build_agent windows amd64 "$AGENT_DIR/imajer-agent-windows-amd64.exe"
build_agent windows arm64 "$AGENT_DIR/imajer-agent-windows-arm64.exe"

if [ ! -f "$KEY_DIR/tool-release-private.pem" ]; then
  echo "Yerel tool-release Ed25519 anahtarı oluşturuluyor: $KEY_DIR"
  openssl genpkey -algorithm ED25519 -out "$KEY_DIR/tool-release-private.pem"
  chmod 600 "$KEY_DIR/tool-release-private.pem"
  openssl pkey -in "$KEY_DIR/tool-release-private.pem" -pubout \
    -out "$KEY_DIR/tool-release-public.pem"
fi
if [ ! -f "$KEY_DIR/tool-release-public.pem" ]; then
  openssl pkey -in "$KEY_DIR/tool-release-private.pem" -pubout \
    -out "$KEY_DIR/tool-release-public.pem"
fi
cp "$KEY_DIR/tool-release-public.pem" "$AGENT_DIR/tool-release-public.pem"

cat >"$AGENT_DIR/tools.yaml" <<EOF
- name: imajer-agent
  version: "$VERSION"
  os: darwin
  arch: amd64
  path: ./imajer-agent-darwin-amd64
  license: Apache-2.0
- name: imajer-agent
  version: "$VERSION"
  os: darwin
  arch: arm64
  path: ./imajer-agent-darwin-arm64
  license: Apache-2.0
- name: imajer-agent
  version: "$VERSION"
  os: linux
  arch: amd64
  path: ./imajer-agent-linux-amd64
  license: Apache-2.0
- name: imajer-agent
  version: "$VERSION"
  os: linux
  arch: arm64
  path: ./imajer-agent-linux-arm64
  license: Apache-2.0
- name: imajer-agent
  version: "$VERSION"
  os: windows
  arch: amd64
  path: ./imajer-agent-windows-amd64.exe
  license: Apache-2.0
- name: imajer-agent
  version: "$VERSION"
  os: windows
  arch: arm64
  path: ./imajer-agent-windows-arm64.exe
  license: Apache-2.0
EOF
(
  cd "$AGENT_DIR"
  "$SIGN_TOOL" tools sign --spec tools.yaml \
    --key "$KEY_DIR/tool-release-private.pem" --out tool-manifest.json
  chmod 644 tool-manifest.json
  "$SIGN_TOOL" tools verify --manifest tool-manifest.json --key tool-release-public.pem
  rm tools.yaml
)

"$GO_BIN" run "$PROJECT_DIR/packaging/icon" "$BUILD_DIR/IMAJER.icns"

make_macos_package() {
  arch=$1
  label=$2
  stage="$BUILD_DIR/macos-$arch"
  app="$stage/IMAJER.app"
  mkdir -p "$app/Contents/MacOS" "$app/Contents/Resources/agents" \
    "$app/Contents/Resources/bin" "$app/Contents/Resources/docs"
  build_macos_shell "$arch" "$app/Contents/MacOS/IMAJER"
  build_controller darwin "$arch" "$app/Contents/MacOS/imajer-core" cli
  build_controller darwin "$arch" "$app/Contents/Resources/bin/imajer-cli" cli
  cp "$PROJECT_DIR/packaging/macos/Info.plist" "$app/Contents/Info.plist"
  sed -i '' "s/__VERSION__/$VERSION/g" "$app/Contents/Info.plist"
  cp "$BUILD_DIR/IMAJER.icns" "$app/Contents/Resources/IMAJER.icns"
  cp "$AGENT_DIR"/* "$app/Contents/Resources/agents/"
  cp "$PROJECT_DIR/MASAUSTU_KULLANIM.md" "$app/Contents/Resources/docs/"
  cp "$PROJECT_DIR/LICENSE" "$app/Contents/Resources/"
  chmod 755 "$app/Contents/MacOS/IMAJER" "$app/Contents/MacOS/imajer-core" \
    "$app/Contents/Resources/bin/imajer-cli"
  codesign --force --deep --sign - "$app"
  codesign --verify --deep --strict "$app"
  ditto -c -k --sequesterRsrc --keepParent "$app" \
    "$PACKAGE_DIR/IMAJER-macOS-$label-$VERSION.zip"
}

make_windows_package() {
  arch=$1
  label=$2
  stage="$BUILD_DIR/IMAJER-Windows-$label"
  mkdir -p "$stage/agents" "$stage/docs"
  build_controller windows "$arch" "$stage/IMAJER.exe" desktop
  build_controller windows "$arch" "$stage/imajer-cli.exe" cli
  cp "$AGENT_DIR"/* "$stage/agents/"
  cp "$PROJECT_DIR/MASAUSTU_KULLANIM.md" "$stage/docs/"
  cp "$PROJECT_DIR/LICENSE" "$stage/"
  cat >"$stage/BASLAT.txt" <<EOF
IMAJER $VERSION

1. Bu ZIP dosyasını tamamen çıkarın.
2. IMAJER.exe dosyasına çift tıklayın.
3. IMAJER sekmesiz ve adres çubuksuz kendi penceresinde açılır.

Ayrıntılar için docs/MASAUSTU_KULLANIM.md dosyasını okuyun.
EOF
  (
    cd "$BUILD_DIR"
    COPYFILE_DISABLE=1 zip -X -q -r \
      "$PACKAGE_DIR/IMAJER-Windows-$label-$VERSION.zip" "$(basename "$stage")"
  )
}

make_macos_package arm64 Apple-Silicon
make_macos_package amd64 Intel
make_windows_package amd64 x64
make_windows_package arm64 ARM64

(
  cd "$PACKAGE_DIR"
  shasum -a 256 ./*.zip >SHA256SUMS
)

cp -R "$BUILD_DIR/macos-arm64/IMAJER.app" "$DIST_DIR/IMAJER.app"
echo
echo "Hazır paketler: $PACKAGE_DIR"
echo "Bu Mac için uygulama: $DIST_DIR/IMAJER.app"
