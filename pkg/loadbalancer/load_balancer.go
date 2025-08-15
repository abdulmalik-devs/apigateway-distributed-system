package loadbalancer

import (
	"math/rand"
	"net/url"
	"sync"
	"sync/atomic"
	"time"
)

// LoadBalancer interface defines load balancing methods
type LoadBalancer interface {
	NextTarget() *url.URL
	AddTarget(target *url.URL)
	RemoveTarget(target *url.URL)
	GetTargets() []*url.URL
}

// HealthChecker interface for health checking
type HealthChecker interface {
	MarkHealthy(target *url.URL)
	MarkUnhealthy(target *url.URL)
	IsHealthy(target *url.URL) bool
}

// RoundRobin implements round-robin load balancing
type RoundRobin struct {
	targets []*url.URL
	current uint64
	mu      sync.RWMutex
}

// NewRoundRobin creates a new round-robin load balancer
func NewRoundRobin(targets []*url.URL) *RoundRobin {
	return &RoundRobin{
		targets: targets,
	}
}

// NextTarget returns the next target using round-robin
func (rr *RoundRobin) NextTarget() *url.URL {
	rr.mu.RLock()
	defer rr.mu.RUnlock()

	if len(rr.targets) == 0 {
		return nil
	}

	index := atomic.AddUint64(&rr.current, 1) % uint64(len(rr.targets))
	return rr.targets[index]
}

// AddTarget adds a new target
func (rr *RoundRobin) AddTarget(target *url.URL) {
	rr.mu.Lock()
	defer rr.mu.Unlock()
	rr.targets = append(rr.targets, target)
}

// RemoveTarget removes a target
func (rr *RoundRobin) RemoveTarget(target *url.URL) {
	rr.mu.Lock()
	defer rr.mu.Unlock()

	for i, t := range rr.targets {
		if t.String() == target.String() {
			rr.targets = append(rr.targets[:i], rr.targets[i+1:]...)
			break
		}
	}
}

// GetTargets returns all targets
func (rr *RoundRobin) GetTargets() []*url.URL {
	rr.mu.RLock()
	defer rr.mu.RUnlock()

	targets := make([]*url.URL, len(rr.targets))
	copy(targets, rr.targets)
	return targets
}

// WeightedRoundRobin implements weighted round-robin load balancing
type WeightedRoundRobin struct {
	targets []*WeightedTarget
	current int
	mu      sync.RWMutex
}

// WeightedTarget represents a target with weight
type WeightedTarget struct {
	URL           *url.URL
	Weight        int
	CurrentWeight int
}

// NewWeightedRoundRobin creates a new weighted round-robin load balancer
func NewWeightedRoundRobin(targets []*url.URL, weights []int) *WeightedRoundRobin {
	weightedTargets := make([]*WeightedTarget, len(targets))

	for i, target := range targets {
		weight := 1 // Default weight
		if i < len(weights) && weights[i] > 0 {
			weight = weights[i]
		}

		weightedTargets[i] = &WeightedTarget{
			URL:           target,
			Weight:        weight,
			CurrentWeight: 0,
		}
	}

	return &WeightedRoundRobin{
		targets: weightedTargets,
	}
}

// NextTarget returns the next target using weighted round-robin
func (wrr *WeightedRoundRobin) NextTarget() *url.URL {
	wrr.mu.Lock()
	defer wrr.mu.Unlock()

	if len(wrr.targets) == 0 {
		return nil
	}

	// Calculate total weight
	totalWeight := 0
	for _, target := range wrr.targets {
		totalWeight += target.Weight
		target.CurrentWeight += target.Weight
	}

	// Find target with highest current weight
	var selected *WeightedTarget
	for _, target := range wrr.targets {
		if selected == nil || target.CurrentWeight > selected.CurrentWeight {
			selected = target
		}
	}

	if selected != nil {
		selected.CurrentWeight -= totalWeight
		return selected.URL
	}

	return nil
}

// AddTarget adds a new target
func (wrr *WeightedRoundRobin) AddTarget(target *url.URL) {
	wrr.mu.Lock()
	defer wrr.mu.Unlock()

	weightedTarget := &WeightedTarget{
		URL:           target,
		Weight:        1,
		CurrentWeight: 0,
	}
	wrr.targets = append(wrr.targets, weightedTarget)
}

// RemoveTarget removes a target
func (wrr *WeightedRoundRobin) RemoveTarget(target *url.URL) {
	wrr.mu.Lock()
	defer wrr.mu.Unlock()

	for i, t := range wrr.targets {
		if t.URL.String() == target.String() {
			wrr.targets = append(wrr.targets[:i], wrr.targets[i+1:]...)
			break
		}
	}
}

// GetTargets returns all targets
func (wrr *WeightedRoundRobin) GetTargets() []*url.URL {
	wrr.mu.RLock()
	defer wrr.mu.RUnlock()

	targets := make([]*url.URL, len(wrr.targets))
	for i, t := range wrr.targets {
		targets[i] = t.URL
	}
	return targets
}

