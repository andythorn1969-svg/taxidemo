// ============================================================
// Taxidemo - Southend Taxi Cooperative Dispatch Demo
// Version: 0.1.0
// Description: Core data structures for booking and dispatch
// ============================================================

package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"time"
)

// ============================================================
// MODELS
// ============================================================

// DriverStatus represents whether a driver is free or occupied
type DriverStatus string

const (
	StatusAvailable DriverStatus = "available"
	StatusBusy      DriverStatus = "busy"
)

// Driver represents a taxi driver in the cooperative
type Driver struct {
	ID     string
	Name   string
	ZoneID string
	Status DriverStatus
	FreeAt time.Time // when they became available - determines trap position
	Lat    float64   // GPS latitude
	Lng    float64   // GPS longitude
}

// Zone represents a geographic dispatch area with an ordered trap queue
type Zone struct {
	ID      string
	Name    string
	Drivers []*Driver // ordered slice - index 0 is trap 1
}

// BookingStatus tracks where a booking is in its lifecycle
type BookingStatus string

const (
	BookingPending    BookingStatus = "pending"
	BookingDispatched BookingStatus = "dispatched"
	BookingCompleted  BookingStatus = "completed"
	BookingCancelled  BookingStatus = "cancelled"
)

// BookingSource tells us how the booking arrived
type BookingSource string

const (
	SourcePhone BookingSource = "phone"
	SourceApp   BookingSource = "app"
)

// Booking represents a passenger's request for a taxi
type Booking struct {
	ID          string
	Passenger   string
	PickupZone  string
	Destination string
	Source      BookingSource
	Status      BookingStatus
	CreatedAt   time.Time
}

// JobStatus tracks the offer/response lifecycle
type JobStatus string

const (
	JobOffered   JobStatus = "offered"
	JobAccepted  JobStatus = "accepted"
	JobDeclined  JobStatus = "declined"
	JobCompleted JobStatus = "completed"
)

// Job links a booking to a driver and tracks the dispatch attempt
type Job struct {
	ID        string
	Booking   *Booking
	Driver    *Driver
	Status    JobStatus
	OfferedAt time.Time
}

// ============================================================
// SEED DATA
// ============================================================

func seedData() ([]*Zone, []*Driver) {
	now := time.Now()

	// Create drivers with varying free times (determines trap order) and
	// approximate GPS coordinates around Southend-on-Sea per zone.
	drivers := []*Driver{
		{ID: "D01", Name: "Alice Brown", ZoneID: "Z01", Status: StatusAvailable, FreeAt: now.Add(-45 * time.Minute), Lat: 51.559, Lng: 0.636}, // North
		{ID: "D02", Name: "Bob Carter", ZoneID: "Z01", Status: StatusAvailable, FreeAt: now.Add(-30 * time.Minute), Lat: 51.557, Lng: 0.639}, // North
		{ID: "D03", Name: "Carol Davies", ZoneID: "Z02", Status: StatusAvailable, FreeAt: now.Add(-60 * time.Minute), Lat: 51.535, Lng: 0.713}, // South
		{ID: "D04", Name: "Dave Ellis", ZoneID: "Z02", Status: StatusAvailable, FreeAt: now.Add(-15 * time.Minute), Lat: 51.532, Lng: 0.716}, // South
		{ID: "D05", Name: "Emma Foster", ZoneID: "Z03", Status: StatusAvailable, FreeAt: now.Add(-90 * time.Minute), Lat: 51.532, Lng: 0.809}, // East
		{ID: "D06", Name: "Frank Green", ZoneID: "Z03", Status: StatusAvailable, FreeAt: now.Add(-20 * time.Minute), Lat: 51.529, Lng: 0.807}, // East
		{ID: "D07", Name: "Grace Hill", ZoneID: "Z04", Status: StatusAvailable, FreeAt: now.Add(-55 * time.Minute), Lat: 51.544, Lng: 0.709}, // Central
		{ID: "D08", Name: "Harry Irving", ZoneID: "Z04", Status: StatusBusy, FreeAt: now.Add(-10 * time.Minute), Lat: 51.542, Lng: 0.712},    // Central
		{ID: "D09", Name: "Isla Jones", ZoneID: "Z05", Status: StatusAvailable, FreeAt: now.Add(-35 * time.Minute), Lat: 51.558, Lng: 0.597}, // West
		{ID: "D10", Name: "Jack King", ZoneID: "Z05", Status: StatusAvailable, FreeAt: now.Add(-25 * time.Minute), Lat: 51.555, Lng: 0.600},  // West
	}

	// Create zones and assign their drivers in trap order (longest wait first)
	// Order: North, South, East, Central, West
	zones := []*Zone{
		{ID: "Z01", Name: "North", Drivers: []*Driver{drivers[0], drivers[1]}},
		{ID: "Z02", Name: "South", Drivers: []*Driver{drivers[2], drivers[3]}},
		{ID: "Z03", Name: "East", Drivers: []*Driver{drivers[4], drivers[5]}},
		{ID: "Z04", Name: "Central", Drivers: []*Driver{drivers[6], drivers[7]}},
		{ID: "Z05", Name: "West", Drivers: []*Driver{drivers[8], drivers[9]}},
	}

	return zones, drivers
}

