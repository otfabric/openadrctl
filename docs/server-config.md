# The VTN: flags and seed file

`openadrctl vtn start` serves the OpenADR 3.1.0 API of a VTN with an OAuth2
token endpoint of its own, seeded from a **seed file**: the clients that may
authenticate, and the programs, VENs, resources, events and resource groups
it starts with. While it runs, Business Logic changes it with `openadrctl bl`
(or any other OpenADR 3 client); it has no administration API of its own.

It is a VTN for development, tests and demonstrations. Tokens are kept in
memory, and the objects too unless `--store` names a file.

## Starting

```console
$ openadrctl vtn start --config seed.yaml --store vtn.db --insecure-webhooks --websocket
2026-10-28T23:47:52Z  listening      http://127.0.0.1:8080 on 127.0.0.1:8080
2026-10-28T23:47:52Z  client         utility (bl)
2026-10-28T23:47:52Z  client         site-42 (ven)
2026-10-28T23:47:52Z  seeded         6 objects
```

| Flag | Default | Meaning |
|------|---------|---------|
| `--listen` | `127.0.0.1:8080` | Address to listen on (`127.0.0.1:0` picks a free port: see the `listening` record) |
| `--public-url` | `http(s)://<listen>` | The URL under which clients reach the API; the token endpoint is announced under it. Give it behind a proxy, or when listening on `0.0.0.0` |
| `--config` | the example seed | The seed file, YAML or JSON |
| `--store` | in memory | A file that keeps the objects across restarts |
| `--tls-cert`, `--tls-key` | | Serve https with this certificate and key (PEM); plain http without them |
| `--webhooks` | off | Accept subscriptions with callback URLs, and notify them; callbacks must be https on public addresses |
| `--insecure-webhooks` | off | As `--webhooks`, and allow http and private addresses (callbacks on this machine) |
| `--mqtt` | | A broker to publish notifications on (`mqtt://` or `mqtts://`); a broker of your own, not part of openadrctl |
| `--mqtt-public-url` | `--mqtt` | The broker's URL as clients reach it, announced in `GET /notifiers` |
| `--mqtt-username` | | User name at the broker; the password comes from `OPENADRCTL_VTN_MQTT_PASSWORD` |
| `--mqtt-client-auth` | `anonymous` | How clients log in to the broker: `anonymous`, or `token`: the client ID as the user name and the client's OAuth2 token as the password, which the broker checks at `POST /auth/mqtt` (see [MQTT logins by token](#mqtt-logins-by-token)) |
| `--websocket` | off | Notify over WebSockets (an extension of OpenLEADR and go-openadr) |
| `--resource-groups` | off | Serve `/resourceGroups` (an extension) |
| `--anonymous` | off | Answer requests without a token, for programs and events without targets |
| `--notify-reads` | off | Notify subscribers of `READ` when an object is read |
| `--rate` | `0` | Requests a second each client may make, twice as many at once; the rest are answered 429. 0: no limit |
| `--token-ttl` | `1h` | Lifetime of the tokens the VTN issues |
| `--advertise` | | Announce the VTN on the local network with DNS-SD (`_openadr3._tcp`) under this instance name |
| `--log-requests` | off | A `request` record per request |
| `--duration` | `0` | Stop after this long; 0: until interrupted |
| `--format` | `text` | `text`, `table` or `json` (JSON Lines) |

Most flags can come from the environment: `OPENADRCTL_VTN_LISTEN`,
`OPENADRCTL_VTN_PUBLIC_URL`, `OPENADRCTL_VTN_CONFIG`, `OPENADRCTL_VTN_STORE`,
`OPENADRCTL_VTN_TLS_CERT`, `OPENADRCTL_VTN_TLS_KEY`, `OPENADRCTL_VTN_MQTT`,
`OPENADRCTL_VTN_MQTT_PUBLIC_URL`, `OPENADRCTL_VTN_MQTT_USERNAME`,
`OPENADRCTL_VTN_MQTT_CLIENT_AUTH`,
`OPENADRCTL_FORMAT`.

