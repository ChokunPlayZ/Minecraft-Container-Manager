# Gateway & Wake-on-Rejoin Spec

Status: **approved for implementation (Phase 1 in progress)**
Date: 2026-08-19

This document specifies how MCM should evolve from a "panel that publishes each
server's game port directly" into a **gateway** that owns the public port,
wakes servers on player connect, holds the player in limbo until the server is
ready, and (Phase 2) runs a Minecraft protocol proxy that drops reconnecting
players into an empty spectator-mode End dimension with a per-server message
until the real server is up, then transfers them into it.

The work is split into two phases. Phase 1 is well-bounded and ships a real
improvement on its own. Phase 2 is the large protocol proxy and is designed
against the Phase 1 foundation.

---

## 1. Context / current architecture

- MCM runs a single Go binary (`cmd/mcm`) that exposes an HTTP API/panel on
  `:8080` and talks to Docker through a mounted socket (`internal/docker`).
- Each Minecraft server is a sibling container created by MCM through the
  Docker API (`Store.ensureContainer` -> `Manager.Create`).
- Today each server container **publishes its game port** to the host:
  container `25565/tcp` is bound to `0.0.0.0:<host_port>` via
  `PortBindings` (`internal/docker/docker.go`, `portBindings`). Players connect
  straight to that published port, bypassing MCM entirely.
- Idle spin-down lives in `internal/spindown`. It stops idle servers and has a
  `HandleJoin`/`Wake` path. The only *automatic* wake today is a fragile
  console-log watcher (`internal/api/spindown.go`,
  `configureConsoleJoinWatcher`) that scans for `"joined the game"` - a chicken-
  and-egg problem because a stopped container emits no log.
- DNS publishing (`internal/dns`) creates Cloudflare SRV records pointing at
  `server.host_port`.

### Why the current auto-wake is broken for this goal

To detect a rejoin and wake the server, the player's connect must reach the
panel. It doesn't today because the server's port is published directly by the
container. The gateway requirement is therefore: **MCM must own the port that
players connect to.**

---

## 2. Goals & non-goals

### Goals
- MCM owns (binds) each server's public game port; nothing else binds it.
- On the first inbound TCP connect to a sleeping/stopped server, MCM wakes it
  automatically, then puts the player into a limbo until the server accepts.
- A sleeping server continues to advertise its **last-known-good MOTD** on the
  server-list ping, so it does not appear "offline" while spun down.
- Per-server customizable wait/void message.
- (Phase 2) A Minecraft protocol-aware proxy that terminates login, enforces
  auth (online/offline), and drops the player into a borderless End dimension
  in spectator mode until the backend accepts, then transfers (warps) them in.

### Non-goals (this iteration)
- No change to the backup, DNS, auth (panel login), or mods subsystems.
- Phase 1 does **not** terminate the Minecraft protocol; it relays raw bytes
  after a handshake-aware hold. Full protocol termination is Phase 2.
- No multi-server network load balancing; one public port maps to one server.

---

## 3. Phase 1 - Gateway foundation

Phase 1 makes MCM the owner of each server's public port, wakes on connect,
holds the connection in limbo, and serves the last-known-good MOTD. This is the
foundation on which Phase 2 builds and is independently valuable.

### 3.1 Port ownership
- Add gateway listeners that bind `server.host_port` on the host.
- Stop the server container from publishing `25565` to the host. In
  `internal/docker/docker.go`, change `Create` so the game port is **exposed
  but not bound** (remove/empty `PortBindings`, keep `ExposedPorts`). This is
  the key topology change: the container listens internally only.
- The gateway must reach the container's internal game port. Plan:
  - Resolve the container address via Docker inspect
    (`NetworkSettings.Networks[*].IPAddress` on port `25565`) as the primary
    route.
  - Fall back to `dockerHost():<host_port>` (the published port) when the
    container IP is not reachable (e.g. remote-daemon setups), to preserve
    cross-deployment compatibility. Gate this behind a small helper
    `containerAddr(ctx, srv) (string, error)`.
- Preserve DNS behavior: `host_port` still represents the public port, so SRV
  records keep pointing at the gateway.

