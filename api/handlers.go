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
	mux.HandleFunc("/api/booking/update", h.HandleUpdateBooking)
	mux.HandleFunc("/api/prebooks", h.HandlePrebookData)
	mux.HandleFunc("/api/customer/lookup", h.HandleCustomerLookup)
	mux.HandleFunc("/api/customers", h.HandleCustomerList)
	mux.HandleFunc("/api/customer/new", h.HandleNewCustomer)
	mux.HandleFunc("/api/customer/update", h.HandleUpdateCustomer)
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

	destZone := ""
	if req.DestLat != 0 || req.DestLng != 0 {
		destZone = dispatch.NearestZone(req.DestLat, req.DestLng)
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
		DestZone:      destZone,
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

	// Peek at driver status and booking phone BEFORE cancelling.
	// CancelBooking resets the driver to StatusAvailable, so we must capture
	// the driver's current status now to distinguish:
	//   StatusOnJob (arrived at pickup) → no-show (driver got there, nobody present)
	//   StatusDispatched (en route)     → cancellation (passenger cancelled early)
	type cancelSnapshot struct {
		phone        string
		bookingTime  time.Time
		driverStatus models.DriverStatus
	}
	var snap cancelSnapshot
	h.State.Mu.RLock()
	for _, b := range h.State.Bookings {
		if b.ID == id {
			snap.phone = b.Phone
			snap.bookingTime = b.RequestedTime
			if snap.bookingTime.IsZero() {
				snap.bookingTime = b.CreatedAt
			}
			break
		}
	}
	if snap.phone != "" {
		for _, j := range h.State.Jobs {
			if j.Booking.ID == id && j.Driver != nil {
				for _, d := range h.State.Drivers {
					if d.ID == j.Driver.ID {
						snap.driverStatus = d.Status
						break
					}
				}
				break
			}
		}
	}
	h.State.Mu.RUnlock()

	if err := dispatch.CancelBooking(id, h.State); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprintf(w, `{"error":%q}`, err.Error())
		return
	}

	// Update customer counts only when a driver was assigned (status set).
	if snap.phone != "" && snap.driverStatus != "" {
		h.State.Mu.Lock()
		var cust *models.Customer
		for _, c := range h.State.Customers {
			if normalisePhone(c.Phone) == normalisePhone(snap.phone) {
				cust = c
				break
			}
		}
		if cust != nil {
			if snap.driverStatus == models.StatusOnJob {
				// Driver arrived at pickup — nobody there: no-show.
				// Apply late-night exclusion.
				if !dispatch.IsLateWeekendBooking(snap.bookingTime) {
					cust.NoShowCount++
				}
			} else {
				// Driver en route but not yet arrived: cancellation.
				// Late-night exclusion does NOT apply to cancellations.
				cust.CancellationCount++
			}
			cust.Flagged = dispatch.ShouldFlagCustomer(cust.Phone, h.State.Customers, h.State.Bookings)
		}
		h.State.Mu.Unlock()
	}

	w.Header().Set("Content-Type", "application/json")
	fmt.Fprint(w, `{"ok":true}`)
}

