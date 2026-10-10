#! /usr/bin/env bash
set -euo pipefail

# Read the docs to see how the project is structured and how to run it

# Style
# All var (and local var) references are fully wrapped in bracers

cd "$(dirname "$0")"

# Supported OS/ARCH combos for the CLI (pure Go, built with CGO disabled)
CLI_TARGETS=(
  "linux/amd64"
  "linux/arm64"
  "darwin/amd64"
  "darwin/arm64"
  "windows/amd64"
)

# Build the CLI for every supported OS/ARCH into .build/cli/
build_cli() {
  echo "Building CLI for supported OS/ARCH..."
  local out_dir=".build/cli"
  mkdir -p "${out_dir}"

  for target in "${CLI_TARGETS[@]}"; do
    local go_os="${target%%/*}"
    local go_arch="${target##*/}"
    local suffix=""
    if [ "${go_os}" = "windows" ]; then
      suffix=".exe"
    fi
    local out_name="guacamole_${go_os}_${go_arch}${suffix}"
    echo "Building ${out_name}..."
    CGO_ENABLED=0 GOOS="${go_os}" GOARCH="${go_arch}" \
      go build -o "${out_dir}/${out_name}" ./cmd/cli
  done

  echo "CLI builds complete in ${out_dir}/"
}

# Build the macOS GUI into a .app bundle at .build/macos/Guacamole.app.
# The GUI uses cgo (fyne/glfw + AppKit) so it can only be built for the host,
# on a macOS machine. The bundle icon (an avocado-green letter G) is drawn by
# cmd/icongen and baked into an .icns with iconutil.
build_macos() {
  if [ "$(uname -s)" != "Darwin" ]; then
    echo "Skipping macOS GUI build (requires a macOS host)"
    return 0
  fi

  local host_arch
  case "$(uname -m)" in
    arm64) host_arch="arm64" ;;
    x86_64) host_arch="amd64" ;;
    *)
      echo "Error: unsupported host arch: $(uname -m)" >&2
      return 1
      ;;
  esac

  local app_name="Guacamole"
  local out_dir=".build/macos"
  local app_dir="${out_dir}/${app_name}.app"
  local iconset_dir="${out_dir}/${app_name}.iconset"

  echo "Building ${app_name}.app for ${host_arch}..."
  rm -rf "${app_dir}" "${iconset_dir}"
  mkdir -p "${app_dir}/Contents/MacOS" "${app_dir}/Contents/Resources"

  CGO_ENABLED=1 go build -o "${app_dir}/Contents/MacOS/${app_name}" ./cmd/macos

  echo "Generating app icon..."
  go run ./cmd/icongen "${iconset_dir}"
  iconutil -c icns "${iconset_dir}" -o "${app_dir}/Contents/Resources/${app_name}.icns"
  rm -rf "${iconset_dir}"

  cat > "${app_dir}/Contents/Info.plist" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>CFBundleDevelopmentRegion</key>
	<string>en</string>
	<key>CFBundleDisplayName</key>
	<string>${app_name}</string>
	<key>CFBundleExecutable</key>
	<string>${app_name}</string>
	<key>CFBundleIconFile</key>
	<string>${app_name}</string>
	<key>CFBundleIdentifier</key>
	<string>com.kncept.guacamole</string>
	<key>CFBundleInfoDictionaryVersion</key>
	<string>6.0</string>
	<key>CFBundleName</key>
	<string>${app_name}</string>
	<key>CFBundlePackageType</key>
	<string>APPL</string>
	<key>CFBundleShortVersionString</key>
	<string>0.1.0</string>
	<key>CFBundleVersion</key>
	<string>1</string>
	<key>LSMinimumSystemVersion</key>
	<string>11.0</string>
	<key>NSHighResolutionCapable</key>
	<true/>
	<key>NSPrincipalClass</key>
	<string>NSApplication</string>
</dict>
</plist>
EOF

  echo "macOS GUI build complete: ${app_dir} (open it with 'open ${app_dir}')"
}

# Map the current host to a Go GOOS.
current_goos() {
  case "$(uname -s)" in
    Darwin) echo "darwin" ;;
    Linux) echo "linux" ;;
    *MINGW*|*MSYS*|*CYGWIN*) echo "windows" ;;
    *) echo "unknown" ;;
  esac
}

# Map the current host to a Go GOARCH.
current_goarch() {
  case "$(uname -m)" in
    arm64|aarch64) echo "arm64" ;;
    x86_64|amd64) echo "amd64" ;;
    *) echo "unknown" ;;
  esac
}

# Build and run the app for the current host platform.
# On macOS the GUI bundle is built and opened; on other platforms the host
# CLI is built and run.
run() {
  local goos goarch
  goos="$(current_goos)"
  goarch="$(current_goarch)"

  if [ "${goos}" = "darwin" ]; then
    build_macos
    echo "Opening .build/macos/Guacamole.app..."
    open ".build/macos/Guacamole.app"
    return 0
  fi

  if [ "${goos}" = "unknown" ] || [ "${goarch}" = "unknown" ]; then
    echo "Error: cannot detect host platform for run (got ${goos}/${goarch})" >&2
    return 1
  fi

  local out=".build/cli/guacamole_${goos}_${goarch}"
  echo "Building host CLI for ${goos}/${goarch}..."
  CGO_ENABLED=0 GOOS="${goos}" GOARCH="${goarch}" \
    go build -o "${out}" ./cmd/cli

  echo "Running ${out}..."
  "${out}"
}

# Build every target
build() {
  build_cli
  build_macos
}

test() {
  echo "Running tests..."
  go test ./...
}

clean() {
  echo "Cleaning generated files..."
  rm -rf .build
  find . -name ".DS_Store" -type f -exec rm -f {} + 2>/dev/null || true
  echo "Clean complete."
}

help() {
  cat <<EOF
Usage: ./run.sh <command>

Commands:
  help         Show this help message
  build        Build every target (CLI for all supported OS/ARCH, macOS GUI for the host)
  build:cli    Build the CLI for every supported OS/ARCH into .build/cli/
  build:macos  Build the macOS GUI into .build/macos/Guacamole.app (requires macOS)
  run          Build and run the app for the host platform (opens the macOS GUI, or runs the host CLI)
  test         Run go tests (go test ./...)
  clean        Remove generated files (.build/) and .DS_Store files
EOF
}

# Run commands in a subshell () just incase a cd command fails
case "${1:-}" in
  build)
    (build)
    ;;
  build:cli)
    (build_cli)
    ;;
  build:macos)
    (build_macos)
    ;;
  run)
    (run)
    ;;
  test)
    (test)
    ;;
  clean)
    (clean)
    ;;
  help|-h|--help)
    (help)
    ;;
  *)
    echo "unknown command: ${1:-}" >&2
    echo
    (help)
    ;;
esac