// Random implements random load balancing
type Random struct {
	targets []*url.URL
	rand    *rand.Rand
	mu      sync.RWMutex
}

// NewRandom creates a new random load balancer
func NewRandom(targets []*url.URL) *Random {
	return &Random{
		targets: targets,
		rand:    rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// NextTarget returns a random target
func (r *Random) NextTarget() *url.URL {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if len(r.targets) == 0 {
		return nil
	}

	index := r.rand.Intn(len(r.targets))
	return r.targets[index]
}

// AddTarget adds a new target
func (r *Random) AddTarget(target *url.URL) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.targets = append(r.targets, target)
}

// RemoveTarget removes a target
func (r *Random) RemoveTarget(target *url.URL) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for i, t := range r.targets {
		if t.String() == target.String() {
			r.targets = append(r.targets[:i], r.targets[i+1:]...)
			break
		}
	}
}

// GetTargets returns all targets
func (r *Random) GetTargets() []*url.URL {
	r.mu.RLock()
	defer r.mu.RUnlock()

	targets := make([]*url.URL, len(r.targets))
	copy(targets, r.targets)
	return targets
}

// LeastConnections implements least connections load balancing
type LeastConnections struct {
	targets []*ConnectionTarget
	mu      sync.RWMutex
}

// ConnectionTarget represents a target with connection count
type ConnectionTarget struct {
	URL         *url.URL
	Connections int64
	Healthy     bool
}

// NewLeastConnections creates a new least connections load balancer
func NewLeastConnections(targets []*url.URL) *LeastConnections {
	connectionTargets := make([]*ConnectionTarget, len(targets))

	for i, target := range targets {
		connectionTargets[i] = &ConnectionTarget{
			URL:         target,
			Connections: 0,
			Healthy:     true,
		}
	}

	return &LeastConnections{
		targets: connectionTargets,
	}
}

// NextTarget returns the target with least connections
func (lc *LeastConnections) NextTarget() *url.URL {
	lc.mu.RLock()
	defer lc.mu.RUnlock()

	if len(lc.targets) == 0 {
		return nil
	}

	var selected *ConnectionTarget
	for _, target := range lc.targets {
		if !target.Healthy {
			continue
		}

		if selected == nil || target.Connections < selected.Connections {
			selected = target
		}
	}

	if selected != nil {
		atomic.AddInt64(&selected.Connections, 1)
		return selected.URL
	}

	return nil
}

// ReleaseConnection releases a connection for a target
func (lc *LeastConnections) ReleaseConnection(target *url.URL) {
	lc.mu.RLock()
	defer lc.mu.RUnlock()

	for _, t := range lc.targets {
		if t.URL.String() == target.String() {
			if t.Connections > 0 {
				atomic.AddInt64(&t.Connections, -1)
			}
			break
		}
	}
}

// AddTarget adds a new target
func (lc *LeastConnections) AddTarget(target *url.URL) {
	lc.mu.Lock()
	defer lc.mu.Unlock()

	connectionTarget := &ConnectionTarget{
		URL:         target,
		Connections: 0,
		Healthy:     true,
	}
	lc.targets = append(lc.targets, connectionTarget)
}

// RemoveTarget removes a target
func (lc *LeastConnections) RemoveTarget(target *url.URL) {
	lc.mu.Lock()
	defer lc.mu.Unlock()

	for i, t := range lc.targets {
		if t.URL.String() == target.String() {
			lc.targets = append(lc.targets[:i], lc.targets[i+1:]...)
			break
		}
	}
}

// GetTargets returns all targets
func (lc *LeastConnections) GetTargets() []*url.URL {
	lc.mu.RLock()
	defer lc.mu.RUnlock()

	targets := make([]*url.URL, len(lc.targets))
	for i, t := range lc.targets {
		targets[i] = t.URL
	}
	return targets
}

// MarkHealthy marks a target as healthy
func (lc *LeastConnections) MarkHealthy(target *url.URL) {
	lc.mu.Lock()
	defer lc.mu.Unlock()

	for _, t := range lc.targets {
		if t.URL.String() == target.String() {
			t.Healthy = true
			break
		}
	}
}

// MarkUnhealthy marks a target as unhealthy
func (lc *LeastConnections) MarkUnhealthy(target *url.URL) {
	lc.mu.Lock()
	defer lc.mu.Unlock()

	for _, t := range lc.targets {
		if t.URL.String() == target.String() {
			t.Healthy = false
			break
		}
	}
}

// IsHealthy checks if a target is healthy
func (lc *LeastConnections) IsHealthy(target *url.URL) bool {
	lc.mu.RLock()
	defer lc.mu.RUnlock()

	for _, t := range lc.targets {
		if t.URL.String() == target.String() {
			return t.Healthy
		}
	}
	return false
}

