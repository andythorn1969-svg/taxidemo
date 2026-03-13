// ============================================================
// Taxidemo - Southend Taxi Cooperative Dispatch Demo
// Version: 0.1.0
// Description: Entry point - initialises state, registers routes, starts server
// ============================================================

package main

import (
	"fmt"
	"net/http"
	"time"

	"taxidemo/api"
	"taxidemo/dispatch"
	"taxidemo/models"
)

func main() {
	zones, drivers := models.SeedData()
	state := &models.AppState{
		Zones:   zones,
		Drivers: drivers,
	}

	now := time.Now()
	ptr := func(t time.Time) *time.Time { return &t }

	state.Bookings = []*models.Booking{
		// Completed immediates
		{ID: dispatch.GenerateID("BK"), CustomerName: "Margaret Holloway", Phone: "01702 441122", PickupAddress: "14 Hamlet Court Road, Westcliff", Destination: "Southend Central Station", PickupZone: "Z17", DestZone: dispatch.NearestZone(51.536, 0.708), Source: models.SourcePhone, Type: models.BookingImmediate, Status: models.BookingCompleted, CreatedAt: now.Add(-3 * time.Hour), RequestedTime: now.Add(-3 * time.Hour), CompletedAt: ptr(now.Add(-2*time.Hour - 30*time.Minute)), Lat: 51.536, Lng: 0.678, DestLat: 51.536, DestLng: 0.708},
		{ID: dispatch.GenerateID("BK"), CustomerName: "Derek Saunders", Phone: "01702 558833", PickupAddress: "Southend Victoria Station", Destination: "Rochford Station", PickupZone: "Z18", DestZone: dispatch.NearestZone(51.581, 0.708), Source: models.SourceApp, Type: models.BookingImmediate, Status: models.BookingCompleted, CreatedAt: now.Add(-2*time.Hour - 45*time.Minute), RequestedTime: now.Add(-2*time.Hour - 45*time.Minute), CompletedAt: ptr(now.Add(-2 * time.Hour)), Lat: 51.538, Lng: 0.713, DestLat: 51.581, DestLng: 0.708},
		{ID: dispatch.GenerateID("BK"), CustomerName: "Patricia Okafor", Phone: "07700 900312", PickupAddress: "Adventure Island, Western Esplanade", Destination: "Southend Hospital", PickupZone: "Z19", DestZone: dispatch.NearestZone(51.549, 0.698), Source: models.SourceApp, Type: models.BookingImmediate, Status: models.BookingCompleted, CreatedAt: now.Add(-2 * time.Hour), RequestedTime: now.Add(-2 * time.Hour), CompletedAt: ptr(now.Add(-1*time.Hour - 30*time.Minute)), Lat: 51.531, Lng: 0.720, DestLat: 51.549, DestLng: 0.698},
		{ID: dispatch.GenerateID("BK"), CustomerName: "Colin Marsh", Phone: "01702 661199", PickupAddress: "30 London Road, Leigh-on-Sea", Destination: "Westcliff Station", PickupZone: "Z16", DestZone: dispatch.NearestZone(51.537, 0.678), Source: models.SourcePhone, Type: models.BookingImmediate, Status: models.BookingCompleted, CreatedAt: now.Add(-90 * time.Minute), RequestedTime: now.Add(-90 * time.Minute), CompletedAt: ptr(now.Add(-70 * time.Minute)), Lat: 51.540, Lng: 0.658, DestLat: 51.537, DestLng: 0.678},
		{ID: dispatch.GenerateID("BK"), CustomerName: "Sandra Whitmore", Phone: "07912 345678", PickupAddress: "Kursaal, Eastern Esplanade", Destination: "Thorpe Bay Station", PickupZone: "Z19", DestZone: dispatch.NearestZone(51.532, 0.762), Source: models.SourceApp, Type: models.BookingImmediate, Status: models.BookingCompleted, CreatedAt: now.Add(-75 * time.Minute), RequestedTime: now.Add(-75 * time.Minute), CompletedAt: ptr(now.Add(-55 * time.Minute)), Lat: 51.532, Lng: 0.718, DestLat: 51.532, DestLng: 0.762},

		// Cancelled
		{ID: dispatch.GenerateID("BK"), CustomerName: "James Fontaine", Phone: "01702 772244", PickupAddress: "Victoria Shopping Centre, Southend", Destination: "London Road shops", PickupZone: "Z18", Source: models.SourcePhone, Type: models.BookingImmediate, Status: models.BookingCancelled, CreatedAt: now.Add(-60 * time.Minute), RequestedTime: now.Add(-60 * time.Minute), Lat: 51.537, Lng: 0.711},
		{ID: dispatch.GenerateID("BK"), CustomerName: "Fiona Blackwell", Phone: "07800 123456", PickupAddress: "Priory Park, Victoria Avenue", Destination: "Southend Airport", PickupZone: "Z12", DestZone: dispatch.NearestZone(51.571, 0.695), Source: models.SourceApp, Type: models.BookingPrebook, Status: models.BookingCancelled, CreatedAt: now.Add(-4 * time.Hour), RequestedTime: now.Add(2 * time.Hour), Lat: 51.547, Lng: 0.717, DestLat: 51.571, DestLng: 0.695},

		// Dispatched / active immediates
		{ID: dispatch.GenerateID("BK"), CustomerName: "Nadia Patel", Phone: "07711 223344", PickupAddress: "52 Hamlet Court Road, Westcliff", Destination: "Southend Central Station", PickupZone: "Z17", DestZone: dispatch.NearestZone(51.536, 0.708), Source: models.SourceApp, Type: models.BookingImmediate, Status: models.BookingDispatched, CreatedAt: now.Add(-20 * time.Minute), RequestedTime: now.Add(-20 * time.Minute), Lat: 51.535, Lng: 0.675, DestLat: 51.536, DestLng: 0.708},
		{ID: dispatch.GenerateID("BK"), CustomerName: "Robert Finch", Phone: "01702 889966", PickupAddress: "Leigh-on-Sea Station", Destination: "Priory Park", PickupZone: "Z15", DestZone: dispatch.NearestZone(51.547, 0.717), Source: models.SourcePhone, Type: models.BookingImmediate, Status: models.BookingDispatched, CreatedAt: now.Add(-12 * time.Minute), RequestedTime: now.Add(-12 * time.Minute), Lat: 51.540, Lng: 0.658, DestLat: 51.547, DestLng: 0.717},
		{ID: dispatch.GenerateID("BK"), CustomerName: "Alison Tran", Phone: "07866 554433", PickupAddress: "8 Southchurch Road, Southend", Destination: "Shoeburyness Station", PickupZone: "Z18", DestZone: dispatch.NearestZone(51.531, 0.798), Source: models.SourceApp, Type: models.BookingImmediate, Status: models.BookingDispatched, CreatedAt: now.Add(-8 * time.Minute), RequestedTime: now.Add(-8 * time.Minute), Lat: 51.533, Lng: 0.709, DestLat: 51.531, DestLng: 0.798},

		// Pending immediate
		{ID: dispatch.GenerateID("BK"), CustomerName: "Trevor Banks", Phone: "01702 334455", PickupAddress: "Shoebury Common Beach", Destination: "Southend Victoria Station", PickupZone: "Z22", DestZone: dispatch.NearestZone(51.538, 0.713), Source: models.SourcePhone, Type: models.BookingImmediate, Status: models.BookingPending, CreatedAt: now.Add(-3 * time.Minute), RequestedTime: now.Add(-3 * time.Minute), Lat: 51.527, Lng: 0.790, DestLat: 51.538, DestLng: 0.713},

		// Prebooks — today
		{ID: dispatch.GenerateID("BK"), CustomerName: "Helen Drummond", Phone: "07923 456789", PickupAddress: "22 The Leas, Westcliff", Destination: "Southend Airport", PickupZone: "Z16", DestZone: dispatch.NearestZone(51.571, 0.695), Source: models.SourceApp, Type: models.BookingPrebook, Status: models.BookingPending, CreatedAt: now.Add(-1 * time.Hour), RequestedTime: now.Add(1 * time.Hour), Notes: "Flight at 14:30 — please do not be late", Lat: 51.537, Lng: 0.660, DestLat: 51.571, DestLng: 0.695},
		{ID: dispatch.GenerateID("BK"), CustomerName: "Graham Osei", Phone: "01702 112233", PickupAddress: "Southend Central Station", Destination: "Rochford Station", PickupZone: "Z18", DestZone: dispatch.NearestZone(51.581, 0.708), Source: models.SourcePhone, Type: models.BookingPrebook, Status: models.BookingPending, CreatedAt: now.Add(-30 * time.Minute), RequestedTime: now.Add(2 * time.Hour), Lat: 51.536, Lng: 0.708, DestLat: 51.581, DestLng: 0.708},
		{ID: dispatch.GenerateID("BK"), CustomerName: "Yvonne Clarke", Phone: "07555 667788", PickupAddress: "47 Thorpe Hall Avenue, Southend", Destination: "Kursaal", PickupZone: "Z20", DestZone: dispatch.NearestZone(51.532, 0.718), Source: models.SourceApp, Type: models.BookingPrebook, Status: models.BookingPending, CreatedAt: now.Add(-15 * time.Minute), RequestedTime: now.Add(3 * time.Hour), IsAccount: true, Notes: "Account: Clarke & Sons Ltd", Lat: 51.529, Lng: 0.748, DestLat: 51.532, DestLng: 0.718},
		{ID: dispatch.GenerateID("BK"), CustomerName: "Martin Everett", Phone: "01702 990011", PickupAddress: "Broadway, Leigh-on-Sea", Destination: "Southend Hospital", PickupZone: "Z15", DestZone: dispatch.NearestZone(51.549, 0.698), Source: models.SourcePhone, Type: models.BookingPrebook, Status: models.BookingDispatched, CreatedAt: now.Add(-2 * time.Hour), RequestedTime: now.Add(30 * time.Minute), Lat: 51.541, Lng: 0.645, DestLat: 51.549, DestLng: 0.698},
		{ID: dispatch.GenerateID("BK"), CustomerName: "Diane Kowalski", Phone: "07999 112233", PickupAddress: "The Royals Shopping Centre, Southend", Destination: "Westcliff Station", PickupZone: "Z18", DestZone: dispatch.NearestZone(51.537, 0.678), Source: models.SourceApp, Type: models.BookingPrebook, Status: models.BookingPending, CreatedAt: now.Add(-45 * time.Minute), RequestedTime: now.Add(4 * time.Hour), IsAccount: true, Notes: "Wheelchair user — please ensure accessible vehicle", Lat: 51.534, Lng: 0.706, DestLat: 51.537, DestLng: 0.678},

		// Prebooks — tomorrow
		{ID: dispatch.GenerateID("BK"), CustomerName: "Stephen Adeyemi", Phone: "07700 445566", PickupAddress: "Chalkwell Station", Destination: "Southend Airport", PickupZone: "Z16", DestZone: dispatch.NearestZone(51.571, 0.695), Source: models.SourceApp, Type: models.BookingPrebook, Status: models.BookingPending, CreatedAt: now, RequestedTime: now.Add(22 * time.Hour), Lat: 51.538, Lng: 0.659, DestLat: 51.571, DestLng: 0.695},
		{ID: dispatch.GenerateID("BK"), CustomerName: "Rachel Nguyen", Phone: "01702 778899", PickupAddress: "85 Eastern Avenue, Southend", Destination: "Shoeburyness Station", PickupZone: "Z13", DestZone: dispatch.NearestZone(51.531, 0.798), Source: models.SourcePhone, Type: models.BookingPrebook, Status: models.BookingPending, CreatedAt: now.Add(-10 * time.Minute), RequestedTime: now.Add(25 * time.Hour), Notes: "Large luggage — moving house", Lat: 51.543, Lng: 0.737, DestLat: 51.531, DestLng: 0.798},
		{ID: dispatch.GenerateID("BK"), CustomerName: "Thomas Hewitt", Phone: "07811 334455", PickupAddress: "Priory Crescent, Southend", Destination: "Leigh-on-Sea Station", PickupZone: "Z12", DestZone: dispatch.NearestZone(51.540, 0.658), Source: models.SourceApp, Type: models.BookingPrebook, Status: models.BookingPending, CreatedAt: now.Add(-5 * time.Minute), RequestedTime: now.Add(28 * time.Hour), IsAccount: true, Lat: 51.545, Lng: 0.722, DestLat: 51.540, DestLng: 0.658},
		{ID: dispatch.GenerateID("BK"), CustomerName: "Josephine Carr", Phone: "01702 223344", PickupAddress: "Blenheim Chase, Leigh-on-Sea", Destination: "Southend Victoria Station", PickupZone: "Z05", DestZone: dispatch.NearestZone(51.538, 0.713), Source: models.SourcePhone, Type: models.BookingPrebook, Status: models.BookingPending, CreatedAt: now, RequestedTime: now.Add(30 * time.Hour), Lat: 51.557, Lng: 0.691, DestLat: 51.538, DestLng: 0.713},
		{ID: dispatch.GenerateID("BK"), CustomerName: "Kevin Andersen", Phone: "07788 990011", PickupAddress: "Progress Road, Leigh-on-Sea", Destination: "Southend Hospital", PickupZone: "Z01", DestZone: dispatch.NearestZone(51.549, 0.698), Source: models.SourceApp, Type: models.BookingPrebook, Status: models.BookingPending, CreatedAt: now, RequestedTime: now.Add(32 * time.Hour), Notes: "Hospital appointment — needs punctuality", Lat: 51.568, Lng: 0.672, DestLat: 51.549, DestLng: 0.698},

		// Prebooks — day after tomorrow
		{ID: dispatch.GenerateID("BK"), CustomerName: "Caroline Briggs", Phone: "07866 112233", PickupAddress: "Thorpe Bay seafront", Destination: "Southend Airport", PickupZone: "Z21", DestZone: dispatch.NearestZone(51.571, 0.695), Source: models.SourceApp, Type: models.BookingPrebook, Status: models.BookingPending, CreatedAt: now.Add(-20 * time.Minute), RequestedTime: now.Add(48 * time.Hour), Lat: 51.527, Lng: 0.756, DestLat: 51.571, DestLng: 0.695},
		{ID: dispatch.GenerateID("BK"), CustomerName: "Peter Holloway", Phone: "01702 556677", PickupAddress: "Fairway Gardens, Leigh-on-Sea", Destination: "Victoria Shopping Centre", PickupZone: "Z04", DestZone: dispatch.NearestZone(51.537, 0.711), Source: models.SourcePhone, Type: models.BookingPrebook, Status: models.BookingPending, CreatedAt: now.Add(-8 * time.Minute), RequestedTime: now.Add(50 * time.Hour), IsAccount: true, Notes: "Account: Holloway Estates", Lat: 51.558, Lng: 0.660, DestLat: 51.537, DestLng: 0.711},
		{ID: dispatch.GenerateID("BK"), CustomerName: "Anita Sharma", Phone: "07900 334455", PickupAddress: "Highlands Boulevard, Leigh-on-Sea", Destination: "Rochford Station", PickupZone: "Z08", DestZone: dispatch.NearestZone(51.581, 0.708), Source: models.SourceApp, Type: models.BookingPrebook, Status: models.BookingPending, CreatedAt: now, RequestedTime: now.Add(52 * time.Hour), Lat: 51.549, Lng: 0.648, DestLat: 51.581, DestLng: 0.708},
		{ID: dispatch.GenerateID("BK"), CustomerName: "David Okonkwo", Phone: "01702 887766", PickupAddress: "Ross Way, Southend", Destination: "Adventure Island", PickupZone: "Z10", DestZone: dispatch.NearestZone(51.531, 0.720), Source: models.SourcePhone, Type: models.BookingPrebook, Status: models.BookingPending, CreatedAt: now, RequestedTime: now.Add(54 * time.Hour), Notes: "School trip — 4 children plus 2 adults, need large vehicle", Lat: 51.548, Lng: 0.696, DestLat: 51.531, DestLng: 0.720},
	}

	// Wire up seed dispatched bookings with drivers and Job records.
	// simTick needs a JobAccepted entry in state.Jobs to know where to move each driver.
	findDriver := func(id string) *models.Driver {
		for _, d := range state.Drivers {
			if d.ID == id {
				return d
			}
		}
		return nil
	}
	removeFromZone := func(d *models.Driver) {
		for _, z := range state.Zones {
			for i, zd := range z.Drivers {
				if zd.ID == d.ID {
					z.Drivers = append(z.Drivers[:i], z.Drivers[i+1:]...)
					return
				}
			}
		}
	}

	// Nadia Patel (Z17)    → D23 Xena Dale   (Westcliff)
	// Robert Finch (Z15)   → D20 Umar Vance  (Broadway)
	// Alison Tran (Z18)    → D25 Zoe Ford    (Town)
	// Martin Everett (Z15) → D21 Vera Walsh  (Broadway)
	seedDispatches := []struct {
		customerName string
		driverID     string
	}{
		{"Nadia Patel", "D23"},
		{"Robert Finch", "D20"},
		{"Alison Tran", "D25"},
		{"Martin Everett", "D21"},
	}
	for _, sd := range seedDispatches {
		var booking *models.Booking
		for _, b := range state.Bookings {
			if b.CustomerName == sd.customerName {
				booking = b
				break
			}
		}
		if booking == nil {
			continue
		}
		driver := findDriver(sd.driverID)
		if driver == nil {
			continue
		}
		driver.Status = models.StatusDispatched
		removeFromZone(driver)
		state.Jobs = append(state.Jobs, &models.Job{
			ID:        dispatch.GenerateID("JB"),
			Booking:   booking,
			Driver:    driver,
			Status:    models.JobAccepted,
			OfferedAt: time.Now(),
		})
	}

	dispatch.StartScheduler(state)
	dispatch.StartSimulation(state)

	mux := http.NewServeMux()
	h := &api.Handler{State: state}
	h.RegisterRoutes(mux)

	fmt.Println("Server starting on http://localhost:8080")
	if err := http.ListenAndServe(":8080", mux); err != nil {
		fmt.Printf("Server error: %v\n", err)
	}
}