The API is at the root of `--public-url`: `/programs`, `/events`,
`/reports`, `/subscriptions`, `/vens`, `/notifiers`, … The token endpoint is
`/auth/token`, announced at `GET /auth/server`.

The VTN checks the seed and its flags before it listens, reports every
problem at once on one line, and exits 3. It exits 4 when it cannot listen.
Ctrl-C (or `--duration`) stops it, after the requests in progress: exit 0.

## The seed file

`openadrctl vtn generate-config` writes the example seed (to stdout, or
`--output seed.yaml`, which it creates with mode 0600 and does not overwrite
without `--force`). Every section is optional.

```yaml
clients:
  - id: utility
    secret: utility-secret
    role: bl
  - id: site-42
    secretEnv: SITE_42_SECRET     # the secret from the environment
    role: ven

programs:
  - id: dynamic-price
    programName: dynamic-price
    payloadDescriptors:
      - objectType: EVENT_PAYLOAD_DESCRIPTOR
        payloadType: PRICE
        units: KWH
        currency: EUR
  - id: flex-group-a
    programName: flex-group-a
    targets: [group-a]            # only VENs granted group-a see it

vens:
  - id: site-42
    venName: site-42
    clientID: site-42             # the client this VEN authenticates as
    targets: [group-a]            # what Business Logic grants it

resources:
  - id: site-42-meter
    resourceName: meter-1
    venID: site-42
    clientID: site-42

events:
  - id: price-2030-01-01
    programID: dynamic-price
    eventName: day-ahead 2030-01-01
    intervalPeriod: {start: "2030-01-01T00:00:00Z", duration: PT1H}
    intervals:
      - {id: 0, payloads: [{type: PRICE, values: [0.17]}]}
      - {id: 1, payloads: [{type: PRICE, values: [0.21]}]}

resourceGroups:                   # with --resource-groups
  - id: feeder-7
    resourceGroupName: feeder-7
    children:
      - {type: ven_resource, id: site-42-meter}
```

### Clients

| Field | Meaning |
|-------|---------|
| `id` | The OAuth2 client ID |
| `secret` or `secretEnv` | The secret, or the name of an environment variable that holds it (one of the two) |
| `role` | `bl`: Business Logic, with the scopes `read_all`, `write_programs`, `write_events`, `write_subscriptions`, `write_vens`. `ven`: a VEN, with `read_targets`, `read_ven_objects`, `write_reports`, `write_subscriptions`, `write_vens` |

A client asks for fewer scopes with `--scope`. A seed without clients is
refused unless `--anonymous` is given: nobody could use the VTN.

### Objects

Objects are written as OpenADR 3.1.0 has them in JSON (`programName`,
`intervalPeriod`, `payloadDescriptors`, …), in YAML or JSON. Each is checked
as a client's would be: against the schema, and for properties OpenADR does
not have (a misspelt `programname` is an error, not a silent loss).

- An object may give its own `id` (1 to 128 characters of `[a-zA-Z0-9_-]`),
  so that the objects after it can refer to it; one without gets a random
  ID.
- Objects are created in the order programs, VENs, resources, events,
  resource groups.
- With `--store`, an object whose ID is already in the store is left as it
  is: the seed adds what is missing, and does not undo what clients changed.
  The `seeded` record counts what it added.

Every problem is reported, with where it is:

```console
$ openadrctl vtn start --config broken.yaml
error: the seed: clients[0]: role "operator": bl or ven; clients[0]: secret or secretEnv is required; programs[0]: not a property of OpenADR 3.1.0: "colour"; events[0]: invalid programID: is required
$ echo $?
3
```

## Notifications

