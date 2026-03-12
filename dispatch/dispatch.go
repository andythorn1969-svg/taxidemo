// ============================================================
// Taxidemo - Southend Taxi Cooperative Dispatch Demo
// Version: 0.1.0
// Package dispatch: Job offer and driver assignment logic
// ============================================================

package dispatch

import (
	"fmt"
	"log"
	"math"
	"math/rand"
	"sync"
	"time"

	"taxidemo/models"
)

// destination represents a named drop-off point with GPS coordinates.
type destination struct {
	Name string
	Lat  float64
	Lng  float64
}

// destinations is the list of common Southend drop-off points used for random assignment.
var destinations = []destination{
	{"Southend Central Station", 51.536, 0.708},
	{"Southend Victoria Station", 51.538, 0.713},
	{"Southend Airport", 51.571, 0.695},
	{"Southend Hospital", 51.549, 0.698},
	{"Southend Seafront", 51.530, 0.717},
	{"Westcliff Station", 51.537, 0.678},
	{"Leigh-on-Sea Station", 51.540, 0.658},
	{"Rochford Station", 51.581, 0.708},
	{"Shoeburyness Station", 51.531, 0.798},
	{"Thorpe Bay Station", 51.532, 0.762},
	{"Victoria Shopping Centre", 51.537, 0.711},
	{"Priory Park", 51.547, 0.717},
	{"Adventure Island", 51.531, 0.720},
	{"Kursaal", 51.532, 0.718},
	{"London Road shops", 51.548, 0.678},
}

// randomDestination returns a randomly chosen destination from the list.
func randomDestination() destination {
	return destinations[rand.Intn(len(destinations))]
}

// zoneCoords maps zone IDs to their approximate centre coordinates.
var zoneCoords = map[string][2]float64{
	"Z01": {51.568, 0.672}, // Progress
	"Z02": {51.565, 0.700}, // Thanet
	"Z03": {51.563, 0.730}, // Blue
	"Z04": {51.558, 0.660}, // Fairway
	"Z05": {51.556, 0.690}, // Blenheim
	"Z06": {51.556, 0.718}, // Temple
	"Z07": {51.554, 0.740}, // Fossett
	"Z08": {51.549, 0.648}, // Highlands
	"Z09": {51.548, 0.672}, // Elms
	"Z10": {51.547, 0.695}, // Ross
	"Z11": {51.545, 0.708}, // Plough
	"Z12": {51.544, 0.720}, // Priory
	"Z13": {51.543, 0.735}, // VAC
	"Z14": {51.541, 0.748}, // Green
	"Z15": {51.540, 0.645}, // Broadway
	"Z16": {51.537, 0.660}, // Chalkwell
	"Z17": {51.535, 0.678}, // Westcliff
	"Z18": {51.533, 0.708}, // Town
	"Z19": {51.531, 0.722}, // Kursaal
	"Z20": {51.530, 0.740}, // Thorpe
	"Z21": {51.528, 0.755}, // Bay
	"Z22": {51.527, 0.775}, // Shoebury
}

// bookingCoords returns coordinates for a booking based on its pickup zone,
// with a small random offset so multiple bookings don't stack.
func bookingCoords(zoneID string) (float64, float64) {
	centre, ok := zoneCoords[zoneID]
	if !ok {
		return 51.538, 0.711 // fall back to Southend centre
	}
	offset := func() float64 { return (rand.Float64()*2 - 1) * 0.003 }
	return centre[0] + offset(), centre[1] + offset()
}

// NearestZone returns the zone ID whose centre coordinate is closest to (lat, lng).
// Falls back to "Z18" (Town — central Southend) if zoneCoords is empty.
func NearestZone(lat, lng float64) string {
	best := ""
	bestDist := math.MaxFloat64
	for id, centre := range zoneCoords {
		dlat := lat - centre[0]
		dlng := lng - centre[1]
		dist := dlat*dlat + dlng*dlng
		if dist < bestDist {
			bestDist = dist
			best = id
		}
	}
	if best == "" {
		return "Z18"
	}
	return best
}

// FindZone returns the zone matching the given ID, or nil if not found.
func FindZone(id string, zones []*models.Zone) *models.Zone {
	for _, z := range zones {
		if z.ID == id {
			return z
		}
	}
	return nil
}

// FindNearestDriver returns the first available driver across all zones.
// Placeholder implementation - will be improved with geo-distance logic later.
func FindNearestDriver(zones []*models.Zone) *models.Driver {
	for _, z := range zones {
		for _, d := range z.Drivers {
			if d.Status == models.StatusAvailable {
				return d
			}
		}
	}
	return nil
}

