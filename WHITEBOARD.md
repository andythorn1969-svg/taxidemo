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

**Version:** 0.1.0
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

---

## Package Structure

```
taxidemo/
├── main.go                  Entry point only (~25 lines)
├── models/
│   └── models.go            Structs, constants, SeedData()
├── dispatch/
│   └── dispatch.go          FindZone, FindNearestDriver, DispatchJob, destinations list
├── api/
│   └── handlers.go          AppState, HTML template, HTTP handlers
├── go.mod
├── .gitignore
└── WHITEBOARD.md
```

### HTTP routes
| Method | Path | Handler | Purpose |
|--------|------|---------|---------|
| GET | `/` | HandleIndex | Main dispatch console |
| POST | `/dispatch` | HandleDispatch | Create and dispatch a booking |
| GET | `/api/drivers` | HandleDriverData | JSON feed for map driver markers (incl. plate number) |
| GET | `/api/bookings` | HandleBookingData | JSON feed for map booking/destination markers |
| GET | `/api/zones` | HandleZoneData | JSON feed for zone polygons and driver counts |

---

## UI Layout (Step 10)

```
┌─────────────────────────────────────────────────────────┐
│  TOPBAR: Title | Zone ▾ | Passenger [       ] [Dispatch] │
├──────────────┬──────────────────────────────────────────┤
│ LEFT (35%)   │  RIGHT (65%)                             │
│ Zone Trap    │  ┌──────────────────────────────────┐    │
│ Queues       │  │  LIVE MAP (flex:1, min 500px)    │    │
│ (scrollable) │  │  - Green/red plate badges        │    │
│              │  │  - Blue pickup markers            │    │
│ Z01 Progress │  │  - Purple destination markers    │    │
│  T1 Alice    │  │  - Dashed journey lines          │    │
│  T2 Bob      │  │  - Zone boundary polygons        │    │
│ Z02 Thanet   │  └──────────────────────────────────┘    │
│  T1 Carol    │  Dispatch Log (max 180px, scrollable)    │
│ ...          │                                          │
└──────────────┴──────────────────────────────────────────┘
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
`api.AppState` holds zones, drivers and jobs. No persistence — restarts wipe the state. Intentional for demo purposes.

### Booking coordinates
Bookings are plotted near their pickup zone centre with a ±0.003° random offset so multiple bookings in the same zone don't stack on the map.

### Destinations
15 real Southend drop-off points defined in `dispatch/dispatch.go`. A random destination is assigned to each booking when created, and its name is shown in the dispatch log and map popups.

### Plate numbers
Each driver is assigned a random plate number (1–500) at startup via `SeedData()`. Plate numbers are shown as badge labels directly on the map markers — no click needed.

### Template rendering
The entire HTML page is a single Go `text/template` embedded in `handlers.go`. No external template files, no external CSS — keeps the project self-contained.

---

## Next Steps

- [ ] Improve `FindNearestDriver` to use actual Haversine geo-distance
- [ ] Add driver "return to zone" logic after completing a job
- [ ] Add a way to mark jobs as completed from the UI
- [ ] Persist state to a file or SQLite so it survives restarts
- [ ] Add websocket or SSE so the UI updates without a page refresh
- [ ] Add a legend to the map (green=available, red=busy, blue=pickup, purple=destination)

---

## Open Questions

- **How should declined jobs be handled?** Currently they sit in the log marked `declined` with no retry. Should the system re-queue them after a timeout?
- **What happens when all drivers are busy?** The booking is logged as `declined`. Should it go into a pending queue and be retried when a driver becomes free?
- **Driver return to zone** — after accepting a job, a driver is marked `busy` and removed from the queue. There is no mechanism yet to bring them back.
- **Multi-zone fallback ordering** — when falling back to `FindNearestDriver`, should we prefer drivers in adjacent zones or just closest by GPS distance?
- **Authentication** — the dispatch form is completely open. For a real system, who should be allowed to create bookings?