// HandleUpdateBooking updates mutable fields on an existing booking by ID.
func (h *Handler) HandleUpdateBooking(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"POST required"}`, http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		ID            string  `json:"id"`
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
		RequestedTime string  `json:"requested_time"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
		return
	}

	h.State.Mu.Lock()
	var booking *models.Booking
	for _, b := range h.State.Bookings {
		if b.ID == req.ID {
			booking = b
			break
		}
	}
	if booking == nil {
		h.State.Mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprintf(w, `{"error":"booking %q not found"}`, req.ID)
		return
	}

	booking.CustomerName  = req.CustomerName
	booking.Passenger     = req.CustomerName
	booking.Phone         = req.Phone
	booking.Notes         = req.Notes
	booking.IsAccount     = req.IsAccount
	booking.PickupAddress = req.PickupAddress
	booking.Lat           = req.PickupLat
	booking.Lng           = req.PickupLng
	booking.DestAddress   = req.DestAddress
	booking.DestLat       = req.DestLat
	booking.DestLng       = req.DestLng
	booking.Type          = models.BookingType(req.Type)

	if req.Type == string(models.BookingPrebook) && req.RequestedTime != "" {
		for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05", "2006-01-02T15:04"} {
			if t, err := time.ParseInLocation(layout, req.RequestedTime, time.Local); err == nil {
				booking.RequestedTime = t
				break
			}
		}
	}

	if req.PickupLat != 0 || req.PickupLng != 0 {
		booking.PickupZone = dispatch.NearestZone(req.PickupLat, req.PickupLng)
	}
	h.State.Mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(booking)
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

	// Build zone ID → name lookup.
	zoneNames := make(map[string]string, len(h.State.Zones))
	for _, z := range h.State.Zones {
		zoneNames[z.ID] = z.Name
	}

	// Build booking ID → assigned driver name from accepted/completed jobs.
	assignedDriver := make(map[string]string)
	for _, j := range h.State.Jobs {
		if j.Driver != nil && (j.Status == models.JobAccepted || j.Status == models.JobCompleted) {
			assignedDriver[j.Booking.ID] = j.Driver.Name
		}
	}
	h.State.Mu.RUnlock()

	resolveName := func(id string) string {
		if id == "" {
			return ""
		}
		if name, ok := zoneNames[id]; ok {
			return name
		}
		return id
	}

	type prebookJSON struct {
		ID             string  `json:"id"`
		CustomerName   string  `json:"customer_name"`
		Phone          string  `json:"phone"`
		PickupAddress  string  `json:"pickup_address"`
		DestAddress    string  `json:"dest_address"`
		PickupZone     string  `json:"pickup_zone"`
		DestZone       string  `json:"dest_zone"`
		AssignedDriver string  `json:"assigned_driver"`
		Status         string  `json:"status"`
		Type           string  `json:"type"`
		RequestedTime  string  `json:"requested_time"`
		Notes          string  `json:"notes"`
		IsAccount      bool    `json:"is_account"`
		Lat            float64 `json:"lat"`
		Lng            float64 `json:"lng"`
		DestLat        float64 `json:"dest_lat"`
		DestLng        float64 `json:"dest_lng"`
	}

	toJSON := func(b *models.Booking) prebookJSON {
		return prebookJSON{
			ID:             b.ID,
			CustomerName:   b.CustomerName,
			Phone:          b.Phone,
			PickupAddress:  b.PickupAddress,
			DestAddress:    func() string {
				if b.DestAddress != "" {
					return b.DestAddress
				}
				return b.Destination
			}(),
			PickupZone:     resolveName(b.PickupZone),
			DestZone:       resolveName(b.DestZone),
			AssignedDriver: assignedDriver[b.ID],
			Status:         string(b.Status),
			Type:           string(b.Type),
			RequestedTime:  b.RequestedTime.Format(time.RFC3339),
			Notes:          b.Notes,
			IsAccount:      b.IsAccount,
			Lat:             b.Lat,
			Lng:             b.Lng,
			DestLat:        b.DestLat,
			DestLng:        b.DestLng,
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

// normalisePhone strips spaces and dashes for flexible phone matching.
func normalisePhone(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, " ", "")
	s = strings.ReplaceAll(s, "-", "")
	return s
}

// HandleCustomerLookup — GET /api/customer/lookup?phone=...
// Returns the customer plus previous_pickups and previous_destinations derived
// from their booking history, ordered most-recent first and deduped.
func (h *Handler) HandleCustomerLookup(w http.ResponseWriter, r *http.Request) {
	phone := normalisePhone(r.URL.Query().Get("phone"))
	if phone == "" {
		http.Error(w, `{"error":"phone param required"}`, http.StatusBadRequest)
		return
	}
	h.State.Mu.RLock()
	var found *models.Customer
	for _, c := range h.State.Customers {
		if normalisePhone(c.Phone) == phone {
			found = c
			break
		}
	}
	var prevPickups, prevDests []string
	if found != nil {
		seenPickup := make(map[string]bool)
		seenDest := make(map[string]bool)
		// Iterate in reverse so most-recent booking appears first.
		for i := len(h.State.Bookings) - 1; i >= 0; i-- {
			b := h.State.Bookings[i]
			if normalisePhone(b.Phone) != normalisePhone(found.Phone) {
				continue
			}
			if b.PickupAddress != "" && !seenPickup[b.PickupAddress] {
				seenPickup[b.PickupAddress] = true
				prevPickups = append(prevPickups, b.PickupAddress)
			}
			dest := b.DestAddress
			if dest == "" {
				dest = b.Destination
			}
			if dest != "" && !seenDest[dest] {
				seenDest[dest] = true
				prevDests = append(prevDests, dest)
			}
		}
	}
	h.State.Mu.RUnlock()
	if found == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error":"not found"}`))
		return
	}
	if prevPickups == nil {
		prevPickups = []string{}
	}
	if prevDests == nil {
		prevDests = []string{}
	}
	resp := struct {
		*models.Customer
		PreviousPickups      []string `json:"previous_pickups"`
		PreviousDestinations []string `json:"previous_destinations"`
	}{
		Customer:             found,
		PreviousPickups:      prevPickups,
		PreviousDestinations: prevDests,
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// customerJSON is the wire format for customer list responses.
type customerJSON struct {
	ID                    string   `json:"id"`
	Name                  string   `json:"name"`
	Phone                 string   `json:"phone"`
	Address               string   `json:"address"`
	Notes                 string   `json:"notes"`
	IsAccount             bool     `json:"is_account"`
	FavouriteDestinations []string `json:"favourite_destinations"`
	BookingCount          int      `json:"booking_count"`
	NoShowCount       int  `json:"no_show_count"`
	CancellationCount int  `json:"cancellation_count"`
	Flagged           bool `json:"flagged"`
	Blocked           bool `json:"blocked"`
}

// HandleCustomerList — GET /api/customers?search=...
// Returns all customers (optionally filtered), with booking_count per customer.
func (h *Handler) HandleCustomerList(w http.ResponseWriter, r *http.Request) {
	search := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("search")))

	h.State.Mu.RLock()
	customers := make([]*models.Customer, len(h.State.Customers))
	copy(customers, h.State.Customers)
	bookings := make([]*models.Booking, len(h.State.Bookings))
	copy(bookings, h.State.Bookings)
	h.State.Mu.RUnlock()

	// Build booking count map keyed by normalised phone.
	bookingCount := make(map[string]int, len(bookings))
	for _, b := range bookings {
		k := normalisePhone(b.Phone)
		if k != "" {
			bookingCount[k]++
		}
	}

	result := make([]customerJSON, 0, len(customers))
	for _, c := range customers {
		if search != "" {
			nameLower := strings.ToLower(c.Name)
			phoneLower := strings.ToLower(c.Phone)
			if !strings.Contains(nameLower, search) && !strings.Contains(phoneLower, search) {
				continue
			}
		}
		favs := c.FavouriteDestinations
		if favs == nil {
			favs = []string{}
		}
		result = append(result, customerJSON{
			ID:                    c.ID,
			Name:                  c.Name,
			Phone:                 c.Phone,
			Address:               c.Address,
			Notes:                 c.Notes,
			IsAccount:             c.IsAccount,
			FavouriteDestinations: favs,
			BookingCount:          bookingCount[normalisePhone(c.Phone)],
			NoShowCount:           c.NoShowCount,
			CancellationCount:     c.CancellationCount,
			Flagged:               c.Flagged,
			Blocked:               c.Blocked,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// HandleNewCustomer — POST /api/customer/new
// Creates a new customer record. Returns 409 if phone already registered.
func (h *Handler) HandleNewCustomer(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, `{"error":"bad request"}`, http.StatusBadRequest)
		return
	}
	var req struct {
		Name                  string   `json:"name"`
		Phone                 string   `json:"phone"`
		Address               string   `json:"address"`
		Notes                 string   `json:"notes"`
		IsAccount             bool     `json:"is_account"`
		FavouriteDestinations []string `json:"favourite_destinations"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
		return
	}
	req.Phone = strings.TrimSpace(req.Phone)
	if req.Phone == "" {
		http.Error(w, `{"error":"phone is required"}`, http.StatusBadRequest)
		return
	}
	normNew := normalisePhone(req.Phone)

	h.State.Mu.Lock()
	for _, c := range h.State.Customers {
		if normalisePhone(c.Phone) == normNew {
			h.State.Mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusConflict)
			w.Write([]byte(`{"error":"phone already registered"}`))
			return
		}
	}
	favs := req.FavouriteDestinations
	if favs == nil {
		favs = []string{}
	}
	c := &models.Customer{
		ID:                    dispatch.GenerateID("CU"),
		Name:                  req.Name,
		Phone:                 req.Phone,
		Address:               req.Address,
		Notes:                 req.Notes,
		IsAccount:             req.IsAccount,
		FavouriteDestinations: favs,
	}
	h.State.Customers = append(h.State.Customers, c)
	h.State.Mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(c)
}

// HandleUpdateCustomer — POST /api/customer/update
// Updates mutable fields on an existing customer by ID. Returns 404 if not found.
func (h *Handler) HandleUpdateCustomer(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, `{"error":"bad request"}`, http.StatusBadRequest)
		return
	}
	var req struct {
		ID                    string   `json:"id"`
		Name                  string   `json:"name"`
		Phone                 string   `json:"phone"`
		Address               string   `json:"address"`
		Notes                 string   `json:"notes"`
		IsAccount             bool     `json:"is_account"`
		FavouriteDestinations []string `json:"favourite_destinations"`
		NoShowCount       int  `json:"no_show_count"`
		CancellationCount int  `json:"cancellation_count"`
		Blocked           bool `json:"blocked"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
		return
	}
	if req.ID == "" {
		http.Error(w, `{"error":"id is required"}`, http.StatusBadRequest)
		return
	}

	h.State.Mu.Lock()
	var found *models.Customer
	for _, c := range h.State.Customers {
		if c.ID == req.ID {
			found = c
			break
		}
	}
	if found == nil {
		h.State.Mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error":"not found"}`))
		return
	}
	favs := req.FavouriteDestinations
	if favs == nil {
		favs = []string{}
	}
	found.Name = req.Name
	found.Phone = req.Phone
	found.Address = req.Address
	found.Notes = req.Notes
	found.IsAccount = req.IsAccount
	found.FavouriteDestinations = favs
	found.NoShowCount = req.NoShowCount
	found.CancellationCount = req.CancellationCount
	found.Blocked = req.Blocked
	found.Flagged = dispatch.ShouldFlagCustomer(found.Phone, h.State.Customers, h.State.Bookings)
	h.State.Mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(found)
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
.topbar{flex-shrink:0;background:#0a0a14;border-bottom:1px solid #1e2a3a;padding:8px 16px;display:flex;align-items:center}
.topbar h1{color:#4fc3f7;font-size:1rem;letter-spacing:.5px}
/* Main layout — three columns side by side, full height */
.main{flex:1;display:flex;flex-direction:row;overflow:hidden}
.left-col{width:22%;flex-shrink:0;height:100%;overflow-y:auto;padding:10px 12px;border-right:1px solid #1e2a3a}
.mid-col{flex:1;display:flex;flex-direction:column;min-width:0;overflow:hidden}
.right-col{width:25%;flex-shrink:0;height:100%;overflow-y:auto;padding:10px 12px;border-left:1px solid #1e2a3a}
/* Map: fixed height inside centre column */
#map{height:50vh;flex-shrink:0;border-radius:8px;border:1px solid #1e2a3a}
/* Bookings panel: fills remaining space in centre column */
.bookings-panel{flex:1;overflow-y:auto;border-top:1px solid #1e2a3a;background:#0d0d1a}
/* Shared panel heading */
.panel-heading{color:#90caf9;font-size:.68rem;text-transform:uppercase;letter-spacing:1.5px;margin-bottom:10px}
/* Booking form */
.form-row{margin-bottom:10px}
.form-label{display:block;font-size:.68rem;color:#78909c;text-transform:uppercase;letter-spacing:.8px;margin-bottom:4px}
.form-input{width:100%;padding:7px 9px;background:#0d1220;border:1px solid #2a3a4a;color:#e0e0e0;border-radius:4px;font-size:.82rem;font-family:'Segoe UI',sans-serif;box-sizing:border-box}
.form-input:focus{outline:none;border-color:#4fc3f7}
.geocode-row{display:flex;gap:6px;align-items:center}
.geocode-status{font-size:16px;width:22px;text-align:center;flex-shrink:0}
.geocode-hint{font-size:.66rem;color:#546e7a;margin-top:3px}
.mode-btn{flex:1;padding:8px;border-radius:4px;cursor:pointer;font-size:.78rem;font-family:'Segoe UI',sans-serif}
.confirm-btn{width:100%;padding:10px;background:#1565c0;color:#fff;border:none;border-radius:5px;font-size:.88rem;cursor:pointer;font-family:'Segoe UI',sans-serif;font-weight:600;margin-top:6px}
.confirm-btn:hover{background:#1976d2}
/* Zone trap queues */
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
/* Override inline max-height on the table wrapper div inside bookings panel */
.bookings-panel>div+div{max-height:none!important;overflow-y:visible!important}
/* Jobs table density */
#jobsTable{font-size:11px!important}
#jobsTable td,#jobsTable th{padding:5px 8px!important}
/* Leaflet */
.leaflet-popup-content-wrapper,.leaflet-popup-tip{background:#1a1a2e;color:#e0e0e0;border:1px solid #2a3a4a}
.leaflet-popup-content b{color:#4fc3f7}
</style>
</head>
<body>

<div class="topbar">
  <h1>Southend Taxi Cooperative &mdash; Dispatch</h1>
  <button onclick="openCustomerPanel()" style="margin-left:auto;padding:10px 22px;background:#7c3aed;border:none;color:#fff;border-radius:5px;font-size:.92rem;font-family:'Segoe UI',sans-serif;font-weight:600;cursor:pointer;letter-spacing:.5px;">&#128101; CUSTOMERS</button>
</div>

<div class="main">

    <!-- LEFT: booking form panel -->
    <div class="left-col">
      <h2 class="panel-heading">New Booking</h2>
      <div class="form-row">
        <div style="display:flex;gap:6px;">
          <button id="modeImmediate" class="mode-btn" style="border:2px solid #4fc3f7;background:#0d2233;color:#4fc3f7;" onclick="setBookingMode('immediate')">⚡ Immediate</button>
          <button id="modePrebook" class="mode-btn" style="border:2px solid #2a3a4a;background:transparent;color:#546e7a;" onclick="setBookingMode('prebook')">🕐 Pre-book</button>
        </div>
      </div>
      <div id="prebookTimeRow" class="form-row" style="display:none;">
        <label class="form-label">Pickup date &amp; time</label>
        <input id="bkRequestedTime" type="datetime-local" class="form-input" />
      </div>
      <div class="form-row">
        <label class="form-label">Passenger name</label>
        <input id="bkCustomerName" type="text" class="form-input" placeholder="e.g. Jane Smith" />
      </div>
      <div class="form-row">
        <label class="form-label">Phone</label>
        <input id="bkPhone" type="tel" class="form-input" placeholder="e.g. 01702 123456" onblur="lookupCustomer()" />
        <div id="customerStatus" style="font-size:.66rem;margin-top:3px;min-height:14px;"></div>
      </div>
      <div class="form-row">
        <label class="form-label">Pickup address</label>
        <div class="geocode-row">
          <input id="bkPickupAddress" type="text" class="form-input" placeholder="e.g. 42 London Road, Southend" onblur="geocodeAddress('pickup')" />
          <span id="bkPickupStatus" class="geocode-status"></span>
        </div>
        <div id="bkPickupZoneTag" class="geocode-hint"></div>
        <div id="pickupSuggestions" style="margin-top:4px;display:none;flex-wrap:wrap;gap:4px;"></div>
      </div>
      <div class="form-row">
        <label class="form-label">Destination address</label>
        <div class="geocode-row">
          <input id="bkDestAddress" type="text" class="form-input" placeholder="e.g. Southend Airport" onblur="geocodeAddress('dest')" />
          <span id="bkDestStatus" class="geocode-status"></span>
        </div>
        <div id="bkDestZoneTag" class="geocode-hint"></div>
        <div id="destSuggestions" style="margin-top:4px;display:none;flex-wrap:wrap;gap:4px;"></div>
      </div>
      <div class="form-row">
        <label class="form-label">Notes</label>
        <textarea id="bkNotes" rows="2" class="form-input" placeholder="e.g. wheelchair access, large luggage" style="resize:vertical;"></textarea>
      </div>
      <div class="form-row" style="display:flex;align-items:center;gap:8px;">
        <input id="bkIsAccount" type="checkbox" style="width:15px;height:15px;" />
        <label for="bkIsAccount" style="font-size:.8rem;color:#90caf9;cursor:pointer;">Account customer</label>
      </div>
      <button class="confirm-btn" onclick="submitBooking()">&#10003; Confirm Booking</button>
    </div>

    <!-- CUSTOMERS: overlay panel — same width as left-col, hidden by default -->
    <div id="customerPanel" style="display:none;width:22%;flex-shrink:0;height:100%;overflow-y:auto;background:#0d0d1a;border-right:1px solid #1e2a3a;padding:10px 12px;flex-direction:column;">
      <div style="display:flex;align-items:center;gap:6px;margin-bottom:10px;">
        <h2 class="panel-heading" style="margin-bottom:0;flex:1;">Customers</h2>
        <button onclick="newCustomer()" style="padding:4px 10px;background:#0d2233;border:1px solid #4fc3f7;color:#4fc3f7;border-radius:3px;font-size:.72rem;font-family:'Segoe UI',sans-serif;cursor:pointer;">+ New</button>
        <button onclick="closeCustomerPanel()" style="padding:4px 10px;background:transparent;border:1px solid #546e7a;color:#546e7a;border-radius:3px;font-size:.72rem;font-family:'Segoe UI',sans-serif;cursor:pointer;">&#10005; Close</button>
      </div>
      <input id="customerSearch" type="text" class="form-input" placeholder="Search name or phone&hellip;" onkeyup="loadCustomers(this.value)" style="margin-bottom:8px;" />
      <div style="overflow-y:auto;flex:1;margin-bottom:8px;">
        <table id="customerTable" style="width:100%;border-collapse:collapse;font-family:monospace;font-size:11px;">
          <thead>
            <tr style="color:#546e7a;text-align:left;">
              <th style="padding:5px 6px;border-bottom:1px solid #1e2a3a;">NAME</th>
              <th style="padding:5px 6px;border-bottom:1px solid #1e2a3a;">PHONE</th>
              <th style="padding:5px 6px;border-bottom:1px solid #1e2a3a;">ACC</th>
              <th style="padding:5px 6px;border-bottom:1px solid #1e2a3a;">BKGS</th>
            </tr>
          </thead>
          <tbody id="customerTableBody">
            <tr><td colspan="4" style="padding:14px;text-align:center;color:#37474f;font-family:monospace;">No customers</td></tr>
          </tbody>
        </table>
      </div>
      <!-- Edit / create form — hidden until a row is clicked or New is pressed -->
      <div id="customerEditForm" style="display:none;border-top:1px solid #1e2a3a;padding-top:10px;">
        <h3 id="customerEditTitle" style="color:#90caf9;font-size:.72rem;text-transform:uppercase;letter-spacing:1px;margin-bottom:8px;">Edit Customer</h3>
        <input type="hidden" id="ceId" />
        <div class="form-row">
          <label class="form-label">Name</label>
          <input id="ceName" type="text" class="form-input" placeholder="Full name" />
        </div>
        <div class="form-row">
          <label class="form-label">Phone</label>
          <input id="cePhone" type="tel" class="form-input" placeholder="e.g. 01702 123456" />
        </div>
        <div class="form-row">
          <label class="form-label">Address</label>
          <input id="ceAddress" type="text" class="form-input" placeholder="Home address" />
        </div>
        <div class="form-row">
          <label class="form-label">Notes</label>
          <textarea id="ceNotes" rows="2" class="form-input" placeholder="Any relevant notes" style="resize:vertical;"></textarea>
        </div>
        <div class="form-row">
          <label class="form-label">Favourite destinations (comma-separated)</label>
          <input id="ceFavDests" type="text" class="form-input" placeholder="e.g. Southend Airport, Victoria Station" />
        </div>
        <div class="form-row" style="display:flex;align-items:center;gap:8px;">
          <input id="ceIsAccount" type="checkbox" style="width:15px;height:15px;" />
          <label for="ceIsAccount" style="font-size:.8rem;color:#90caf9;cursor:pointer;">Account customer</label>
        </div>
        <div class="form-row" style="display:flex;align-items:center;gap:8px;">
          <input id="ceBlocked" type="checkbox" style="width:15px;height:15px;" />
          <label for="ceBlocked" style="font-size:.8rem;color:#ef4444;cursor:pointer;">Blocked</label>
        </div>
        <div class="form-row" style="display:flex;align-items:center;gap:8px;justify-content:space-between;">
          <span style="font-size:.8rem;color:#78909c;">No-shows: <span id="ceNoShowDisplay" style="color:#e0e0e0;font-weight:600;">0</span></span>
          <button onclick="resetNoShowCount()" style="padding:3px 10px;background:transparent;border:1px solid #546e7a;color:#546e7a;border-radius:3px;font-size:.72rem;cursor:pointer;font-family:'Segoe UI',sans-serif;">Reset</button>
          <input type="hidden" id="ceNoShowCount" value="0" />
        </div>
        <div class="form-row" style="display:flex;align-items:center;gap:8px;justify-content:space-between;">
          <span style="font-size:.8rem;color:#78909c;">Cancellations: <span id="ceCancellationDisplay" style="color:#e0e0e0;font-weight:600;">0</span></span>
          <button onclick="resetCancellationCount()" style="padding:3px 10px;background:transparent;border:1px solid #546e7a;color:#546e7a;border-radius:3px;font-size:.72rem;cursor:pointer;font-family:'Segoe UI',sans-serif;">Reset</button>
          <input type="hidden" id="ceCancellationCount" value="0" />
        </div>
        <div id="ceFlaggedIndicator" style="display:none;padding:5px 8px;background:#3a2a00;border:1px solid #f59e0b;border-radius:4px;color:#f59e0b;font-size:.75rem;margin-bottom:4px;">&#9888; Flagged — no-show rate exceeds policy threshold</div>
        <div style="display:flex;gap:6px;margin-top:6px;">
          <button onclick="saveCustomer()" style="flex:1;padding:8px;background:#1565c0;color:#fff;border:none;border-radius:4px;font-size:.82rem;cursor:pointer;font-family:'Segoe UI',sans-serif;font-weight:600;">Save</button>
          <button onclick="cancelCustomerEdit()" style="padding:8px 14px;background:transparent;border:1px solid #546e7a;color:#546e7a;border-radius:4px;font-size:.82rem;cursor:pointer;font-family:'Segoe UI',sans-serif;">Cancel</button>
        </div>
      </div>
    </div>

    <!-- CENTRE: map + bookings panel -->
    <div class="mid-col">
      <div id="map">
        <div style="position:absolute;bottom:24px;left:8px;z-index:1000;background:rgba(13,13,26,0.88);border:1px solid #1e2a3a;border-radius:6px;padding:8px 11px;font-family:monospace;font-size:11px;color:#b0bec5;line-height:1.9;pointer-events:none;">
          <div><span style="display:inline-block;width:14px;height:14px;background:#2e7d32;border:2px solid #81c784;border-radius:3px;vertical-align:middle;margin-right:6px;"></span>Available driver</div>
          <div><span style="display:inline-block;width:14px;height:14px;background:#c62828;border:2px solid #ef9a9a;border-radius:3px;vertical-align:middle;margin-right:6px;"></span>Dispatched / On job</div>
          <div><span style="display:inline-block;width:14px;height:14px;background:#1565c0;border:2px solid #90caf9;border-radius:50%;vertical-align:middle;margin-right:6px;"></span>Pickup point</div>
          <div><span style="display:inline-block;width:14px;height:14px;background:#7b1fa2;border:2px solid #ce93d8;border-radius:50%;vertical-align:middle;margin-right:6px;"></span>Destination</div>
          <div style="margin-top:4px;padding-top:4px;border-top:1px solid #1e2a3a;color:#546e7a;">Zone boundaries shown</div>
        </div>
      </div>
      <div class="bookings-panel">
        <div style="display:flex;align-items:center;justify-content:space-between;padding:8px 14px;background:#111122;border-bottom:1px solid #1e2a3a;">
          <span style="color:#4fc3f7;font-size:.68rem;text-transform:uppercase;letter-spacing:1.5px;font-weight:bold;">Bookings</span>
          <div style="display:flex;gap:6px;">
            <button onclick="setJobFilter('active')" id="filterActive" style="padding:4px 10px;font-family:monospace;font-size:11px;border-radius:3px;cursor:pointer;border:1px solid #4fc3f7;background:#0d2233;color:#4fc3f7;">ACTIVE</button>
            <button onclick="setJobFilter('completed')" id="filterCompleted" style="padding:4px 10px;font-family:monospace;font-size:11px;border-radius:3px;cursor:pointer;border:1px solid #2a3a4a;background:transparent;color:#546e7a;">COMPLETED</button>
            <button onclick="setJobFilter('all')" id="filterAll" style="padding:4px 10px;font-family:monospace;font-size:11px;border-radius:3px;cursor:pointer;border:1px solid #2a3a4a;background:transparent;color:#546e7a;">ALL</button>
            <button onclick="openPrebookModal()" style="padding:4px 12px;font-family:monospace;font-size:11px;border-radius:3px;cursor:pointer;border:none;background:#1565c0;color:#fff;font-weight:bold;margin-left:8px;">+ NEW BOOKING</button>
          </div>
        </div>
        <div style="overflow-x:auto;max-height:160px;overflow-y:auto;">
          <table id="jobsTable" style="width:100%;border-collapse:collapse;font-family:monospace;font-size:12px;">
            <thead>
              <tr style="background:#111122;color:#546e7a;text-align:left;">
                <th style="padding:6px 10px;border-bottom:1px solid #1e2a3a;">TIME</th>
                <th style="padding:6px 10px;border-bottom:1px solid #1e2a3a;">PASSENGER</th>
                <th style="padding:6px 10px;border-bottom:1px solid #1e2a3a;">PICKUP</th>
                <th style="padding:6px 10px;border-bottom:1px solid #1e2a3a;">ZONE</th>
                <th style="padding:6px 10px;border-bottom:1px solid #1e2a3a;">DESTINATION</th>
                <th style="padding:6px 10px;border-bottom:1px solid #1e2a3a;">DEST ZONE</th>
                <th style="padding:6px 10px;border-bottom:1px solid #1e2a3a;">DRIVER</th>
                <th style="padding:6px 10px;border-bottom:1px solid #1e2a3a;">STATUS</th>
                <th style="padding:6px 10px;border-bottom:1px solid #1e2a3a;"></th>
              </tr>
            </thead>
            <tbody id="jobsTableBody">
              <tr><td colspan="9" style="padding:16px;text-align:center;color:#37474f;font-family:monospace;">No bookings yet</td></tr>
            </tbody>
          </table>
        </div>
      </div>
    </div>

    <!-- RIGHT: zone trap queues -->
    <div class="right-col">
      <h2 class="panel-heading">Zone Trap Queues</h2>
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

// ─── Booking form state ──────────────────────────────────────────────────────
let bookingMode = 'immediate';
let pickupCoords = null;
let destCoords = null;
let pickupMarker = null;
let destMarker = null;
let jobFilter = 'active';
let editingBookingId = null;
let currentBookings = [];
let selectedRow = null;
let foundCustomer = null; // null=no lookup, 'new'=new customer, object=existing customer

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
  document.getElementById('customerStatus').textContent = '';
  clearPickupSuggestions();
  clearDestSuggestions();
  foundCustomer = null;
  pickupCoords = null;
  destCoords = null;
  if (pickupMarker) { map.removeLayer(pickupMarker); pickupMarker = null; }
  if (destMarker)   { map.removeLayer(destMarker);   destMarker = null; }
  setBookingMode('immediate');
}

// ─── Customer lookup ─────────────────────────────────────────────────────────
async function lookupCustomer() {
  const phone = document.getElementById('bkPhone').value.trim();
  const statusEl = document.getElementById('customerStatus');
  if (!phone) { statusEl.textContent = ''; foundCustomer = null; clearPickupSuggestions(); clearDestSuggestions(); return; }
  try {
    const resp = await fetch('/api/customer/lookup?phone=' + encodeURIComponent(phone));
    if (resp.ok) {
      const c = await resp.json();
      foundCustomer = c;
      document.getElementById('bkCustomerName').value = c.name  || '';
      document.getElementById('bkNotes').value         = c.notes || '';
      document.getElementById('bkIsAccount').checked   = !!c.is_account;
      if (c.blocked) {
        statusEl.innerHTML = '<span style="color:#ef4444;">&#128683; Blocked customer</span>';
      } else if (c.flagged) {
        statusEl.innerHTML = '<span style="color:#f59e0b;">&#9888; ' + (c.no_show_count||0) + ' no-shows — review recommended</span>';
      } else {
        statusEl.innerHTML = '<span style="color:#81c784;">&#10003; Customer found: ' + (c.name || phone) + '</span>';
      }
      // Merge favourite_destinations with previous_destinations, dedup preserving order.
      const destSeen = new Set();
      const destSuggs = [...(c.favourite_destinations||[]), ...(c.previous_destinations||[])].filter(f => {
        if (destSeen.has(f)) return false; destSeen.add(f); return true;
      });
      showDestSuggestions(destSuggs);
      showPickupSuggestions(c.previous_pickups || []);
    } else if (resp.status === 404) {
      foundCustomer = 'new';
      statusEl.innerHTML = '<span style="color:#78909c;">New customer — record will be created on booking</span>';
      clearPickupSuggestions();
      clearDestSuggestions();
    } else {
      foundCustomer = null;
      statusEl.textContent = '';
      clearPickupSuggestions();
      clearDestSuggestions();
    }
  } catch(e) {
    foundCustomer = null;
    statusEl.textContent = '';
    clearPickupSuggestions();
    clearDestSuggestions();
    console.error('Customer lookup error:', e);
  }
}

function showDestSuggestions(favs) {
  const el = document.getElementById('destSuggestions');
  if (!favs || favs.length === 0) { el.style.display = 'none'; el.innerHTML = ''; return; }
  el.style.display = 'flex';
  el.innerHTML = favs.map(f =>
    '<span onclick="useFavDest(\'' + f.replace(/\\/g,'\\\\').replace(/'/g,"\\'") + '\')" style="cursor:pointer;padding:2px 8px;background:#0d1220;border:1px solid #2a3a4a;color:#90caf9;border-radius:10px;font-size:.66rem;white-space:nowrap;">' + f + '</span>'
  ).join('');
}

function clearDestSuggestions() {
  const el = document.getElementById('destSuggestions');
  el.style.display = 'none';
  el.innerHTML = '';
}

function useFavDest(addr) {
  document.getElementById('bkDestAddress').value = addr;
  geocodeAddress('dest');
}

function showPickupSuggestions(suggs) {
  const el = document.getElementById('pickupSuggestions');
  if (!suggs || suggs.length === 0) { el.style.display = 'none'; el.innerHTML = ''; return; }
  el.style.display = 'flex';
  el.innerHTML = suggs.map(f =>
    '<span onclick="usePickupSugg(\'' + f.replace(/\\/g,'\\\\').replace(/'/g,"\\'") + '\')" style="cursor:pointer;padding:2px 8px;background:#0d1220;border:1px solid #2a3a4a;color:#90caf9;border-radius:10px;font-size:.66rem;white-space:nowrap;">' + f + '</span>'
  ).join('');
}

function clearPickupSuggestions() {
  const el = document.getElementById('pickupSuggestions');
  el.style.display = 'none';
  el.innerHTML = '';
}

function usePickupSugg(addr) {
  document.getElementById('bkPickupAddress').value = addr;
  geocodeAddress('pickup');
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

// ─── Edit mode ───────────────────────────────────────────────────────────────
function enterEditMode(b) {
  editingBookingId = b.id;

  const heading = document.querySelector('.panel-heading');
  heading.innerHTML = 'Edit Booking<span style="display:block;font-size:.6rem;color:#546e7a;font-weight:normal;letter-spacing:0;margin-top:2px;">' + b.id + '</span>';

  document.getElementById('bkCustomerName').value = b.customer_name || '';
  document.getElementById('bkPhone').value         = b.phone || '';
  document.getElementById('bkPickupAddress').value = b.pickup_address || '';
  document.getElementById('bkDestAddress').value   = b.dest_address || '';
  document.getElementById('bkNotes').value         = b.notes || '';
  document.getElementById('bkIsAccount').checked   = !!b.is_account;

  if (b.lat && b.lng)           pickupCoords = {lat: b.lat, lng: b.lng};
  if (b.dest_lat && b.dest_lng) destCoords   = {lat: b.dest_lat, lng: b.dest_lng};

  setBookingMode(b.type === 'prebook' ? 'prebook' : 'immediate');
  if (b.type === 'prebook' && b.requested_time) {
    const dt = new Date(b.requested_time);
    dt.setMinutes(dt.getMinutes() - dt.getTimezoneOffset());
    document.getElementById('bkRequestedTime').value = dt.toISOString().slice(0, 16);
  }

  document.getElementById('bkPickupStatus').textContent  = b.lat      ? '✅' : '';
  document.getElementById('bkPickupZoneTag').textContent = b.lat      ? 'Coordinates loaded from booking' : '';
  document.getElementById('bkDestStatus').textContent    = b.dest_lat ? '✅' : '';
  document.getElementById('bkDestZoneTag').textContent   = b.dest_lat ? 'Coordinates loaded from booking' : '';

  const confirmBtn = document.querySelector('.confirm-btn');
  confirmBtn.textContent = '✓ Update Booking';

  let cancelBtn = document.getElementById('bkCancelEditBtn');
  if (!cancelBtn) {
    cancelBtn = document.createElement('button');
    cancelBtn.id = 'bkCancelEditBtn';
    cancelBtn.textContent = 'Cancel edit';
    cancelBtn.style.cssText = 'width:100%;padding:8px;margin-top:6px;background:transparent;border:1px solid #2a3a4a;color:#78909c;border-radius:5px;font-size:.82rem;cursor:pointer;font-family:\'Segoe UI\',sans-serif;';
    cancelBtn.onclick = exitEditMode;
    confirmBtn.insertAdjacentElement('afterend', cancelBtn);
  }
  cancelBtn.style.display = 'block';
}

function exitEditMode() {
  editingBookingId = null;
  document.querySelector('.panel-heading').textContent = 'New Booking';
  const cancelBtn = document.getElementById('bkCancelEditBtn');
  if (cancelBtn) cancelBtn.style.display = 'none';
  document.querySelector('.confirm-btn').textContent = '✓ Confirm Booking';
  if (selectedRow) {
    selectedRow.style.borderLeft = '3px solid transparent';
    selectedRow.style.background = '';
    selectedRow = null;
  }
  resetBookingForm();
}

function selectBookingRow(id, rowEl) {
  if (selectedRow) {
    selectedRow.style.borderLeft = '3px solid transparent';
    selectedRow.style.background = '';
  }
  const b = currentBookings.find(x => x.id === id);
  if (!b) return;
  selectedRow = rowEl;
  rowEl.style.borderLeft = '3px solid #4fc3f7';
  rowEl.style.background = '#0d1e2d';
  enterEditMode(b);
}

// ─── Update booking ──────────────────────────────────────────────────────────
async function updateBooking(id) {
  if (!pickupCoords) {
    alert('Please enter and locate a pickup address first.');
    return;
  }
  const body = {
    id:             id,
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
    const resp = await fetch('/api/booking/update', {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify(body)
    });
    const result = await resp.json();
    if (result.error) { alert('Update error: ' + result.error); return; }
    if (foundCustomer === 'new') {
      fetch('/api/customer/new', {
        method: 'POST',
        headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({
          name:                  body.customer_name,
          phone:                 body.phone,
          notes:                 body.notes,
          is_account:            body.is_account,
          favourite_destinations: []
        })
      }).catch(() => {});
    }
    exitEditMode();
    loadJobs();
  } catch(e) {
    alert('Failed to update booking — check console');
    console.error(e);
  }
}

// ─── Submit booking ─────────────────────────────────────────────────────────
async function submitBooking() {
  if (editingBookingId) { await updateBooking(editingBookingId); return; }
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
    if (foundCustomer === 'new') {
      fetch('/api/customer/new', {
        method: 'POST',
        headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({
          name:                  body.customer_name,
          phone:                 body.phone,
          notes:                 body.notes,
          is_account:            body.is_account,
          favourite_destinations: []
        })
      }).catch(() => {});
    }
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
    currentBookings = bookings;
    renderJobsTable(bookings);
  } catch(e) { console.error('Failed to load jobs:', e); }
}

function renderJobsTable(bookings) {
  const tbody = document.getElementById('jobsTableBody');
  if (!bookings || bookings.length === 0) {
    tbody.innerHTML = '<tr><td colspan="9" style="padding:20px;text-align:center;color:#555;font-family:monospace;">No bookings to display</td></tr>';
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
      '<button onclick="event.stopPropagation();completeJob(\''+b.id+'\')" style="padding:3px 8px;background:#22c55e22;border:1px solid #22c55e;color:#22c55e;border-radius:3px;cursor:pointer;font-family:monospace;font-size:10px;margin-right:4px;">✓</button>' +
      '<button onclick="event.stopPropagation();cancelJob(\''+b.id+'\')"   style="padding:3px 8px;background:#ef444422;border:1px solid #ef4444;color:#ef4444;border-radius:3px;cursor:pointer;font-family:monospace;font-size:10px;">✕</button>';
    return '<tr onclick="selectBookingRow(\''+b.id+'\',this)" style="cursor:pointer;'+dim+'border-bottom:1px solid #1a1a2e;border-left:3px solid transparent;">' +
      '<td style="padding:8px 10px;color:#e0e0e0;white-space:nowrap;">'+typeTag+timeStr+'</td>' +
      '<td style="padding:8px 10px;color:#e0e0e0;">'+( b.customer_name||'—')+(b.is_account?'<span style="color:#a78bfa;font-size:10px;margin-left:4px;">ACC</span>':'')+'</td>' +
      '<td style="padding:8px 10px;color:#ccc;max-width:160px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;">'+(b.pickup_address||'—')+'</td>' +
      '<td style="padding:8px 10px;color:#888;font-size:11px;">'+(b.pickup_zone||'—')+'</td>' +
      '<td style="padding:8px 10px;color:#ccc;max-width:160px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;">'+(b.dest_address||'—')+'</td>' +
      '<td style="padding:8px 10px;color:#888;font-size:11px;">'+(b.dest_zone||'—')+'</td>' +
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

// ─── Customer panel ──────────────────────────────────────────────────────────
let customersCache = [];
let currentEditCustomer = null;

function openCustomerPanel() {
  document.querySelector('.left-col').style.display = 'none';
  const panel = document.getElementById('customerPanel');
  panel.style.display = 'flex';
  document.getElementById('customerSearch').value = '';
  document.getElementById('customerEditForm').style.display = 'none';
  loadCustomers('');
}

function closeCustomerPanel() {
  document.getElementById('customerPanel').style.display = 'none';
  document.querySelector('.left-col').style.display = '';
}

async function loadCustomers(search) {
  try {
    const resp = await fetch('/api/customers?search=' + encodeURIComponent(search || ''));
    const customers = await resp.json();
    renderCustomerTable(customers || []);
  } catch(e) { console.error('Failed to load customers:', e); }
}

function renderCustomerTable(customers) {
  customersCache = customers;
  const tbody = document.getElementById('customerTableBody');
  if (!customers || customers.length === 0) {
    tbody.innerHTML = '<tr><td colspan="4" style="padding:14px;text-align:center;color:#37474f;font-family:monospace;">No customers found</td></tr>';
    return;
  }
  tbody.innerHTML = customers.map((c, i) =>
    '<tr onclick="editCustomer(' + i + ')" style="cursor:pointer;border-bottom:1px solid #1a1a2e;">' +
    '<td style="padding:5px 6px;color:#e0e0e0;">' + (c.name||'—') + '</td>' +
    '<td style="padding:5px 6px;color:#90caf9;font-size:10px;">' + (c.phone||'—') + '</td>' +
    '<td style="padding:5px 6px;">' + (c.is_account ? '<span style="color:#a78bfa;font-size:10px;">ACC</span>' : '') + '</td>' +
    '<td style="padding:5px 6px;color:#546e7a;">' + (c.booking_count||0) + '</td>' +
    '</tr>'
  ).join('');
}

function editCustomer(idx) {
  const c = customersCache[idx];
  currentEditCustomer = c;
  document.getElementById('customerEditTitle').textContent = 'Edit Customer';
  document.getElementById('ceId').value          = c.id || '';
  document.getElementById('ceName').value        = c.name || '';
  document.getElementById('cePhone').value       = c.phone || '';
  document.getElementById('ceAddress').value     = c.address || '';
  document.getElementById('ceNotes').value       = c.notes || '';
  document.getElementById('ceIsAccount').checked = !!c.is_account;
  document.getElementById('ceBlocked').checked   = !!c.blocked;
  document.getElementById('ceFavDests').value    = (c.favourite_destinations||[]).join(', ');
  const ns = c.no_show_count || 0;
  document.getElementById('ceNoShowCount').value         = ns;
  document.getElementById('ceNoShowDisplay').textContent = ns;
  const ca = c.cancellation_count || 0;
  document.getElementById('ceCancellationCount').value         = ca;
  document.getElementById('ceCancellationDisplay').textContent = ca;
  document.getElementById('ceFlaggedIndicator').style.display = c.flagged ? 'block' : 'none';
  document.getElementById('customerEditForm').style.display = 'block';
}

function newCustomer() {
  currentEditCustomer = null;
  document.getElementById('customerEditTitle').textContent = 'New Customer';
  document.getElementById('ceId').value          = '';
  document.getElementById('ceName').value        = '';
  document.getElementById('cePhone').value       = '';
  document.getElementById('ceAddress').value     = '';
  document.getElementById('ceNotes').value       = '';
  document.getElementById('ceIsAccount').checked = false;
  document.getElementById('ceBlocked').checked   = false;
  document.getElementById('ceFavDests').value    = '';
  document.getElementById('ceNoShowCount').value         = '0';
  document.getElementById('ceNoShowDisplay').textContent = '0';
  document.getElementById('ceCancellationCount').value         = '0';
  document.getElementById('ceCancellationDisplay').textContent = '0';
  document.getElementById('ceFlaggedIndicator').style.display = 'none';
  document.getElementById('customerEditForm').style.display = 'block';
  document.getElementById('ceName').focus();
}

function cancelCustomerEdit() {
  document.getElementById('customerEditForm').style.display = 'none';
  currentEditCustomer = null;
}

function resetNoShowCount() {
  document.getElementById('ceNoShowCount').value = '0';
  document.getElementById('ceNoShowDisplay').textContent = '0';
  saveCustomer();
}

function resetCancellationCount() {
  document.getElementById('ceCancellationCount').value = '0';
  document.getElementById('ceCancellationDisplay').textContent = '0';
  saveCustomer();
}

async function saveCustomer() {
  const id      = document.getElementById('ceId').value.trim();
  const name    = document.getElementById('ceName').value.trim();
  const phone   = document.getElementById('cePhone').value.trim();
  const address = document.getElementById('ceAddress').value.trim();
  const notes   = document.getElementById('ceNotes').value.trim();
  const isAcct   = document.getElementById('ceIsAccount').checked;
  const blocked  = document.getElementById('ceBlocked').checked;
  const noShows     = parseInt(document.getElementById('ceNoShowCount').value, 10) || 0;
  const cancels     = parseInt(document.getElementById('ceCancellationCount').value, 10) || 0;
  const favRaw      = document.getElementById('ceFavDests').value;
  const favs        = favRaw.split(',').map(s => s.trim()).filter(s => s.length > 0);
  if (!name || !phone) { alert('Name and phone are required'); return; }
  const url  = id ? '/api/customer/update' : '/api/customer/new';
  const body = { id, name, phone, address, notes, is_account: isAcct, blocked, no_show_count: noShows, cancellation_count: cancels, favourite_destinations: favs };
  try {
    const resp = await fetch(url, {
      method: 'POST',
      headers: {'Content-Type':'application/json'},
      body: JSON.stringify(body)
    });
    if (!resp.ok) { alert('Save failed: ' + (await resp.text())); return; }
    document.getElementById('customerEditForm').style.display = 'none';
    currentEditCustomer = null;
    loadCustomers(document.getElementById('customerSearch').value);
  } catch(e) {
    alert('Save failed — check console');
    console.error(e);
  }
}
</script>
<div id="prebookModal" style="display:none;"></div>
</body>
</html>
`
