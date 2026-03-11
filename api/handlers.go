// ============================================================
// Taxidemo - Southend Taxi Cooperative Dispatch Demo
// Version: 0.1.0
// Package api: HTTP handlers and web UI template
// ============================================================

package api

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"time"

	"taxidemo/dispatch"
	"taxidemo/models"
)

// AppState holds the live state of zones, drivers, and dispatch history.
var AppState struct {
	Zones   []*models.Zone
	Drivers []*models.Driver
	Jobs    []*models.Job
}

// pageData is the template context passed to HandleIndex.
type pageData struct {
	Zones []*models.Zone
	Jobs  []*models.Job // most recent first
}

// indexTmpl is the single-page HTML UI for the dispatch demo.
var indexTmpl = template.Must(template.New("index").Funcs(template.FuncMap{
	// inc renders 1-based trap numbers in the template.
	"inc": func(i int) int { return i + 1 },
	// waitMins returns how many whole minutes a driver has been waiting.
	"waitMins": func(d *models.Driver) int { return int(time.Since(d.FreeAt).Minutes()) },
	// badgeClass maps a DriverStatus to the matching CSS class.
	"badgeClass": func(s models.DriverStatus) string {
		if s == models.StatusAvailable {
			return "badge-available"
		}
		return "badge-busy"
	},
	// zoneName resolves a zone ID to its display name.
	"zoneName": func(id string) string {
		for _, z := range AppState.Zones {
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
const bookingsLayer = L.layerGroup().addTo(map);

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
    .catch(() => {});
}

function refreshBookings() {
  fetch('/api/bookings')
    .then(r => r.json())
    .then(bookings => {
      bookingsLayer.clearLayers();
      bookings.forEach(b => {
        const driver = b.driver ? b.driver : 'Unassigned';
        const dest = b.destination ? b.destination : 'Not specified';
        L.circleMarker([b.lat, b.lng], {
          radius: 12,
          fillColor: '#1565c0',
          color: '#90caf9',
          weight: 2,
          opacity: 1,
          fillOpacity: 0.75
        })
        .bindPopup(
          '<b>' + b.passenger + '</b><br>' +
          'To: ' + dest + '<br>' +
          'Driver: ' + driver + '<br>' +
          'Status: ' + b.status
        )
        .addTo(bookingsLayer);
      });
    })
    .catch(() => {});
}

refreshDrivers();
refreshBookings();
setInterval(refreshDrivers, 5000);
setInterval(refreshBookings, 5000);
</script>
</body>
</html>
`))

// HandleDriverData returns all drivers as JSON for the live map.
func HandleDriverData(w http.ResponseWriter, r *http.Request) {
	type driverJSON struct {
		Name   string  `json:"name"`
		Status string  `json:"status"`
		Zone   string  `json:"zone"`
		Lat    float64 `json:"lat"`
		Lng    float64 `json:"lng"`
	}

	data := make([]driverJSON, 0, len(AppState.Drivers))
	for _, d := range AppState.Drivers {
		zoneName := d.ZoneID
		for _, z := range AppState.Zones {
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

// HandleBookingData returns all active jobs as JSON for the live map.
func HandleBookingData(w http.ResponseWriter, r *http.Request) {
	type bookingJSON struct {
		ID          string  `json:"id"`
		Passenger   string  `json:"passenger"`
		Zone        string  `json:"zone"`
		Destination string  `json:"destination"`
		Driver      string  `json:"driver"`
		Status      string  `json:"status"`
		Lat         float64 `json:"lat"`
		Lng         float64 `json:"lng"`
	}

	data := make([]bookingJSON, 0, len(AppState.Jobs))
	for _, j := range AppState.Jobs {
		driverName := ""
		if j.Driver != nil {
			driverName = j.Driver.Name
		}
		zoneName := j.Booking.PickupZone
		for _, z := range AppState.Zones {
			if z.ID == j.Booking.PickupZone {
				zoneName = z.Name
				break
			}
		}
		data = append(data, bookingJSON{
			ID:          j.Booking.ID,
			Passenger:   j.Booking.Passenger,
			Zone:        zoneName,
			Destination: j.Booking.Destination,
			Driver:      driverName,
			Status:      string(j.Status),
			Lat:         j.Booking.Lat,
			Lng:         j.Booking.Lng,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

// HandleIndex renders the main dispatch UI.
func HandleIndex(w http.ResponseWriter, r *http.Request) {
	// Reverse job slice so most recent appears first.
	reversed := make([]*models.Job, len(AppState.Jobs))
	for i, j := range AppState.Jobs {
		reversed[len(AppState.Jobs)-1-i] = j
	}
	if err := indexTmpl.Execute(w, pageData{Zones: AppState.Zones, Jobs: reversed}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// HandleDispatch processes a booking form POST and redirects back to the index.
func HandleDispatch(w http.ResponseWriter, r *http.Request) {
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

	booking := &models.Booking{
		ID:         fmt.Sprintf("B%03d", len(AppState.Jobs)+1),
		Passenger:  passenger,
		PickupZone: zoneID,
		Source:     models.SourceApp,
		Status:     models.BookingPending,
		CreatedAt:  time.Now(),
	}

	job := dispatch.DispatchJob(booking, AppState.Zones)
	AppState.Jobs = append(AppState.Jobs, job)

	http.Redirect(w, r, "/", http.StatusSeeOther)
}
