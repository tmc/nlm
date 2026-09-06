---
title: Named identities
---
# Named identities

Tests use local stores and scripted API responses. Live cross-account behavior
has not been validated by this change.

## Named sessions

An identity names a saved `nlmauth.Session`: cookies, auth token, browser
profile, Google account index, and refresh metadata. The browser profile picks
the cookie jar; the account index picks an account within it. An empty account
index means the default account.

```
nlm auth login --as work --profile 'Profile 3' --authuser 2
nlm auth list
nlm auth use work
nlm --identity work notebook list
nlm auth remove work
```

A login without `--as` refreshes the selected identity. A login with `--as`
updates that identity without switching the current selection. The first named
login on a fresh installation becomes current. A new identity does not inherit
another identity's browser profile, account index, or refresh metadata.

`auth list` shows stored profiles, account indexes, and the current selection.
It does not measure expiration or report a notebook inventory.

## Selection and refresh

Selection follows this order:

1. Complete `NLM_AUTH_TOKEN` and `NLM_COOKIES` environment credentials bypass
   stored identity selection.
2. `--identity` selects a named session for this invocation.
3. `NLM_IDENTITY` supplies the same selection when the flag is absent.
4. Otherwise use the current identity, initially `default`.

Unknown explicit names are errors. Empty fields in a selected session stay
empty; the current identity cannot fill them from another account. Refresh
reads and writes the active identity, including its browser profile and account
index, without changing the current selection or another identity's mirror.

## File store

```
~/.nlm/
    env                       compatibility mirror of the current identity
    current                   selected identity name
    identities/<name>.env     named session
```

`nlmauth.Store` has Get, Set, Delete, and List operations. Names use
`[a-z0-9][a-z0-9._-]{0,63}`. Files are created with mode 0600 and directories
with mode 0700. Stores do not coordinate concurrent processes; the last writer
wins. A failed write is reported, and operations spanning multiple files are
not transactional.

A legacy `env` file is copied to identity `default` on first use when the
identities directory does not exist. `LoadSession` reads the current identity;
`Save` writes it and refreshes the compatibility mirror. `SaveIdentity` mirrors
only when writing the current identity. The CLI refreshes the mirror when
switching identities and removes it after deleting the last identity.

## Validation

Store tests cover migration, selection, identity isolation, invalid names,
listing, and deletion. CLI tests cover named login, refresh persistence,
empty account fields, command parsing, and removal of the last identity.
Tests do not establish a private service permission contract.