// ============================================================
// DISPATCH LOGIC
// ============================================================

// findZone returns the zone matching the given ID, or nil if not found.
func findZone(id string, zones []*Zone) *Zone {
	for _, z := range zones {
		if z.ID == id {
			return z
		}
	}
	return nil
}

// findNearestDriver returns the first available driver across all zones.
// Placeholder implementation - will be improved with geo-distance logic later.
func findNearestDriver(zones []*Zone) *Driver {
	for _, z := range zones {
		for _, d := range z.Drivers {
			if d.Status == StatusAvailable {
				return d
			}
		}
	}
	return nil
}

// dispatchJob attempts to offer a booking to drivers in the pickup zone's trap queue.
// Drivers who have waited over 30 minutes accept; others decline.
// Falls back to findNearestDriver if the zone has no available drivers.
// Returns a Job reflecting the final outcome.
func dispatchJob(booking *Booking, zones []*Zone) *Job {
	zone := findZone(booking.PickupZone, zones)

	job := &Job{
		ID:        "J-" + booking.ID,
		Booking:   booking,
		OfferedAt: time.Now(),
		Status:    JobOffered,
	}

	// Collect available drivers from the zone queue, preserving trap order.
	var candidates []*Driver
	if zone != nil {
		for _, d := range zone.Drivers {
			if d.Status == StatusAvailable {
				candidates = append(candidates, d)
			}
		}
	}

	// Zone is empty - fall back to nearest available driver across all zones.
	if len(candidates) == 0 {
		fmt.Printf("Zone %q empty - finding nearest driver\n", booking.PickupZone)
		nearest := findNearestDriver(zones)
		if nearest == nil {
			fmt.Println("  No drivers available anywhere - booking cannot be dispatched")
			job.Status = JobDeclined
			return job
		}
		candidates = []*Driver{nearest}
	}

	// Walk the trap queue until someone accepts.
	for trapPos, driver := range candidates {
		waitMins := time.Since(driver.FreeAt).Minutes()
		fmt.Printf("  Offering job to %s (trap %d in %s zone)\n",
			driver.Name, trapPos+1, zoneNameForDriver(driver, zones))

		if waitMins >= 30 {
			// Driver accepts - remove them from their zone queue.
			fmt.Printf("  %s accepted the job\n", driver.Name)
			removeDriverFromZone(driver, zones)
			driver.Status = StatusBusy
			job.Driver = driver
			job.Status = JobAccepted
			booking.Status = BookingDispatched
			return job
		}

		fmt.Printf("  %s declined (only waited %d mins), trying next driver\n",
			driver.Name, int(waitMins))
	}

	// All candidates declined.
	fmt.Println("  All drivers declined - booking remains pending")
	job.Status = JobDeclined
	return job
}

// removeDriverFromZone removes a driver from their zone's trap queue.
func removeDriverFromZone(driver *Driver, zones []*Zone) {
	for _, z := range zones {
		for i, d := range z.Drivers {
			if d.ID == driver.ID {
				z.Drivers = append(z.Drivers[:i], z.Drivers[i+1:]...)
				return
			}
		}
	}
}

// zoneNameForDriver returns the zone name for a given driver, or "unknown".
func zoneNameForDriver(driver *Driver, zones []*Zone) string {
	for _, z := range zones {
		for _, d := range z.Drivers {
			if d.ID == driver.ID {
				return z.Name
			}
		}
	}
	// Driver may have already been removed - fall back to ZoneID.
	return driver.ZoneID
}

// ============================================================
// APP STATE
// ============================================================

// appState holds the live state of zones, drivers, and dispatch history.
var appState struct {
	Zones   []*Zone
	Drivers []*Driver
	Jobs    []*Job
}

// ============================================================
// WEB HANDLERS
// ============================================================

// pageData is the template context passed to handleIndex.
type pageData struct {
	Zones []*Zone
	Jobs  []*Job // most recent first
}

