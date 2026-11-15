# Give an agent access to the library

A scoped token gives a program or an AI agent access to some folders
only. This guide creates such a token and gives it to an agent. You need
an owner token.

## 1. Create the folders

Create the folders that the agent can use, if they do not exist:

```console
$ rmhub mkdir Agent
$ rmhub mkdir Reference
```

## 2. Create a scoped token

Give one `--grant` for each folder, with `read` or `write`. Set an
expiry:

```console
$ rmhub tokens create --label agent --grant /Agent:write --grant /Reference:read --expires 720h
rmh_…
Token qnkuf4n2vu5objdo (label "agent", expires 2026-12-15T15:33:13Z). Store it now: the server does not keep it.
```

The first line is the token. The server does not show it again.

## 3. Give the token and the guide to the agent

Set these variables in the environment of the agent:

```sh
export RMHUB_URL=https://rmhub.example.org
export RMHUB_TOKEN=rmh_…
```

Give the agent the file [`skills/rmhub/SKILL.md`](https://github.com/brianmcgee/rmhub/blob/main/skills/rmhub/SKILL.md).
It tells the agent how to use each command, and what to do for each
exit code.

## 4. Stop the access

List the tokens and revoke the token of the agent:

```console
$ rmhub tokens list
$ rmhub tokens revoke qnkuf4n2vu5objdo
```

The revocation also revokes each token that the agent created with its
token.

[Tokens and access](../explanation/access.md) explains what a scoped
token can and cannot do.
