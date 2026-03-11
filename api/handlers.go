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
html,body{height:100%;overflow:hidden}
body{background:#0d0d1a;color:#e0e0e0;font-family:'Segoe UI',sans-serif;display:flex;flex-direction:column}
/* Top bar */
.topbar{flex-shrink:0;background:#0a0a14;border-bottom:1px solid #1e2a3a;padding:8px 16px;display:flex;align-items:center;gap:16px}
.topbar h1{color:#4fc3f7;font-size:1rem;letter-spacing:.5px;white-space:nowrap;margin-right:auto}
.topbar form{display:flex;align-items:center;gap:10px}
.topbar .fg{display:flex;align-items:center;gap:5px}
.topbar label{font-size:.72rem;color:#78909c;white-space:nowrap}
.topbar select,.topbar input[type=text]{background:#0d0d1a;border:1px solid #2a3a4a;color:#e0e0e0;padding:5px 9px;border-radius:5px;font-size:.83rem}
.topbar input[type=text]{min-width:160px}
.topbar select:focus,.topbar input:focus{outline:none;border-color:#4fc3f7}
.topbar button{background:#1565c0;color:#fff;border:none;padding:6px 16px;border-radius:5px;font-size:.83rem;cursor:pointer}
.topbar button:hover{background:#1976d2}
/* Main two-column layout */
.main{flex:1;display:flex;overflow:hidden}
/* Left column — zone queues */
.left-col{width:35%;min-width:240px;max-width:380px;overflow-y:auto;padding:10px 12px;border-right:1px solid #1e2a3a;flex-shrink:0}
.left-col h2{color:#90caf9;font-size:.7rem;text-transform:uppercase;letter-spacing:1.5px;margin-bottom:8px}
.zones-grid{display:flex;flex-direction:column;gap:6px}
.zone-card{background:#131325;border:1px solid #1e2a3a;border-radius:6px;padding:7px 9px}
.zone-card h3{color:#4fc3f7;font-size:.76rem;margin-bottom:5px;border-bottom:1px solid #1e2a3a;padding-bottom:4px}
.driver-row{display:flex;align-items:center;gap:6px;padding:2px 0;border-bottom:1px solid #1a1a2e;font-size:.74rem}
.driver-row:last-child{border-bottom:none}
.trap{color:#546e7a;font-size:.66rem;width:40px;flex-shrink:0}
.dname{flex:1;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
.wait{color:#546e7a;font-size:.66rem;text-align:right;flex-shrink:0}
.badge{font-size:.62rem;padding:1px 5px;border-radius:8px;flex-shrink:0}
.badge-available{background:#1b3a20;color:#81c784}
.badge-busy{background:#3a1a10;color:#ff8a65}
.empty{color:#37474f;font-size:.74rem;font-style:italic}
/* Right column — map + log */
.right-col{flex:1;display:flex;flex-direction:column;overflow:hidden;padding:10px 12px;gap:8px;min-width:0}
#map{flex:1;min-height:500px;border-radius:8px;border:1px solid #1e2a3a}
/* Dispatch log */
.log-section{flex-shrink:0}
.log-section h2{color:#90caf9;font-size:.7rem;text-transform:uppercase;letter-spacing:1.5px;margin-bottom:6px}
.log-card{background:#131325;border:1px solid #1e2a3a;border-radius:6px;padding:8px 10px;max-height:180px;overflow-y:auto}
.log-entry{display:grid;grid-template-columns:52px 1fr 1fr 1fr 80px;align-items:center;gap:8px;padding:5px 2px;border-bottom:1px solid #1a1a2e;font-size:.76rem}
.log-entry:last-child{border-bottom:none}
.log-id{color:#37474f;font-size:.68rem}
.log-zone{color:#78909c}
.log-driver{color:#90caf9}
.log-status-accepted{color:#66bb6a;text-align:right}
.log-status-declined{color:#ef5350;text-align:right}
.log-status-offered{color:#ffa726;text-align:right}
.no-jobs{color:#37474f;font-style:italic;font-size:.76rem}
.leaflet-popup-content-wrapper,.leaflet-popup-tip{background:#1a1a2e;color:#e0e0e0;border:1px solid #2a3a4a}
.leaflet-popup-content b{color:#4fc3f7}
</style>
</head>
<body>

<div class="topbar">
  <h1>Southend Taxi Cooperative &mdash; Dispatch</h1>
  <form action="/dispatch" method="POST">
    <div class="fg"><label>Zone</label><select name="zone">{{range .Zones}}<option value="{{.ID}}">{{.Name}}</option>{{end}}</select></div>
    <div class="fg"><label>Passenger</label><input type="text" name="passenger" placeholder="e.g. Jane Smith" required></div>
    <button type="submit">Dispatch</button>
  </form>
</div>

<div class="main">
  <div class="left-col">
    <h2>Zone Trap Queues</h2>
    <div class="zones-grid">
    {{range .Zones}}<div class="zone-card">
      <h3>{{.Name}}</h3>
      {{if .Drivers}}{{range $i, $d := .Drivers}}<div class="driver-row">
        <span class="trap">T{{inc $i}}</span>
        <span class="dname">{{$d.Name}}</span>
        <span class="wait">{{waitMins $d}}m</span>
        <span class="badge {{badgeClass $d.Status}}">{{$d.Status}}</span>
      </div>{{end}}
      {{else}}<p class="empty">No drivers</p>{{end}}
    </div>{{end}}
    </div>
  </div>

  <div class="right-col">
    <div id="map"></div>
    <div class="log-section">
      <h2>Dispatch Log</h2>
      <div class="log-card">
        {{if .Jobs}}{{range .Jobs}}<div class="log-entry">
          <span class="log-id">{{.ID}}</span>
          <span>{{.Booking.Passenger}}</span>
          <span class="log-zone">{{zoneName .Booking.PickupZone}}</span>
          <span class="log-driver">{{if .Driver}}{{.Driver.Name}}{{else}}&mdash;{{end}}</span>
          <span class="log-status-{{.Status}}">{{.Status}}</span>
        </div>{{end}}
        {{else}}<p class="no-jobs">No dispatches yet</p>{{end}}
      </div>
    </div>
  </div>
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
const bookingLinesLayer = L.layerGroup().addTo(map);
const zonesLayer = L.layerGroup().addTo(map);

const zoneColors = [
  '#e74c3c','#e67e22','#f1c40f','#2ecc71','#1abc9c',
  '#3498db','#9b59b6','#e91e63','#ff5722','#8bc34a',
  '#00bcd4','#673ab7','#ff9800','#4caf50','#2196f3',
  '#f44336','#009688','#cddc39','#ff4081','#7c4dff',
  '#64ffda','#ffd740'
];

const zoneBounds = [
  {id:'Z01',name:'Progress',  coords:[[51.572,0.658],[51.572,0.688],[51.562,0.688],[51.562,0.658]]},
  {id:'Z02',name:'Thanet',    coords:[[51.572,0.688],[51.572,0.718],[51.562,0.718],[51.562,0.688]]},
  {id:'Z03',name:'Blue',      coords:[[51.572,0.718],[51.572,0.748],[51.562,0.748],[51.562,0.718]]},
  {id:'Z04',name:'Fairway',   coords:[[51.562,0.648],[51.562,0.678],[51.552,0.678],[51.552,0.648]]},
  {id:'Z05',name:'Blenheim',  coords:[[51.562,0.678],[51.562,0.708],[51.552,0.708],[51.552,0.678]]},
  {id:'Z06',name:'Temple',    coords:[[51.562,0.708],[51.562,0.738],[51.552,0.738],[51.552,0.708]]},
  {id:'Z07',name:'Fossett',   coords:[[51.562,0.738],[51.562,0.758],[51.552,0.758],[51.552,0.738]]},
  {id:'Z08',name:'Highlands', coords:[[51.552,0.638],[51.552,0.665],[51.542,0.665],[51.542,0.638]]},
  {id:'Z09',name:'Elms',      coords:[[51.552,0.665],[51.552,0.688],[51.542,0.688],[51.542,0.665]]},
  {id:'Z10',name:'Ross',      coords:[[51.552,0.688],[51.552,0.708],[51.542,0.708],[51.542,0.688]]},
  {id:'Z11',name:'Plough',    coords:[[51.552,0.708],[51.552,0.725],[51.542,0.725],[51.542,0.708]]},
  {id:'Z12',name:'Priory',    coords:[[51.552,0.725],[51.552,0.745],[51.542,0.745],[51.542,0.725]]},
  {id:'Z13',name:'VAC',       coords:[[51.552,0.745],[51.552,0.762],[51.542,0.762],[51.542,0.745]]},
  {id:'Z14',name:'Green',     coords:[[51.552,0.762],[51.552,0.778],[51.542,0.778],[51.542,0.762]]},
  {id:'Z15',name:'Broadway',  coords:[[51.542,0.635],[51.542,0.660],[51.532,0.660],[51.532,0.635]]},
  {id:'Z16',name:'Chalkwell', coords:[[51.542,0.648],[51.542,0.672],[51.530,0.672],[51.530,0.648]]},
  {id:'Z17',name:'Westcliff', coords:[[51.542,0.672],[51.542,0.695],[51.528,0.695],[51.528,0.672]]},
  {id:'Z18',name:'Town',      coords:[[51.542,0.695],[51.542,0.722],[51.528,0.722],[51.528,0.695]]},
  {id:'Z19',name:'Kursaal',   coords:[[51.542,0.722],[51.542,0.742],[51.528,0.742],[51.528,0.722]]},
  {id:'Z20',name:'Thorpe',    coords:[[51.542,0.742],[51.542,0.762],[51.528,0.762],[51.528,0.742]]},
  {id:'Z21',name:'Bay',       coords:[[51.535,0.762],[51.535,0.785],[51.522,0.785],[51.522,0.762]]},
  {id:'Z22',name:'Shoebury',  coords:[[51.535,0.785],[51.535,0.810],[51.522,0.810],[51.522,0.785]]},
];

function refreshZones() {
  fetch('/api/zones')
    .then(r => r.json())
    .then(zones => {
      const countMap = {};
      zones.forEach(z => { countMap[z.id] = z.driver_count; });
      zonesLayer.clearLayers();
      zoneBounds.forEach((z, i) => {
        const color = zoneColors[i % zoneColors.length];
        const count = countMap[z.id] !== undefined ? countMap[z.id] : 0;
        L.polygon(z.coords, {
          color: color,
          weight: 2,
          opacity: 0.8,
          fillColor: color,
          fillOpacity: 0.2
        })
        .bindPopup('<b>' + z.name + '</b><br>Drivers: ' + count)
        .addTo(zonesLayer);
      });
    })
    .catch(() => {});
}

function refreshDrivers() {
  fetch('/api/drivers')
    .then(r => r.json())
    .then(drivers => {
      driversLayer.clearLayers();
      drivers.forEach(d => {
        const bg = d.status === 'available' ? '#2e7d32' : '#c62828';
        const border = d.status === 'available' ? '#81c784' : '#ef9a9a';
        const icon = L.divIcon({
          className: '',
          html: '<div style="background:' + bg + ';border:2px solid ' + border + ';border-radius:4px;padding:2px 5px;color:#fff;font-weight:bold;font-size:11px;font-family:sans-serif;white-space:nowrap;line-height:1.4">' + d.plate + '</div>',
          iconAnchor: [16, 10]
        });
        L.marker([d.lat, d.lng], { icon })
          .bindPopup('<b>' + d.name + '</b><br>Plate: ' + d.plate + '<br>' + d.zone + ' zone<br>' + d.status)
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
      bookingLinesLayer.clearLayers();
      bookings.forEach(b => {
        const driver = b.driver ? b.driver : 'Unassigned';
        const dest = b.destination ? b.destination : 'Not specified';
        const popup =
          '<b>' + b.passenger + '</b><br>' +
          'To: ' + dest + '<br>' +
          'Driver: ' + driver + '<br>' +
          'Status: ' + b.status;

        // Blue pickup marker
        L.circleMarker([b.lat, b.lng], {
          radius: 10,
          fillColor: '#1565c0',
          color: '#90caf9',
          weight: 2,
          opacity: 1,
          fillOpacity: 0.85
        })
        .bindPopup(popup)
        .addTo(bookingsLayer);

        // Purple destination marker and dashed line (when coordinates are set)
        if (b.dest_lat && b.dest_lng) {
          L.circleMarker([b.dest_lat, b.dest_lng], {
            radius: 10,
            fillColor: '#7b1fa2',
            color: '#ce93d8',
            weight: 2,
            opacity: 1,
            fillOpacity: 0.85
          })
          .bindPopup(popup)
          .addTo(bookingsLayer);

          L.polyline([[b.lat, b.lng], [b.dest_lat, b.dest_lng]], {
            color: '#ce93d8',
            weight: 2,
            opacity: 0.7,
            dashArray: '6, 6'
          })
          .addTo(bookingLinesLayer);
        }
      });
    })
    .catch(() => {});
}

refreshZones();
refreshDrivers();
refreshBookings();
setInterval(refreshZones, 5000);
setInterval(refreshDrivers, 5000);
setInterval(refreshBookings, 5000);
</script>
</body>
</html>
`))

// HandleDriverData returns all drivers as JSON for the live map.
func HandleDriverData(w http.ResponseWriter, r *http.Request) {
	type driverJSON struct {
		Name        string  `json:"name"`
		Status      string  `json:"status"`
		Zone        string  `json:"zone"`
		Lat         float64 `json:"lat"`
		Lng         float64 `json:"lng"`
		PlateNumber int     `json:"plate"`
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
			Name:        d.Name,
			Status:      string(d.Status),
			Zone:        zoneName,
			Lat:         d.Lat,
			Lng:         d.Lng,
			PlateNumber: d.PlateNumber,
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
		DestLat     float64 `json:"dest_lat"`
		DestLng     float64 `json:"dest_lng"`
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
			DestLat:     j.Booking.DestLat,
			DestLng:     j.Booking.DestLng,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

// HandleZoneData returns all zones with their current driver counts as JSON.
func HandleZoneData(w http.ResponseWriter, r *http.Request) {
	type zoneJSON struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		DriverCount int    `json:"driver_count"`
	}

	data := make([]zoneJSON, 0, len(AppState.Zones))
	for _, z := range AppState.Zones {
		available := 0
		for _, d := range z.Drivers {
			if d.Status == models.StatusAvailable {
				available++
			}
		}
		data = append(data, zoneJSON{
			ID:          z.ID,
			Name:        z.Name,
			DriverCount: available,
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
