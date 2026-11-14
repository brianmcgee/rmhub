# Syncs rmapi from nixpkgs with rmhub on loopback ports. rmapi talks to
# the reMarkable cloud, so this check proves the Sync 1.5 protocol of
# rmhub for one client that the cloud accepts.
{ pkgs, perSystem, ... }:
let
  inherit (perSystem.self) rmhub;
in
pkgs.runCommand "rmapi-conformance-check"
  {
    nativeBuildInputs = [
      rmhub
      pkgs.rmapi
      pkgs.curl
      pkgs.jq
    ];
  }
  ''
    set -euo pipefail
    export HOME=$TMPDIR/home
    mkdir -p $HOME
    export XDG_CONFIG_HOME=$TMPDIR/config

    rmhub admin token generate > $TMPDIR/admin-token
    chmod 600 $TMPDIR/admin-token
    cat > $TMPDIR/config.toml <<EOF
    data_dir = "$TMPDIR/data"
    [admin]
    token_file = "$TMPDIR/admin-token"
    [listen.tablet]
    address = "127.0.0.1:18080"
    [listen.api]
    address = "127.0.0.1:18081"
    EOF

    rmhub serve --config $TMPDIR/config.toml > $TMPDIR/rmhub.log 2>&1 &
    server=$!
    trap 'kill $server 2>/dev/null || true' EXIT
    for _ in $(seq 1 100); do
      curl -sf -o /dev/null http://127.0.0.1:18081/healthz && break
      sleep 0.1
    done

    export RMHUB_URL=http://127.0.0.1:18081
    account=$(RMHUB_TOKEN=$(cat $TMPDIR/admin-token) rmhub admin user add --email ada@example.org --json | jq -r .id)
    export RMHUB_TOKEN=$(RMHUB_TOKEN=$(cat $TMPDIR/admin-token) rmhub admin token issue "$account")
    export RMAPI_HOST=http://127.0.0.1:18080

    fail() {
      echo "FAIL: $*" >&2
      echo "--- rmhub log" >&2
      cat $TMPDIR/rmhub.log >&2
      exit 1
    }

    # rmapi_as <name> <args>: runs rmapi with its own configuration and
    # home directory.
    rmapi_as() {
      local name=$1
      shift
      mkdir -p $TMPDIR/$name
      HOME=$TMPDIR/$name RMAPI_CONFIG=$TMPDIR/$name/rmapi.conf rmapi "$@" 2>&1
    }

    # pair <name>: pairs an rmapi instance with a code from the CLI.
    pair() {
      local code
      code=$(rmhub pair --json | jq -r .code)
      echo "$code" | rmapi_as "$1" ls >/dev/null || fail "pairing of $1"
    }

    # root <name>: prints the generation and the first line of the root
    # index, read through the tablet protocol with the user token of rmapi.
    root() {
      local tok gen hash
      tok=$(sed -n 's/^usertoken: //p' $TMPDIR/$1/rmapi.conf)
      gen=$(curl -sf -H "Authorization: Bearer $tok" $RMAPI_HOST/sync/v4/root | jq -r .generation)
      hash=$(curl -sf -H "Authorization: Bearer $tok" $RMAPI_HOST/sync/v4/root | jq -r .hash)
      echo "$gen $(curl -sf -H "Authorization: Bearer $tok" $RMAPI_HOST/sync/v3/files/$hash | head -n1)"
    }

    printf '%%PDF-1.4\n1 0 obj<<>>endobj\ntrailer<<>>\n%%%%EOF\n' > $TMPDIR/doc.pdf

    # scenario <name> <folder>: mkdir, put, mv, ls, get and rm.
    scenario() {
      local n=$1 d=$2 listing
      rmapi_as "$n" mkdir "/$d" || fail "mkdir /$d"
      rmapi_as "$n" put $TMPDIR/doc.pdf "/$d" || fail "put into /$d"
      rmapi_as "$n" ls "/$d" | grep -q '\[f\].*doc$' || fail "ls /$d after put"
      rmapi_as "$n" mv "/$d/doc" "/$d/renamed" || fail "mv in /$d"
      listing=$(rmapi_as "$n" ls "/$d")
      echo "$listing" | grep -q '\[f\].*renamed$' || fail "ls /$d after mv: $listing"
      echo "$listing" | grep -q '\[f\].*doc$' && fail "ls /$d after mv still has doc: $listing"
      mkdir -p $TMPDIR/get-$d
      (cd $TMPDIR/get-$d && rmapi_as "$n" get "/$d/renamed") || fail "get /$d/renamed"
      [ -s $TMPDIR/get-$d/renamed.rmdoc ] || fail "get gave no file"
      rmapi_as "$n" rm "/$d/renamed" || fail "rm /$d/renamed"
      rmapi_as "$n" ls "/$d" | grep -q renamed && fail "ls /$d after rm still has renamed"
      rmapi_as "$n" ls / | grep -q "\[d\].*$d$" || fail "ls / has no /$d"
    }

    echo "== schema 4 (rmapi default)"
    pair one
    scenario one Books
    set -- $(root one)
    [ "$2" = 4 ] || fail "stored root has schema $2, not 4"

    echo "== schema 3 (RMAPI_FORCE_SCHEMA_VERSION=3)"
    export RMAPI_FORCE_SCHEMA_VERSION=3
    scenario one Papers
    set -- $(root one)
    [ "$2" = 3 ] || fail "stored root has schema $2, not 3"
    unset RMAPI_FORCE_SCHEMA_VERSION

    echo "== two rmapi instances at the same time"
    pair two
    set -- $(root one)
    before=$1
    rmapi_as one mkdir /Left > $TMPDIR/left.log &
    left=$!
    rmapi_as two mkdir /Right > $TMPDIR/right.log &
    right=$!
    wait $left || fail "mkdir /Left: $(cat $TMPDIR/left.log)"
    wait $right || fail "mkdir /Right: $(cat $TMPDIR/right.log)"
    listing=$(rmapi_as one ls /)
    echo "$listing" | grep -q '\[d\].*Left$' || fail "final tree has no /Left: $listing"
    echo "$listing" | grep -q '\[d\].*Right$' || fail "final tree has no /Right: $listing"
    set -- $(root one)
    [ "$1" = $((before + 2)) ] || fail "root log has $(($1 - before)) entries for 2 changes"

    echo "all rmapi conformance steps passed"
    touch $out
  ''
