package core

import (
	"time"

	"github.com/evcc-io/evcc/api"
	"github.com/evcc-io/evcc/core/planner"
	"github.com/evcc-io/evcc/core/soc"
	"golang.org/x/exp/slices"
)

// setPlanActive updates plan active flag
func (lp *Loadpoint) setPlanActive(active bool) {
	if !active {
		lp.planSlotEnd = time.Time{}
	}
	lp.planActive = active
	lp.publish("planActive", lp.planActive)
}

// planRequiredDuration is the estimated total charging duration
func (lp *Loadpoint) planRequiredDuration(maxPower float64) time.Duration {
	var requiredDuration time.Duration

	if energy, ok := lp.remainingChargeEnergy(); ok {
		requiredDuration = time.Duration(energy * 1e3 / maxPower * float64(time.Hour))
	} else {
		// TODO vehicle soc limit
		targetSoc := lp.Soc.target
		if targetSoc == 0 {
			targetSoc = 100
		}

		requiredDuration = lp.socEstimator.RemainingChargeDuration(targetSoc, maxPower)

		// anticipate lower charge rates at end of charging curve
		var additionalDuration time.Duration

		if targetSoc > 80 && maxPower > 15000 {
			additionalDuration = 5 * time.Duration(float64(targetSoc-80)/(float64(targetSoc)-lp.vehicleSoc)*float64(requiredDuration))
			lp.log.DEBUG.Printf("add additional charging time %v for soc > 80%%", additionalDuration.Round(time.Minute))
		} else if targetSoc > 90 && maxPower > 4000 {
			additionalDuration = 3 * time.Duration(float64(targetSoc-90)/(float64(targetSoc)-lp.vehicleSoc)*float64(requiredDuration))
			lp.log.DEBUG.Printf("add additional charging time %v for soc > 90%%", additionalDuration.Round(time.Minute))
		}

		requiredDuration += additionalDuration
	}
	requiredDuration = time.Duration(float64(requiredDuration) / soc.ChargeEfficiency)

	return requiredDuration
}

func (lp *Loadpoint) GetPlannerUnit() string {
	return lp.planner.Unit()
}

// GetPlan creates a charging plan
//
// Results:
// - required total charging duration
// - actual charging plan as rate table
func (lp *Loadpoint) GetPlan(targetTime time.Time, maxPower float64) (time.Duration, api.Rates, error) {
	if lp.planner == nil || lp.socEstimator == nil || targetTime.IsZero() {
		return 0, nil, nil
	}

	// don't start planning into the past
	if targetTime.Before(lp.clock.Now()) && !lp.planActive {
		return 0, nil, nil
	}

	requiredDuration := lp.planRequiredDuration(maxPower)
	plan, err := lp.planner.Plan(requiredDuration, targetTime)

	// sort plan by time
	slices.SortStableFunc(plan, planner.SortByTime)

	return requiredDuration, plan, err
}

// plannerActive checks if charging plan is active
func (lp *Loadpoint) plannerActive() (active bool) {
	defer func() {
		lp.setPlanActive(active)
		lp.publish(targetTimeActive, active)
	}()

	maxPower := lp.GetMaxPower()

	requiredDuration, plan, err := lp.GetPlan(lp.GetTargetTime(), maxPower)
	if err != nil {
		lp.log.ERROR.Println("planner:", err)
		return false
	}

	// nothing to do
	if requiredDuration == 0 {
		return false
	}

	planStart := planner.Start(plan)
	lp.publish(targetTimeProjectedStart, planStart)

	lp.log.DEBUG.Printf("planned %v until %v at %.0fW: total plan duration: %v, avg cost: %.3f",
		requiredDuration.Round(time.Second), lp.targetTime.Round(time.Second).Local(), maxPower,
		planner.Duration(plan).Round(time.Second), planner.AverageCost(plan))

	// sort plan by time
	for _, slot := range plan {
		lp.log.TRACE.Printf("  slot from: %v to %v cost %.3f", slot.Start.Round(time.Second).Local(), slot.End.Round(time.Second).Local(), slot.Price)
	}

	activeSlot := planner.ActiveSlot(lp.clock, plan)
	active = !activeSlot.End.IsZero()

	if active {
		// ignore short plans if not already active
		if !lp.planActive && lp.clock.Until(activeSlot.End) < smallSlotDuration {
			lp.log.DEBUG.Printf("plan too short- ignoring remaining %v", requiredDuration.Round(time.Second))
			return false
		}

		// remember last active plan's end time
		lp.setPlanActive(true)
		lp.planSlotEnd = activeSlot.End
	} else if lp.planActive {
		// planner was active (any slot, not necessarily previous slot) and charge goal has not yet been met
		switch {
		case lp.clock.Now().After(lp.targetTime) && !lp.targetTime.IsZero():
			// if the plan did not (entirely) work, we may still be charging beyond plan end- in that case, continue charging
			// TODO check when schedule is implemented
			lp.log.DEBUG.Println("continuing after target time")
			active = true
		case lp.clock.Now().Before(lp.planSlotEnd) && !lp.planSlotEnd.IsZero():
			// don't stop an already running slot if goal was not met
			lp.log.DEBUG.Println("continuing until end of slot")
			active = true
		case requiredDuration < 30*time.Minute:
			lp.log.DEBUG.Printf("continuing for remaining %v", requiredDuration.Round(time.Second))
			active = true
		case lp.clock.Until(planStart) < smallSlotDuration:
			lp.log.DEBUG.Printf("plan will re-start shortly, continuing for remaining %v", lp.clock.Until(planStart).Round(time.Second))
			return true
		}
	}

	return active
}
