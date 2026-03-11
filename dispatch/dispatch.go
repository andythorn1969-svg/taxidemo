// ============================================================
// Taxidemo - Southend Taxi Cooperative Dispatch Demo
// Version: 0.1.0
// Package dispatch: Job offer and driver assignment logic
// ============================================================

package dispatch

import (
	"fmt"
	"time"

	"taxidemo/models"
)

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