// DispatchJob attempts to offer a booking to drivers in the pickup zone's trap queue.
// Drivers who have waited over 30 minutes accept; others decline.
// Falls back to FindNearestDriver if the zone has no available drivers.
// Returns a Job reflecting the final outcome.
func DispatchJob(booking *models.Booking, zones []*models.Zone) *models.Job {
	zone := FindZone(booking.PickupZone, zones)

	// Assign pickup coordinates based on zone, with a small random offset.
	booking.Lat, booking.Lng = bookingCoords(booking.PickupZone)

	// Assign a random destination if one hasn't been set.
	if booking.Destination == "" {
		dest := randomDestination()
		booking.Destination = dest.Name
		booking.DestLat = dest.Lat
		booking.DestLng = dest.Lng
	}

	job := &models.Job{
		ID:        "J-" + booking.ID,
		Booking:   booking,
		OfferedAt: time.Now(),
		Status:    models.JobOffered,
	}

	// Collect available drivers from the zone queue, preserving trap order.
	var candidates []*models.Driver
	if zone != nil {
		for _, d := range zone.Drivers {
			if d.Status == models.StatusAvailable {
				candidates = append(candidates, d)
			}
		}
	}

	// Zone is empty - fall back to nearest available driver across all zones.
	if len(candidates) == 0 {
		fmt.Printf("Zone %q empty - finding nearest driver\n", booking.PickupZone)
		nearest := FindNearestDriver(zones)
		if nearest == nil {
			fmt.Println("  No drivers available anywhere - booking cannot be dispatched")
			job.Status = models.JobDeclined
			return job
		}
		candidates = []*models.Driver{nearest}
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
			driver.Status = models.StatusBusy
			job.Driver = driver
			job.Status = models.JobAccepted
			booking.Status = models.BookingDispatched
			return job
		}

		fmt.Printf("  %s declined (only waited %d mins), trying next driver\n",
			driver.Name, int(waitMins))
	}

	// All candidates declined.
	fmt.Println("  All drivers declined - booking remains pending")
	job.Status = models.JobDeclined
	return job
}

// removeDriverFromZone removes a driver from their zone's trap queue.
func removeDriverFromZone(driver *models.Driver, zones []*models.Zone) {
	for _, z := range zones {
		for i, d := range z.Drivers {
			if d.ID == driver.ID {
				z.Drivers = append(z.Drivers[:i], z.Drivers[i+1:]...)
				return
			}
		}
	}
}

// zoneNameForDriver returns the zone name for a given driver, or falls back to ZoneID.
func zoneNameForDriver(driver *models.Driver, zones []*models.Zone) string {
	for _, z := range zones {
		for _, d := range z.Drivers {
			if d.ID == driver.ID {
				return z.Name
			}
		}
	}
	return driver.ZoneID
}

// GenerateID returns a unique string ID with the given prefix.
func GenerateID(prefix string) string {
	return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
}

// CompleteBooking marks a booking as completed and returns its driver to available status.
// The driver reference is resolved through the matching Job record.
func CompleteBooking(bookingID string, state *models.AppState) error {
	state.Mu.Lock()
	defer state.Mu.Unlock()

	// Find the booking.
	var booking *models.Booking
	for _, b := range state.Bookings {
		if b.ID == bookingID {
			booking = b
			break
		}
	}
	if booking == nil {
		return fmt.Errorf("booking %q not found", bookingID)
	}

	now := time.Now()
	booking.Status = models.BookingCompleted
	booking.CompletedAt = &now

	// Find the job to get the assigned driver.
	for _, j := range state.Jobs {
		if j.Booking.ID == bookingID && j.Driver != nil {
			// Return the driver to available status.
			for _, d := range state.Drivers {
				if d.ID == j.Driver.ID {
					d.Status = models.StatusAvailable
					d.FreeAt = now
					break
				}
			}
			break
		}
	}

	return nil
}

// CancelBooking marks a booking as cancelled and frees any assigned driver.
func CancelBooking(bookingID string, state *models.AppState) error {
	state.Mu.Lock()
	defer state.Mu.Unlock()

	var booking *models.Booking
	for _, b := range state.Bookings {
		if b.ID == bookingID {
			booking = b
			break
		}
	}
	if booking == nil {
		return fmt.Errorf("booking %q not found", bookingID)
	}

	booking.Status = models.BookingCancelled

	// If a driver was assigned via a job, free them.
	for _, j := range state.Jobs {
		if j.Booking.ID == bookingID && j.Driver != nil {
			for _, d := range state.Drivers {
				if d.ID == j.Driver.ID {
					d.Status = models.StatusAvailable
					d.FreeAt = time.Now()
					break
				}
			}
			break
		}
	}

	return nil
}

// approachMinutesForZone returns how many minutes before RequestedTime to dispatch a prebook.
// Uses the zone's AverageApproachMinutes, defaulting to 10 if the zone is not found.
func approachMinutesForZone(zoneID string, state *models.AppState) int {
	for _, z := range state.Zones {
		if z.ID == zoneID {
			return z.AverageApproachMinutes
		}
	}
	log.Printf("approachMinutesForZone: zone %q not found, using default 10 mins", zoneID)
	return 10
}

// runSchedulerCycle checks all pending prebooks and dispatches any that are due.
func runSchedulerCycle(state *models.AppState) {
	now := time.Now()

	// Collect due prebooks under a read lock.
	state.Mu.RLock()
	var due []*models.Booking
	for _, b := range state.Bookings {
		if b.Type == models.BookingPrebook && b.Status == models.BookingPending {
			approachMins := approachMinutesForZone(b.PickupZone, state)
			dispatchTime := b.RequestedTime.Add(-time.Duration(approachMins) * time.Minute)
			if !now.Before(dispatchTime) {
				due = append(due, b)
			}
		}
	}
	state.Mu.RUnlock()

	// Dispatch each due booking outside the read lock.
	for _, b := range due {
		job := DispatchJob(b, state.Zones)
		log.Printf("scheduler: dispatched prebook %s — job %s status=%s", b.ID, job.ID, job.Status)
	}
}

// StartScheduler launches a background goroutine that checks for due prebooks every 60 seconds.
func StartScheduler(state *models.AppState) {
	log.Println("dispatch scheduler started (60s interval)")
	go func() {
		ticker := time.NewTicker(60 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			runSchedulerCycle(state)
		}
	}()
}

// Ensure math and sync are used; these are referenced by callers building on this package.
// math.Round is available for future geo-distance work; sync.Once is used in AppState.Mu.
var _ = math.Round
var _ sync.Mutex
