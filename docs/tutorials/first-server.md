# Run your first server

In this tutorial you start an rmhub server on your computer. You create
an account, upload a PDF, and render a page of it as an image. You do
not need a tablet.

You need the `rmhub` command and a PDF file. To build the command from a
clone of the repository, run `nix build .#rmhub`. The command is then
`./result/bin/rmhub`.

The server uses the ports 8080 and 8081 on the loopback interface.

## 1. Generate the admin token

The admin token manages the accounts. Put it in a file that only you can
read:

```console
$ install -d -m 700 ~/.config/rmhub
$ rmhub admin token generate > ~/.config/rmhub/admin-token
$ chmod 600 ~/.config/rmhub/admin-token
```

## 2. Start the server

Write the configuration file `~/.config/rmhub/config.toml`. Replace
`/home/ada` with your home directory:

```toml
[admin]
token_file = "/home/ada/.config/rmhub/admin-token"
```

Start the server and let it run in this terminal:

```console
$ rmhub serve
time=… level=INFO msg=listening listener=tablet address=127.0.0.1:8080 tls=false
time=… level=INFO msg=listening listener=api address=127.0.0.1:8081 tls=false
```

The server has two listeners. Tablets use the first one. The `rmhub`
command uses the second one, the rmhub API.

## 3. Create an account

Open a second terminal. Tell the client where the server is, and give it
the admin token:

```console
$ export RMHUB_URL=http://127.0.0.1:8081
$ export RMHUB_TOKEN=$(cat ~/.config/rmhub/admin-token)
$ rmhub admin user add --email ada@example.org --name Ada
Created account oadi6myzjaxm7cqcze2mktevhs for ada@example.org
```

Your account ID is different. Use your ID in the next step.

## 4. Issue an owner token

An owner token has full access to one account:

```console
$ rmhub admin token issue oadi6myzjaxm7cqcze2mktevhs --label laptop
rmh_oadi6myzjaxm7cqcze2mktevhs_yawvsmavreyej7tx_…
Token yawvsmavreyej7tx (label "laptop", expires never). Store it now: the server does not keep it.
```

The first line is the token. Use it from now on:

```console
$ export RMHUB_TOKEN=rmh_oadi6myzjaxm7cqcze2mktevhs_yawvsmavreyej7tx_…
$ rmhub account
ID:      oadi6myzjaxm7cqcze2mktevhs
Email:   ada@example.org
Name:    Ada
Schema:  3
```

## 5. Upload a document

Create a folder and upload your PDF into it. This tutorial uses the file
`hello.pdf`:

```console
$ rmhub mkdir Papers
ID:        d3de0fab-9ddc-449b-bf69-e23525428a4e
Name:      Papers
Type:      folder
Path:      /Papers
…
$ rmhub upload hello.pdf --to /Papers
ID:        71e54995-a7c3-4831-bced-f34688d467e8
Name:      hello
Type:      pdf
Path:      /Papers/hello
…
$ rmhub ls /Papers
ID                                    TYPE  MODIFIED          NAME
71e54995-a7c3-4831-bced-f34688d467e8  pdf   2026-11-15 15:33  hello
```

The name of the document is the file name without the extension.

## 6. Render a page

List the pages of the document, then render the first page:

```console
$ rmhub pages /Papers/hello
NUMBER  ID  PDF  STROKES
1       -   1    no
$ rmhub render /Papers/hello --page 1 -o page.png
page.png
```

Open `page.png`. It shows the first page of your PDF. After a tablet
adds handwriting to the page, the image shows the handwriting too.

## 7. Stop the server

Go back to the first terminal and press Ctrl+C. The server keeps the
library in `~/.local/share/rmhub`. Start `rmhub serve` again, and
`rmhub ls /` shows the same folder.

## What you did

You ran a server, created an account with the admin token, and changed
and read the library with an owner token.

Next steps:

- [Connect a tablet](../how-to/connect-a-tablet.md) to sync the library.
- [Deploy on NixOS](../how-to/deploy-on-nixos.md) for a permanent server.
- Read [Architecture](../explanation/architecture.md) to learn how the
  parts work together.
