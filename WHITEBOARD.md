# Taxidemo Whiteboard

## What We're Building

A taxi dispatch demo application for **Southend Taxi Cooperative** — a member-owned cooperative of taxi drivers serving Southend-on-Sea.

The goal is a realistic dispatch simulation showing:
- Drivers organised into geographic zones with ordered **trap queues** (longest-waiting driver gets the next job — trap 1)
- Real-time **live map** of driver and booking positions using Leaflet.js and OpenStreetMap
- Web-based **dispatch console** where operators can create bookings and watch them get assigned
- Eventually: a proper backend for a real cooperative dispatch system

This is a demo / proof-of-concept. There is no database — all state lives in memory and resets on server restart.

---

## Current Status

**Version:** 0.4.0
**Server:** `http://localhost:8080`
**Branch:** `master`
**Remote:** `https://github.com/andythorn1969-svg/taxidemo`

### Completed steps
| Step | Description |
|------|-------------|
| 3 | Working web UI with dispatch logic |
| 4 | Live Leaflet map with driver positions (green=available, red=busy) |
| 5 | Refactored into proper Go package structure |
| 6 | Booking markers on live map (blue circles) |
| 7 | 22 real Southend zones, 30 drivers, ngrok tested |
| 8 | Zone boundary polygons on live map |
| 9 | Destination markers (purple) and dashed journey lines per booking |
| 10 | Landscape layout redesign — two-column dispatch console, plate number badges on driver markers |
| 11 | Pre-book system — modal, Nominatim geocoding, jobs list panel, prebook scheduler, zone matching |
| 12 | Three-column layout redesign — booking form left, map centre, trap queues right |
| 13 | Left panel as single always-visible action panel; click-to-edit from jobs list |
| 14 | Driver movement simulation — StatusDispatched/StatusOnJob, fixed linear step, 5s tick |
| 15 | Zone names resolved in jobs table; driver name on dispatched jobs; destination zone column |
| 16 | Map legend — available/dispatched/pickup/destination/zone boundaries |
| 17 | Customer records — phone lookup, auto-populate form, favourite destinations, previous pickups |
| 18 | Customer list panel — searchable overlay, ACC badge, click-to-edit |
| 19 | No-show / cancellation flagging — weighted scoring, late-night exclusion, cooperative policy config |

---

## Package Structure

```
taxidemo/
├── main.go                  Entry point only (~32 lines)
├── models/
│   └── models.go            Structs, constants, SeedData(), AppState with RWMutex
├── dispatch/
│   └── dispatch.go          FindZone, FindNearestDriver, NearestZone, DispatchJob,
│                            CompleteBooking, CancelBooking, IsLateWeekendBooking,
│                            ShouldFlagCustomer, StartScheduler, StartSimulation
├── api/
│   └── handlers.go          Handler struct, RegisterRoutes, HTML template, HTTP handlers
├── config/
│   └── config.go            Config struct, Policy var — cooperative business policy
├── go.mod
├── .gitignore
└── WHITEBOARD.md
```

### HTTP routes
| Method | Path | Handler | Purpose |
|--------|------|---------|---------|
| GET | `/` | HandleIndex | Main dispatch console |
| POST | `/dispatch` | HandleDispatch | Legacy form-based booking (immediate only) |
| GET | `/api/drivers` | HandleDriverData | JSON feed for map driver markers (incl. plate number) |
| GET | `/api/bookings` | HandleBookingData | JSON feed for map booking/destination markers |
| GET | `/api/zones` | HandleZoneData | JSON feed for zone polygons and driver counts |
| GET | `/api/geocode` | HandleGeocode | Nominatim proxy — returns `{lat, lng}` for an address |
| POST | `/api/booking/new` | HandleNewBooking | Create immediate or pre-booked job from JSON |
| POST | `/api/booking/complete` | HandleCompleteBooking | Mark a booking completed, free driver |
| POST | `/api/booking/cancel` | HandleCancelBooking | Cancel booking; updates no-show/cancellation counts |
| GET | `/api/prebooks` | HandlePrebookData | JSON list of prebooks (filter: active/completed/all) |
| GET | `/api/customers` | HandleCustomerList | Searchable customer list with booking counts |
| GET | `/api/customer/lookup` | HandleCustomerLookup | Lookup by phone — returns customer + booking history |
| POST | `/api/customer/new` | HandleNewCustomer | Create customer record |
| POST | `/api/customer/update` | HandleUpdateCustomer | Update customer fields incl. blocked/no-show reset |

---

## UI Layout (Step 18 — current)