### 3.2 Gateway lifecycle / reconciliation
- New package `internal/gateway`. It owns one `net.Listener` per server's
  `host_port`.
- Reconcile listeners against the server list on startup and periodically
  (e.g. every 30s) and on server create/delete, so:
  - Listeners are opened for every server in the DB (including stopped ones -
    that is the point: a stopped server still accepts connects so it can wake).
  - Listeners are removed when a server is deleted (`DeleteServer`).
- A single server must have exactly one listener; guard against duplicate
  binds with the same `host_port` (report/log and skip duplicates).
- Shutdown on SIGTERM: close all listeners and in-flight connections cleanly.
- The gateway should be **enabled only when spin-down is enabled** (the user
  asked: "make it so mcm runs as the gateway if spindown is turned on"). Tie
  it to the spindown-enabled state; when spindown is disabled, fall back to the
  existing direct-publish topology.

### 3.3 Wake-on-connect
- On `Accept`, resolve the server by `host_port`.
- Look up current state. If sleeping (state in
  `stopped`/`stopping`/`error`) or otherwise not accepting:
  1. Call `spindown.Service.Wake(ctx, id)` to start the container and seed its
     activity clock.
  2. Enter the **limbo hold** (below) until the backend accepts.
- If the server is already running, skip wake and relay immediately.

### 3.4 Limbo hold (handshake-aware)
- Problem: vanilla login is a single handshake->login->play flow. If the proxy
  forwards the client's first bytes before the backend listens, login fails and
  the client disconnects with "failed to connect".
- Solution (Phase 1, protocol-light): buffer the client's handshake, do **not**
  forward it yet, and once the backend accepts, begin a fresh relay of the
  buffered bytes then stream bidirectionally. Concretely:
  - Read the first length-prefixed packet from the client (compression off at
    this stage), stash it, and hold the socket open while the backend boots.
  - Poll the backend address until a dial succeeds (with a bounded timeout).
  - Then `write` the stashed buffer to the backend and start forwarding both
    directions.
- Honest limitation (documented + surfaced in Phase 2): holding just the
  first packet is a best-effort limbo. If the backend is too slow, the client
  may still time out; the durable "hold the player in a real void" behavior is
  Phase 2. Keep the hold window generous (e.g. up to ~90s) and log a clear
  message on timeout.
- Per-server wait message: when the hold is active, log/expose the message and
  (Phase 2) send it into the client's fake server. In Phase 1 the message is
  surfaced via the panel UI + MOTD status and logged server-side.

### 3.5 Connection handling
- One goroutine per connection. Standard `io.Copy` both ways with sane
  half-close handling (`TCPCloseWrite` where supported).
- Enforce read/write deadlines so zombie connects on sleeping servers don't
  leak: e.g. an idle deadline of ~2 minutes and a total relay timeout.
- Don't hold the gateway's accept loop on a waking server; spawn a goroutine
  per connect.

### 3.6 Last-known-good MOTD while sleeping
- Capture a server's MOTD while it is running, persist it, and serve it while
  sleeping so the server list still shows a real description.
- Capture source: perform a small Minecraft server-list (status) probe against
  the running container and record `description.text` + `version.name` (+
  optionally `favicon`) as the "last known good".
- Persistence: add a new migration (`0007_gateway.sql`) adding columns to
  `servers`:
  - `last_motd TEXT` (JSON or plain description)
  - `last_motd_updated TEXT` (RFC3339)
  - `wake_message TEXT` (per-server wait/void message; NULL = global default)
- Serve it:
  - While sleeping, the gateway can answer a **status ping** itself with the
    last-known-good MOTD so the client server-list shows the message even when
    the server is down. This is a small Minecraft status-ping responder
    (length-prefixed status response). Implement it in Phase 1 as a lightweight
    server-list ping responder on the gateway listener, and hand the full
    login/limbo behavior to Phase 2.
  - Expose in the API (see 3.7).

### 3.7 API & settings
- Add a global setting (existing `settings` table, like `idle_timeout_minutes`):
  - `gateway_enabled` (default `false`)
  - `wake_message_default` (global fallback wait message; default something
    like `"Server is starting up, please wait..."`)
