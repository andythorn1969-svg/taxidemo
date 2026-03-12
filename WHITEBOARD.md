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

**Version:** 0.2.0
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

---

## Package Structure

```
taxidemo/
├── main.go                  Entry point only (~32 lines)
├── models/
│   └── models.go            Structs, constants, SeedData(), AppState with RWMutex
├── dispatch/
│   └── dispatch.go          FindZone, FindNearestDriver, NearestZone, DispatchJob,
│                            CompleteBooking, CancelBooking, StartScheduler
├── api/
│   └── handlers.go          Handler struct, RegisterRoutes, HTML template, HTTP handlers
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
| POST | `/api/booking/cancel` | HandleCancelBooking | Cancel a booking, free driver if assigned |
| GET | `/api/prebooks` | HandlePrebookData | JSON list of prebooks (filter: active/completed/all) |

---

## UI Layout (Step 11 — current)

```
┌──────────────────────────────────────────────────────────────┐
│  TOPBAR: Title | Zone ▾ | Passenger [            ] [Dispatch] │
├──────────────┬───────────────────────────────────────────────┤
│ LEFT (35%)   │  RIGHT (65%)                                  │
│ Zone Trap    │  ┌─────────────────────────────────────────┐  │
│ Queues       │  │  LIVE MAP (flex:1, min 500px)           │  │
│ (scrollable) │  │  - Green/red plate badges               │  │
│              │  │  - Blue pickup markers                  │  │
│ Z01 Progress │  │  - Purple destination markers           │  │
│  T1 Alice    │  │  - Dashed journey lines                 │  │
│  T2 Bob      │  │  - Zone boundary polygons               │  │
│ Z02 Thanet   │  └─────────────────────────────────────────┘  │
│  T1 Carol    │  Dispatch Log (max 180px, scrollable)         │
│ ...          │  [+ NEW BOOKING] button                       │
│              │  Jobs List (filter: all/active/done)          │
└──────────────┴───────────────────────────────────────────────┘

New Booking modal (overlay):
  Customer name, phone, notes, account flag
  Pickup address → geocode → draggable pin on mini-map
  Destination address → geocode → draggable pin
  Booking type (immediate / pre-book) + requested time
  [Submit] → POST /api/booking/new
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
`models.AppState` holds zones, drivers, bookings and jobs behind a `sync.RWMutex`. No persistence — restarts wipe the state. Intentional for demo purposes.

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

---

## Next Steps

- [ ] Improve `FindNearestDriver` to use actual Haversine geo-distance
- [ ] Add driver "return to zone" logic after completing a job
- [ ] Persist state to a file or SQLite so it survives restarts
- [ ] Add websocket or SSE so the UI updates without a page refresh
- [ ] Add a legend to the map (green=available, red=busy, blue=pickup, purple=destination)

---

## Session 3 To-Do

Priority order for next session:

1. **Three-column layout redesign** — equal thirds: booking form left, live map centre, trap queues right. Jobs list full width below all three columns.
2. **Left panel as single action panel** — all booking creation and editing happens in the left column. No separate modal. The panel is always visible.
3. **Edit mode via jobs list** — clicking a row in the jobs list populates the left panel in edit mode (pre-filled fields, submit updates rather than creates).
4. **Zone names showing as IDs** — zone column in jobs list displays raw IDs (e.g. `Z18`) instead of readable names (e.g. `Town`). Needs the same `zoneName` lookup used in the template.
5. **Driver name missing on dispatched jobs** — jobs table shows no driver name after dispatch. Driver reference on `Job.Driver` needs surfacing in the JSON/render path.
6. **Destination zone column empty** — destination zone not being set or returned for jobs. Investigate whether `DestZone` field is missing from `Booking` or just not populated.
7. **General cosmetic tweaks** — tighten spacing, consistent font sizes, readable colour contrast throughout.

### Future milestone
- **OSRM live routing** — replace the fixed `AverageApproachMinutes` zone lookup with a real road-distance call to an OSRM instance. Approach time would be calculated from driver GPS position to pickup GPS position at dispatch time.

---

## Open Questions

- **How should declined jobs be handled?** Currently they sit in the log marked `declined` with no retry. Should the system re-queue them after a timeout?
- **What happens when all drivers are busy?** The booking is logged as `declined`. Should it go into a pending queue and be retried when a driver becomes free?
- **Driver return to zone** — after accepting a job, a driver is marked `busy` and removed from the queue. There is no mechanism yet to bring them back.
- **Multi-zone fallback ordering** — when falling back to `FindNearestDriver`, should we prefer drivers in adjacent zones or just closest by GPS distance?
- **Authentication** — the dispatch form is completely open. For a real system, who should be allowed to create bookings?
