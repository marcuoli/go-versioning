#!/usr/bin/env bash

# A subshell keeps the caller's environment and working directory unchanged.
main() (
  set -euo pipefail
  build_only=false
  race=false
  for argument in "$@"; do
    case "$argument" in
      --build-only) build_only=true ;;
      --race) race=true ;;
      -h|--help)
        printf '%s\n' 'Usage: bash scripts/validate.sh [--build-only | --race]'
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
  export GOWORK=off
  export GOFLAGS=-mod=readonly
  export CGO_ENABLED=0

  active_go="$(go env GOVERSION)"
  if [[ "$active_go" != go1.27.3 ]]; then
    printf 'Go 1.27.3 is required; found %s.\n' "$active_go" >&2
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

  output=bin/go-versioning
  if [[ "$(go env GOOS)" == windows ]]; then
    output+=.exe
  fi
  mkdir -p bin
  printf 'Building local CLI snapshot: %s\n' "$output"
  go build -trimpath -buildvcs=false -o "$output" ./cmd/go-versioning
)

main "$@"