| Mechanism | Flag | What clients do |
|-----------|------|-----------------|
| Webhook | `--webhooks`, `--insecure-webhooks` | Create a subscription with a `callbackUrl`; the VTN checks it can reach it, then posts notifications to it. With `--store`, a notification not yet delivered is sent after a restart |
| MQTT | `--mqtt` | Read the broker from `GET /notifiers` and topics from `GET /notifiers/mqtt/topics/…`, and subscribe at the broker. The VTN publishes every change |
| WebSocket | `--websocket` | Create a subscription with the WebSocket mechanism, and open the VTN's WebSocket (an extension) |

`openadrctl ven listen` and `bl listen` do each of these; see the
[guide](guide.md#8-notifications-websocket-webhook-mqtt).

### MQTT logins by token

By default the VTN announces its broker with `ANONYMOUS`: who may log in is
the broker's business. With `--mqtt-client-auth token` it announces
`OAUTH2_BEARER_TOKEN` with the user name `{clientID}`: a client logs in with
its client ID and its OAuth2 token, as `openadrctl listen --via mqtt` does by
itself. For the broker to check the token, the VTN serves `POST /auth/mqtt`,
in the form the HTTP authenticators of brokers send:

| Request | Answer |
|---------|--------|
| `{"username": "site-42", "password": "<token>"}` (JSON, or the same as a form) | `200 {"result": "allow"}` when the token is one this VTN issued to `site-42` and has not expired |
| `{"username": "vtn", "password": "<OPENADRCTL_VTN_MQTT_PASSWORD>"}` | `200 {"result": "allow"}`: the VTN's own publisher, with `--mqtt-username` |
| anything else | `403 {"result": "deny"}` |

Each answer is a `mqtt_login` record. The endpoint checks who logs in, not
which topics they read: a broker that should keep a VEN to its own topics
needs rules of its own. EMQX, for instance:

```bash
openadrctl vtn start --listen 0.0.0.0:8080 --public-url http://vtn.example:8080 \
    --mqtt mqtt://emqx.example:1883 --mqtt-client-auth token --mqtt-username vtn   # OPENADRCTL_VTN_MQTT_PASSWORD set

docker run -d -p 1883:1883 \
  -e EMQX_AUTHENTICATION__1__MECHANISM=password_based \
  -e EMQX_AUTHENTICATION__1__BACKEND=http \
  -e EMQX_AUTHENTICATION__1__METHOD=post \
  -e EMQX_AUTHENTICATION__1__URL=http://vtn.example:8080/auth/mqtt \
  -e 'EMQX_AUTHENTICATION__1__BODY={"username":"${username}","password":"${password}"}' \
  -e 'EMQX_AUTHENTICATION__1__HEADERS={"content-type":"application/json"}' \
  emqx/emqx:6.3.1
```

EMQX logs a 403 as an error of its authenticator, and refuses the login.
When the broker starts before the VTN answers, EMQX marks the authenticator
unavailable for a few seconds and refuses logins meanwhile; clients and the
VTN's publisher try again. `scripts/mqtt-token.sh` runs this setup.

## Records

The VTN writes one record per thing that happens: text by default, JSON Lines
with `--format json` (fields in [json-output.md](json-output.md#vtn-start)).

```console
$ openadrctl vtn start --log-requests
2026-10-28T23:59:19Z  listening      http://127.0.0.1:8080 on 127.0.0.1:8080
...
2026-10-28T23:59:19Z  request        GET /auth/server 200 0ms
2026-10-28T23:59:19Z  request        POST /auth/token 200 0ms
2026-10-28T23:59:19Z  request        GET /events 200 0ms
```

## In a container

openadrctl is one static binary; a VTN in a container needs the binary, a
seed and a port:

```dockerfile
FROM gcr.io/distroless/static-debian12
COPY openadrctl /openadrctl
COPY seed.yaml /seed.yaml
EXPOSE 8080
ENTRYPOINT ["/openadrctl", "vtn", "start", "--listen", "0.0.0.0:8080", "--config", "/seed.yaml"]
```

Give `--public-url` with the URL clients use, so that the VTN announces its
token endpoint under it.