- Per-server setting (new column): `wake_message` (overrides the global).
- New/updated API surface:
  - `GET /api/servers/{id}/gateway` - returns `{ enabled, wake_message,
    last_motd, last_motd_updated }` for a server.
  - `PUT /api/servers/{id}/gateway` - set per-server `wake_message`.
  - Extend `GET /api/servers/{id}/status` (or add fields to `Server`) to
    include `last_motd` so the UI can render it.
  - `GET /api/settings` / `PUT /api/settings` already generic; add the new keys.
- Wire gateway enable/disable into spin-down enable/disable. When spin-down is
  enabled, the gateway is active (per the user's requirement).

### 3.8 Frontend
- `web/src/api/types.ts`: add `LastMotd`, `wake_message`, `last_motd`,
  `last_motd_updated` fields to `ServerStatus` (and/or a `GatewayInfo` type).
- `web/src/api/client.ts`: add `serverGateway`, `putServerGateway` methods.
- Server detail page (`web/src/routes/servers.$id.tsx`):
  - Show the last-known-good MOTD (rendered as the message text) when the
    server is sleeping.
  - Add a "Wake message" input in server settings to set the per-server wait
    message.
- Keep visual styling consistent with the existing shadcn/React components.

### 3.9 Config
- Add an env knob to the gateway, e.g. `MCM_GATEWAY` defaulting to `auto`
  (auto = enabled when spin-down is enabled). Read it in `internal/config`.

---

## 4. Phase 2 - Protocol proxy (limbo void + warp-in)

Phase 2 is the full Minecraft protocol proxy. It is the large, version-fragile
part and is designed against the Phase 1 foundation (the gateway already owns
the port and wakes on connect). **Do not start Phase 2 until Phase 1 is merged,
tested, and reviewed.**

### 4.1 Architecture
- Replace the Phase 1 "stash one packet and relay" limbo with a real protocol
  terminator inside `internal/gateway` (or a new `internal/proxy` package):
  - Terminate the client connection at the TCP socket.
  - Speak the Minecraft handshake/status/login/play protocol to the client.
  - Maintain a **separate** connection to the backend server as the player.
- This is the architecture of Velocity / BungeeCord with a limbo/fallback
  server.

### 4.2 Protocol support
- Implement a minimal Minecraft Java protocol stack for the modern protocol
  (target the newest protocol version; keep a table of supported versions and
  their protocol IDs).
- Packet framing: variable-length packets (`VarInt` length + `VarInt` packet
  ID + payload), including compression handling (set-compression) and the
  optional threshold/length-in-varint framing.
- Need: `VarInt`, `VarLong`, `String`, `Position`, `UUID`, `Chat`/`Text`
  component JSON, `NBT` (at least tags needed for chunk/`JoinGame`/light).
- Data types package (`internal/proxy/protocol`): reader/writer with these
  primitives and unit tests (table-driven against known vectors).

### 4.3 Handshake
- Read the client's handshake packet: protocol version, server address, port,
  next state (`1` = status, `2` = login).
- Record the protocol version and route it to the right encoder.
- Status vs login split.

### 4.4 Status (server-list ping)
- Answer `status` requests with the **last-known-good MOTD** while sleeping,
  including `version.name`, `protocol`, `players.online/max`, and favicon if
  captured. This replaces the lightweight Phase 1 status responder with the
  real one.

### 4.5 Authentication
- Required by the user: yes, authentication must be implemented.
- Online mode: implement the Mojang session-server flow. In 1.19.3+ (protocol
  763+) the client sends `login start` and the server, if requiring online
  mode, sends `encryption request`. The proxy must:
  - Generate RSA keypair, send `Login Encryption Request`, verify the client's
    `Login` (hashed server id), and validate the player against Mojang's
    `https://sessionserver.mojang.com/session/minecraft/hasJoined?username=...&serverId=...`.
  - Then connect to the backend as that authenticated player.
- Offline mode: accept name, optionally ping the backend for its online-mode
  stance.
- Keep this in a dedicated `internal/proxy/auth` package with its own tests; a
  fake/mocked session server for unit tests.

### 4.6 Limbo void: Empty End in spectator mode
- After login, run the login -> configuration -> play handshake against the
  client **inside the proxy**, then send the client into a minimal End
  dimension:
  - `Join Game` packet with dimension type = `minecraft:the_end`, gamemode =
    spectator (`3`).
  - Empty chunk (only air / void). No blocks, dark void.
  - Player info, teleport to a fixed position in the End, spectator flying so
    nothing loads.
  - Show the per-server wait message (`wake_message`) via actionbar/title/
    subtitle or chat.
  - Periodic keep-alive so the client doesn't time out; respond to client
    keep-alives.
  - Handle ping/client-status during limbo; ignore movement/input.
- This is a self-contained "limbo server" the proxy runs while waiting for the
  backend.

### 4.7 Warp-in / world transfer
- Poll the backend address until it accepts.
- Then perform the handoff:
  - Option A (preferred, when the backend is a server the proxy controls):
    connect to the backend as the player, run the login flow, and switch the
    client to the backend via a real transfer (1.20.5+/protocol 766+ `Transfer`
    packet) or a `Respawn`/redirect as appropriate.
  - Option B: transparently bridge the existing client socket to the backend
    after the backend handshake is established, forking the play stream. This
    is fragile and version-dependent; prefer A.
- Whatever the mechanism, ensure the player is "warped" into the real world
  seamlessly (spawn + chunks downloaded from the real server).

### 4.8 Versioning
- Build a `protocol registry`: protocol id -> supported packet schemas.
- Support the MC versions this panel can install (Paper/Fabric/vanilla/Spigot/
  Forge/NeoForge); if a client's protocol version is unknown/unsupported, send
  a clear disconnect rather than crash.
- Gate Phase 2 features behind the same `gateway_enabled` setting.

### 4.9 Testing
- Table-driven `VarInt`/framing tests using known byte vectors.
- Auth tests against a mock Mojang session endpoint.
- Limbo handshake: a stub "server" goroutine that verifies the proxy sends the
  expected `Join Game` + teleport + message for a given protocol version.
- Integration: spin a real (small) backend container; assert player connect ->
  wake -> limbo -> warp into online player list.

---

## 5. Migration

`migrations/0007_gateway.sql` (exact column list finalized during Phase 1
implementation):
```sql
-- Gateway / wake-on-rejoin: per-server wait message and last-known-good MOTD.
ALTER TABLE servers ADD COLUMN wake_message TEXT;
ALTER TABLE servers ADD COLUMN last_motd TEXT;
ALTER TABLE servers ADD COLUMN last_motd_updated TEXT;

INSERT OR IGNORE INTO settings (key, value) VALUES ('gateway_enabled', 'false');
INSERT OR IGNORE INTO settings (key, value) VALUES ('wake_message_default',
  'Server is waking up, please wait...');
```

---

## 6. TODO

### Phase 1 - Gateway foundation (this iteration)
- [x] 1. Add `migrations/0007_gateway.sql`; update `internal/servers` Store to
      read/write `wake_message`, `last_motd`, `last_motd_updated`; add
      accessors.
- [x] 2. Create `internal/gateway` package: listener manager with reconcile
      loop (startup + periodic + on create/delete), per-server `host_port`
      listeners, duplicate-bind guard, clean shutdown.
- [x] 3. Change `internal/docker/docker.go` `Create` to expose `25565` without
      binding it to the host; add `containerAddr(ctx, srv)` resolving the
      container internal IP with published-port fallback.
- [x] 4. Wake-on-connect: on accept, resolve server, call `spindown.Wake` if
      stopped/sleeping.
- [x] 5. Limbo hold: buffer first length-prefixed packet, poll backend until
      it accepts (bounded ~90s), then replay buffered bytes and stream
      bidirectionally. Half-close + deadlines.
- [x] 6. Status ping responder on the gateway listener serving last-known-good
      MOTD while sleeping (lightweight, status-only for Phase 1).
- [x] 7. Capture last-known-good MOTD while running (status probe against the
      running container; store to DB).
- [x] 8. `internal/config`: `MCM_GATEWAY` env knob (default `auto`).
- [x] 9. Wire gateway enable to spin-down enable; start/stop gateway in
      `cmd/mcm/main.go`.
- [x] 10. API: `GET/PUT /api/servers/{id}/gateway`, extend status endpoint
      with `last_motd`, add `gateway_enabled`/`wake_message_default` to
      settings.
- [x] 11. Frontend: types + client methods; server detail shows MOTD when
      sleeping; per-server wake-message input in settings; global gateway +
      default-message settings.
- [x] 12. Tests: gateway package (listener reconcile, wake-on-connect, limbo
      hold with a stub backend, status responder), MOTD capture, API handlers,
      existing tests still pass (`go test ./...`).
- [x] 13. Docs: update `README.md` + `TODO.md`; document migration + config.

### Phase 2 - Protocol proxy (limbo void + warp-in)
- [x] 1. `internal/proxy/protocol`: VarInt/VarLong/String/Position/UUID/Chat/NBT
      reader+writer with unit tests.
- [x] 2. Packet framing incl. compression (set-compression) + version registry.
- [x] 3. Handshake routing (status vs login), status responder (real).
- [x] 4. `internal/proxy/auth`: online-mode via Mojang session server (+ mock
      endpoint tests) and offline mode.
- [x] 5. Login -> configuration -> play handshake against the client.
      (Implemented for the modern configuration-phase revisions 764+ via the
      `internal/proxy/confstate` handshake; the classic login->play revisions
      762-763 keep the original flow. Play packet IDs for 764+ limbo are still
      unvalidated - see section 9.)
- [x] 6. Limbo void: Empty End, spectator, `Join Game` + teleport + wait
      message (actionbar/title/chat); keep-alive loop.
- [~] 7. Warp-in: connect to backend as player, seamless transfer to real
      world. (Proxy now authenticates to an online-mode backend and reaches
      Login Success; the seamless play-state mediation/warp for online
      backends remains a documented limitation - see section 9.)
- [x] 8. Version support matrix + graceful disconnect for unknown versions.
- [x] 9. Tests incl. stub-server integration asserting connect -> wake -> limbo
      -> warp.
- [ ] 10. Full integration against a live server + docs + deploy notes
      (compose port exposure for the gateway listener range) - pending.

---

## 7. Open questions / risks

- **Docker networking across deployments.** Phase 1 reaches the container IP
  via inspect with a published-port fallback. On remote-daemon setups the
  fallback keeps it working; on the compose (mcm in a container) the panel may
  need to join the server network or use host-network. Verify both paths in
  integration testing.
- **Phase 1 limbo fidelity.** "Buffer first packet, hold, replay" works in many
  cases but is not a true void; some clients will time out on very slow boots.
  Phase 2 replaces this with real limbo.
- **Phase 2 auth scope.** Online-mode reimplementation is the highest-risk item
  (changed several times across MC versions: encryption flow and `hasJoined`
  are version-sensitive). Budget accordingly and gate carefully.
- **Transfers/warp.** The "seamless warp into the real world" has several
  version-specific mechanisms; pick per protocol version and test against a
  live Paper server before calling it done.
- **Security.** The proxy terminates encryption connections; never log
  session-server auth tokens or player session data. Keep settings read-only
  via the existing CSRF-protected API.

---

## 8. Implementation notes (Phase 1, 2026-08-19)

Recorded decisions and assumptions from the Phase 1 implementation, to keep
the approved spec intact while capturing how the code landed.

- **Gateway enable semantics.** `MCM_GATEWAY` defaults to `auto`. In `auto`
  mode the gateway follows the live `gateway_enabled` setting (read each
  reconcile) so toggling the setting in the UI takes effect without a restart.
  `on`/`off` force it. The `gateway_enabled` setting doubles as the spin-down
  master switch, matching the user's "run as gateway if spindown is turned on".
- **Container address helper.** The docker helper is `ContainerAddr(ctx,
  containerID, hostPort)` rather than `containerAddr(ctx, srv)` because Docker
  does not know about server records. The gateway composites it: it inspects
  the container's internal IP on the exposed port and falls back to
  `HostAddress():hostPort`.
- **Status responder.** Phase 1 includes a lightweight, protocol-light
  server-list responder that parses the Handshake `next_state`; a status
  handshake (state 1) answers with the last-known-good MOTD and never wakes the
  server, while a login handshake (state 2) triggers wake + limbo. A status
  probe (`internal/gateway/motd.go`) captures the MOTD from running containers.
- **Limbo fidelity.** Phase 1 holds only the first buffered handshake and
  replays it once the backend accepts (bounded ~90s). This is best-effort; the
  durable void/warp is Phase 2, as designed.
- **Frontend.** Global gateway + default-message settings live on a new
  `Settings` route; per-server wait message and sleeping MOTD live in the
  server detail page's gateway panel. The TanStack router plugin regenerates
  `web/src/routeTree.gen.ts` during `pnpm build`.

## 9. Implementation notes (Phase 2, 2026-08-19)

Recorded decisions and open risks from the Phase 2 (protocol proxy)
implementation, committed as `88deb89`.

- **New `internal/proxy/` tree.** `protocol/` (VarInt/VarLong/String/UUID/
  Position/Chat + minimal NBT + zlib-compressed framing + version registry),
  `auth/` (Mojang `hasJoined` session client with a mockable `SessionClient`,
  RSA encryption-request/response, AES/CFB8 stream cipher, offline UUID,
  signed-hex `ServerHash`), `limbo/` (End dimension, spectator, Join Game,
  teleport, per-server actionbar, keep-alive loop), and `transfer/` (warp-in).
  `session.go` orchestrates login -> limbo -> warp through the encrypted
  connection. No player/session/token data is logged.
- **Gateway integration.** Login handshakes now route to the proxy instead of
  the Phase 1 buffer-and-relay; the status/MOTD responder and wake-on-connect
  are preserved. Unknown protocol versions get a clean login disconnect.
- **Version scope.** The version registry now covers through protocol 776 =
  MC "26.2" (the newest entries use the year-based naming, e.g. 774/775/776 =
  26.0/26.1/26.2). The classic login->play revisions 762-763 (MC 1.19.4-1.20.1)
  are implemented and unit/stub-tested, including the limbo and the transparent
  play bridge for offline backends.
- **AES/CFB8 stream-state fix.** The encrypted connection wrapper
  (`auth/encrypt.go`) now retains a single persistent CFB8 reader and writer
  across framing calls instead of recreating them per read/write. AES/CFB8 is a
  streaming cipher whose keystream depends on prior ciphertext, so a per-call
  reset corrupted any stream longer than one packet; this is fixed.
- **Configuration-state handshake.** The config handshake for 764+ landed in
  `internal/proxy/confstate` and is wired into `session.go` before the
  play-state limbo. Serverbound/clientbound configuration packet IDs are
  bucketed by revision (764-765, 766-768, 769+). These IDs, and the
  play-state limbo packet IDs for 764+ (which still use the protocol 763
  table), have NOT been validated against a live server and must be confirmed
  before production use.
- **Warp mechanism.** A seamless Transfer is coded for protocol 766+, but the
  exercised limbo path is 763 and uses the transparent play bridge on an
  offline backend. The proxy now completes online-mode backend login: it
  answers the backend's Login Encryption Request, negotiates a shared AES
  secret, switches the backend connection to the cipher, and reaches Login
  Success rather than logging and dropping. The seamless play-state
  mediation/warp for online backends is still a documented limitation.
- **Online-mode default.** Gateway online mode is gated behind an
  `Options.OnlineMode` flag (default off, matching typical offline
  deployments); the online path is implemented in `proxy/auth` and unit-tested.
- **Play-state limbo IDs for 764+ (known limitation).** The config-phase
  revisions reach the play-state limbo, but the limbo still serves play packets
  using the protocol 763 table (Join Game 0x28, Player Position 0x3C, etc.).
  These IDs may have shifted in 764+ and are unvalidated/assumed; the proxy
  logs a prominent warning before entering limbo for a config-phase version.
  They must be verified against a live server before production use rather
  than guessing corrected IDs without a reliable source.
- **Outstanding (Phase 2 TODO item 10).** Full integration against a live
  server, live validation of the config-phase and 764+ play packet IDs, the
  seamless online-backend warp, and deploy notes for compose port exposure of
  the gateway listener range remain open before production enablement.