// indexTmpl is the single-page HTML UI for the dispatch demo.
var indexTmpl = template.Must(template.New("index").Funcs(template.FuncMap{
	// inc renders 1-based trap numbers in the template.
	"inc": func(i int) int { return i + 1 },
	// waitMins returns how many whole minutes a driver has been waiting.
	"waitMins": func(d *Driver) int { return int(time.Since(d.FreeAt).Minutes()) },
	// badgeClass maps a DriverStatus to the matching CSS class.
	"badgeClass": func(s DriverStatus) string {
		if s == StatusAvailable {
			return "badge-available"
		}
		return "badge-busy"
	},
	// zoneName resolves a zone ID to its display name.
	"zoneName": func(id string) string {
		for _, z := range appState.Zones {
			if z.ID == id {
				return z.Name
			}
		}
		return id
	},
}).Parse(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Southend Taxi Co-op - Dispatch Demo</title>
<link rel="stylesheet" href="https://cdnjs.cloudflare.com/ajax/libs/leaflet/1.9.4/leaflet.min.css"/>
<style>
*{box-sizing:border-box;margin:0;padding:0}
body{background:#0d0d1a;color:#e0e0e0;font-family:'Segoe UI',sans-serif;padding:28px;max-width:1100px;margin:0 auto}
h1{color:#4fc3f7;margin-bottom:28px;font-size:1.4rem;letter-spacing:.5px}
h2{color:#90caf9;font-size:.8rem;text-transform:uppercase;letter-spacing:1.5px;margin-bottom:14px;margin-top:32px}
h3{color:#4fc3f7;font-size:.9rem;margin-bottom:10px;border-bottom:1px solid #1e2a3a;padding-bottom:8px}
.zones-grid{display:grid;grid-template-columns:repeat(auto-fill,minmax(200px,1fr));gap:14px}
.zone-card{background:#131325;border:1px solid #1e2a3a;border-radius:8px;padding:14px}
.driver-row{display:flex;align-items:center;gap:8px;padding:6px 0;border-bottom:1px solid #1a1a2e;font-size:.82rem}
.driver-row:last-child{border-bottom:none}
.trap{color:#546e7a;font-size:.72rem;width:46px;flex-shrink:0}
.dname{flex:1;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
.wait{color:#546e7a;font-size:.72rem;text-align:right;flex-shrink:0}
.badge{font-size:.68rem;padding:2px 7px;border-radius:10px;flex-shrink:0}
.badge-available{background:#1b3a20;color:#81c784}
.badge-busy{background:#3a1a10;color:#ff8a65}
.empty{color:#37474f;font-size:.82rem;font-style:italic}
.form-card{background:#131325;border:1px solid #1e2a3a;border-radius:8px;padding:18px}
.form-row{display:flex;gap:12px;align-items:flex-end;flex-wrap:wrap}
.fg{display:flex;flex-direction:column;gap:5px}
label{font-size:.75rem;color:#78909c}
select,input[type=text]{background:#0d0d1a;border:1px solid #2a3a4a;color:#e0e0e0;padding:8px 12px;border-radius:6px;font-size:.88rem;min-width:150px}
select:focus,input:focus{outline:none;border-color:#4fc3f7}
button{background:#1565c0;color:#fff;border:none;padding:9px 22px;border-radius:6px;font-size:.88rem;cursor:pointer;height:36px}
button:hover{background:#1976d2}
.log-card{background:#131325;border:1px solid #1e2a3a;border-radius:8px;padding:14px}
.log-entry{display:grid;grid-template-columns:54px 1fr 1fr 1fr 80px;align-items:center;gap:10px;padding:9px 4px;border-bottom:1px solid #1a1a2e;font-size:.82rem}
.log-entry:last-child{border-bottom:none}
.log-id{color:#37474f;font-size:.72rem}
.log-zone{color:#78909c}
.log-driver{color:#90caf9}
.log-status-accepted{color:#66bb6a;text-align:right}
.log-status-declined{color:#ef5350;text-align:right}
.log-status-offered{color:#ffa726;text-align:right}
.no-jobs{color:#37474f;font-style:italic;font-size:.82rem}
#map{width:100%;height:450px;border-radius:8px;border:1px solid #1e2a3a}
.leaflet-popup-content-wrapper,.leaflet-popup-tip{background:#1a1a2e;color:#e0e0e0;border:1px solid #2a3a4a}
.leaflet-popup-content b{color:#4fc3f7}
</style>
</head>
<body>
<h1>Southend Taxi Cooperative &mdash; Dispatch Demo</h1>

<h2>Zone Trap Queues</h2>
<div class="zones-grid">
{{range .Zones}}<div class="zone-card">
  <h3>{{.Name}}</h3>
  {{if .Drivers}}{{range $i, $d := .Drivers}}<div class="driver-row">
    <span class="trap">Trap {{inc $i}}</span>
    <span class="dname">{{$d.Name}}</span>
    <span class="wait">{{waitMins $d}}m</span>
    <span class="badge {{badgeClass $d.Status}}">{{$d.Status}}</span>
  </div>{{end}}
  {{else}}<p class="empty">No drivers available</p>{{end}}
</div>
{{end}}
</div>

<h2>Live Driver Map</h2>
<div id="map"></div>

<h2>New Booking</h2>
<div class="form-card">
  <form action="/dispatch" method="POST">
    <div class="form-row">
      <div class="fg">
        <label>Pickup Zone</label>
        <select name="zone">{{range .Zones}}<option value="{{.ID}}">{{.Name}}</option>{{end}}</select>
      </div>
      <div class="fg">
        <label>Passenger Name</label>
        <input type="text" name="passenger" placeholder="e.g. Jane Smith" required>
      </div>
      <button type="submit">Dispatch</button>
    </div>
  </form>
</div>

<h2>Dispatch Log</h2>
<div class="log-card">
  {{if .Jobs}}
  {{range .Jobs}}<div class="log-entry">
    <span class="log-id">{{.ID}}</span>
    <span>{{.Booking.Passenger}}</span>
    <span class="log-zone">{{zoneName .Booking.PickupZone}}</span>
    <span class="log-driver">{{if .Driver}}{{.Driver.Name}}{{else}}&mdash;{{end}}</span>
    <span class="log-status-{{.Status}}">{{.Status}}</span>
  </div>{{end}}
  {{else}}<p class="no-jobs">No dispatches yet</p>{{end}}
</div>
<script src="https://cdnjs.cloudflare.com/ajax/libs/leaflet/1.9.4/leaflet.min.js"></script>
<script>
const map = L.map('map').setView([51.538, 0.711], 13);
L.tileLayer('https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png', {
  attribution: '&copy; OpenStreetMap contributors',
  maxZoom: 18
}).addTo(map);

const driversLayer = L.layerGroup().addTo(map);

function refreshDrivers() {
  fetch('/api/drivers')
    .then(r => r.json())
    .then(drivers => {
      driversLayer.clearLayers();
      drivers.forEach(d => {
        L.circleMarker([d.lat, d.lng], {
          radius: 9,
          fillColor: d.status === 'available' ? '#4caf50' : '#f44336',
          color: '#ffffff',
          weight: 2,
          opacity: 1,
          fillOpacity: 0.85
        })
        .bindPopup('<b>' + d.name + '</b><br>' + d.zone + ' zone<br>' + d.status)
        .addTo(driversLayer);
      });
    })
    .catch(() => {}); // silently ignore network errors during refresh
}

refreshDrivers();
setInterval(refreshDrivers, 5000);
</script>
</body>
</html>
`))

// handleDriverData returns all drivers as JSON for the live map.
func handleDriverData(w http.ResponseWriter, r *http.Request) {
	type driverJSON struct {
		Name   string  `json:"name"`
		Status string  `json:"status"`
		Zone   string  `json:"zone"`
		Lat    float64 `json:"lat"`
		Lng    float64 `json:"lng"`
	}

	data := make([]driverJSON, 0, len(appState.Drivers))
	for _, d := range appState.Drivers {
		zoneName := d.ZoneID
		for _, z := range appState.Zones {
			if z.ID == d.ZoneID {
				zoneName = z.Name
				break
			}
		}
		data = append(data, driverJSON{
			Name:   d.Name,
			Status: string(d.Status),
			Zone:   zoneName,
			Lat:    d.Lat,
			Lng:    d.Lng,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

// handleIndex renders the main dispatch UI.
func handleIndex(w http.ResponseWriter, r *http.Request) {
	// Reverse job slice so most recent appears first.
	reversed := make([]*Job, len(appState.Jobs))
	for i, j := range appState.Jobs {
		reversed[len(appState.Jobs)-1-i] = j
	}
	if err := indexTmpl.Execute(w, pageData{Zones: appState.Zones, Jobs: reversed}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// handleDispatch processes a booking form POST and redirects back to the index.
func handleDispatch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	passenger := r.FormValue("passenger")
	zoneID := r.FormValue("zone")
	if passenger == "" {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	booking := &Booking{
		ID:         fmt.Sprintf("B%03d", len(appState.Jobs)+1),
		Passenger:  passenger,
		PickupZone: zoneID,
		Source:     SourceApp,
		Status:     BookingPending,
		CreatedAt:  time.Now(),
	}

	job := dispatchJob(booking, appState.Zones)
	appState.Jobs = append(appState.Jobs, job)

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// ============================================================
// MAIN
// ============================================================

func main() {
	appState.Zones, appState.Drivers = seedData()

	http.HandleFunc("/", handleIndex)
	http.HandleFunc("/dispatch", handleDispatch)
	http.HandleFunc("/api/drivers", handleDriverData)

	fmt.Println("Server starting on http://localhost:8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		fmt.Printf("Server error: %v\n", err)
	}
}