```
┌──────────────────────────────────────────────────────────────────┐
│  TOPBAR: Southend Taxi Cooperative — Dispatch      [👥 CUSTOMERS] │
├──────────────────┬───────────────────────────────────────────────┤
│ LEFT (22%)       │  CENTRE (flex)                                │
│ Booking form     │  ┌─────────────────────────────────────────┐  │
│  or              │  │  LIVE MAP                               │  │
│ Customer panel   │  │  - Green/red plate badges               │  │
│ (overlay)        │  │  - Blue pickup markers                  │  │
│                  │  │  - Purple destination markers           │  │
│ Phone lookup     │  │  - Dashed journey lines                 │  │
│  → ACC badge     │  │  - Zone boundary polygons               │  │
│  → 🚫 Blocked    │  └─────────────────────────────────────────┘  │
│  → ⚠ Flagged     │  Bookings panel (filter: active/completed/all) │
│  → fav dest pills│  Jobs table with ACC badge, status, actions   │
│  → pickup pills  │                                               │
│                  │  RIGHT (25%)                                  │
│                  │  Zone Trap Queues                             │
│                  │   Z01 Progress T1 Alice T2 Bob …              │
└──────────────────┴───────────────────────────────────────────────┘

Customer panel (overlays left column):
  Search input → live filter
  Table: Name | Phone | ACC | Bookings
  Click row → edit form:
    Name, Phone, Address, Notes, Fav destinations
    Account checkbox, Blocked checkbox
    No-shows (read-only + Reset), Cancellations (read-only + Reset)
    ⚠ Flagged indicator (amber, read-only)
```

---

## Key Decisions

### Trap queue ordering
Drivers are ordered by `FreeAt` time — whoever has been waiting longest is at trap 1 (index 0 of `Zone.Drivers`). This mirrors real cooperative dispatch rules.

### Accept/decline simulation
A driver accepts a job if they have waited ≥ 30 minutes, otherwise they decline. This is a placeholder — in a real system drivers would respond via an app.

### Fallback dispatch
If a zone has no available drivers, `FindNearestDriver` scans all zones and returns the first available driver it finds. No geo-distance calculation yet — just linear scan.

### In-memory state
`models.AppState` holds zones, drivers, bookings, jobs and customers behind a `sync.RWMutex`. No persistence — restarts wipe the state. Intentional for demo purposes.

### Zone matching from coordinates
`dispatch.NearestZone(lat, lng)` finds the closest zone centre from the `zoneCoords` map using squared Euclidean distance. Called in `HandleNewBooking` whenever geocoded coordinates are present — client-supplied zone ID is ignored when coordinates are available.

### Geocoding
`HandleGeocode` proxies Nominatim (OpenStreetMap). Automatically appends "Southend-on-Sea, Essex" context if the query doesn't already mention Southend or Essex.

### Prebook scheduler
`dispatch.StartScheduler` runs a 60-second ticker goroutine. Each tick calls `runSchedulerCycle`, which finds all pending prebooks whose `RequestedTime - AverageApproachMinutes` has passed and dispatches them via `DispatchJob`.

### Booking coordinates
Bookings are plotted near their pickup zone centre with a ±0.003° random offset so multiple bookings in the same zone don't stack on the map.

### Destinations
15 real Southend drop-off points defined in `dispatch/dispatch.go`. A random destination is assigned to each booking when created, and its name is shown in the dispatch log and map popups.

### Plate numbers
Each driver is assigned a random plate number (1–500) at startup via `SeedData()`. Plate numbers are shown as badge labels directly on the map markers — no click needed.

### Template rendering
The entire HTML page is a single Go `text/template` const (`indexHTML`) embedded in `handlers.go`. Template is built once in `RegisterRoutes` with FuncMap closures over `h.State`. No external template files, no external CSS — keeps the project self-contained.

### Cooperative policy configuration
All business-rule thresholds live in `config/config.go` as a single `Config` struct with a `Policy` var. Each field is documented with who would change it and why. Values accessible throughout the codebase as `config.Policy.FieldName`. No environment variables or config files needed for demo — change the defaults in the source.

### No-show vs cancellation distinction
`HandleCancelBooking` peeks at the driver's status under a read lock **before** calling `CancelBooking` (which resets the driver to `StatusAvailable`):
- `StatusOnJob` (driver arrived at pickup, nobody present) → `NoShowCount++`, late-night exclusion applies
- `StatusDispatched` (driver still en route) → `CancellationCount++`, no exclusion

`ShouldFlagCustomer` uses a weighted score: `(NoShowCount × NoShowWeight) + (CancellationCount × CancellationWeight)`. Both a minimum weighted score and a minimum rate must be exceeded to flag a customer.

---

## Open Questions

- **How should declined jobs be handled?** Currently they sit in the log marked `declined` with no retry. Should the system re-queue them after a timeout?
- **What happens when all drivers are busy?** The booking is logged as `declined`. Should it go into a pending queue and be retried when a driver becomes free?
- **Driver return to zone** — after accepting a job, a driver is marked `busy` and removed from the queue. There is no mechanism yet to bring them back.
- **Multi-zone fallback ordering** — when falling back to `FindNearestDriver`, should we prefer drivers in adjacent zones or just closest by GPS distance?
- **Authentication** — the dispatch form is completely open. For a real system, who should be allowed to create bookings?

---

## Session 3 Summary

Delivered in Session 3:

