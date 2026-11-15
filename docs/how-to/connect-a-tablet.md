# Connect a tablet

This guide pairs a reMarkable tablet with an account on your rmhub
server. You need a server that runs and an owner token for the account.

If the tablet uses rmfakecloud now, follow
[Switch from rmfakecloud](switch-from-rmfakecloud.md) instead.

## 1. Send the tablet to rmhub

The tablet talks to the reMarkable cloud over HTTPS. To send it to rmhub,
install [rmfakecloud-proxy](https://github.com/ddvk/rmfakecloud-proxy) on
the tablet. The installer of the proxy adds a CA certificate and host
entries, so that the cloud host names go to the proxy on the tablet.

Set the upstream URL of the proxy to the tablet listener of rmhub. The
installer asks for it. You can also set the `upstream` key of its
configuration file:

```yaml
# configuration file of rmfakecloud-proxy on the tablet
upstream: https://rmhub.example.org:8443
```

Put the tablet listener behind TLS (`listen.tablet.tls_cert_file`) or a
TLS reverse proxy when the network between the tablet and the server is
not trusted. See [TLS for the tablet](deploy-on-nixos.md#tls-for-the-tablet).

## 2. Check the announced host

The discovery endpoints tell the tablet which host to use for sync and
notifications. `tablet.announced_host` sets that host. The default is
`local.appspot.com`. The host must resolve to the proxy on the tablet.

Check the host entries that the proxy installer adds. Set this key if
your installer uses a different name:

```toml
[tablet]
announced_host = "local.appspot.com"
```

## 3. Pair the tablet

Make a pairing code with the owner token:

```console
$ rmhub pair
pxxdaymp  (expires 2026-11-15T15:38:13Z)
```

Type the code on the tablet within 5 minutes. A code works one time.

## 4. Check the result

The tablet is in the device list after it pairs:

```console
$ rmhub devices list
```

After the first sync, `rmhub ls /` shows the library of the tablet.

## If the tablet has problems

- **The tablet behaves badly after a sync.** Set `tablet.check_files` to
  `none`. Then the server answers `POST /sync/v3/check-files` with
  `{"missingFiles":null}`, like the reMarkable cloud. The default,
  `truthful`, names the blobs that the server does not have.
- **The tablet needs a different root index schema.** The server
  announces a schema version for root indexes to the tablet. The default
  is 3. Set it for each account:

  ```console
  $ rmhub account set-schema 4
  ```

  The server accepts root indexes of schema 3 and 4 for each setting.
- **A document does not sync.** The tablet cannot sync a file larger
  than `tablet.max_blob_bytes`.

To stop the access of a tablet, run `rmhub devices remove <id>`.
