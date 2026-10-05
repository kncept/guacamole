#! /usr/bin/env bash
set -euo pipefail

# Read the docs to see how the project is structured and how to run it

# Style
# All var (and local var) references are fully wrapped in bracers


# Build an array of OS/ARCH binaries into .build/
# Usage: ./run.sh build
build() {
  echo "Building binaries for multiple OS/ARCH combinations into .build/..."

  # Define the OS/ARCH combinations to build
  local -a combos=(
    "linux/amd64"
    "linux/arm64"
    "darwin/amd64"
    "darwin/arm64"
    "windows/amd64"
  )

  # Create the .build directory (gitignored)
  mkdir -p .build

  for combo in "${combos[@]}"; do
    IFS='/' read -r goos goarch <<< "$combo"

    # Determine the binary name based on OS
    local binary_name
    if [ "$goos" = "windows" ]; then
      binary_name="guacamole_${goos}_${goarch}.exe"
    else
      binary_name="guacamole_${goos}_${goarch}"
    fi

    local output_path=".build/${binary_name}"

    echo "Building ${binary_name} for ${goos}/${goarch}..."

    CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" go build \
      -o "$output_path" .

    # Set execute permission on the binary
    chmod +x "$output_path"

    echo "  -> Output: ${output_path}"
  done

  echo "Build complete. Binaries in .build/:"
  ls -la .build/
}

# Run go tests
test() {
  echo "Running Go tests..."
  go test ./...
}

help() {
  cat <<EOF
Usage: ./run.sh <command>

Commands:
  help             Show this help message
  build            Build binaries for multiple OS/ARCH combinations into .build/
  test             Run Go tests
  clean            Remove .build directory and other generated files
EOF
}

# Clean up generated files
clean() {
  echo "Cleaning .build directory..."
  rm -rf .build
  echo "Clean complete."
}

# Run commands in a subshell () just in case a CD command fails
case "${1:-}" in
  build)
    (build)
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