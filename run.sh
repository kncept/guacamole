#! /usr/bin/env bash
set -euo pipefail

# Read the docs to see how the project is structured and how to run it

# Style
# All var (and local var) references are fully wrapped in bracers

cd "$(dirname "$0")"

# Build the CLI for the current host into .build/cli/.
# The CLI is pure Go (built with CGO disabled), but it is only built for the
# host platform; the binary keeps the guacamole_<os>_<arch> naming convention.
build_cli() {
  local goos goarch
  goos="$(current_goos)"
  goarch="$(current_goarch)"

  if [ "${goos}" = "unknown" ] || [ "${goarch}" = "unknown" ]; then
    echo "Error: cannot detect host platform for build:cli (got ${goos}/${goarch})" >&2
    return 1
  fi

  local suffix=""
  if [ "${goos}" = "windows" ]; then
    suffix=".exe"
  fi

  local out_dir=".build/cli"
  local out_name="guacamole_${goos}_${goarch}${suffix}"
  echo "Building ${out_name}..."
  mkdir -p "${out_dir}"
  CGO_ENABLED=0 GOOS="${goos}" GOARCH="${goarch}" \
    go build -o "${out_dir}/${out_name}" ./cmd/cli

  echo "CLI build complete in ${out_dir}/"
}

# Build the desktop GUI for the current host into .build/gui/.
# Fyne binds to the native window system through cgo, so the GUI is host-only
# and cannot be cross-compiled: the matching native toolchain must be installed
# first (see the 'dependencies' command). On macOS the .app bundle built by
# build:macos already contains the GUI, so this is skipped there.
build_gui() {
  local goos goarch
  goos="$(current_goos)"
  goarch="$(current_goarch)"

  if [ "${goos}" = "darwin" ]; then
    echo "Skipping host GUI build (macOS uses the .app from build:macos)"
    return 0
  fi

  if [ "${goos}" = "unknown" ] || [ "${goarch}" = "unknown" ]; then
    echo "Error: cannot detect host platform for build:gui (got ${goos}/${goarch})" >&2
    return 1
  fi

  local suffix=""
  if [ "${goos}" = "windows" ]; then
    suffix=".exe"
  fi

  local out_dir=".build/gui"
  local out="${out_dir}/guacamole_gui_${goos}_${goarch}${suffix}"
  echo "Building host GUI for ${goos}/${goarch}..."
  mkdir -p "${out_dir}"
  CGO_ENABLED=1 go build -o "${out}" ./cmd/gui
  echo "GUI build complete: ${out}"
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

# Map the current host to a GOARCH.
current_goarch() {
  case "$(uname -m)" in
    arm64|aarch64) echo "arm64" ;;
    x86_64|amd64) echo "amd64" ;;
    *) echo "unknown" ;;
  esac
}

# Install the native libraries Fyne needs to build on macOS.
# Fyne links against AppKit through cgo, which needs the Xcode command line
# tools (they also provide iconutil, used for the .app icon).
dependencies_macos() {
  if xcode-select -p >/dev/null 2>&1; then
    echo "Xcode command line tools already installed: $(xcode-select -p)"
    return 0
  fi

  echo "Installing Xcode command line tools (required to build the Fyne GUI)..."
  xcode-select --install || true
  echo "Re-run './run.sh dependencies' once the installation finishes."
}

# True when the native libraries Fyne needs to build on Linux are already
# present: a C compiler plus pkg-config and the OpenGL/X11/xkbcommon/Wayland
# development headers. Lets dependencies_linux skip the (slow) package-manager
# update/install when nothing is missing.
linux_deps_present() {
  command -v gcc >/dev/null 2>&1 || return 1
  command -v pkg-config >/dev/null 2>&1 || return 1
  pkg-config --exists gl x11 xkbcommon wayland-client 2>/dev/null
}

