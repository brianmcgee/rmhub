# Runs the S3 contract tests against versitygw on a loopback port.
{ pkgs, perSystem, ... }:
let
  inherit (perSystem.self) rmhub;
  # The packages whose tests need an S3 server.
  testPackages = [
    "./internal/blob/s3"
    "./internal/state/s3"
  ];
in
pkgs.runCommand "s3-contract-check"
  {
    nativeBuildInputs = [
      pkgs.go
      pkgs.versitygw
      pkgs.curl
    ];
  }
  ''
    export HOME=$(mktemp -d)
    export GOCACHE=$TMPDIR/gocache
    export GOPROXY=off
    export GOFLAGS=-mod=vendor
    export CGO_ENABLED=0

    workdir=$(mktemp -d)
    cp -r ${rmhub.src}/* "$workdir"/
    chmod -R u+w "$workdir"
    cp -r ${rmhub.goModules} "$workdir"/vendor
    chmod -R u+w "$workdir"/vendor

    # The build sandbox has no xattrs, so versitygw keeps its metadata in
    # a sidecar directory.
    mkdir -p $TMPDIR/s3data $TMPDIR/s3meta
    ROOT_ACCESS_KEY=rmhub ROOT_SECRET_KEY=rmhub-secret-key \
      versitygw --port 127.0.0.1:7070 posix --sidecar $TMPDIR/s3meta $TMPDIR/s3data >$TMPDIR/vgw.log 2>&1 &
    vgw=$!
    trap 'kill $vgw 2>/dev/null || true' EXIT
    ready=
    for _ in $(seq 1 200); do
      if curl -s -o /dev/null http://127.0.0.1:7070/; then
        ready=1
        break
      fi
      sleep 0.1
    done
    if [ -z "$ready" ]; then
      echo "versitygw did not start:" >&2
      cat $TMPDIR/vgw.log >&2
      exit 1
    fi

    export RMHUB_TEST_S3_ENDPOINT=http://127.0.0.1:7070
    export RMHUB_TEST_S3_ACCESS_KEY_ID=rmhub
    export RMHUB_TEST_S3_SECRET_ACCESS_KEY=rmhub-secret-key

    cd "$workdir"
    # -v shows that the contract tests ran and did not skip.
    go test -count=1 -v ${toString testPackages} | tee $TMPDIR/test.log
    if grep -q -- '--- SKIP' $TMPDIR/test.log; then
      echo "a contract test skipped" >&2
      exit 1
    fi

    touch $out
  ''
