// ============================================================
// Taxidemo - Southend Taxi Cooperative Dispatch Demo
// Version: 0.1.0
// Package models: Core data structures, constants and seed data
// ============================================================

package models

import "time"

// DriverStatus represents whether a driver is free or occupied.
type DriverStatus string

const (
	StatusAvailable DriverStatus = "available"
	StatusBusy      DriverStatus = "busy"
)

// Driver represents a taxi driver in the cooperative.
type Driver struct {
	ID     string
	Name   string
	ZoneID string
	Status DriverStatus
	FreeAt time.Time // when they became available - determines trap position
	Lat    float64   // GPS latitude
	Lng    float64   // GPS longitude
}

// Zone represents a geographic dispatch area with an ordered trap queue.
type Zone struct {
	ID      string
	Name    string
	Drivers []*Driver // ordered slice - index 0 is trap 1
}

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
	ID          string
	Passenger   string
	PickupZone  string
	Destination string
	Source      BookingSource
	Status      BookingStatus
	CreatedAt   time.Time
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
}

// SeedData creates the initial zones and drivers for the demo.
func SeedData() ([]*Zone, []*Driver) {
	now := time.Now()

	drivers := []*Driver{
		{ID: "D01", Name: "Alice Brown", ZoneID: "Z01", Status: StatusAvailable, FreeAt: now.Add(-45 * time.Minute), Lat: 51.559, Lng: 0.636},
		{ID: "D02", Name: "Bob Carter", ZoneID: "Z01", Status: StatusAvailable, FreeAt: now.Add(-30 * time.Minute), Lat: 51.557, Lng: 0.639},
		{ID: "D03", Name: "Carol Davies", ZoneID: "Z02", Status: StatusAvailable, FreeAt: now.Add(-60 * time.Minute), Lat: 51.535, Lng: 0.713},
		{ID: "D04", Name: "Dave Ellis", ZoneID: "Z02", Status: StatusAvailable, FreeAt: now.Add(-15 * time.Minute), Lat: 51.532, Lng: 0.716},
		{ID: "D05", Name: "Emma Foster", ZoneID: "Z03", Status: StatusAvailable, FreeAt: now.Add(-90 * time.Minute), Lat: 51.532, Lng: 0.809},
		{ID: "D06", Name: "Frank Green", ZoneID: "Z03", Status: StatusAvailable, FreeAt: now.Add(-20 * time.Minute), Lat: 51.529, Lng: 0.807},
		{ID: "D07", Name: "Grace Hill", ZoneID: "Z04", Status: StatusAvailable, FreeAt: now.Add(-55 * time.Minute), Lat: 51.544, Lng: 0.709},
		{ID: "D08", Name: "Harry Irving", ZoneID: "Z04", Status: StatusBusy, FreeAt: now.Add(-10 * time.Minute), Lat: 51.542, Lng: 0.712},
		{ID: "D09", Name: "Isla Jones", ZoneID: "Z05", Status: StatusAvailable, FreeAt: now.Add(-35 * time.Minute), Lat: 51.558, Lng: 0.597},
		{ID: "D10", Name: "Jack King", ZoneID: "Z05", Status: StatusAvailable, FreeAt: now.Add(-25 * time.Minute), Lat: 51.555, Lng: 0.600},
	}

	zones := []*Zone{
		{ID: "Z01", Name: "North", Drivers: []*Driver{drivers[0], drivers[1]}},
		{ID: "Z02", Name: "South", Drivers: []*Driver{drivers[2], drivers[3]}},
		{ID: "Z03", Name: "East", Drivers: []*Driver{drivers[4], drivers[5]}},
		{ID: "Z04", Name: "Central", Drivers: []*Driver{drivers[6], drivers[7]}},
		{ID: "Z05", Name: "West", Drivers: []*Driver{drivers[8], drivers[9]}},
	}

	return zones, drivers
}
