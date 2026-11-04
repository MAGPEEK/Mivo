package images

import "time"

// Waiting exposes the number of requests sharing the render of key to the tests.
func (service *Service) Waiting(key Key) int {
	return service.flights.waiting(key)
}

// NewServiceWithLimits returns a service with a custom render limit and timeout.
func NewServiceWithLimits(dependencies Dependencies, maxConcurrent int, timeout time.Duration) *Service {
	return newServiceWithLimits(dependencies, maxConcurrent, timeout)
}
