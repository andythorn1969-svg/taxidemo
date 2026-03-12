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
	zones, drivers := models.SeedData()
	state := &models.AppState{
		Zones:   zones,
		Drivers: drivers,
	}

	mux := http.NewServeMux()
	h := &api.Handler{State: state}
	h.RegisterRoutes(mux)

	fmt.Println("Server starting on http://localhost:8080")
	if err := http.ListenAndServe(":8080", mux); err != nil {
		fmt.Printf("Server error: %v\n", err)
	}
}
