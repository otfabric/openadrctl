# Command reference

Every command of openadrctl with its flags, as `openadrctl <command> --help`
prints them. Run `--help` on a command for its description and examples.
Flags with `[env: …]` read that environment variable when the flag is not
given; see the [README](../README.md#environment-variables) for the order in which
flags, environment and defaults apply.

Every command also takes `--debug` (show the causes of an error on stderr)
and `-h`, `--help`. `openadrctl completion bash|zsh|fish|powershell` writes a
shell completion script; `openadrctl help <command>` is the same as
`openadrctl <command> --help`.

## Connection flags

The commands that talk to a VTN (`ping`, `token` and every `bl` and `ven`
command marked *connection flags* below) take these flags:

| Flag | Type | Default | Meaning |
|------|------|---------|---------|
| `--ca` | string |  | CA certificates that verify the VTN (PEM; default: the system roots) [env: OPENADRCTL_CA] |
| `--cert` | string |  | Client certificate for mutual TLS (PEM) [env: OPENADRCTL_CERT] |
| `--client-id` | string |  | OAuth2 client ID (client-credentials grant) [env: OPENADRCTL_CLIENT_ID] |
| `--client-secret-file` | string |  | File holding the OAuth2 client secret (or OPENADRCTL_CLIENT_SECRET; never a flag) [env: OPENADRCTL_CLIENT_SECRET_FILE] |
| `--format` | string | text | Output format: text, json or table [env: OPENADRCTL_FORMAT] |
| `--insecure` | bool |  | Do not verify the VTN's certificate (testing only) [env: OPENADRCTL_INSECURE] |
| `--key` | string |  | Private key of --cert (PEM) [env: OPENADRCTL_KEY] |
| `--retries` | int |  | Retry a request that failed in a way another attempt can cure (no answer, 429, 502-504) up to this many times; a create whose fate is not known is never resent [env: OPENADRCTL_RETRIES] |
| `--scope` | strings |  | Scopes to ask the token endpoint for (default: all the client has) [env: OPENADRCTL_SCOPE] |
| `--spec` | string | 3.1.0 | Revision of OpenADR the VTN speaks: 3.1.0 or 3.0.1 [env: OPENADRCTL_SPEC] |
| `--timeout` | duration | 30s | Time limit of each command's exchange with the VTN [env: OPENADRCTL_TIMEOUT] |
| `--token-file` | string |  | File holding a bearer token, instead of client credentials (or OPENADRCTL_TOKEN) [env: OPENADRCTL_TOKEN_FILE] |
| `--token-url` | string |  | Token endpoint (default: what the VTN names at GET /auth/server; with --spec 3.0.1, /auth/token of the VTN) [env: OPENADRCTL_TOKEN_URL] |
| `--url` | string |  | Base URL of the VTN's API, the one /programs is appended to, such as https://vtn.example/openadr3/3.1.0 [env: OPENADRCTL_URL] |

## Commands

### openadrctl bl

Talk to a VTN as Business Logic.

Subcommands: `events`, `programs`, `reports`, `resources`, `vens`, `listen`, `notifiers`, `subscriptions`, `topics`.

### openadrctl bl events

Events: what a program asks of VENs, interval by interval.

Alias: `event`.

Subcommands: `create`, `delete`, `get`, `list`, `timeline`, `update`, `watch`.

### openadrctl bl events create

Create an event from flags or a JSON file.

```text
openadrctl bl events create [flags]
```

| Flag | Type | Default | Meaning |
|------|------|---------|---------|
| `--csv` | string |  | Intervals from a timeline CSV (interval,start,end,type,values; see openadrctl timeline) |
| `--dry-run` | bool |  | Validate and print the object that would be sent; send nothing |
| `--duration` | string |  | Duration of the whole event (ISO 8601 or 1h), when not that of its intervals |
| `--file` | string |  | The object as JSON (of the revision of --spec), - for stdin; flags change what it says |
| `--interval` | string |  | Length of each interval: PT15M, 1h, ... |
| `--name` | string |  | Name of the event |
| `--priority` | int |  | Priority: 0 is the highest |
| `--program` | string |  | ID of the program of the event |
| `--report` | strings |  | Ask VENs for a report of this payload type (repeatable), such as USAGE |
| `--start` | string |  | Start of the first interval: now, or RFC 3339 |
| `--target` | strings |  | Targets of the event (repeatable) |
| `--type` | string |  | Payload type of the intervals, such as PRICE, SIMPLE or USAGE |
| `--value` | strings |  | A value per interval, in order (repeatable; a decimal number, true/false, x:y or text) |
| `--yes` | bool |  | Confirm the change |

Plus the [connection flags](#connection-flags).

### openadrctl bl events delete

Delete an event.

```text
openadrctl bl events delete <id> [flags]
```

Alias: `rm`.

| Flag | Type | Default | Meaning |
|------|------|---------|---------|
| `--yes` | bool |  | Confirm the deletion |

Plus the [connection flags](#connection-flags).

### openadrctl bl events get

Read an event by its ID.

```text
openadrctl bl events get <id> [flags]
```

Flags: the [connection flags](#connection-flags).

### openadrctl bl events list

Search the events.

```text
openadrctl bl events list [flags]
```

Alias: `ls`.

| Flag | Type | Default | Meaning |
|------|------|---------|---------|
| `--active` | bool |  | Leave out events that are over |
| `--limit` | int |  | Ask for at most this many objects, 50 at most (one page only); default: all, page by page |
| `--program` | string |  | Events of this program |
| `--skip` | int |  | Skip this many objects (one page only) |
| `--target` | strings |  | Events with one of these targets (repeatable) |

Plus the [connection flags](#connection-flags).

### openadrctl bl events timeline

Lay out the intervals of an event in time.

```text
openadrctl bl events timeline <event-id> [flags]
```

| Flag | Type | Default | Meaning |
|------|------|---------|---------|
| `--at` | string |  | Only the rows in force at this time: now, or RFC 3339 |
| `--csv` | bool |  | Write the table as CSV (the form --csv of events create reads) |
| `--sub-intervals` | bool |  | Split a payload with several values into intervals of equal length |
| `--until` | string |  | Lay out an event that repeats without end up to this time (RFC 3339) |

Plus the [connection flags](#connection-flags).

### openadrctl bl events update

Change an event: read it, apply --file and the flags, send it back.

```text
openadrctl bl events update <id> [flags]
```

| Flag | Type | Default | Meaning |
|------|------|---------|---------|
| `--csv` | string |  | Intervals from a timeline CSV (interval,start,end,type,values; see openadrctl timeline) |
| `--dry-run` | bool |  | Validate and print the object that would be sent; send nothing |
| `--duration` | string |  | Duration of the whole event (ISO 8601 or 1h), when not that of its intervals |
| `--file` | string |  | The object as JSON (of the revision of --spec), - for stdin; flags change what it says |
| `--interval` | string |  | Length of each interval: PT15M, 1h, ... |
| `--name` | string |  | Name of the event |
| `--priority` | int |  | Priority: 0 is the highest |
| `--program` | string |  | ID of the program of the event |
| `--report` | strings |  | Ask VENs for a report of this payload type (repeatable), such as USAGE |
| `--start` | string |  | Start of the first interval: now, or RFC 3339 |
| `--target` | strings |  | Targets of the event (repeatable) |
| `--type` | string |  | Payload type of the intervals, such as PRICE, SIMPLE or USAGE |
| `--value` | strings |  | A value per interval, in order (repeatable; a decimal number, true/false, x:y or text) |
| `--yes` | bool |  | Confirm the change |

Plus the [connection flags](#connection-flags).

### openadrctl bl events watch

Search for events again and again, and print what changed.

```text
openadrctl bl events watch [flags]
```

| Flag | Type | Default | Meaning |
|------|------|---------|---------|
| `--active` | bool |  | Leave out events that are over |
| `--changes-only` | bool |  | Leave out the events that are there at the start |
| `--count` | int |  | Stop after this many records (0: no limit) |
| `--duration` | duration |  | Stop after this long (0: until interrupted) |
| `--interval` | duration | 30s | Time between two searches |
| `--program` | string |  | Events of this program |
| `--target` | strings |  | Events with one of these targets (repeatable) |

Plus the [connection flags](#connection-flags).

### openadrctl bl listen

Subscribe to notifications and print them as they arrive.

```text
openadrctl bl listen [flags]
```

| Flag | Type | Default | Meaning |
|------|------|---------|---------|
| `--broker` | string |  | mqtt: broker URL, mqtt:// or mqtts:// (default: the first the VTN announces) [env: OPENADRCTL_MQTT_BROKER] |
| `--callback-url` | string |  | webhook: URL under which the VTN reaches --listen (default: http://<listen>/) [env: OPENADRCTL_CALLBACK_URL] |
| `--client-name` | string |  | Client name of the subscription (default: openadrctl) |
| `--count` | int |  | Stop after this many notifications (0: no limit) |
| `--duration` | duration |  | Stop after this long (0: until interrupted) |
| `--keep` | bool |  | Leave the subscription on the VTN when the command ends |
| `--listen` | string | 127.0.0.1:8081 | webhook: address to receive notifications on [env: OPENADRCTL_LISTEN] |
| `--mqtt-ca` | string |  | mqtt: CA certificates that verify an mqtts broker (PEM) [env: OPENADRCTL_MQTT_CA] |
| `--mqtt-client-id` | string |  | mqtt: client ID at the broker (default: openadrctl-<random>) [env: OPENADRCTL_MQTT_CLIENT_ID] |
| `--mqtt-password-file` | string |  | mqtt: file holding the password (or OPENADRCTL_MQTT_PASSWORD; with OAUTH2_BEARER_TOKEN the client's token is used) [env: OPENADRCTL_MQTT_PASSWORD_FILE] |
| `--mqtt-username` | string |  | mqtt: user name (default: what the VTN announces) [env: OPENADRCTL_MQTT_USERNAME] |
| `--object` | strings |  | Kinds of object (repeatable): PROGRAM, EVENT, REPORT, SUBSCRIPTION, VEN, RESOURCE (default: EVENT) |
| `--operation` | strings |  | Operations (repeatable): CREATE, UPDATE, DELETE, READ (default: CREATE, UPDATE, DELETE) |
| `--program` | string |  | Only this program, its events and the reports on them |
| `--target` | strings |  | Targets to name in the subscription, as in a search (repeatable) |
| `--tls-cert` | string |  | webhook: serve https with this certificate (PEM) |
| `--tls-key` | string |  | webhook: private key of --tls-cert (PEM) |
| `--topics` | strings |  | mqtt: sets of topics to subscribe to (repeatable), such as events or ven-events:<id>; see topics |
| `--via` | string |  | How to be notified: webhook, mqtt or websocket (required) [env: OPENADRCTL_VIA] |

Plus the [connection flags](#connection-flags).

### openadrctl bl notifiers

Show how the VTN notifies: webhooks, MQTT, WebSockets.

```text
openadrctl bl notifiers [flags]
```

Flags: the [connection flags](#connection-flags).

### openadrctl bl programs

Programs: the demand-response offers of the VTN.

Alias: `program`.

Subcommands: `create`, `delete`, `get`, `list`, `update`.

### openadrctl bl programs create

Create a program from flags or a JSON file.

```text
openadrctl bl programs create [flags]
```

| Flag | Type | Default | Meaning |
|------|------|---------|---------|
| `--attribute` | stringArray |  | Attribute as TYPE=value[,value...] (repeatable) |
| `--description-url` | strings |  | URL of a description of the program (repeatable) |
| `--dry-run` | bool |  | Validate and print the object that would be sent; send nothing |
| `--file` | string |  | The object as JSON (of the revision of --spec), - for stdin; flags change what it says |
| `--name` | string |  | Name of the program (unique on the VTN) |
| `--payload` | strings |  | Payload of its events as TYPE[:units[:currency]] (repeatable), such as PRICE::EUR |
| `--target` | strings |  | Targets of the program (repeatable): only VENs granted one see it |

Plus the [connection flags](#connection-flags).

### openadrctl bl programs delete

Delete a program.

```text
openadrctl bl programs delete <id> [flags]
```

Alias: `rm`.

| Flag | Type | Default | Meaning |
|------|------|---------|---------|
| `--yes` | bool |  | Confirm the deletion |

Plus the [connection flags](#connection-flags).

### openadrctl bl programs get

Read a program by its ID.

```text
openadrctl bl programs get <id> [flags]
```

Flags: the [connection flags](#connection-flags).

### openadrctl bl programs list

Search the programs.

```text
openadrctl bl programs list [flags]
```

Alias: `ls`.

| Flag | Type | Default | Meaning |
|------|------|---------|---------|
| `--limit` | int |  | Ask for at most this many objects, 50 at most (one page only); default: all, page by page |
| `--skip` | int |  | Skip this many objects (one page only) |
| `--target` | strings |  | Programs with one of these targets (repeatable) |

Plus the [connection flags](#connection-flags).

### openadrctl bl programs update

Change a program: read it, apply --file and the flags, send it back.

```text
openadrctl bl programs update <id> [flags]
```

| Flag | Type | Default | Meaning |
|------|------|---------|---------|
| `--attribute` | stringArray |  | Attribute as TYPE=value[,value...] (repeatable) |
| `--description-url` | strings |  | URL of a description of the program (repeatable) |
| `--dry-run` | bool |  | Validate and print the object that would be sent; send nothing |
| `--file` | string |  | The object as JSON (of the revision of --spec), - for stdin; flags change what it says |
| `--name` | string |  | Name of the program (unique on the VTN) |
| `--payload` | strings |  | Payload of its events as TYPE[:units[:currency]] (repeatable), such as PRICE::EUR |
| `--target` | strings |  | Targets of the program (repeatable): only VENs granted one see it |

Plus the [connection flags](#connection-flags).

### openadrctl bl reports

Reports: what VENs say about events.

Alias: `report`.

Subcommands: `get`, `list`.

### openadrctl bl reports get

Read a report by its ID.

```text
openadrctl bl reports get <id> [flags]
```

Flags: the [connection flags](#connection-flags).

### openadrctl bl reports list

Search the reports.

```text
openadrctl bl reports list [flags]
```

Alias: `ls`.

| Flag | Type | Default | Meaning |
|------|------|---------|---------|
| `--client-name` | string |  | Reports with this client name |
| `--event` | string |  | Reports on this event |
| `--limit` | int |  | Ask for at most this many objects, 50 at most (one page only); default: all, page by page |
| `--program` | string |  | Reports on events of this program |
| `--skip` | int |  | Skip this many objects (one page only) |

Plus the [connection flags](#connection-flags).

### openadrctl bl resource-groups

Resource groups are not part of OpenADR 3.1.0: they are an extension of OpenLEADR that.

Alias: `groups`, `resource-group`.

Subcommands: `create`, `delete`, `get`, `list`, `update`.

### openadrctl bl resource-groups create

Create a resource group from flags or a JSON file.

```text
openadrctl bl resource-groups create [flags]
```

| Flag | Type | Default | Meaning |
|------|------|---------|---------|
| `--attribute` | stringArray |  | Attribute as TYPE=value[,value...] (repeatable) |
| `--child` | strings |  | Member as resource:<id> or group:<id> (repeatable) |
| `--dry-run` | bool |  | Validate and print the object that would be sent; send nothing |
| `--file` | string |  | The object as JSON (of the revision of --spec), - for stdin; flags change what it says |
| `--name` | string |  | Name of the group |
| `--target` | strings |  | Targets of the group (repeatable) |

Plus the [connection flags](#connection-flags).

### openadrctl bl resource-groups delete

Delete a resource group.

```text
openadrctl bl resource-groups delete <id> [flags]
```

Alias: `rm`.

| Flag | Type | Default | Meaning |
|------|------|---------|---------|
| `--yes` | bool |  | Confirm the deletion |

Plus the [connection flags](#connection-flags).

### openadrctl bl resource-groups get

Read a resource group by its ID.

```text
openadrctl bl resource-groups get <id> [flags]
```

Flags: the [connection flags](#connection-flags).

### openadrctl bl resource-groups list

Search the resource groups.

```text
openadrctl bl resource-groups list [flags]
```

Alias: `ls`.

| Flag | Type | Default | Meaning |
|------|------|---------|---------|
| `--limit` | int |  | Ask for at most this many objects, 50 at most (one page only); default: all, page by page |
| `--name` | string |  | Groups of this name |
| `--skip` | int |  | Skip this many objects (one page only) |
| `--target` | strings |  | Groups with one of these targets (repeatable) |

Plus the [connection flags](#connection-flags).

### openadrctl bl resource-groups update

Change a resource group: read it, apply --file and the flags, send it back.

```text
openadrctl bl resource-groups update <id> [flags]
```

| Flag | Type | Default | Meaning |
|------|------|---------|---------|
| `--attribute` | stringArray |  | Attribute as TYPE=value[,value...] (repeatable) |
| `--child` | strings |  | Member as resource:<id> or group:<id> (repeatable) |
| `--dry-run` | bool |  | Validate and print the object that would be sent; send nothing |
| `--file` | string |  | The object as JSON (of the revision of --spec), - for stdin; flags change what it says |
| `--name` | string |  | Name of the group |
| `--target` | strings |  | Targets of the group (repeatable) |

Plus the [connection flags](#connection-flags).

### openadrctl bl resources

Resources: the devices behind a VEN.

Alias: `resource`.

Subcommands: `create`, `delete`, `get`, `list`, `update`.

### openadrctl bl resources create

Create a resource from flags or a JSON file.

```text
openadrctl bl resources create [flags]
```

| Flag | Type | Default | Meaning |
|------|------|---------|---------|
| `--attribute` | stringArray |  | Attribute as TYPE=value[,value...] (repeatable) |
| `--dry-run` | bool |  | Validate and print the object that would be sent; send nothing |
| `--file` | string |  | The object as JSON (of the revision of --spec), - for stdin; flags change what it says |
| `--name` | string |  | Name of the resource (unique within its VEN) |
| `--target` | strings |  | Targets granted to the resource (repeatable) |
| `--ven` | string |  | ID of the VEN of the resource |
| `--ven-client-id` | string |  | Client ID of the VEN that owns the resource |

Plus the [connection flags](#connection-flags).

### openadrctl bl resources delete

Delete a resource.

```text
openadrctl bl resources delete <id> [flags]
```

Alias: `rm`.

| Flag | Type | Default | Meaning |
|------|------|---------|---------|
| `--ven` | string |  | ID of the VEN of the resource: required with --spec 3.0.1, which reaches a resource through its VEN |
| `--yes` | bool |  | Confirm the deletion |

Plus the [connection flags](#connection-flags).

### openadrctl bl resources get

Read a resource by its ID.

```text
openadrctl bl resources get <id> [flags]
```

| Flag | Type | Default | Meaning |
|------|------|---------|---------|
| `--ven` | string |  | ID of the VEN of the resource: required with --spec 3.0.1, which reaches a resource through its VEN |

Plus the [connection flags](#connection-flags).

### openadrctl bl resources list

Search the resources.

```text
openadrctl bl resources list [flags]
```

Alias: `ls`.

| Flag | Type | Default | Meaning |
|------|------|---------|---------|
| `--limit` | int |  | Ask for at most this many objects, 50 at most (one page only); default: all, page by page |
| `--name` | string |  | Resources of this name |
| `--skip` | int |  | Skip this many objects (one page only) |
| `--target` | strings |  | Resources with one of these targets (repeatable) |
| `--ven` | string |  | Resources of this VEN |

Plus the [connection flags](#connection-flags).

### openadrctl bl resources update

Change a resource: read it, apply --file and the flags, send it back.

```text
openadrctl bl resources update <id> [flags]
```

| Flag | Type | Default | Meaning |
|------|------|---------|---------|
| `--attribute` | stringArray |  | Attribute as TYPE=value[,value...] (repeatable) |
| `--dry-run` | bool |  | Validate and print the object that would be sent; send nothing |
| `--file` | string |  | The object as JSON (of the revision of --spec), - for stdin; flags change what it says |
| `--name` | string |  | Name of the resource (unique within its VEN) |
| `--target` | strings |  | Targets granted to the resource (repeatable) |
| `--ven` | string |  | ID of the VEN of the resource |
| `--ven-client-id` | string |  | Client ID of the VEN that owns the resource |

Plus the [connection flags](#connection-flags).

### openadrctl bl subscriptions

Subscriptions: what the VTN notifies a client of, and where.

Alias: `subscription`, `subs`.

Subcommands: `create`, `delete`, `get`, `list`, `update`.

### openadrctl bl subscriptions create

Create a subscription from flags or a JSON file.

```text
openadrctl bl subscriptions create [flags]
```

| Flag | Type | Default | Meaning |
|------|------|---------|---------|
| `--callback-url` | string |  | Webhook to notify (https; the VTN checks that it answers first) |
| `--client-name` | string |  | Name of the subscribing client |
| `--dry-run` | bool |  | Validate and print the object that would be sent; send nothing |
| `--file` | string |  | The object as JSON (of the revision of --spec), - for stdin; flags change what it says |
| `--object` | strings |  | Kinds of object (repeatable): PROGRAM, EVENT, REPORT, SUBSCRIPTION, VEN, RESOURCE (default: EVENT) |
| `--operation` | strings |  | Operations (repeatable): CREATE, UPDATE, DELETE, READ (default: CREATE, UPDATE, DELETE) |
| `--program` | string |  | Only this program, its events and the reports on them |
| `--target` | strings |  | Targets the subscriber names, as in a search (repeatable) |
| `--webhook-token` | string |  | Bearer token the VTN sends to the webhook, made up for the subscription |
| `--websocket` | bool |  | Notify over the client's WebSocket instead of a webhook (extension) |

Plus the [connection flags](#connection-flags).

### openadrctl bl subscriptions delete

Delete a subscription.

```text
openadrctl bl subscriptions delete <id> [flags]
```

Alias: `rm`.

| Flag | Type | Default | Meaning |
|------|------|---------|---------|
| `--yes` | bool |  | Confirm the deletion |

Plus the [connection flags](#connection-flags).

### openadrctl bl subscriptions get

Read a subscription by its ID.

```text
openadrctl bl subscriptions get <id> [flags]
```

Flags: the [connection flags](#connection-flags).

### openadrctl bl subscriptions list

Search the subscriptions.

```text
openadrctl bl subscriptions list [flags]
```

Alias: `ls`.

| Flag | Type | Default | Meaning |
|------|------|---------|---------|
| `--client-name` | string |  | Subscriptions of this client name |
| `--limit` | int |  | Ask for at most this many objects, 50 at most (one page only); default: all, page by page |
| `--object` | strings |  | Subscriptions to one of these kinds of object (repeatable): PROGRAM, EVENT, ... |
| `--program` | string |  | Subscriptions to this program |
| `--skip` | int |  | Skip this many objects (one page only) |

Plus the [connection flags](#connection-flags).

### openadrctl bl subscriptions update

Change a subscription: read it, apply --file and the flags, send it back.

```text
openadrctl bl subscriptions update <id> [flags]
```

| Flag | Type | Default | Meaning |
|------|------|---------|---------|
| `--callback-url` | string |  | Webhook to notify (https; the VTN checks that it answers first) |
| `--client-name` | string |  | Name of the subscribing client |
| `--dry-run` | bool |  | Validate and print the object that would be sent; send nothing |
| `--file` | string |  | The object as JSON (of the revision of --spec), - for stdin; flags change what it says |
| `--object` | strings |  | Kinds of object (repeatable): PROGRAM, EVENT, REPORT, SUBSCRIPTION, VEN, RESOURCE (default: EVENT) |
| `--operation` | strings |  | Operations (repeatable): CREATE, UPDATE, DELETE, READ (default: CREATE, UPDATE, DELETE) |
| `--program` | string |  | Only this program, its events and the reports on them |
| `--target` | strings |  | Targets the subscriber names, as in a search (repeatable) |
| `--webhook-token` | string |  | Bearer token the VTN sends to the webhook, made up for the subscription |
| `--websocket` | bool |  | Notify over the client's WebSocket instead of a webhook (extension) |

Plus the [connection flags](#connection-flags).

### openadrctl bl topics

Show the MQTT topics of a set of objects.

```text
openadrctl bl topics <set> [flags]
```

Flags: the [connection flags](#connection-flags).

### openadrctl bl vens

VENs: the VEN objects the VTN knows.

Alias: `ven`.

Subcommands: `create`, `delete`, `get`, `list`, `update`.

### openadrctl bl vens create

Create a VEN from flags or a JSON file.

```text
openadrctl bl vens create [flags]
```

| Flag | Type | Default | Meaning |
|------|------|---------|---------|
| `--attribute` | stringArray |  | Attribute as TYPE=value[,value...] (repeatable) |
| `--dry-run` | bool |  | Validate and print the object that would be sent; send nothing |
| `--file` | string |  | The object as JSON (of the revision of --spec), - for stdin; flags change what it says |
| `--name` | string |  | Name of the VEN (unique on the VTN) |
| `--target` | strings |  | Targets granted to the VEN (repeatable) |
| `--ven-client-id` | string |  | Client ID the VEN authenticates with |

Plus the [connection flags](#connection-flags).

### openadrctl bl vens delete

Delete a VEN.

```text
openadrctl bl vens delete <id> [flags]
```

Alias: `rm`.

| Flag | Type | Default | Meaning |
|------|------|---------|---------|
| `--yes` | bool |  | Confirm the deletion |

Plus the [connection flags](#connection-flags).

### openadrctl bl vens get

Read a VEN by its ID.

```text
openadrctl bl vens get <id> [flags]
```

Flags: the [connection flags](#connection-flags).

### openadrctl bl vens list

Search the vens.

```text
openadrctl bl vens list [flags]
```

Alias: `ls`.

| Flag | Type | Default | Meaning |
|------|------|---------|---------|
| `--limit` | int |  | Ask for at most this many objects, 50 at most (one page only); default: all, page by page |
| `--name` | string |  | The VEN of this name |
| `--skip` | int |  | Skip this many objects (one page only) |
| `--target` | strings |  | VENs with one of these targets (repeatable) |

Plus the [connection flags](#connection-flags).

### openadrctl bl vens update

Change a VEN: read it, apply --file and the flags, send it back.

```text
openadrctl bl vens update <id> [flags]
```

| Flag | Type | Default | Meaning |
|------|------|---------|---------|
| `--attribute` | stringArray |  | Attribute as TYPE=value[,value...] (repeatable) |
| `--dry-run` | bool |  | Validate and print the object that would be sent; send nothing |
| `--file` | string |  | The object as JSON (of the revision of --spec), - for stdin; flags change what it says |
| `--name` | string |  | Name of the VEN (unique on the VTN) |
| `--target` | strings |  | Targets granted to the VEN (repeatable) |
| `--ven-client-id` | string |  | Client ID the VEN authenticates with |

Plus the [connection flags](#connection-flags).

### openadrctl completion

Generate shell completion scripts.

```text
openadrctl completion [bash|zsh|fish|powershell] [flags]
```

Subcommands: `bash:`, `zsh:`, `fish:`.

No flags of its own.

### openadrctl discover

Find VTNs on the local network (DNS-SD over multicast DNS).

```text
openadrctl discover [flags]
```

| Flag | Type | Default | Meaning |
|------|------|---------|---------|
| `--first` | bool |  | Stop at the first VTN that answers |
| `--format` | string | text | Output format: text, json or table [env: OPENADRCTL_FORMAT] |
| `--wait` | duration | 3s | How long to wait for answers [env: OPENADRCTL_DISCOVER_WAIT] |

### openadrctl ping

Check a VTN step by step: connect, TLS, token, API, notifiers.

```text
openadrctl ping [flags]
```

Subcommands: `connect`, `tls`, `token`, `api`, `notifiers`.

Flags: the [connection flags](#connection-flags).

### openadrctl timeline

Lay out the intervals of an event read from a file, offline.

```text
openadrctl timeline <event-file> [flags]
```

| Flag | Type | Default | Meaning |
|------|------|---------|---------|
| `--at` | string |  | Only the rows in force at this time: now, or RFC 3339 |
| `--csv` | bool |  | Write the table as CSV (the form --csv of events create reads) |
| `--format` | string | text | Output format: text, json or table [env: OPENADRCTL_FORMAT] |
| `--now` | string |  | The time "now" stands for in the event: RFC 3339 (default: the current time) |
| `--sub-intervals` | bool |  | Split a payload with several values into intervals of equal length |
| `--until` | string |  | Lay out an event that repeats without end up to this time (RFC 3339) |

### openadrctl token

Fetch an OAuth2 token with the client credentials.

```text
openadrctl token [flags]
```

Flags: the [connection flags](#connection-flags).

### openadrctl validate

Check an object against the OpenADR schema, offline.

```text
openadrctl validate <kind> <file> [flags]
```

| Flag | Type | Default | Meaning |
|------|------|---------|---------|
| `--format` | string | text | Output format: text, json or table [env: OPENADRCTL_FORMAT] |
| `--spec` | string | 3.1.0 | Revision of OpenADR the JSON is of: 3.1.0 or 3.0.1 [env: OPENADRCTL_SPEC] |

### openadrctl ven

Talk to a VTN as a VEN.

Subcommands: `events`, `programs`, `reports`, `resources`, `vens`, `listen`, `notifiers`, `subscriptions`, `topics`.

### openadrctl ven events

Events: what a program asks of VENs, interval by interval.

Alias: `event`.

Subcommands: `get`, `list`, `timeline`, `watch`.

### openadrctl ven events get

Read an event by its ID.

```text
openadrctl ven events get <id> [flags]
```

Flags: the [connection flags](#connection-flags).

### openadrctl ven events list

Search the events.

```text
openadrctl ven events list [flags]
```

Alias: `ls`.

| Flag | Type | Default | Meaning |
|------|------|---------|---------|
| `--active` | bool |  | Leave out events that are over |
| `--limit` | int |  | Ask for at most this many objects, 50 at most (one page only); default: all, page by page |
| `--program` | string |  | Events of this program |
| `--skip` | int |  | Skip this many objects (one page only) |
| `--target` | strings |  | Events with one of these targets (repeatable) |

Plus the [connection flags](#connection-flags).

### openadrctl ven events timeline

Lay out the intervals of an event in time.

```text
openadrctl ven events timeline <event-id> [flags]
```

| Flag | Type | Default | Meaning |
|------|------|---------|---------|
| `--at` | string |  | Only the rows in force at this time: now, or RFC 3339 |
| `--csv` | bool |  | Write the table as CSV (the form --csv of events create reads) |
| `--sub-intervals` | bool |  | Split a payload with several values into intervals of equal length |
| `--until` | string |  | Lay out an event that repeats without end up to this time (RFC 3339) |

Plus the [connection flags](#connection-flags).

### openadrctl ven events watch

Search for events again and again, and print what changed.

```text
openadrctl ven events watch [flags]
```

| Flag | Type | Default | Meaning |
|------|------|---------|---------|
| `--active` | bool |  | Leave out events that are over |
| `--changes-only` | bool |  | Leave out the events that are there at the start |
| `--count` | int |  | Stop after this many records (0: no limit) |
| `--duration` | duration |  | Stop after this long (0: until interrupted) |
| `--interval` | duration | 30s | Time between two searches |
| `--program` | string |  | Events of this program |
| `--target` | strings |  | Events with one of these targets (repeatable) |

Plus the [connection flags](#connection-flags).

### openadrctl ven listen

Subscribe to notifications and print them as they arrive.

```text
openadrctl ven listen [flags]
```

| Flag | Type | Default | Meaning |
|------|------|---------|---------|
| `--broker` | string |  | mqtt: broker URL, mqtt:// or mqtts:// (default: the first the VTN announces) [env: OPENADRCTL_MQTT_BROKER] |
| `--callback-url` | string |  | webhook: URL under which the VTN reaches --listen (default: http://<listen>/) [env: OPENADRCTL_CALLBACK_URL] |
| `--client-name` | string |  | Client name of the subscription (default: openadrctl) |
| `--count` | int |  | Stop after this many notifications (0: no limit) |
| `--duration` | duration |  | Stop after this long (0: until interrupted) |
| `--keep` | bool |  | Leave the subscription on the VTN when the command ends |
| `--listen` | string | 127.0.0.1:8081 | webhook: address to receive notifications on [env: OPENADRCTL_LISTEN] |
| `--mqtt-ca` | string |  | mqtt: CA certificates that verify an mqtts broker (PEM) [env: OPENADRCTL_MQTT_CA] |
| `--mqtt-client-id` | string |  | mqtt: client ID at the broker (default: openadrctl-<random>) [env: OPENADRCTL_MQTT_CLIENT_ID] |
| `--mqtt-password-file` | string |  | mqtt: file holding the password (or OPENADRCTL_MQTT_PASSWORD; with OAUTH2_BEARER_TOKEN the client's token is used) [env: OPENADRCTL_MQTT_PASSWORD_FILE] |
| `--mqtt-username` | string |  | mqtt: user name (default: what the VTN announces) [env: OPENADRCTL_MQTT_USERNAME] |
| `--object` | strings |  | Kinds of object (repeatable): PROGRAM, EVENT, REPORT, SUBSCRIPTION, VEN, RESOURCE (default: EVENT) |
| `--operation` | strings |  | Operations (repeatable): CREATE, UPDATE, DELETE, READ (default: CREATE, UPDATE, DELETE) |
| `--program` | string |  | Only this program, its events and the reports on them |
| `--target` | strings |  | Targets to name in the subscription, as in a search (repeatable) |
| `--tls-cert` | string |  | webhook: serve https with this certificate (PEM) |
| `--tls-key` | string |  | webhook: private key of --tls-cert (PEM) |
| `--topics` | strings |  | mqtt: sets of topics to subscribe to (repeatable), such as events or ven-events:<id>; see topics |
| `--via` | string |  | How to be notified: webhook, mqtt or websocket (required) [env: OPENADRCTL_VIA] |

Plus the [connection flags](#connection-flags).

### openadrctl ven notifiers

Show how the VTN notifies: webhooks, MQTT, WebSockets.

```text
openadrctl ven notifiers [flags]
```

Flags: the [connection flags](#connection-flags).

### openadrctl ven programs

Programs: the demand-response offers of the VTN.

Alias: `program`.

Subcommands: `get`, `list`.

### openadrctl ven programs get

Read a program by its ID.

```text
openadrctl ven programs get <id> [flags]
```

Flags: the [connection flags](#connection-flags).

### openadrctl ven programs list

Search the programs.

```text
openadrctl ven programs list [flags]
```

Alias: `ls`.

| Flag | Type | Default | Meaning |
|------|------|---------|---------|
| `--limit` | int |  | Ask for at most this many objects, 50 at most (one page only); default: all, page by page |
| `--skip` | int |  | Skip this many objects (one page only) |
| `--target` | strings |  | Programs with one of these targets (repeatable) |

Plus the [connection flags](#connection-flags).

### openadrctl ven reports

Reports: what VENs say about events.

Alias: `report`.

Subcommands: `create`, `delete`, `get`, `list`, `update`.

### openadrctl ven reports create

Create a report from flags or a JSON file.

```text
openadrctl ven reports create [flags]
```

| Flag | Type | Default | Meaning |
|------|------|---------|---------|
| `--client-name` | string |  | Name of the reporting client |
| `--csv` | string |  | Intervals from a timeline CSV (interval,start,end,type,values; see openadrctl timeline) |
| `--dry-run` | bool |  | Validate and print the object that would be sent; send nothing |
| `--event` | string |  | ID of the event the report is on |
| `--file` | string |  | The object as JSON (of the revision of --spec), - for stdin; flags change what it says |
| `--interval` | string |  | Length of each interval: PT15M, 1h, ... |
| `--name` | string |  | Name of the report |
| `--reading-type` | string |  | Reading type of the payload, such as DIRECT_READ |
| `--resource` | string | AGGREGATED_REPORT | Name of the resource the intervals are of |
| `--start` | string |  | Start of the first interval: now, or RFC 3339 |
| `--type` | string |  | Payload type of the readings, such as PRICE, SIMPLE or USAGE |
| `--units` | string |  | Units of the payload, such as KWH |
| `--value` | strings |  | A value per interval, in order (repeatable; a decimal number, true/false, x:y or text) |

Plus the [connection flags](#connection-flags).

### openadrctl ven reports delete

Delete a report.

```text
openadrctl ven reports delete <id> [flags]
```

Alias: `rm`.

| Flag | Type | Default | Meaning |
|------|------|---------|---------|
| `--yes` | bool |  | Confirm the deletion |

Plus the [connection flags](#connection-flags).

### openadrctl ven reports get

Read a report by its ID.

```text
openadrctl ven reports get <id> [flags]
```

Flags: the [connection flags](#connection-flags).

### openadrctl ven reports list

Search the reports.

```text
openadrctl ven reports list [flags]
```

Alias: `ls`.

| Flag | Type | Default | Meaning |
|------|------|---------|---------|
| `--client-name` | string |  | Reports with this client name |
| `--event` | string |  | Reports on this event |
| `--limit` | int |  | Ask for at most this many objects, 50 at most (one page only); default: all, page by page |
| `--program` | string |  | Reports on events of this program |
| `--skip` | int |  | Skip this many objects (one page only) |

Plus the [connection flags](#connection-flags).

### openadrctl ven reports update

Change a report: read it, apply --file and the flags, send it back.

```text
openadrctl ven reports update <id> [flags]
```

| Flag | Type | Default | Meaning |
|------|------|---------|---------|
| `--client-name` | string |  | Name of the reporting client |
| `--csv` | string |  | Intervals from a timeline CSV (interval,start,end,type,values; see openadrctl timeline) |
| `--dry-run` | bool |  | Validate and print the object that would be sent; send nothing |
| `--event` | string |  | ID of the event the report is on |
| `--file` | string |  | The object as JSON (of the revision of --spec), - for stdin; flags change what it says |
| `--interval` | string |  | Length of each interval: PT15M, 1h, ... |
| `--name` | string |  | Name of the report |
| `--reading-type` | string |  | Reading type of the payload, such as DIRECT_READ |
| `--resource` | string | AGGREGATED_REPORT | Name of the resource the intervals are of |
| `--start` | string |  | Start of the first interval: now, or RFC 3339 |
| `--type` | string |  | Payload type of the readings, such as PRICE, SIMPLE or USAGE |
| `--units` | string |  | Units of the payload, such as KWH |
| `--value` | strings |  | A value per interval, in order (repeatable; a decimal number, true/false, x:y or text) |

Plus the [connection flags](#connection-flags).

### openadrctl ven resource-groups

Resource groups are not part of OpenADR 3.1.0: they are an extension of OpenLEADR that.

Alias: `groups`, `resource-group`.

Subcommands: `get`, `list`.

### openadrctl ven resource-groups get

Read a resource group by its ID.

```text
openadrctl ven resource-groups get <id> [flags]
```

Flags: the [connection flags](#connection-flags).

### openadrctl ven resource-groups list

Search the resource groups.

```text
openadrctl ven resource-groups list [flags]
```

Alias: `ls`.

| Flag | Type | Default | Meaning |
|------|------|---------|---------|
| `--limit` | int |  | Ask for at most this many objects, 50 at most (one page only); default: all, page by page |
| `--name` | string |  | Groups of this name |
| `--skip` | int |  | Skip this many objects (one page only) |
| `--target` | strings |  | Groups with one of these targets (repeatable) |

Plus the [connection flags](#connection-flags).

### openadrctl ven resources

Resources: the devices behind a VEN.

Alias: `resource`.

Subcommands: `create`, `delete`, `get`, `list`, `update`.

### openadrctl ven resources create

Create a resource from flags or a JSON file.

```text
openadrctl ven resources create [flags]
```

| Flag | Type | Default | Meaning |
|------|------|---------|---------|
| `--attribute` | stringArray |  | Attribute as TYPE=value[,value...] (repeatable) |
| `--dry-run` | bool |  | Validate and print the object that would be sent; send nothing |
| `--file` | string |  | The object as JSON (of the revision of --spec), - for stdin; flags change what it says |
| `--name` | string |  | Name of the resource (unique within its VEN) |
| `--ven` | string |  | ID of the VEN of the resource |

Plus the [connection flags](#connection-flags).

### openadrctl ven resources delete

Delete a resource.

```text
openadrctl ven resources delete <id> [flags]
```

Alias: `rm`.

| Flag | Type | Default | Meaning |
|------|------|---------|---------|
| `--ven` | string |  | ID of the VEN of the resource: required with --spec 3.0.1, which reaches a resource through its VEN |
| `--yes` | bool |  | Confirm the deletion |

Plus the [connection flags](#connection-flags).

### openadrctl ven resources get

Read a resource by its ID.

```text
openadrctl ven resources get <id> [flags]
```

| Flag | Type | Default | Meaning |
|------|------|---------|---------|
| `--ven` | string |  | ID of the VEN of the resource: required with --spec 3.0.1, which reaches a resource through its VEN |

Plus the [connection flags](#connection-flags).

### openadrctl ven resources list

Search the resources.

```text
openadrctl ven resources list [flags]
```

Alias: `ls`.

| Flag | Type | Default | Meaning |
|------|------|---------|---------|
| `--limit` | int |  | Ask for at most this many objects, 50 at most (one page only); default: all, page by page |
| `--name` | string |  | Resources of this name |
| `--skip` | int |  | Skip this many objects (one page only) |
| `--target` | strings |  | Resources with one of these targets (repeatable) |
| `--ven` | string |  | Resources of this VEN |

Plus the [connection flags](#connection-flags).

### openadrctl ven resources update

Change a resource: read it, apply --file and the flags, send it back.

```text
openadrctl ven resources update <id> [flags]
```

| Flag | Type | Default | Meaning |
|------|------|---------|---------|
| `--attribute` | stringArray |  | Attribute as TYPE=value[,value...] (repeatable) |
| `--dry-run` | bool |  | Validate and print the object that would be sent; send nothing |
| `--file` | string |  | The object as JSON (of the revision of --spec), - for stdin; flags change what it says |
| `--name` | string |  | Name of the resource (unique within its VEN) |
| `--ven` | string |  | ID of the VEN of the resource |

Plus the [connection flags](#connection-flags).

### openadrctl ven subscriptions

Subscriptions: what the VTN notifies a client of, and where.

Alias: `subscription`, `subs`.

Subcommands: `create`, `delete`, `get`, `list`, `update`.

### openadrctl ven subscriptions create

Create a subscription from flags or a JSON file.

```text
openadrctl ven subscriptions create [flags]
```

| Flag | Type | Default | Meaning |
|------|------|---------|---------|
| `--callback-url` | string |  | Webhook to notify (https; the VTN checks that it answers first) |
| `--client-name` | string |  | Name of the subscribing client |
| `--dry-run` | bool |  | Validate and print the object that would be sent; send nothing |
| `--file` | string |  | The object as JSON (of the revision of --spec), - for stdin; flags change what it says |
| `--object` | strings |  | Kinds of object (repeatable): PROGRAM, EVENT, REPORT, SUBSCRIPTION, VEN, RESOURCE (default: EVENT) |
| `--operation` | strings |  | Operations (repeatable): CREATE, UPDATE, DELETE, READ (default: CREATE, UPDATE, DELETE) |
| `--program` | string |  | Only this program, its events and the reports on them |
| `--target` | strings |  | Targets the subscriber names, as in a search (repeatable) |
| `--webhook-token` | string |  | Bearer token the VTN sends to the webhook, made up for the subscription |
| `--websocket` | bool |  | Notify over the client's WebSocket instead of a webhook (extension) |

Plus the [connection flags](#connection-flags).

### openadrctl ven subscriptions delete

Delete a subscription.

```text
openadrctl ven subscriptions delete <id> [flags]
```

Alias: `rm`.

| Flag | Type | Default | Meaning |
|------|------|---------|---------|
| `--yes` | bool |  | Confirm the deletion |

Plus the [connection flags](#connection-flags).

### openadrctl ven subscriptions get

Read a subscription by its ID.

```text
openadrctl ven subscriptions get <id> [flags]
```

Flags: the [connection flags](#connection-flags).

### openadrctl ven subscriptions list

Search the subscriptions.

```text
openadrctl ven subscriptions list [flags]
```

Alias: `ls`.

| Flag | Type | Default | Meaning |
|------|------|---------|---------|
| `--client-name` | string |  | Subscriptions of this client name |
| `--limit` | int |  | Ask for at most this many objects, 50 at most (one page only); default: all, page by page |
| `--object` | strings |  | Subscriptions to one of these kinds of object (repeatable): PROGRAM, EVENT, ... |
| `--program` | string |  | Subscriptions to this program |
| `--skip` | int |  | Skip this many objects (one page only) |

Plus the [connection flags](#connection-flags).

### openadrctl ven subscriptions update

Change a subscription: read it, apply --file and the flags, send it back.

```text
openadrctl ven subscriptions update <id> [flags]
```

| Flag | Type | Default | Meaning |
|------|------|---------|---------|
| `--callback-url` | string |  | Webhook to notify (https; the VTN checks that it answers first) |
| `--client-name` | string |  | Name of the subscribing client |
| `--dry-run` | bool |  | Validate and print the object that would be sent; send nothing |
| `--file` | string |  | The object as JSON (of the revision of --spec), - for stdin; flags change what it says |
| `--object` | strings |  | Kinds of object (repeatable): PROGRAM, EVENT, REPORT, SUBSCRIPTION, VEN, RESOURCE (default: EVENT) |
| `--operation` | strings |  | Operations (repeatable): CREATE, UPDATE, DELETE, READ (default: CREATE, UPDATE, DELETE) |
| `--program` | string |  | Only this program, its events and the reports on them |
| `--target` | strings |  | Targets the subscriber names, as in a search (repeatable) |
| `--webhook-token` | string |  | Bearer token the VTN sends to the webhook, made up for the subscription |
| `--websocket` | bool |  | Notify over the client's WebSocket instead of a webhook (extension) |

Plus the [connection flags](#connection-flags).

### openadrctl ven topics

Show the MQTT topics of a set of objects.

```text
openadrctl ven topics <set> [flags]
```

Flags: the [connection flags](#connection-flags).

### openadrctl ven vens

VENs: the VEN objects the VTN knows.

Alias: `ven`.

Subcommands: `create`, `delete`, `get`, `list`, `update`.

### openadrctl ven vens create

Create a VEN from flags or a JSON file.

```text
openadrctl ven vens create [flags]
```

| Flag | Type | Default | Meaning |
|------|------|---------|---------|
| `--attribute` | stringArray |  | Attribute as TYPE=value[,value...] (repeatable) |
| `--dry-run` | bool |  | Validate and print the object that would be sent; send nothing |
| `--file` | string |  | The object as JSON (of the revision of --spec), - for stdin; flags change what it says |
| `--name` | string |  | Name of the VEN (unique on the VTN) |

Plus the [connection flags](#connection-flags).

### openadrctl ven vens delete

Delete a VEN.

```text
openadrctl ven vens delete <id> [flags]
```

Alias: `rm`.

| Flag | Type | Default | Meaning |
|------|------|---------|---------|
| `--yes` | bool |  | Confirm the deletion |

Plus the [connection flags](#connection-flags).

### openadrctl ven vens get

Read a VEN by its ID.

```text
openadrctl ven vens get <id> [flags]
```

Flags: the [connection flags](#connection-flags).

### openadrctl ven vens list

Search the vens.

```text
openadrctl ven vens list [flags]
```

Alias: `ls`.

| Flag | Type | Default | Meaning |
|------|------|---------|---------|
| `--limit` | int |  | Ask for at most this many objects, 50 at most (one page only); default: all, page by page |
| `--name` | string |  | The VEN of this name |
| `--skip` | int |  | Skip this many objects (one page only) |
| `--target` | strings |  | VENs with one of these targets (repeatable) |

Plus the [connection flags](#connection-flags).

### openadrctl ven vens update

Change a VEN: read it, apply --file and the flags, send it back.

```text
openadrctl ven vens update <id> [flags]
```

| Flag | Type | Default | Meaning |
|------|------|---------|---------|
| `--attribute` | stringArray |  | Attribute as TYPE=value[,value...] (repeatable) |
| `--dry-run` | bool |  | Validate and print the object that would be sent; send nothing |
| `--file` | string |  | The object as JSON (of the revision of --spec), - for stdin; flags change what it says |
| `--name` | string |  | Name of the VEN (unique on the VTN) |

Plus the [connection flags](#connection-flags).

### openadrctl version

Print the version of openadrctl.

```text
openadrctl version [flags]
```

No flags of its own.

### openadrctl vtn

Serve OpenADR 3.1.0 as a VTN.

Subcommands: `start`.

### openadrctl vtn generate-config

Write the example seed of vtn start, with comments: two clients, two programs, a VEN, a resource, an event and a resource group. docs/server-config.md describes every field.

```text
openadrctl vtn generate-config [flags]
```

| Flag | Type | Default | Meaning |
|------|------|---------|---------|
| `--force` | bool |  | Overwrite --output |
| `--output` | string |  | Write to this file instead of stdout |

### openadrctl vtn start

Serve the OpenADR 3.1.0 API, seeded from a file.

```text
openadrctl vtn start [flags]
```

| Flag | Type | Default | Meaning |
|------|------|---------|---------|
| `--advertise` | string |  | Announce the VTN on the local network with DNS-SD under this instance name |
| `--anonymous` | bool |  | Answer requests without a token, for programs and events without targets |
| `--config` | string |  | Seed file (YAML or JSON): clients and objects; see vtn generate-config (default: the example seed) [env: OPENADRCTL_VTN_CONFIG] |
| `--duration` | duration |  | Stop after this long (0: until interrupted) |
| `--format` | string | text | Output format: text, json or table [env: OPENADRCTL_FORMAT] |
| `--insecure-webhooks` | bool |  | As --webhooks, and allow http and private addresses as callback URLs (local tests) |
| `--listen` | string | 127.0.0.1:8080 | Address to listen on (port 0: any free port) [env: OPENADRCTL_VTN_LISTEN] |
| `--log-requests` | bool |  | Write a record for every request |
| `--mqtt` | string |  | MQTT broker to publish notifications to, such as mqtt://127.0.0.1:1883 [env: OPENADRCTL_VTN_MQTT] |
| `--mqtt-client-auth` | string |  | How clients log in to the broker: anonymous, or token (their OAuth2 token as the password, checked by the broker at POST /auth/mqtt) [env: OPENADRCTL_VTN_MQTT_CLIENT_AUTH] |
| `--mqtt-public-url` | string |  | URL of the broker as clients reach it (default: --mqtt) [env: OPENADRCTL_VTN_MQTT_PUBLIC_URL] |
| `--mqtt-username` | string |  | User name at the broker [env: OPENADRCTL_VTN_MQTT_USERNAME] |
| `--notify-reads` | bool |  | Notify subscribers of READ when an object is read |
| `--public-url` | string |  | URL under which clients reach the API (default: http(s)://<listen>) [env: OPENADRCTL_VTN_PUBLIC_URL] |
| `--rate` | float |  | Requests a second taken from each client, twice as many at once; the rest are answered 429 (0: no limit) |
| `--resource-groups` | bool |  | Serve resource groups (extension, not OpenADR 3.1.0) |
| `--store` | string |  | File to keep the objects in across restarts (default: in memory) [env: OPENADRCTL_VTN_STORE] |
| `--tls-cert` | string |  | Serve https with this certificate (PEM) [env: OPENADRCTL_VTN_TLS_CERT] |
| `--tls-key` | string |  | Private key of --tls-cert (PEM) [env: OPENADRCTL_VTN_TLS_KEY] |
| `--token-ttl` | duration | 1h0m0s | Lifetime of the tokens the VTN issues |
| `--webhooks` | bool |  | Serve subscriptions with callback URLs, and notify them (https, public addresses) |
| `--websocket` | bool |  | Notify over WebSockets (extension, not OpenADR 3.1.0) |

