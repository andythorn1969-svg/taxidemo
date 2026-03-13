// ============================================================
// Taxidemo - Southend Taxi Cooperative Dispatch Demo
// Version: 0.1.0
// Package models: Core data structures, constants and seed data
// ============================================================

package models

import (
	"math/rand"
	"sync"
	"time"
)

// DriverStatus represents whether a driver is free or occupied.
type DriverStatus string

const (
	StatusAvailable  DriverStatus = "available"
	StatusBusy       DriverStatus = "busy"
	StatusDispatched DriverStatus = "dispatched" // en route to pickup
	StatusOnJob      DriverStatus = "on_job"     // passenger aboard, en route to destination
)

// Driver represents a taxi driver in the cooperative.
type Driver struct {
	ID          string
	Name        string
	ZoneID      string
	Status      DriverStatus
	FreeAt      time.Time // when they became available - determines trap position
	Lat         float64   // GPS latitude
	Lng         float64   // GPS longitude
	PlateNumber int       // cooperative cab number (1-500)
}

// Zone represents a geographic dispatch area with an ordered trap queue.
type Zone struct {
	ID                     string
	Name                   string
	Drivers                []*Driver // ordered slice - index 0 is trap 1
	AverageApproachMinutes int
}

// BookingType distinguishes immediate bookings from pre-booked ones.
type BookingType string

const (
	BookingImmediate BookingType = "immediate"
	BookingPrebook   BookingType = "prebook"
)

// BookingStatus tracks where a booking is in its lifecycle.
type BookingStatus string

const (
	BookingPending    BookingStatus = "pending"
	BookingDispatched BookingStatus = "dispatched"
	BookingCompleted  BookingStatus = "completed"
	BookingCancelled  BookingStatus = "cancelled"
)

// BookingSource tells us how the booking arrived.
type BookingSource string

const (
	SourcePhone BookingSource = "phone"
	SourceApp   BookingSource = "app"
)

// Booking represents a passenger's request for a taxi.
type Booking struct {
	ID            string
	Passenger     string
	PickupZone    string
	Destination   string
	Source        BookingSource
	Status        BookingStatus
	CreatedAt     time.Time
	Lat           float64 // pickup GPS latitude
	Lng           float64 // pickup GPS longitude
	DestLat       float64 // destination GPS latitude
	DestLng       float64 // destination GPS longitude
	Type          BookingType
	CustomerName  string
	Phone         string
	Notes         string
	IsAccount     bool
	PickupAddress string
	DestAddress   string
	DestZone      string
	RequestedTime time.Time
	CompletedAt   *time.Time
}

// JobStatus tracks the offer/response lifecycle.
type JobStatus string

const (
	JobOffered   JobStatus = "offered"
	JobAccepted  JobStatus = "accepted"
	JobDeclined  JobStatus = "declined"
	JobCompleted JobStatus = "completed"
)

// Job links a booking to a driver and tracks the dispatch attempt.
type Job struct {
	ID        string
	Booking   *Booking
	Driver    *Driver
	Status    JobStatus
	OfferedAt time.Time

	// Fixed step deltas computed once on the first simulation tick of each leg.
	// Using a fixed step ensures the driver travels the full distance in exactly
	// SimJourneyMinutes regardless of geographic distance.
	PickupStepSet bool
	PickupStepLat float64
	PickupStepLng float64
	DestStepSet   bool
	DestStepLat   float64
	DestStepLng   float64
}

// Customer represents a known passenger, potentially an account holder.
type Customer struct {
	ID                    string   `json:"id"`
	Name                  string   `json:"name"`
	Phone                 string   `json:"phone"`
	Address               string   `json:"address"`
	Notes                 string   `json:"notes"`
	FavouriteDestinations []string `json:"favourite_destinations"`
	IsAccount             bool     `json:"is_account"`
	NoShowCount       int  `json:"no_show_count"`
	CancellationCount int  `json:"cancellation_count"`
	Flagged           bool `json:"flagged"`
	Blocked           bool `json:"blocked"`
}

// AppState holds all runtime state for the dispatch system.
type AppState struct {
	Mu        sync.RWMutex
	Drivers   []*Driver
	Zones     []*Zone
	Bookings  []*Booking
	Jobs      []*Job
	Customers []*Customer
}

