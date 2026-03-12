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
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"taxidemo/dispatch"
	"taxidemo/models"
)

// Handler holds application state and serves HTTP requests.
type Handler struct {
	State *models.AppState
	tmpl  *template.Template
}

// pageData is the template context passed to HandleIndex.
type pageData struct {
	Zones []*models.Zone
	Jobs  []*models.Job // most recent first
}

// RegisterRoutes builds the HTML template (with closures over h.State) and
// registers all HTTP routes on the provided ServeMux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	h.tmpl = template.Must(template.New("index").Funcs(template.FuncMap{
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
			h.State.Mu.RLock()
			defer h.State.Mu.RUnlock()
			for _, z := range h.State.Zones {
				if z.ID == id {
					return z.Name
				}
			}
			return id
		},
	}).Parse(indexHTML))

	mux.HandleFunc("/", h.HandleIndex)
	mux.HandleFunc("/dispatch", h.HandleDispatch)
	mux.HandleFunc("/api/drivers", h.HandleDriverData)
	mux.HandleFunc("/api/bookings", h.HandleBookingData)
	mux.HandleFunc("/api/zones", h.HandleZoneData)
	mux.HandleFunc("/api/geocode", h.HandleGeocode)
	mux.HandleFunc("/api/booking/new", h.HandleNewBooking)
	mux.HandleFunc("/api/booking/complete", h.HandleCompleteBooking)
	mux.HandleFunc("/api/booking/cancel", h.HandleCancelBooking)
	mux.HandleFunc("/api/prebooks", h.HandlePrebookData)
}

