# Store data in S3

rmhub has two stores. The blob store keeps the files of the documents.
The state store keeps small values, such as the account records and the
root of each library. Each store can use an S3 bucket. This guide puts
the blobs on Cloudflare R2 and keeps the state in SQLite.

## 1. Put the secret key in a file

The configuration must not contain the secret: the key
`secret_access_key` is an error. Write the secret access key into a file
that only the server can read. On NixOS, give the file in
`services.rmhub.credentials`.

## 2. Set the blob backend

```toml
data_dir = "/var/lib/rmhub"

[blob]
backend = "s3"
s3.endpoint = "https://<account>.r2.cloudflarestorage.com"
s3.region = "auto"
s3.bucket = "rmhub"
s3.prefix = "blobs/"
s3.path_style = true
s3.access_key_id = "<key id>"
s3.secret_access_key_file = "/run/credentials/rmhub.service/s3"

[state]
backend = "sqlite"
```

Without `access_key_id`, the server uses the standard AWS credential
sources.

## 3. Optional: put the state in S3 too

Set `state.backend = "s3"` and the `state.s3.*` keys. The blob store and
the state store can share one bucket with different prefixes.

At startup, the server checks that the state backend refuses a stale
conditional write. A backend that accepts the write stops the server.

## 4. Restart the server

Restart the server to apply the configuration.
[Server configuration](../reference/configuration.md#s3-settings) lists
each S3 key.