// SeedData creates the initial zones and drivers for the demo.
func SeedData() ([]*Zone, []*Driver) {
	now := time.Now()
	plate := func() int { return rand.Intn(500) + 1 }

	// 30 drivers spread across 22 real Southend Taxi Cooperative zones.
	// FreeAt times vary to simulate realistic trap queue positions.
	// Coordinates are near each zone centre with small offsets.
	drivers := []*Driver{
		// Z01 Progress
		{ID: "D01", Name: "Alice Brown", ZoneID: "Z01", Status: StatusAvailable, FreeAt: now.Add(-55 * time.Minute), Lat: 51.569, Lng: 0.671, PlateNumber: plate()},
		{ID: "D02", Name: "Bob Carter", ZoneID: "Z01", Status: StatusAvailable, FreeAt: now.Add(-32 * time.Minute), Lat: 51.567, Lng: 0.674, PlateNumber: plate()},
		// Z02 Thanet
		{ID: "D03", Name: "Carol Davies", ZoneID: "Z02", Status: StatusAvailable, FreeAt: now.Add(-48 * time.Minute), Lat: 51.566, Lng: 0.701, PlateNumber: plate()},
		// Z03 Blue
		{ID: "D04", Name: "Dave Ellis", ZoneID: "Z03", Status: StatusAvailable, FreeAt: now.Add(-20 * time.Minute), Lat: 51.562, Lng: 0.731, PlateNumber: plate()},
		// Z04 Fairway
		{ID: "D05", Name: "Emma Foster", ZoneID: "Z04", Status: StatusAvailable, FreeAt: now.Add(-70 * time.Minute), Lat: 51.559, Lng: 0.659, PlateNumber: plate()},
		{ID: "D06", Name: "Frank Green", ZoneID: "Z04", Status: StatusAvailable, FreeAt: now.Add(-40 * time.Minute), Lat: 51.557, Lng: 0.661, PlateNumber: plate()},
		// Z05 Blenheim
		{ID: "D07", Name: "Grace Hill", ZoneID: "Z05", Status: StatusAvailable, FreeAt: now.Add(-35 * time.Minute), Lat: 51.557, Lng: 0.691, PlateNumber: plate()},
		// Z06 Temple
		{ID: "D08", Name: "Harry Irving", ZoneID: "Z06", Status: StatusBusy, FreeAt: now.Add(-10 * time.Minute), Lat: 51.557, Lng: 0.717, PlateNumber: plate()},
		{ID: "D09", Name: "Isla Jones", ZoneID: "Z06", Status: StatusAvailable, FreeAt: now.Add(-45 * time.Minute), Lat: 51.555, Lng: 0.719, PlateNumber: plate()},
		// Z07 Fossett
		{ID: "D10", Name: "Jack King", ZoneID: "Z07", Status: StatusAvailable, FreeAt: now.Add(-28 * time.Minute), Lat: 51.553, Lng: 0.741, PlateNumber: plate()},
		// Z08 Highlands
		{ID: "D11", Name: "Karen Lee", ZoneID: "Z08", Status: StatusAvailable, FreeAt: now.Add(-62 * time.Minute), Lat: 51.550, Lng: 0.647, PlateNumber: plate()},
		// Z09 Elms
		{ID: "D12", Name: "Liam Morris", ZoneID: "Z09", Status: StatusAvailable, FreeAt: now.Add(-38 * time.Minute), Lat: 51.549, Lng: 0.671, PlateNumber: plate()},
		{ID: "D13", Name: "Mia Nash", ZoneID: "Z09", Status: StatusAvailable, FreeAt: now.Add(-15 * time.Minute), Lat: 51.547, Lng: 0.673, PlateNumber: plate()},
		// Z10 Ross
		{ID: "D14", Name: "Noah Owen", ZoneID: "Z10", Status: StatusAvailable, FreeAt: now.Add(-50 * time.Minute), Lat: 51.548, Lng: 0.696, PlateNumber: plate()},
		// Z11 Plough
		{ID: "D15", Name: "Olivia Page", ZoneID: "Z11", Status: StatusAvailable, FreeAt: now.Add(-80 * time.Minute), Lat: 51.546, Lng: 0.707, PlateNumber: plate()},
		{ID: "D16", Name: "Peter Quinn", ZoneID: "Z11", Status: StatusAvailable, FreeAt: now.Add(-22 * time.Minute), Lat: 51.544, Lng: 0.709, PlateNumber: plate()},
		// Z12 Priory
		{ID: "D17", Name: "Rachel Reed", ZoneID: "Z12", Status: StatusAvailable, FreeAt: now.Add(-33 * time.Minute), Lat: 51.545, Lng: 0.721, PlateNumber: plate()},
		// Z13 VAC
		{ID: "D18", Name: "Sam Scott", ZoneID: "Z13", Status: StatusBusy, FreeAt: now.Add(-8 * time.Minute), Lat: 51.542, Lng: 0.736, PlateNumber: plate()},
		// Z14 Green
		{ID: "D19", Name: "Tina Turner", ZoneID: "Z14", Status: StatusAvailable, FreeAt: now.Add(-42 * time.Minute), Lat: 51.540, Lng: 0.749, PlateNumber: plate()},
		// Z15 Broadway
		{ID: "D20", Name: "Umar Vance", ZoneID: "Z15", Status: StatusAvailable, FreeAt: now.Add(-58 * time.Minute), Lat: 51.541, Lng: 0.644, PlateNumber: plate()},
		{ID: "D21", Name: "Vera Walsh", ZoneID: "Z15", Status: StatusAvailable, FreeAt: now.Add(-18 * time.Minute), Lat: 51.539, Lng: 0.646, PlateNumber: plate()},
		// Z16 Chalkwell
		{ID: "D22", Name: "Will Cross", ZoneID: "Z16", Status: StatusAvailable, FreeAt: now.Add(-44 * time.Minute), Lat: 51.538, Lng: 0.659, PlateNumber: plate()},
		// Z17 Westcliff
		{ID: "D23", Name: "Xena Dale", ZoneID: "Z17", Status: StatusAvailable, FreeAt: now.Add(-65 * time.Minute), Lat: 51.536, Lng: 0.677, PlateNumber: plate()},
		{ID: "D24", Name: "Yusuf Evans", ZoneID: "Z17", Status: StatusAvailable, FreeAt: now.Add(-27 * time.Minute), Lat: 51.534, Lng: 0.679, PlateNumber: plate()},
		// Z18 Town
		{ID: "D25", Name: "Zoe Ford", ZoneID: "Z18", Status: StatusAvailable, FreeAt: now.Add(-90 * time.Minute), Lat: 51.534, Lng: 0.707, PlateNumber: plate()},
		{ID: "D26", Name: "Aaron Gray", ZoneID: "Z18", Status: StatusAvailable, FreeAt: now.Add(-36 * time.Minute), Lat: 51.532, Lng: 0.709, PlateNumber: plate()},
		// Z19 Kursaal
		{ID: "D27", Name: "Beth Hunt", ZoneID: "Z19", Status: StatusAvailable, FreeAt: now.Add(-52 * time.Minute), Lat: 51.532, Lng: 0.723, PlateNumber: plate()},
		// Z20 Thorpe
		{ID: "D28", Name: "Colin Irons", ZoneID: "Z20", Status: StatusAvailable, FreeAt: now.Add(-30 * time.Minute), Lat: 51.529, Lng: 0.741, PlateNumber: plate()},
		// Z21 Bay
		{ID: "D29", Name: "Donna Jay", ZoneID: "Z21", Status: StatusAvailable, FreeAt: now.Add(-47 * time.Minute), Lat: 51.527, Lng: 0.756, PlateNumber: plate()},
		// Z22 Shoebury
		{ID: "D30", Name: "Eddie Kane", ZoneID: "Z22", Status: StatusAvailable, FreeAt: now.Add(-25 * time.Minute), Lat: 51.528, Lng: 0.774, PlateNumber: plate()},
	}

	zones := []*Zone{
		{ID: "Z01", Name: "Progress", Drivers: []*Driver{drivers[0], drivers[1]}},
		{ID: "Z02", Name: "Thanet", Drivers: []*Driver{drivers[2]}},
		{ID: "Z03", Name: "Blue", Drivers: []*Driver{drivers[3]}},
		{ID: "Z04", Name: "Fairway", Drivers: []*Driver{drivers[4], drivers[5]}},
		{ID: "Z05", Name: "Blenheim", Drivers: []*Driver{drivers[6]}},
		{ID: "Z06", Name: "Temple", Drivers: []*Driver{drivers[7], drivers[8]}},
		{ID: "Z07", Name: "Fossett", Drivers: []*Driver{drivers[9]}},
		{ID: "Z08", Name: "Highlands", Drivers: []*Driver{drivers[10]}},
		{ID: "Z09", Name: "Elms", Drivers: []*Driver{drivers[11], drivers[12]}},
		{ID: "Z10", Name: "Ross", Drivers: []*Driver{drivers[13]}},
		{ID: "Z11", Name: "Plough", Drivers: []*Driver{drivers[14], drivers[15]}},
		{ID: "Z12", Name: "Priory", Drivers: []*Driver{drivers[16]}},
		{ID: "Z13", Name: "VAC", Drivers: []*Driver{drivers[17]}},
		{ID: "Z14", Name: "Green", Drivers: []*Driver{drivers[18]}},
		{ID: "Z15", Name: "Broadway", Drivers: []*Driver{drivers[19], drivers[20]}},
		{ID: "Z16", Name: "Chalkwell", Drivers: []*Driver{drivers[21]}},
		{ID: "Z17", Name: "Westcliff", Drivers: []*Driver{drivers[22], drivers[23]}},
		{ID: "Z18", Name: "Town", Drivers: []*Driver{drivers[24], drivers[25]}},
		{ID: "Z19", Name: "Kursaal", Drivers: []*Driver{drivers[26]}},
		{ID: "Z20", Name: "Thorpe", Drivers: []*Driver{drivers[27]}},
		{ID: "Z21", Name: "Bay", Drivers: []*Driver{drivers[28]}},
		{ID: "Z22", Name: "Shoebury", Drivers: []*Driver{drivers[29]}},
	}

	return zones, drivers
}