// HandleIndex renders the main dispatch UI.
func (h *Handler) HandleIndex(w http.ResponseWriter, r *http.Request) {
	// Snapshot zones and jobs under a read lock so the template executes
	// against a consistent copy and does not hold the lock during rendering.
	h.State.Mu.RLock()
	zones := make([]*models.Zone, len(h.State.Zones))
	copy(zones, h.State.Zones)
	jobs := make([]*models.Job, len(h.State.Jobs))
	copy(jobs, h.State.Jobs)
	h.State.Mu.RUnlock()

	// Reverse job slice so most recent appears first.
	reversed := make([]*models.Job, len(jobs))
	for i, j := range jobs {
		reversed[len(jobs)-1-i] = j
	}
	if err := h.tmpl.Execute(w, pageData{Zones: zones, Jobs: reversed}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// HandleDispatch processes a booking form POST and redirects back to the index.
func (h *Handler) HandleDispatch(w http.ResponseWriter, r *http.Request) {
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

	h.State.Mu.Lock()
	booking := &models.Booking{
		ID:         fmt.Sprintf("B%03d", len(h.State.Jobs)+1),
		Passenger:  passenger,
		PickupZone: zoneID,
		Source:     models.SourceApp,
		Status:     models.BookingPending,
		CreatedAt:  time.Now(),
	}
	job := dispatch.DispatchJob(booking, h.State.Zones)
	h.State.Jobs = append(h.State.Jobs, job)
	h.State.Mu.Unlock()

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// HandleDriverData returns all drivers as JSON for the live map.
func (h *Handler) HandleDriverData(w http.ResponseWriter, r *http.Request) {
	type driverJSON struct {
		Name        string  `json:"name"`
		Status      string  `json:"status"`
		Zone        string  `json:"zone"`
		Lat         float64 `json:"lat"`
		Lng         float64 `json:"lng"`
		PlateNumber int     `json:"plate"`
	}

	h.State.Mu.RLock()
	data := make([]driverJSON, 0, len(h.State.Drivers))
	for _, d := range h.State.Drivers {
		zoneName := d.ZoneID
		for _, z := range h.State.Zones {
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
	h.State.Mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

// HandleBookingData returns all active jobs as JSON for the live map.
func (h *Handler) HandleBookingData(w http.ResponseWriter, r *http.Request) {
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

	h.State.Mu.RLock()
	data := make([]bookingJSON, 0, len(h.State.Jobs))
	for _, j := range h.State.Jobs {
		driverName := ""
		if j.Driver != nil {
			driverName = j.Driver.Name
		}
		zoneName := j.Booking.PickupZone
		for _, z := range h.State.Zones {
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
	h.State.Mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

// HandleZoneData returns all zones with their current driver counts as JSON.
func (h *Handler) HandleZoneData(w http.ResponseWriter, r *http.Request) {
	type zoneJSON struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		DriverCount int    `json:"driver_count"`
	}

	h.State.Mu.RLock()
	data := make([]zoneJSON, 0, len(h.State.Zones))
	for _, z := range h.State.Zones {
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
	h.State.Mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

// HandleGeocode proxies an address lookup to Nominatim and returns lat/lng.
func (h *Handler) HandleGeocode(w http.ResponseWriter, r *http.Request) {
	address := r.URL.Query().Get("address")
	if address == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `{"error":"address required"}`)
		return
	}

	query := address
	lower := strings.ToLower(address)
	if !strings.Contains(lower, "southend") && !strings.Contains(lower, "essex") {
		query = address + ", Southend-on-Sea, Essex, UK"
	}

	params := url.Values{}
	params.Set("q", query)
	params.Set("format", "json")
	params.Set("limit", "1")
	params.Set("countrycodes", "gb")
	reqURL := "https://nominatim.openstreetmap.org/search?" + params.Encode()

	client := &http.Client{Timeout: 5 * time.Second}
	req, err := http.NewRequest(http.MethodGet, reqURL, nil)
	if err != nil {
		http.Error(w, `{"error":"request build failed"}`, http.StatusInternalServerError)
		return
	}
	req.Header.Set("User-Agent", "SouthendTaxiCooperative/2.0 (dispatch-demo)")

	resp, err := client.Do(req)
	if err != nil {
		http.Error(w, `{"error":"geocode request failed"}`, http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		http.Error(w, `{"error":"geocode read failed"}`, http.StatusBadGateway)
		return
	}

	var results []struct {
		Lat string `json:"lat"`
		Lon string `json:"lon"`
	}
	if err := json.Unmarshal(body, &results); err != nil || len(results) == 0 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"error":"address not found"}`)
		return
	}

	lat, _ := strconv.ParseFloat(results[0].Lat, 64)
	lng, _ := strconv.ParseFloat(results[0].Lon, 64)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(struct {
		Lat float64 `json:"lat"`
		Lng float64 `json:"lng"`
	}{lat, lng})
}

// HandleNewBooking creates an immediate or pre-booked job from a JSON POST body.
func (h *Handler) HandleNewBooking(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"POST required"}`, http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Type          string  `json:"type"`
		CustomerName  string  `json:"customer_name"`
		Phone         string  `json:"phone"`
		Notes         string  `json:"notes"`
		IsAccount     bool    `json:"is_account"`
		PickupAddress string  `json:"pickup_address"`
		PickupLat     float64 `json:"pickup_lat"`
		PickupLng     float64 `json:"pickup_lng"`
		DestAddress   string  `json:"dest_address"`
		DestLat       float64 `json:"dest_lat"`
		DestLng       float64 `json:"dest_lng"`
		PickupZone    string  `json:"pickup_zone"`
		RequestedTime string  `json:"requested_time"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
		return
	}

	var requestedTime time.Time
	if req.Type == string(models.BookingPrebook) {
		var err error
		requestedTime, err = time.Parse(time.RFC3339Nano, req.RequestedTime)
		if err != nil {
			requestedTime, err = time.Parse(time.RFC3339, req.RequestedTime)
		}
		if err != nil {
			requestedTime, err = time.ParseInLocation("2006-01-02T15:04:05", req.RequestedTime, time.Local)
		}
		if err != nil {
			requestedTime, err = time.ParseInLocation("2006-01-02T15:04", req.RequestedTime, time.Local)
		}
		if err != nil {
			http.Error(w, `{"error":"invalid requested_time"}`, http.StatusBadRequest)
			return
		}
	} else {
		requestedTime = time.Now()
	}

	// Derive pickup zone from coordinates if available; fall back to any
	// zone the client sent (legacy form path) or the geographic centre.
	pickupZone := req.PickupZone
	if req.PickupLat != 0 || req.PickupLng != 0 {
		pickupZone = dispatch.NearestZone(req.PickupLat, req.PickupLng)
	}

	booking := &models.Booking{
		ID:            dispatch.GenerateID("BK"),
		Passenger:     req.CustomerName,
		CustomerName:  req.CustomerName,
		Phone:         req.Phone,
		Notes:         req.Notes,
		IsAccount:     req.IsAccount,
		PickupAddress: req.PickupAddress,
		PickupZone:    pickupZone,
		Lat:           req.PickupLat,
		Lng:           req.PickupLng,
		DestAddress:   req.DestAddress,
		DestLat:       req.DestLat,
		DestLng:       req.DestLng,
		Source:        models.SourceApp,
		Status:        models.BookingPending,
		Type:          models.BookingType(req.Type),
		CreatedAt:     time.Now(),
		RequestedTime: requestedTime,
	}

	h.State.Mu.Lock()
	h.State.Bookings = append(h.State.Bookings, booking)
	if req.Type == string(models.BookingImmediate) {
		job := dispatch.DispatchJob(booking, h.State.Zones)
		h.State.Jobs = append(h.State.Jobs, job)
	}
	h.State.Mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(booking)
}

// HandleCompleteBooking marks a booking as completed and frees its driver.
func (h *Handler) HandleCompleteBooking(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"POST required"}`, http.StatusMethodNotAllowed)
		return
	}
	id := r.URL.Query().Get("id")
	if err := dispatch.CompleteBooking(id, h.State); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprintf(w, `{"error":%q}`, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprint(w, `{"ok":true}`)
}

// HandleCancelBooking marks a booking as cancelled and frees any assigned driver.
func (h *Handler) HandleCancelBooking(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"POST required"}`, http.StatusMethodNotAllowed)
		return
	}
	id := r.URL.Query().Get("id")
	if err := dispatch.CancelBooking(id, h.State); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprintf(w, `{"error":%q}`, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprint(w, `{"ok":true}`)
}

// HandlePrebookData returns pre-booked bookings, optionally filtered by status.
// Query param: filter = active | completed | all (default: all)
func (h *Handler) HandlePrebookData(w http.ResponseWriter, r *http.Request) {
	filter := r.URL.Query().Get("filter")
	if filter == "" {
		filter = "all"
	}

	h.State.Mu.RLock()
	all := make([]*models.Booking, len(h.State.Bookings))
	copy(all, h.State.Bookings)
	h.State.Mu.RUnlock()

	type prebookJSON struct {
		ID            string  `json:"id"`
		CustomerName  string  `json:"customer_name"`
		Phone         string  `json:"phone"`
		PickupAddress string  `json:"pickup_address"`
		DestAddress   string  `json:"dest_address"`
		PickupZone    string  `json:"pickup_zone"`
		Status        string  `json:"status"`
		Type          string  `json:"type"`
		RequestedTime string  `json:"requested_time"`
		Notes         string  `json:"notes"`
		IsAccount     bool    `json:"is_account"`
		Lat           float64 `json:"lat"`
		Lng           float64 `json:"lng"`
		DestLat       float64 `json:"dest_lat"`
		DestLng       float64 `json:"dest_lng"`
	}

	toJSON := func(b *models.Booking) prebookJSON {
		return prebookJSON{
			ID:            b.ID,
			CustomerName:  b.CustomerName,
			Phone:         b.Phone,
			PickupAddress: b.PickupAddress,
			DestAddress:   b.DestAddress,
			PickupZone:    b.PickupZone,
			Status:        string(b.Status),
			Type:          string(b.Type),
			RequestedTime: b.RequestedTime.Format(time.RFC3339),
			Notes:         b.Notes,
			IsAccount:     b.IsAccount,
			Lat:           b.Lat,
			Lng:           b.Lng,
			DestLat:       b.DestLat,
			DestLng:       b.DestLng,
		}
	}

	var active, completed []*models.Booking
	for _, b := range all {
		if b.Status == models.BookingCompleted || b.Status == models.BookingCancelled {
			completed = append(completed, b)
		} else {
			active = append(active, b)
		}
	}

	// Sort active by RequestedTime ascending.
	sort.Slice(active, func(i, j int) bool {
		return active[i].RequestedTime.Before(active[j].RequestedTime)
	})

	// Sort completed by CompletedAt descending; nil CompletedAt sorts last.
	sort.Slice(completed, func(i, j int) bool {
		if completed[i].CompletedAt == nil {
			return false
		}
		if completed[j].CompletedAt == nil {
			return true
		}
		return completed[i].CompletedAt.After(*completed[j].CompletedAt)
	})

	var src []*models.Booking
	switch filter {
	case "active":
		src = active
	case "completed":
		src = completed
	default:
		src = append(active, completed...)
	}

	result := make([]prebookJSON, 0, len(src))
	for _, b := range src {
		result = append(result, toJSON(b))
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// indexHTML is the complete single-page dispatch UI template.
const indexHTML = `<!DOCTYPE html>
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
<div style="margin-top:12px;background:#0d0d1a;border:1px solid #333;border-radius:6px;overflow:hidden;">
  <div style="display:flex;align-items:center;justify-content:space-between;padding:10px 14px;background:#111122;border-bottom:1px solid #333;">
    <span style="color:#00d4ff;font-family:monospace;font-size:13px;font-weight:bold;">📋 BOOKINGS</span>
    <div style="display:flex;gap:6px;">
      <button onclick="setJobFilter('active')" id="filterActive" style="padding:4px 10px;font-family:monospace;font-size:11px;border-radius:3px;cursor:pointer;border:1px solid #00d4ff;background:#00d4ff22;color:#00d4ff;">ACTIVE</button>
      <button onclick="setJobFilter('completed')" id="filterCompleted" style="padding:4px 10px;font-family:monospace;font-size:11px;border-radius:3px;cursor:pointer;border:1px solid #444;background:transparent;color:#666;">COMPLETED</button>
      <button onclick="setJobFilter('all')" id="filterAll" style="padding:4px 10px;font-family:monospace;font-size:11px;border-radius:3px;cursor:pointer;border:1px solid #444;background:transparent;color:#666;">ALL</button>
      <button onclick="openPrebookModal()" style="padding:4px 12px;font-family:monospace;font-size:11px;border-radius:3px;cursor:pointer;border:none;background:#00d4ff;color:#000;font-weight:bold;margin-left:8px;">+ NEW BOOKING</button>
    </div>
  </div>
  <div style="overflow-x:auto;">
    <table id="jobsTable" style="width:100%;border-collapse:collapse;font-family:monospace;font-size:12px;">
      <thead>
        <tr style="background:#111122;color:#666;text-align:left;">
          <th style="padding:8px 10px;border-bottom:1px solid #222;">TIME</th>
          <th style="padding:8px 10px;border-bottom:1px solid #222;">PASSENGER</th>
          <th style="padding:8px 10px;border-bottom:1px solid #222;">PICKUP</th>
          <th style="padding:8px 10px;border-bottom:1px solid #222;">ZONE</th>
          <th style="padding:8px 10px;border-bottom:1px solid #222;">DESTINATION</th>
          <th style="padding:8px 10px;border-bottom:1px solid #222;">DRIVER</th>
          <th style="padding:8px 10px;border-bottom:1px solid #222;">STATUS</th>
          <th style="padding:8px 10px;border-bottom:1px solid #222;"></th>
        </tr>
      </thead>
      <tbody id="jobsTableBody">
        <tr><td colspan="8" style="padding:20px;text-align:center;color:#555;font-family:monospace;">No bookings yet</td></tr>
      </tbody>
    </table>
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

// ─── Booking modal state ────────────────────────────────────────────────────
let bookingMode = 'immediate';
let pickupCoords = null;
let destCoords = null;
let pickupMarker = null;
let destMarker = null;
let jobFilter = 'active';

// ─── Modal open/close ───────────────────────────────────────────────────────
function openPrebookModal() {
  document.getElementById('prebookModal').style.display = 'flex';
  const now = new Date();
  now.setMinutes(now.getMinutes() - now.getTimezoneOffset());
  document.getElementById('bkRequestedTime').min = now.toISOString().slice(0,16);
}

function closePrebookModal() {
  document.getElementById('prebookModal').style.display = 'none';
  resetBookingForm();
}

function setBookingMode(mode) {
  bookingMode = mode;
  const isPrebook = mode === 'prebook';
  document.getElementById('prebookTimeRow').style.display = isPrebook ? 'block' : 'none';
  document.getElementById('modePrebook').style.cssText = isPrebook
    ? 'flex:1;padding:10px;border:2px solid #00d4ff;background:#00d4ff22;color:#00d4ff;border-radius:4px;cursor:pointer;font-family:monospace;font-size:13px;'
    : 'flex:1;padding:10px;border:2px solid #444;background:transparent;color:#888;border-radius:4px;cursor:pointer;font-family:monospace;font-size:13px;';
  document.getElementById('modeImmediate').style.cssText = isPrebook
    ? 'flex:1;padding:10px;border:2px solid #444;background:transparent;color:#888;border-radius:4px;cursor:pointer;font-family:monospace;font-size:13px;'
    : 'flex:1;padding:10px;border:2px solid #00d4ff;background:#00d4ff22;color:#00d4ff;border-radius:4px;cursor:pointer;font-family:monospace;font-size:13px;';
}

function resetBookingForm() {
  ['bkCustomerName','bkPhone','bkPickupAddress','bkDestAddress','bkNotes'].forEach(id => {
    document.getElementById(id).value = '';
  });
  document.getElementById('bkIsAccount').checked = false;
  document.getElementById('bkPickupStatus').textContent = '';
  document.getElementById('bkDestStatus').textContent = '';
  document.getElementById('bkPickupZoneTag').textContent = '';
  document.getElementById('bkDestZoneTag').textContent = '';
  pickupCoords = null;
  destCoords = null;
  if (pickupMarker) { map.removeLayer(pickupMarker); pickupMarker = null; }
  if (destMarker)   { map.removeLayer(destMarker);   destMarker = null; }
  setBookingMode('immediate');
}

// ─── Geocoding ──────────────────────────────────────────────────────────────
async function geocodeAddress(type) {
  const isPickup  = type === 'pickup';
  const inputId   = isPickup ? 'bkPickupAddress' : 'bkDestAddress';
  const statusId  = isPickup ? 'bkPickupStatus'  : 'bkDestStatus';
  const zoneTagId = isPickup ? 'bkPickupZoneTag' : 'bkDestZoneTag';
  const address   = document.getElementById(inputId).value.trim();
  if (!address) return;

  document.getElementById(statusId).textContent = '⏳';
  try {
    const resp = await fetch('/api/geocode?address=' + encodeURIComponent(address));
    const data = await resp.json();
    if (data.error) {
      document.getElementById(statusId).textContent = '❌';
      document.getElementById(zoneTagId).textContent = 'Address not found — try adding postcode';
      return;
    }
    const lat = parseFloat(data.lat);
    const lng = parseFloat(data.lng);
    document.getElementById(statusId).textContent = '✅';
    document.getElementById(zoneTagId).textContent = 'Pin dropped — drag to adjust if needed';

    const icon = L.divIcon({
      className: '',
      html: isPickup
        ? '<div style="background:#00d4ff;color:#000;padding:3px 6px;border-radius:3px;font-size:11px;font-family:monospace;border:2px solid #fff;white-space:nowrap;">📍 PICKUP</div>'
        : '<div style="background:#ff6b35;color:#fff;padding:3px 6px;border-radius:3px;font-size:11px;font-family:monospace;border:2px solid #fff;white-space:nowrap;">🏁 DEST</div>',
      iconAnchor: [30, 10]
    });

    if (isPickup) {
      if (pickupMarker) map.removeLayer(pickupMarker);
      pickupCoords = {lat, lng};
      pickupMarker = L.marker([lat, lng], {draggable: true, icon}).addTo(map);
      pickupMarker.on('dragend', e => {
        pickupCoords = {lat: e.target.getLatLng().lat, lng: e.target.getLatLng().lng};
        document.getElementById(zoneTagId).textContent = 'Pin adjusted manually';
      });
      map.setView([lat, lng], 15);
    } else {
      if (destMarker) map.removeLayer(destMarker);
      destCoords = {lat, lng};
      destMarker = L.marker([lat, lng], {draggable: true, icon}).addTo(map);
      destMarker.on('dragend', e => {
        destCoords = {lat: e.target.getLatLng().lat, lng: e.target.getLatLng().lng};
        document.getElementById(zoneTagId).textContent = 'Pin adjusted manually';
      });
    }
  } catch(e) {
    document.getElementById(statusId).textContent = '❌';
    document.getElementById(zoneTagId).textContent = 'Geocode error — check connection';
    console.error('Geocode error:', e);
  }
}

// ─── Submit booking ─────────────────────────────────────────────────────────
async function submitBooking() {
  if (!pickupCoords) {
    alert('Please enter and locate a pickup address first.');
    return;
  }
  const body = {
    type:           bookingMode,
    customer_name:  document.getElementById('bkCustomerName').value.trim(),
    phone:          document.getElementById('bkPhone').value.trim(),
    notes:          document.getElementById('bkNotes').value.trim(),
    is_account:     document.getElementById('bkIsAccount').checked,
    pickup_address: document.getElementById('bkPickupAddress').value.trim(),
    pickup_lat:     pickupCoords.lat,
    pickup_lng:     pickupCoords.lng,
    dest_address:   document.getElementById('bkDestAddress').value.trim(),
    dest_lat:       destCoords ? destCoords.lat : 0,
    dest_lng:       destCoords ? destCoords.lng : 0,
    requested_time: bookingMode === 'prebook'
      ? document.getElementById('bkRequestedTime').value
      : new Date().toISOString()
  };
  try {
    const resp = await fetch('/api/booking/new', {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify(body)
    });
    const booking = await resp.json();
    if (booking.error) { alert('Booking error: ' + booking.error); return; }
    closePrebookModal();
    loadJobs();
  } catch(e) {
    alert('Failed to create booking — check console');
    console.error(e);
  }
}

// ─── Jobs list ──────────────────────────────────────────────────────────────
function setJobFilter(filter) {
  jobFilter = filter;
  ['active','completed','all'].forEach(f => {
    const btn = document.getElementById('filter' + f.charAt(0).toUpperCase() + f.slice(1));
    const active = f === filter;
    btn.style.borderColor  = active ? '#00d4ff' : '#444';
    btn.style.background   = active ? '#00d4ff22' : 'transparent';
    btn.style.color        = active ? '#00d4ff' : '#666';
  });
  loadJobs();
}

async function loadJobs() {
  try {
    const resp = await fetch('/api/prebooks?filter=' + jobFilter);
    const bookings = await resp.json();
    renderJobsTable(bookings);
  } catch(e) { console.error('Failed to load jobs:', e); }
}

function renderJobsTable(bookings) {
  const tbody = document.getElementById('jobsTableBody');
  if (!bookings || bookings.length === 0) {
    tbody.innerHTML = '<tr><td colspan="8" style="padding:20px;text-align:center;color:#555;font-family:monospace;">No bookings to display</td></tr>';
    return;
  }
  const statusColour = {pending:'#888', dispatched:'#ffb347', accepted:'#00d4ff', completed:'#444', cancelled:'#555'};
  const statusLabel  = {pending:'PENDING', dispatched:'DISPATCHED', accepted:'EN ROUTE', completed:'COMPLETED', cancelled:'CANCELLED'};
  tbody.innerHTML = bookings.map(b => {
    const done    = b.status === 'completed' || b.status === 'cancelled';
    const dim     = done ? 'opacity:0.4;' : '';
    const timeStr = b.type === 'prebook'
      ? new Date(b.requested_time).toLocaleString('en-GB',{day:'2-digit',month:'short',hour:'2-digit',minute:'2-digit'})
      : 'IMMEDIATE';
    const typeTag = b.type === 'prebook'
      ? '<span style="color:#a78bfa;font-size:10px;margin-right:3px;">PRE</span>'
      : '<span style="color:#ffb347;font-size:10px;margin-right:3px;">NOW</span>';
    const actions = done ? '' :
      '<button onclick="completeJob(\''+b.id+'\')" style="padding:3px 8px;background:#22c55e22;border:1px solid #22c55e;color:#22c55e;border-radius:3px;cursor:pointer;font-family:monospace;font-size:10px;margin-right:4px;">✓</button>' +
      '<button onclick="cancelJob(\''+b.id+'\')"   style="padding:3px 8px;background:#ef444422;border:1px solid #ef4444;color:#ef4444;border-radius:3px;cursor:pointer;font-family:monospace;font-size:10px;">✕</button>';
    return '<tr style="'+dim+'border-bottom:1px solid #1a1a2e;">' +
      '<td style="padding:8px 10px;color:#e0e0e0;white-space:nowrap;">'+typeTag+timeStr+'</td>' +
      '<td style="padding:8px 10px;color:#e0e0e0;">'+( b.customer_name||'—')+(b.is_account?'<span style="color:#a78bfa;font-size:10px;margin-left:4px;">ACC</span>':'')+'</td>' +
      '<td style="padding:8px 10px;color:#ccc;max-width:160px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;">'+(b.pickup_address||'—')+'</td>' +
      '<td style="padding:8px 10px;color:#888;font-size:11px;">'+(b.pickup_zone||'—')+'</td>' +
      '<td style="padding:8px 10px;color:#ccc;max-width:160px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;">'+(b.dest_address||'—')+'</td>' +
      '<td style="padding:8px 10px;color:#aaa;">'+(b.assigned_driver||'—')+'</td>' +
      '<td style="padding:8px 10px;"><span style="color:'+(statusColour[b.status]||'#888')+';font-size:11px;">'+(statusLabel[b.status]||b.status.toUpperCase())+'</span></td>' +
      '<td style="padding:8px 10px;white-space:nowrap;">'+actions+'</td>' +
    '</tr>';
  }).join('');
}

async function completeJob(id) {
  await fetch('/api/booking/complete?id=' + id, {method:'POST'});
  loadJobs();
}

async function cancelJob(id) {
  if (!confirm('Cancel this booking?')) return;
  await fetch('/api/booking/cancel?id=' + id, {method:'POST'});
  loadJobs();
}

// Initial load + poll every 30 seconds
loadJobs();
setInterval(loadJobs, 30000);
</script>
<div id="prebookModal" style="display:none;position:fixed;inset:0;background:rgba(0,0,0,0.75);z-index:2000;align-items:center;justify-content:center;">
  <div style="background:#1a1a2e;border:1px solid #444;border-radius:8px;width:680px;max-width:95vw;max-height:90vh;overflow-y:auto;padding:24px;color:#e0e0e0;font-family:monospace;">
    <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:20px;">
      <h2 style="margin:0;color:#00d4ff;font-size:18px;">📋 New Booking</h2>
      <button onclick="closePrebookModal()" style="background:none;border:none;color:#888;font-size:20px;cursor:pointer;">✕</button>
    </div>
    <div style="display:flex;gap:8px;margin-bottom:20px;">
      <button id="modeImmediate" onclick="setBookingMode('immediate')" style="flex:1;padding:10px;border:2px solid #00d4ff;background:#00d4ff22;color:#00d4ff;border-radius:4px;cursor:pointer;font-family:monospace;font-size:13px;">⚡ IMMEDIATE</button>
      <button id="modePrebook" onclick="setBookingMode('prebook')" style="flex:1;padding:10px;border:2px solid #444;background:transparent;color:#888;border-radius:4px;cursor:pointer;font-family:monospace;font-size:13px;">🕐 PRE-BOOK</button>
    </div>
    <div id="prebookTimeRow" style="display:none;margin-bottom:16px;">
      <label style="display:block;font-size:11px;color:#888;margin-bottom:4px;">PICKUP DATE &amp; TIME</label>
      <input id="bkRequestedTime" type="datetime-local" style="width:100%;padding:8px;background:#0d0d1a;border:1px solid #444;color:#e0e0e0;border-radius:4px;font-family:monospace;box-sizing:border-box;" />
    </div>
    <div style="display:grid;grid-template-columns:1fr 1fr;gap:12px;margin-bottom:16px;">
      <div>
        <label style="display:block;font-size:11px;color:#888;margin-bottom:4px;">PASSENGER NAME</label>
        <input id="bkCustomerName" type="text" placeholder="e.g. Jane Smith" style="width:100%;padding:8px;background:#0d0d1a;border:1px solid #444;color:#e0e0e0;border-radius:4px;font-family:monospace;box-sizing:border-box;" />
      </div>
      <div>
        <label style="display:block;font-size:11px;color:#888;margin-bottom:4px;">PHONE</label>
        <input id="bkPhone" type="tel" placeholder="e.g. 01702 123456" style="width:100%;padding:8px;background:#0d0d1a;border:1px solid #444;color:#e0e0e0;border-radius:4px;font-family:monospace;box-sizing:border-box;" />
      </div>
    </div>
    <div style="margin-bottom:16px;">
      <label style="display:block;font-size:11px;color:#888;margin-bottom:4px;">PICKUP ADDRESS</label>
      <div style="display:flex;gap:8px;align-items:center;">
        <input id="bkPickupAddress" type="text" placeholder="e.g. 42 London Road, Southend" style="flex:1;padding:8px;background:#0d0d1a;border:1px solid #444;color:#e0e0e0;border-radius:4px;font-family:monospace;" onblur="geocodeAddress('pickup')" />
        <span id="bkPickupStatus" style="font-size:18px;width:24px;text-align:center;"></span>
      </div>
      <div id="bkPickupZoneTag" style="font-size:11px;color:#888;margin-top:4px;"></div>
    </div>
    <div style="margin-bottom:16px;">
      <label style="display:block;font-size:11px;color:#888;margin-bottom:4px;">DESTINATION ADDRESS</label>
      <div style="display:flex;gap:8px;align-items:center;">
        <input id="bkDestAddress" type="text" placeholder="e.g. Southend Airport" style="flex:1;padding:8px;background:#0d0d1a;border:1px solid #444;color:#e0e0e0;border-radius:4px;font-family:monospace;" onblur="geocodeAddress('dest')" />
        <span id="bkDestStatus" style="font-size:18px;width:24px;text-align:center;"></span>
      </div>
      <div id="bkDestZoneTag" style="font-size:11px;color:#888;margin-top:4px;"></div>
    </div>
    <div style="margin-bottom:16px;">
      <label style="display:block;font-size:11px;color:#888;margin-bottom:4px;">NOTES / SPECIAL INSTRUCTIONS</label>
      <textarea id="bkNotes" rows="2" placeholder="e.g. wheelchair access, large luggage" style="width:100%;padding:8px;background:#0d0d1a;border:1px solid #444;color:#e0e0e0;border-radius:4px;font-family:monospace;box-sizing:border-box;resize:vertical;"></textarea>
    </div>
    <div style="display:flex;align-items:center;gap:8px;margin-bottom:20px;">
      <input id="bkIsAccount" type="checkbox" style="width:16px;height:16px;" />
      <label for="bkIsAccount" style="font-size:13px;color:#aaa;cursor:pointer;">Account customer</label>
    </div>
    <div style="display:flex;gap:8px;justify-content:flex-end;">
      <button onclick="closePrebookModal()" style="padding:10px 20px;background:transparent;border:1px solid #555;color:#888;border-radius:4px;cursor:pointer;font-family:monospace;">Cancel</button>
      <button onclick="submitBooking()" style="padding:10px 24px;background:#00d4ff;border:none;color:#000;border-radius:4px;cursor:pointer;font-family:monospace;font-weight:bold;font-size:14px;">✓ CONFIRM BOOKING</button>
    </div>
  </div>
</div>
</body>
</html>
`