- **Three-column layout** — booking form (left 22%), live map + bookings panel (centre flex), trap queues (right 25%). Jobs list full width below centre column.
- **Left panel always-visible action panel** — booking creation and editing in one place, no modal. Click any row in the jobs list to enter edit mode with pre-filled fields.
- **Driver movement simulation** — `StatusDispatched` (en route to pickup) → `StatusOnJob` (passenger aboard) → `StatusAvailable` (job complete). Fixed linear step per tick computed at leg start so journeys complete in exactly `SimJourneyMinutes`. 5-second tick goroutine.
- **Seed dispatched jobs** — four seed bookings wired to real driver pointers with `JobAccepted` job records so the simulation has something to animate on startup.
- **Zone names in jobs table** — `HandlePrebookData` resolves `PickupZone` and `DestZone` IDs to readable names (e.g. `Z18` → `Town`) before serialising.
- **Driver name on dispatched jobs** — `HandlePrebookData` joins `state.Jobs` to find the assigned driver and includes their name as `assigned_driver` in the JSON.
- **Destination zone column** — `DestZone` field added to `models.Booking`; derived via `NearestZone` in `HandleNewBooking` and pre-computed for all seed bookings.
- **Map legend** — compact absolutely-positioned panel in the map corner: available driver, dispatched/on-job, pickup point, destination, zone boundaries.
- **`GenerateID` fix** — switched from `time.Now().UnixNano()` to `rand.Int63()` to prevent duplicate IDs when many bookings are created within the same nanosecond.

---

## Session 4 Summary — Customer Records & Flagging

Delivered in Session 4:

- **Customer struct extended** — `NoShowCount`, `CancellationCount`, `Flagged`, `Blocked` added to `models.Customer` with JSON tags.
- **Phone lookup on booking form** — `onblur` on the phone field calls `lookupCustomer()`, which fetches `/api/customer/lookup?phone=…`. On match: auto-populates name, notes, account flag, favourite destinations pills, and previous pickup pills. Status indicator shows green ✓, amber ⚠ (flagged), or red 🚫 (blocked).
- **Auto-create customer on booking** — if the phone number is new (`foundCustomer === 'new'`), a silent POST to `/api/customer/new` is fired after a successful booking submission.
- **Customer list panel** — `👥 CUSTOMERS` button in topbar opens a panel that overlays the left booking-form column. Searchable table (name, phone, ACC badge, booking count). Click any row to open the edit form.
- **Customer edit form** — name, phone, address, notes, favourite destinations (comma-separated), account flag, blocked flag, no-show count (read-only + Reset), cancellation count (read-only + Reset), flagged indicator (read-only amber).
- **No-show / cancellation distinction** — `HandleCancelBooking` captures driver status before calling `CancelBooking` (which resets it). `StatusOnJob` → no-show (late-night exclusion applies); `StatusDispatched` → cancellation (no exclusion).
- **Weighted flagging** — `ShouldFlagCustomer` computes a weighted score; both an absolute threshold and a rate threshold must be exceeded.
- **Late-night weekend exclusion** — `IsLateWeekendBooking` handles midnight crossover: early-morning hours check the previous calendar day against `LateNightDays`.
- **Cooperative policy config** — `config/config.go` introduced. `NoShowMinCount`, `NoShowMinRate`, `NoShowWeight`, `CancellationWeight`, `ExcludeLateWeekend`, `DefaultApproachMinutes`, `SimTickSeconds`, `SimJourneyMinutes` all live there. `SimTickSeconds` and `SimJourneyMinutes` migrated from `dispatch/dispatch.go`.
- **ACC badge in jobs table** — `is_account` already serialised in booking JSON; purple `ACC` span shown in the Passenger column.

---

## Session 5 Plan — State Persistence & Live Updates

### Priority items

1. **State persistence** — persist bookings, jobs, driver positions, and customer records to a file or embedded SQLite (`modernc.org/sqlite` — pure Go, no CGO) so state survives server restarts. Customers are the most important to persist; bookings/jobs second.

2. **WebSocket live updates** — replace the current 5-second `setInterval` polling of `/api/prebooks` and `/api/drivers` with a single WebSocket connection pushing JSON diff events. The browser should update the map markers and jobs table immediately when state changes, not on the next poll. Consider `gorilla/websocket` or the standard `golang.org/x/net/websocket`.

### Remaining backlog (post Session 5, priority order)

1. Haversine distance for `FindNearestDriver` — replace linear scan with actual geo-distance
2. Driver return-to-zone logic after job completion
3. Authentication — shared-secret login for operators
4. Cosmetic polish pass — tighten spacing, consistent font sizes, mobile-friendly review
5. OSRM live routing — replace fixed `AverageApproachMinutes` with real road-distance calls

### Future milestones

- **OSRM live routing** — replace the fixed `AverageApproachMinutes` zone lookup with a real road-distance call to an OSRM instance. Approach time would be calculated from driver GPS position to pickup GPS position at dispatch time.
- **CTI phone integration** — integrate with a Computer Telephony Integration (CTI) system (e.g. Twilio, 3CX, or a SIP gateway) so that when a call arrives at the dispatch desk, the caller's phone number is automatically passed to the booking form and `lookupCustomer()` fires immediately. Customer records would pre-populate before the operator says a word. This is the primary path to making the customer lookup feature genuinely useful in production.
