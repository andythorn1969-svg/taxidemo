// ============================================================
// Taxidemo - Southend Taxi Cooperative Dispatch Demo
// Version: 0.1.0
// Description: Entry point - initialises state, registers routes, starts server
// ============================================================

package main

import (
	"fmt"
	"net/http"

	"taxidemo/api"
	"taxidemo/models"
)

func main() {
	api.AppState.Zones, api.AppState.Drivers = models.SeedData()

	http.HandleFunc("/", api.HandleIndex)
	http.HandleFunc("/dispatch", api.HandleDispatch)
	http.HandleFunc("/api/drivers", api.HandleDriverData)
	http.HandleFunc("/api/bookings", api.HandleBookingData)

	fmt.Println("Server starting on http://localhost:8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		fmt.Printf("Server error: %v\n", err)
	}
}