# Install the native libraries Fyne needs to build on Linux: a C compiler plus
# the OpenGL, X11, xkbcommon and Wayland development headers (GLFW needs the
# Wayland headers even for its X11 backend). No-op when they are all installed.
dependencies_linux() {
  if linux_deps_present; then
    echo "Fyne build dependencies already installed"
    return 0
  fi

  if command -v apt-get >/dev/null 2>&1; then
    echo "Installing Fyne build dependencies with apt-get..."
    sudo apt-get update
    sudo apt-get install -y gcc pkg-config libgl1-mesa-dev xorg-dev libxkbcommon-dev libwayland-dev
  elif command -v dnf >/dev/null 2>&1; then
    echo "Installing Fyne build dependencies with dnf..."
    sudo dnf install -y gcc pkgconf-pkg-config libXcursor-devel libXrandr-devel mesa-libGL-devel \
      libXi-devel libXinerama-devel libXxf86vm-devel libxkbcommon-devel wayland-devel
  elif command -v pacman >/dev/null 2>&1; then
    echo "Installing Fyne build dependencies with pacman..."
    sudo pacman -S --noconfirm --needed gcc pkgconf libx11 libxcursor libxrandr \
      libxinerama libxi libxxf86vm mesa libxkbcommon wayland
  else
    echo "Error: no supported package manager found (apt-get, dnf, or pacman)." >&2
    echo "Install a C compiler plus the OpenGL/X11/xkbcommon/Wayland development headers." >&2
    return 1
  fi
}

# Fyne on Windows builds through cgo with a MinGW-w64 gcc. Git for Windows
# ships one on the PATH of its bash; MSYS2's mingw64 is used as a fallback and,
# under GitHub Actions, added to PATH for the steps that follow.
dependencies_windows() {
  if command -v gcc >/dev/null 2>&1; then
    echo "Found $(gcc --version | head -n 1)"
    return 0
  fi

  local mingw_bin="C:/msys64/mingw64/bin"
  if [ -d "${mingw_bin}" ]; then
    export PATH="${mingw_bin}:${PATH}"
    if [ -n "${GITHUB_PATH:-}" ]; then
      echo "${mingw_bin}" >> "${GITHUB_PATH}"
    fi
    echo "Using MinGW-w64 from ${mingw_bin}"
    return 0
  fi

  echo "Error: no C compiler found. Install MinGW-w64 (e.g. via MSYS2) to build the GUI." >&2
  return 1
}

# Install the native build dependencies for the current platform by
# dispatching to the matching dependencies:<os> command.
dependencies() {
  local goos
  goos="$(current_goos)"
  case "${goos}" in
    darwin) (dependencies_macos) ;;
    linux) (dependencies_linux) ;;
    windows) (dependencies_windows) ;;
    *)
      echo "Error: unsupported host for dependencies: ${goos}" >&2
      return 1
      ;;
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

# Build every target. Ensures the native build dependencies are present first
# (a no-op when they already are), so a plain './run.sh build' just works.
build() {
  dependencies
  build_cli
  build_gui
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
  help                 Show this help message
  dependencies         Install the native build dependencies for the host (autoselects the OS)
  dependencies:macos   Install the native build dependencies for macOS (Xcode command line tools)
  dependencies:linux   Install the native build dependencies for Linux (gcc + OpenGL/X11/xkbcommon/Wayland headers)
  dependencies:windows Install the native build dependencies for Windows (MinGW-w64 gcc)
  build                Build every target (installs missing native deps, then host CLI, host GUI, macOS .app)
  build:cli            Build the CLI for the current host into .build/cli/
  build:gui            Build the desktop GUI for the host into .build/gui/
  build:macos          Build the macOS GUI into .build/macos/Guacamole.app (requires macOS)
  run                  Build and run the app for the host platform (opens the macOS GUI, or runs the host CLI)
  test                 Run go tests (go test ./...)
  clean                Remove generated files (.build/) and .DS_Store files
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
  build:gui)
    (build_gui)
    ;;
  build:macos)
    (build_macos)
    ;;
  dependencies)
    (dependencies)
    ;;
  dependencies:macos)
    (dependencies_macos)
    ;;
  dependencies:linux)
    (dependencies_linux)
    ;;
  dependencies:windows)
    (dependencies_windows)
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