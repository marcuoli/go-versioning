#!/usr/bin/env bash

# A subshell keeps the caller's environment and working directory unchanged.
main() (
  set -euo pipefail
  build_only=false
  race=false
  go_version=1.27.4
  for argument in "$@"; do
    case "$argument" in
      --build-only) build_only=true ;;
      --race) race=true ;;
      --go-version=1.27.3) go_version=1.27.3 ;;
      --go-version=1.27.4) go_version=1.27.4 ;;
      -h|--help)
        printf '%s\n' 'Usage: bash scripts/validate.sh [--build-only | --race] [--go-version=1.27.3|1.27.4]'
        exit 0
        ;;
      *) printf 'Unknown argument: %s\n' "$argument" >&2; exit 2 ;;
    esac
  done
  if [[ "$build_only" == true && "$race" == true ]]; then
    printf '%s\n' '--build-only and --race cannot be combined.' >&2
    exit 2
  fi

  cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.."
  export GOTOOLCHAIN=local
  export GOWORK=off
  export GOFLAGS=-mod=readonly
  export CGO_ENABLED=0

  active_go="$(go env GOVERSION)"
  if [[ "$active_go" != "go$go_version" ]]; then
    printf 'Go %s is required; found %s. Select that compiler explicitly.\n' "$go_version" "$active_go" >&2
    exit 1
  fi

  if [[ "$build_only" != true ]]; then
    printf '%s\n' 'Running tests.'
    go test -count=1 ./...
    printf '%s\n' 'Running go vet.'
    go vet ./...
    printf '%s\n' 'Verifying module checksums.'
    go mod verify
    if [[ "$race" == true ]]; then
      printf '%s\n' 'Running race tests (requires an existing supported C compiler in CC or PATH).'
      CGO_ENABLED=1 go test -race -count=1 ./...
    fi
  fi

  output_directory="${GO_VERSIONING_BUILD_DIR:-}"
  if [[ -z "$output_directory" ]]; then
    if [[ -d /mnt/v/Development ]]; then
      output_directory=/mnt/v/Development/go-versioning/artifacts/build/local
    elif [[ -d V:/Development ]]; then
      output_directory=V:/Development/go-versioning/artifacts/build/local
    elif [[ -n "${RUNNER_TEMP:-}" ]]; then
      output_directory="$RUNNER_TEMP/go-versioning-build"
    else
      printf '%s\n' 'Set GO_VERSIONING_BUILD_DIR to an external build directory.' >&2
      exit 1
    fi
  fi
  output="$output_directory/go-versioning"
  if [[ "$(go env GOOS)" == windows ]]; then
    output+=.exe
  fi
  mkdir -p -- "$output_directory"
  printf 'Building local CLI snapshot: %s\n' "$output"
  go build -trimpath -buildvcs=false -o "$output" ./cmd/go-versioning
)

main "$@"
