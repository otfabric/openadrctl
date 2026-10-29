# openadrctl guide: common scenarios

Each scenario shows the commands and what they print. The output is real:
recorded against openadrctl's own VTN (`vtn start` with its example seed)
and, where noted, against the reference VTNs of
[openadr-interop](https://github.com/otfabric/openadr-interop): OpenLEADR
v0.2.7 of its v0.4.0 (section 11), and OpenLEADR v0.1.4 for OpenADR 3.0.1,
built from the source of its v0.5.0 (section 12; `scripts/interop301.sh`
runs the same commands against the published image).
Identifiers, times and durations differ on every run.

1. [A VTN for development](#1-a-vtn-for-development)
2. [Check a VTN: ping and token](#2-check-a-vtn-ping-and-token)
3. [Read as a VEN: programs, events, timelines](#3-read-as-a-ven-programs-events-timelines)
4. [Publish prices and limits as Business Logic](#4-publish-prices-and-limits-as-business-logic)
5. [Change an object from a file](#5-change-an-object-from-a-file)
6. [Reports](#6-reports)
7. [VENs, resources, resource groups and targets](#7-vens-resources-resource-groups-and-targets)
8. [Notifications: WebSocket, webhook, MQTT](#8-notifications-websocket-webhook-mqtt)
9. [Polling: events watch](#9-polling-events-watch)
10. [A VTN with TLS, a store, MQTT and discovery](#10-a-vtn-with-tls-a-store-mqtt-and-discovery)
11. [Against another VTN: OpenLEADR](#11-against-another-vtn-openleadr)
12. [OpenADR 3.0.1: OpenLEADR v0.1.4](#12-openadr-301-openleadr-v014)
13. [Scripting: JSON and exit codes](#13-scripting-json-and-exit-codes)
14. [Troubleshooting](#14-troubleshooting)

---

## 1. A VTN for development

`vtn start` serves OpenADR 3.1.0 from a seed file. Without `--config` it
serves the example seed: a Business Logic client `utility`, a VEN client
`site-42`, two programs, the VEN object of site-42 with a meter, and a
day-ahead price event (the whole file: `openadrctl vtn generate-config`).

```console
$ openadrctl vtn start --insecure-webhooks --websocket --resource-groups
2026-10-28T23:47:52Z  listening      http://127.0.0.1:8080 on 127.0.0.1:8080
2026-10-28T23:47:52Z  client         utility (bl)
2026-10-28T23:47:52Z  client         site-42 (ven)
2026-10-28T23:47:52Z  seeded         6 objects
```

`--insecure-webhooks` lets the VTN call back to `http://127.0.0.1`, which a
production VTN refuses; `--websocket` and `--resource-groups` turn on two
extensions. The VTN runs until Ctrl-C. Its seed, flags and records are in
[server-config.md](server-config.md).

The client commands below take the VTN and the client from the environment:

```bash
export OPENADRCTL_URL=http://127.0.0.1:8080
export OPENADRCTL_CLIENT_ID=site-42 OPENADRCTL_CLIENT_SECRET=site-42-secret     # the VEN
```

Business Logic commands use `OPENADRCTL_CLIENT_ID=utility` and
`OPENADRCTL_CLIENT_SECRET=utility-secret`. In a real deployment, put the
secret in a file and give `--client-secret-file`: a secret is never a flag.

## 2. Check a VTN: ping and token

`ping` runs every step a client needs, and stops at the first that fails:

```console
$ openadrctl ping
connect    ok           0ms  127.0.0.1:8080
tls        skipped      0ms  plain http
token      ok           1ms  from http://127.0.0.1:8080/auth/token, valid 1h0m0s
api        ok           0ms  GET /programs: dynamic-price, ...
notifiers  ok           0ms  webhook, websocket
http://127.0.0.1:8080 answers
```

Without credentials no token is sent, and this VTN wants one:

```console
$ OPENADRCTL_CLIENT_ID= OPENADRCTL_CLIENT_SECRET= openadrctl ping
connect    ok           0ms  127.0.0.1:8080
tls        skipped      0ms  plain http
token      skipped      0ms  anonymous
api        failed       1ms  GET /programs: the VTN answered 401 Unauthorized: a bearer token is required (give --client-id and a secret, or a token)
notifiers  skipped      0ms
http://127.0.0.1:8080 does not answer as it should
$ echo $?
6
```

`token` fetches a token the way every command does: the token endpoint comes
from `GET /auth/server` (or `--token-url`). Its text form is the token alone,
for `curl -H "Authorization: Bearer $(openadrctl token)"`; JSON shows the
scopes:

```console
$ openadrctl token --format json
{
  "access_token": "b902ee8c525766da9dc7739feadc80195feae8284d65ec3427a40810a85a49b8",
  "token_type": "Bearer",
  "expires_in": 3600,
  "scope": "read_all write_programs write_events write_subscriptions write_vens",
  "token_url": "http://127.0.0.1:8080/auth/token",
  "expires": "2026-10-29T00:47:53Z"
}
```

## 3. Read as a VEN: programs, events, timelines

A VEN sees the programs without targets, and those whose targets it names:

```console
$ openadrctl ven programs list
ID             NAME           TARGETS  PAYLOADS
--             ----           -------  --------
dynamic-price  dynamic-price  -        PRICE

$ openadrctl ven programs list --target group-a
ID            NAME          TARGETS  PAYLOADS
--            ----          -------  --------
flex-group-a  flex-group-a  group-a  -
```

Every list fetches every page; `--limit` and `--skip` fetch one. An event,
in text and as OpenADR has it:

```console
$ openadrctl ven events get price-2030-01-01
id:               price-2030-01-01
name:             day-ahead 2030-01-01
program:          dynamic-price
priority:         -
start:            2030-01-01T00:00:00Z
intervals:        3
types:            PRICE
targets:          -
interval period:  start 2030-01-01T00:00:00Z, duration PT1H
duration:         -
reports asked:    USAGE
created:          2026-10-28T23:47:52Z
modified:         2026-10-28T23:47:52Z

$ openadrctl ven events get price-2030-01-01 --format json
{
  "id": "price-2030-01-01",
  "createdDateTime": "2026-10-28T23:47:52Z",
  "modificationDateTime": "2026-10-28T23:47:52Z",
  "objectType": "EVENT",
  "programID": "dynamic-price",
  "eventName": "day-ahead 2030-01-01",
  "reportDescriptors": [
    {
      "payloadType": "USAGE",
      "readingType": "DIRECT_READ",
      "units": "KWH",
      "aggregate": false,
      "startInterval": -1,
      "numIntervals": -1,
      "historical": true,
      "frequency": -1,
      "repeat": 1,
      "reportIntervals": "INTERVALS"
    }
  ],
  "intervalPeriod": {
    "start": "2030-01-01T00:00:00Z",
    "duration": "PT1H"
  },
  "intervals": [
    { "id": 0, "payloads": [ { "type": "PRICE", "values": [ 0.17 ] } ] },
    { "id": 1, "payloads": [ { "type": "PRICE", "values": [ 0.21 ] } ] },
    { "id": 2, "payloads": [ { "type": "PRICE", "values": [ 0.25 ] } ] }
  ]
}
```

(The intervals are folded here; openadrctl indents every level.)

An event's intervals are relative: each has an ID, and the period of the event
or its own. `timeline` works out when each one is in force:

```console
$ openadrctl ven events timeline price-2030-01-01
INTERVAL  START                 END                   TYPE   VALUES
--------  -----                 ---                   ----   ------
0         2030-01-01T00:00:00Z  2030-01-01T01:00:00Z  PRICE  0.17
1         2030-01-01T01:00:00Z  2030-01-01T02:00:00Z  PRICE  0.21
2         2030-01-01T02:00:00Z  2030-01-01T03:00:00Z  PRICE  0.25

$ openadrctl ven events timeline price-2030-01-01 --csv
interval,sub,start,end,type,values,randomize_start
0,0,2030-01-01T00:00:00Z,2030-01-01T01:00:00Z,PRICE,0.17,
1,0,2030-01-01T01:00:00Z,2030-01-01T02:00:00Z,PRICE,0.21,
2,0,2030-01-01T02:00:00Z,2030-01-01T03:00:00Z,PRICE,0.25,
```

`--at <time>` keeps the rows in force at a time, which answers "what is the
price now?"; `--sub-intervals` splits a payload with several values. The same
works offline on a file:

```console
$ openadrctl ven events get price-2030-01-01 --format json > event.json
$ openadrctl timeline event.json --at 2030-01-01T01:30:00Z
INTERVAL  START                 END                   TYPE   VALUES
--------  -----                 ---                   ----   ------
1         2030-01-01T01:00:00Z  2030-01-01T02:00:00Z  PRICE  0.21
```

## 4. Publish prices and limits as Business Logic

From here, as Business Logic:

```bash
export OPENADRCTL_CLIENT_ID=utility OPENADRCTL_CLIENT_SECRET=utility-secret
```

An event from flags: one `--type`, a `--value` per interval, the start and
length of the intervals. VENs act on events, so creating, changing and
deleting one needs `--yes`:

```console
$ openadrctl bl events create --program dynamic-price --name evening-peak --type PRICE \
    --start 2030-01-01T17:00:00Z --interval PT1H --value 0.35 --value 0.42 --yes
created event 5e80b71cdd49704f1f080c225570d7f6 (evening-peak)

$ openadrctl bl events create --program dynamic-price --name evening-peak --type PRICE --value 0.35
error: events create changes the VTN: confirm with --yes
$ echo $?
2
```

A price curve from a CSV file, in the form `timeline --csv` writes, asking
VENs for a `USAGE` report:

```console
$ cat prices.csv
interval,sub,start,end,type,values,randomize_start
0,0,2030-01-02T00:00:00Z,2030-01-02T01:00:00Z,PRICE,0.18,
1,0,2030-01-02T01:00:00Z,2030-01-02T02:00:00Z,PRICE,0.16,
2,0,2030-01-02T02:00:00Z,2030-01-02T03:00:00Z,PRICE,0.14,

$ openadrctl bl events create --program dynamic-price --name day-ahead-2030-01-02 \
    --csv prices.csv --report USAGE --yes
created event fd1f60c34f90c3742faaec298e8b2cda (day-ahead-2030-01-02)
```

A load limit for the VENs of `group-a`, checked with `--dry-run` first: the
object is built and validated, and nothing is sent (a dry-run create needs no
VTN at all):

```console
$ openadrctl bl events create --program flex-group-a --name evening-limit --target group-a \
    --priority 1 --type IMPORT_CAPACITY_LIMIT --start 2030-01-01T17:00:00Z --interval PT2H \
    --value 5 --dry-run
name:             evening-limit
program:          flex-group-a
priority:         1
start:            2030-01-01T17:00:00Z
intervals:        1
types:            IMPORT_CAPACITY_LIMIT
targets:          group-a
interval period:  start 2030-01-01T17:00:00Z, duration PT2H
duration:         -
reports asked:    -
```

A program, with a payload descriptor (`TYPE:units:currency`) and a link to
its terms:

```console
$ openadrctl bl programs create --name ev-charging --target group-a \
    --payload IMPORT_CAPACITY_LIMIT:KW: --description-url https://utility.example/ev
created program 74f19caab905ec9eac9d85074a28c081 (ev-charging)
```

An object that breaks the schema is not sent, and the error names the
property at fault:

```console
$ openadrctl bl events create --program 'not an id' --type PRICE --value 1 --dry-run --format json
{
  "error": {
    "kind": "invalid_input",
    "code": "invalid_object",
    "message": "invalid programID: is not an object ID (1 to 128 characters of [a-zA-Z0-9_-]) (nothing was sent)"
  },
  "openadr": {
    "field": "programID"
  }
}
$ echo $?
3
```

## 5. Change an object from a file

`get --format json` writes an object as OpenADR has it; `update --file`
sends it back. The file replaces the whole object, and flags apply on top of
it; without `--file`, `update <id> --name x` changes only the name and keeps
everything else the VTN has.

```console
$ openadrctl bl programs get dynamic-price --format json > program.json
$ sed -i 's/"programName": "dynamic-price"/"programName": "dynamic-price-2030"/' program.json
$ openadrctl bl programs update dynamic-price --file program.json
updated program dynamic-price (dynamic-price-2030)
```

A misspelt property is an error, not a silent loss:

```console
$ echo '{"objectType":"PROGRAM","programName":"x","programname":"y"}' | openadrctl validate program -
error: -: not a property of OpenADR 3.1.0: "programname"
$ echo $?
3
```

## 6. Reports

A VEN reports on an event, a value per interval, for one of its resources:

```console
$ openadrctl ven reports create --event price-2030-01-01 --client-name site-42 --resource meter-1 \
    --type USAGE --units KWH --start 2030-01-01T00:00:00Z --interval PT1H \
    --value 1.2 --value 0.9 --value 1.4
created report 0217c7c22a5b4652cd4e511151641f70
```

Business Logic reads them:

```console
$ openadrctl bl reports list --event price-2030-01-01
ID                                NAME  EVENT             CLIENT_NAME  RESOURCES
--                                ----  -----             -----------  ---------
0217c7c22a5b4652cd4e511151641f70  -     price-2030-01-01  site-42      meter-1

$ openadrctl bl reports get 0217c7c22a5b4652cd4e511151641f70
id:                0217c7c22a5b4652cd4e511151641f70
name:              -
event:             price-2030-01-01
client_name:       site-42
resources:         meter-1
client id:         site-42
resource meter-1:  3 intervals
created:           2026-10-28T23:59:35Z
modified:          2026-10-28T23:59:35Z
```

`--format json` has every value.

## 7. VENs, resources, resource groups and targets

Business Logic registers a VEN with the client ID it authenticates as, and
grants it targets:

```console
$ openadrctl bl vens create --name site-43 --ven-client-id site-43 --target group-b
created VEN c298adeb5d2a28f0f9ab76c568247d1c (site-43)

$ openadrctl bl vens update site-42 --target group-a --target group-b
updated VEN site-42 (site-42)
```

A VEN manages the resources behind it:

```console
$ openadrctl ven resources create --name battery-1 --ven site-42 --attribute MAX_POWER_CONSUMPTION=5
created resource 273777690777d2b787770e7369ac2595 (battery-1)

$ openadrctl ven resources list
ID                                NAME       VEN      CLIENT_ID  TARGETS
--                                ----       ---      ---------  -------
site-42-meter                     meter-1    site-42  site-42    -
273777690777d2b787770e7369ac2595  battery-1  site-42  site-42    -
```

Resource groups (an extension, `vtn start --resource-groups`) name resources
and other groups:

```console
$ openadrctl bl resource-groups create --name feeder-8 --child resource:site-42-meter
created resource group 5528d863e8b0eacc78e51b64deb105bf (feeder-8)

$ openadrctl bl resource-groups list
ID                                NAME      CHILDREN                    TARGETS
--                                ----      --------                    -------
feeder-7                          feeder-7  ven_resource:site-42-meter  -
5528d863e8b0eacc78e51b64deb105bf  feeder-8  ven_resource:site-42-meter  -
```

The VTN decides what each client may do by its token's scopes. A VEN's
credentials on a Business Logic command:

```console
$ OPENADRCTL_CLIENT_ID=site-42 OPENADRCTL_CLIENT_SECRET=site-42-secret openadrctl bl programs create --name mine
error: POST /programs: the VTN answered 403 Forbidden: the token lacks the scope write_programs
```

## 8. Notifications: WebSocket, webhook, MQTT

`notifiers` tells how a VTN notifies. This one was started with
`--insecure-webhooks --websocket --mqtt mqtt://127.0.0.1:1883`:

```console
$ openadrctl bl notifiers
webhook:    yes
websocket:  yes
mqtt:       mqtt://127.0.0.1:1883
  auth:     ANONYMOUS
```

`listen` subscribes, prints one record per notification, and deletes its
subscription when it ends (`--count`, `--duration`, Ctrl-C), unless `--keep`.
Progress goes to stderr, records to stdout.

**WebSocket** (an extension of OpenLEADR and go-openadr): the client connects
out to the VTN, so it works behind NAT.

```console
$ openadrctl ven listen --via websocket --object EVENT --count 1
subscribed: subscription c4ddda8c8362de1d492bfbc319bdc1bb, CREATE,UPDATE,DELETE of EVENT
listening: WebSocket open
2026-10-28T23:49:30Z  websocket EVENT        CREATE 81dd06eb4a42f2509a14e5c8296e59df (late)
unsubscribed: subscription c4ddda8c8362de1d492bfbc319bdc1bb
```

**Webhook**: openadrctl serves the callback on `--listen` and subscribes with
`--callback-url` (by default `http://<listen>/`). The VTN must reach it; a
production VTN wants https on a public address (`--tls-cert`, `--tls-key`).

```console
$ openadrctl ven listen --via webhook --listen 127.0.0.1:8081 --object EVENT --count 1
listening: webhook on 127.0.0.1:8081, callback URL http://127.0.0.1:8081/
subscribed: subscription f4c81a29c57c8e62ce395c1f18a6436b, CREATE,UPDATE,DELETE of EVENT
2026-10-29T00:00:10Z  webhook   EVENT        CREATE 706998eeb3a8de16f128c0642644b3b3 (hook-test)
unsubscribed: subscription f4c81a29c57c8e62ce395c1f18a6436b
```

A VTN checks a callback URL before it accepts a subscription: one it cannot
reach is refused.

```console
$ openadrctl bl subscriptions create --client-name dispatch --object EVENT --object REPORT \
    --operation CREATE --callback-url http://127.0.0.1:9000/notify --webhook-token s3cret
error: POST /subscriptions: the VTN answered 400 Bad Request: objectOperations[0].callbackUrl could not be reached for verification
```

**MQTT**: openadrctl reads the broker from `GET /notifiers` and the topics of
`--topics` from the VTN, and subscribes at the broker; no OpenADR
subscription is made. The topic sets: `programs`, `events`, `reports`,
`subscriptions`, `vens`, `resources`, and with an ID `program:<id>`,
`program-events:<id>`, `ven:<id>`, `ven-events:<id>`, `ven-programs:<id>`,
`ven-resources:<id>`. A VEN may only ask for those of its own VEN object:

```console
$ openadrctl ven topics ven-events:site-42
create: vens/site-42/events/create
update: vens/site-42/events/update
delete: vens/site-42/events/delete
all:    vens/site-42/events/+

$ openadrctl bl listen --via mqtt --topics events --count 1 --format json
listening: mqtt://127.0.0.1:1883, events/+
{"time":"2026-10-29T00:00:14.514206Z","via":"mqtt","topic":"events/create","objectType":"EVENT","operation":"CREATE","id":"2bac12b055e83779680b0fa38e257a51","name":"mqtt-json","object":{"id":"2bac12b055e83779680b0fa38e257a51","createdDateTime":"2026-10-29T00:00:14.513151Z","modificationDateTime":"2026-10-29T00:00:14.513151Z","objectType":"EVENT","programID":"dynamic-price","eventName":"mqtt-json","intervalPeriod":{"start":"2030-01-06T00:00:00Z","duration":"PT1H"},"intervals":[{"id":0,"payloads":[{"type":"PRICE","values":[0.2]}]}]}}
```

When the VTN announces `OAUTH2_BEARER_TOKEN` at its broker, openadrctl logs in
with its client ID and its token; `--mqtt-username` and `--mqtt-password-file`
(or `OPENADRCTL_MQTT_PASSWORD`) override that, and `--broker` overrides the URL
(useful when the VTN announces a name only its own network resolves). With
`vtn start --mqtt-client-auth token` and a broker that checks tokens (EMQX,
from `scripts/mqtt-token.sh`):

```console
$ openadrctl bl notifiers
webhook:    no
websocket:  no
mqtt:       mqtt://127.0.0.1:11883
  auth:     OAUTH2_BEARER_TOKEN
  username: {clientID}

$ OPENADRCTL_CLIENT_ID=site-42 OPENADRCTL_CLIENT_SECRET=site-42-secret \
    openadrctl ven listen --via mqtt --topics ven-events:site-42 --count 1
listening: mqtt://127.0.0.1:11883, vens/site-42/events/+
2026-10-29T00:28:10Z  mqtt      EVENT        CREATE c1f7474e7b3b63ba50688b425792a095 (for-site-42)

$ OPENADRCTL_MQTT_PASSWORD=guess openadrctl bl listen --via mqtt --topics events --timeout 5s
error: the broker mqtt://127.0.0.1:11883: not subscribed (a login the broker refuses? see --mqtt-username and --mqtt-password-file): context deadline exceeded
$ echo $?
4
```

The VTN writes a `mqtt_login` record per check: `site-42 allowed`,
`utility denied`.

## 9. Polling: events watch

`events watch` searches every `--interval` and prints what changed. It needs
no subscription, callback or broker, and works with every VTN:

```console
$ openadrctl ven events watch --interval 1s --count 1
2026-10-28T23:49:35Z  present  event price-2030-01-01 (day-ahead 2030-01-01)  start 2030-01-01T00:00:00Z  3 intervals PRICE

$ openadrctl ven events watch --interval 1s --changes-only --count 2
2026-10-29T00:00:16Z  updated  event 706998eeb3a8de16f128c0642644b3b3 (hook-test)  start 2030-01-04T00:00:00Z  1 intervals PRICE
2026-10-29T00:00:17Z  deleted  event 706998eeb3a8de16f128c0642644b3b3 (hook-test)  start 2030-01-04T00:00:00Z  1 intervals PRICE
```

With `--format json` each record carries the whole event.

## 10. A VTN with TLS, a store, MQTT and discovery

A test certificate:

```bash
openssl req -x509 -newkey ec -pkeyopt ec_paramgen_curve:P-256 -nodes -days 30 -subj /CN=vtn.local \
  -addext "subjectAltName=DNS:localhost,IP:127.0.0.1" -keyout vtn.key -out vtn.pem
```

`--tls-cert` and `--tls-key` serve https; `--store` keeps the objects in a
file across restarts:

```console
$ openadrctl vtn start --listen 127.0.0.1:8443 --tls-cert vtn.pem --tls-key vtn.key --store vtn.db
2026-10-29T00:01:28Z  listening      https://127.0.0.1:8443 on 127.0.0.1:8443
2026-10-29T00:01:28Z  client         utility (bl)
2026-10-29T00:01:28Z  client         site-42 (ven)
2026-10-29T00:01:28Z  seeded         6 objects
```

Clients verify the VTN against the system's roots, or `--ca`:

```console
$ OPENADRCTL_URL=https://127.0.0.1:8443 openadrctl ping
connect    ok           0ms  127.0.0.1:8443
tls        failed      27ms  the TLS handshake failed: tls: failed to verify certificate: x509: certificate signed by unknown authority (--ca names the CA of the VTN)
token      skipped      0ms
api        skipped      0ms
notifiers  skipped      0ms
https://127.0.0.1:8443 does not answer as it should

$ OPENADRCTL_URL=https://127.0.0.1:8443 openadrctl ping --ca vtn.pem
connect    ok           0ms  127.0.0.1:8443
tls        ok           1ms  TLS 1.3, vtn.local
token      ok           3ms  from https://127.0.0.1:8443/auth/token, valid 1h0m0s
api        ok           0ms  GET /programs: dynamic-price, ...
notifiers  ok           0ms  none (501)
https://127.0.0.1:8443 answers
```

A program created, the VTN stopped and started again: the seed adds nothing
that is already in the store, and the program is still there.

```console
$ openadrctl bl programs create --name kept
created program 44e8b7bff31367e56fc20764ca70f2c1 (kept)

$ openadrctl vtn start --listen 127.0.0.1:8443 --tls-cert vtn.pem --tls-key vtn.key --store vtn.db
2026-10-29T00:01:35Z  listening      https://127.0.0.1:8443 on 127.0.0.1:8443
2026-10-29T00:01:35Z  client         utility (bl)
2026-10-29T00:01:35Z  client         site-42 (ven)
2026-10-29T00:01:35Z  seeded         0 objects

$ openadrctl bl programs list
ID                                NAME           TARGETS  PAYLOADS
--                                ----           -------  --------
dynamic-price                     dynamic-price  -        PRICE
flex-group-a                      flex-group-a   group-a  -
44e8b7bff31367e56fc20764ca70f2c1  kept           -        -
```

**MQTT**: `--mqtt` names a broker of your own (Mosquitto, NanoMQ, EMQX, …);
the VTN publishes every change on it and announces it in `GET /notifiers`.
`--mqtt-public-url` is the broker's URL as clients reach it, when that
differs.

```bash
openadrctl vtn start --mqtt mqtt://127.0.0.1:1883 --insecure-webhooks --websocket
```

**Discovery**: `--advertise` announces the VTN with DNS-SD
(`_openadr3._tcp`), and `discover` finds it on the local network
(host name shortened here):

```console
$ openadrctl vtn start --listen 127.0.0.1:8090 --advertise "Test VTN"
...
2026-10-29T00:01:45Z  advertised     _openadr3._tcp Test VTN

$ openadrctl discover
INSTANCE  URL                       VERSION  AUTH   ADDRESSES  PROGRAMS
--------  ---                       -------  ----   ---------  --------
Test VTN  http://myhost.local:8090  3.1.0    token  127.0.0.1  -
```

## 11. Against another VTN: OpenLEADR

The OpenLEADR VTN of openadr-interop, with its baseline fixture (its client
secrets are public, which is why its ports stay on loopback):

```bash
docker run -d --rm --name openleadr -p 127.0.0.1:18090:8080 -p 127.0.0.1:18883:1883 \
  ghcr.io/otfabric/openadr-interop-openleadr:v0.4.0 \
  server --fixture /fixtures/baseline/fixture.json --public-url http://127.0.0.1:18090
export OPENADRCTL_URL=http://127.0.0.1:18090
export OPENADRCTL_CLIENT_ID=interop-bl OPENADRCTL_CLIENT_SECRET=interop-bl-secret
```

```console
$ openadrctl ping
connect    ok           0ms  127.0.0.1:18090
tls        skipped      0ms  plain http
token      ok          17ms  from http://127.0.0.1:18090/auth/token, valid 720h0m0s
api        ok           2ms  GET /programs: group-a-flex, ...
notifiers  ok           0ms  mqtt mqtt://06ca440489f2:1883, websocket
http://127.0.0.1:18090 answers

$ openadrctl bl events list
ID                                    NAME              PROGRAM                               PRIORITY  START                 INTERVALS  TYPES
--                                    ----              -------                               --------  -----                 ---------  -----
c1185662-07cf-49ed-a9da-644c47a77eaa  day-ahead-prices  047d6cfb-e15f-48af-a617-94751b5baf9a  1         2030-01-01T00:00:00Z  3          PRICE
c9ddb1d3-471d-4276-8a2b-052a66b401c5  group-a-limit     db9a7fed-fcd1-4797-bd60-319595ea3832  -         2030-01-01T17:00:00Z  1          IMPORT_CAPACITY_LIMIT

$ openadrctl bl notifiers
webhook:    no
websocket:  yes
mqtt:       mqtt://06ca440489f2:1883
  auth:     OAUTH2BEARERTOKEN
  username: {clientID}

$ openadrctl bl listen --via websocket --object PROGRAM --count 1
subscribed: subscription 44a3f81c-a640-4399-b1c3-b6cf46bc751d, CREATE,UPDATE,DELETE of PROGRAM
listening: WebSocket open
2026-10-28T23:51:04Z  websocket PROGRAM      CREATE 2147e26d-831d-475f-8b78-7ce50b0d525a (ws-check)
unsubscribed: subscription 44a3f81c-a640-4399-b1c3-b6cf46bc751d
```

OpenLEADR announces its broker under the container's own host name, and the
Mosquitto in the image takes the client secret as the password, so MQTT
needs `--broker`, `--mqtt-username` and the secret:

```console
$ OPENADRCTL_MQTT_PASSWORD=interop-bl-secret openadrctl bl listen --via mqtt \
    --broker mqtt://127.0.0.1:18883 --mqtt-username interop-bl --topics programs --count 1
listening: mqtt://127.0.0.1:18883, programs/+
2026-10-28T23:52:12Z  mqtt      PROGRAM      CREATE ce345b0e-2d87-4055-970b-01addda79bed (mqtt-check)
```

OpenLEADR writes the method `OAUTH2BEARERTOKEN`, where the schema has
`OAUTH2_BEARER_TOKEN`; openadrctl reads both. It accepts a subscription with
a callback URL but never posts to it: its VTN has no webhook sender. It keeps
an event that has reports: deleting it is refused with 409 until the reports
are deleted. `scripts/interop.sh` runs all of this, and
more, as 25 checks.

## 12. OpenADR 3.0.1: OpenLEADR v0.1.4

A VTN of the earlier revision, OpenADR 3.0.1: OpenLEADR v0.1.4, the last
OpenLEADR for 3.0, as the `openleadr301` image of openadr-interop serves it.
`--spec 3.0.1` (or `OPENADRCTL_SPEC`) is all that changes on the command line:

```bash
docker run -d --rm --name openleadr301 -p 127.0.0.1:18091:8080 \
  ghcr.io/otfabric/openadr-interop-openleadr301:v0.5.0 \
  server --fixture /fixtures/baseline-301/fixture.json --public-url http://127.0.0.1:18091
export OPENADRCTL_URL=http://127.0.0.1:18091 OPENADRCTL_SPEC=3.0.1
export OPENADRCTL_CLIENT_ID=interop-bl OPENADRCTL_CLIENT_SECRET=interop-bl-secret
```

The token comes from `/auth/token` (3.0.1 has no `GET /auth/server`), and
there are no notifiers to ask for:

```console
$ openadrctl ping
connect    ok           0ms  127.0.0.1:18091
tls        skipped      0ms  plain http
token      ok           7ms  from http://127.0.0.1:18091/auth/token, valid 720h0m0s
api        ok           3ms  GET /programs: group-a-flex, ...
notifiers  skipped      0ms  not in OpenADR 3.0.1
http://127.0.0.1:18091 answers
```

Targets have a type, written `TYPE=value`; resources are reached through
their VEN:

```console
$ openadrctl bl vens list
ID                                    NAME    CLIENT_ID  TARGETS
--                                    ----    ---------  -------
4ba541ca-a513-4c1d-aca0-0698447b9aae  site-2  -          -
cf0cca29-f7d7-48e1-9e34-c8710eeefe97  site-1  -          GROUP=group-a

$ openadrctl bl resources list --ven cf0cca29-f7d7-48e1-9e34-c8710eeefe97
ID                                    NAME     VEN                                   CLIENT_ID  TARGETS
--                                    ----     ---                                   ---------  -------
cb80da6b-ca52-4cf3-a64a-dedd03cdd3f2  battery  cf0cca29-f7d7-48e1-9e34-c8710eeefe97  -          -

$ openadrctl bl programs create --name evening-flex --target VEN_NAME=site-1 --target GROUP=group-a --attribute COUNTRY=NL
created program 9c0b8b3a-d5df-495e-a05e-986cb95e8af5 (evening-flex)

$ openadrctl bl programs list --target GROUP=group-a
ID                                    NAME          TARGETS                        PAYLOADS
--                                    ----          -------                        --------
9c0b8b3a-d5df-495e-a05e-986cb95e8af5  evening-flex  GROUP=group-a,VEN_NAME=site-1  -
```

`--format json` is the JSON of 3.0.1: the attribute `COUNTRY` is the
property `country` there, and targets are values maps. `--file` and
`validate --spec 3.0.1` read it back:

```console
$ openadrctl bl programs get 9c0b8b3a-d5df-495e-a05e-986cb95e8af5 --format json
{
  "id": "9c0b8b3a-d5df-495e-a05e-986cb95e8af5",
  "createdDateTime": "2026-10-29T01:57:36.233438Z",
  "modificationDateTime": "2026-10-29T01:57:36.233438Z",
  "objectType": "PROGRAM",
  "programName": "evening-flex",
  "country": "NL",
  "targets": [
    {
      "type": "GROUP",
      "values": [
        "group-a"
      ]
    },
    {
      "type": "VEN_NAME",
      "values": [
        "site-1"
      ]
    }
  ]
}
```

A 3.0.1 report names its program; openadrctl reads it from the event:

```console
$ openadrctl bl events create --program 9c0b8b3a-d5df-495e-a05e-986cb95e8af5 --name evening-limit \
    --type IMPORT_CAPACITY_LIMIT --start 2030-01-01T17:00:00Z --interval PT1H --value 5 --value 3 --yes
created event a8483314-131a-4fae-9a55-e46c5aeaba6c (evening-limit)

$ export OPENADRCTL_CLIENT_ID=interop-ven-1 OPENADRCTL_CLIENT_SECRET=interop-ven-1-secret
$ openadrctl ven reports create --event a8483314-131a-4fae-9a55-e46c5aeaba6c --client-name site-1 --type USAGE --value 2.5
created report 7f44bc98-6bc5-40f5-b3b7-d1e31ccbcd5c

$ openadrctl ven reports get 7f44bc98-6bc5-40f5-b3b7-d1e31ccbcd5c --format json | jq '{programID, eventID, clientName}'
{
  "programID": "9c0b8b3a-d5df-495e-a05e-986cb95e8af5",
  "eventID": "a8483314-131a-4fae-9a55-e46c5aeaba6c",
  "clientName": "site-1"
}

$ openadrctl ven programs list
ID                                    NAME          TARGETS        PAYLOADS
--                                    ----          -------        --------
9c0b8b3a-d5df-495e-a05e-986cb95e8af5  evening-flex  GROUP=group-a  -
70586709-98f5-4891-8eb3-4728df69f784  group-a-flex  -              -
67cbd9b6-7973-4e96-83db-ead1da601702  day-ahead     -              PRICE
```

The VEN sees the two programs targeted at `site-1`, and OpenLEADR does not
show it their `VEN_NAME` targets. What 3.0.1 does not have, or cannot say,
exits 3 before anything is sent:

```console
$ openadrctl bl programs list --target group-a
error: --target: "group-a": OpenADR 3.0.1 targets have a type: TYPE=value, such as GROUP=group-1

$ openadrctl bl resources list
error: --ven is required with --spec 3.0.1, which reaches resources through their VEN

$ openadrctl bl notifiers
error: GET /notifiers is not in OpenADR 3.0.1: unsupported operation (nothing was sent)
```

Two things OpenLEADR v0.1.4 does its own way: it searches a program's
targets but not its `VEN_NAME` ones (`--target VEN_NAME=site-1` finds
nothing), and it lets only Business Logic delete a report, where the
specification says VENs; a VEN's `reports delete` is answered 403. It has no
subscriptions. `scripts/interop301.sh` runs all of this, and more.

## 13. Scripting: JSON and exit codes

Every command with a result takes `--format json`. Lists are `{"<kind>": [...]}` with the
objects as OpenADR has them:

```console
$ openadrctl bl events list --format json | jq -r '.events[] | [.id, .eventName] | @tsv'
price-2030-01-01	day-ahead 2030-01-01
fd1f60c34f90c3742faaec298e8b2cda	day-ahead-2030-01-02
```

A failure is one JSON object on stdout, with the VTN's problem object:

```console
$ openadrctl bl programs delete dynamic-price --yes --format json
{
  "error": {
    "kind": "refused",
    "code": "conflict",
    "message": "DELETE /programs/dynamic-price: the VTN answered 409 Conflict: program dynamic-price still has events"
  },
  "openadr": {
    "method": "DELETE",
    "path": "/programs/dynamic-price",
    "status": 409,
    "problem": {
      "title": "Conflict",
      "status": 409,
      "detail": "program dynamic-price still has events"
    }
  }
}
$ echo $?
6
```

An object the schema rejects names the property:

```console
$ echo '{"objectType":"EVENT","eventName":"x","intervals":[{"id":0,"payloads":[{"type":"PRICE","values":[1]}]}]}' > bad.json
$ openadrctl validate event bad.json --format json
{
  "error": {
    "kind": "invalid_input",
    "code": "invalid_object",
    "message": "invalid programID: is required"
  },
  "openadr": {
    "field": "programID"
  }
}
$ echo $?
3
```

A pattern for scripts:

```bash
if ! out=$(openadrctl bl events create --program dynamic-price --type PRICE --value 0.3 --yes --format json); then
  case $? in
    3) echo "invalid event: $(jq -r .error.message <<<"$out")" ;;
    5) echo "no answer: list the events before sending it again" ;;
    6) echo "refused: $(jq -r '.openadr.status // .error.code' <<<"$out")" ;;
    *) echo "failed: $out" ;;
  esac
  exit 1
fi
id=$(jq -r .id <<<"$out")
```

The codes are in [exitcodes.md](exitcodes.md), the fields in
[json-output.md](json-output.md).

## 14. Troubleshooting

| Symptom | What to check |
|---------|---------------|
| `ping` fails at `connect` (exit 4) | The host and port of `--url`; a firewall; `--url` is the API's base, such as `https://vtn.example/openadr3/3.1.0` |
| `ping` fails at `tls` | `--ca` with the VTN's CA; the certificate's names must include the host of `--url` |
| `ping` fails at `token` | The client ID and secret; `--token-url` when `GET /auth/server` is missing or wrong |
| 401 at `api` | No credentials given: openadrctl sent no token |
| 403 | A scope the token lacks: a VEN's credentials on a `bl` command, or a client the VTN gave fewer scopes; `token --format json` lists them |
| A VEN sees no program or event | Targets: search with `--target`, and have Business Logic grant the target to the VEN object |
| A subscription is refused (400) | The VTN could not reach `--callback-url`, or wants https |
| `listen --via mqtt` exits 4: `not subscribed` | The broker refused the login: `--mqtt-username`, `--mqtt-password-file`; or the VTN announces a broker URL you cannot reach: `--broker` |
| A create exits 5 with `outcome_unknown` | It may have happened: list the objects before you send it again |
| `discover` finds nothing | Multicast does not cross routers or most container networks; give `--url` |

`--debug` prints the causes of an error, one `cause:` line each.
