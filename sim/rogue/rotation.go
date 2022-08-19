package rogue

import (
	"math"
	"sort"
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

func (rogue *Rogue) OnEnergyGain(sim *core.Simulation) {
	if rogue.KillingSpreeAura.IsActive() {
		rogue.DoNothing()
		return
	}
	rogue.TryUseCooldowns(sim)
	if rogue.GCD.IsReady(sim) {
		rogue.simpleRotation(sim)
	}
}

func (rogue *Rogue) OnGCDReady(sim *core.Simulation) {
	if rogue.KillingSpreeAura.IsActive() {
		rogue.DoNothing()
		return
	}
	rogue.TryUseCooldowns(sim)
	if rogue.IsWaitingForEnergy() {
		rogue.DoNothing()
		return
	}
	if rogue.GCD.IsReady(sim) {
		rogue.simpleRotation(sim)
	}
}

type RogueRotationItem struct {
	ExpiresAt            time.Duration
	MinimumBuildDuration time.Duration
	MaximumBuildDuration time.Duration
	PrioIndex            int
}

type RoguePriorityItem struct {
	Aura               *core.Aura
	CastCount          int32
	EnergyCost         float64
	GetSpell           func(*Rogue, int32) *core.Spell
	MaximumComboPoints int32
	MaxCasts           int32
	MinimumComboPoints int32
}

type ShouldCastRotationItemResult int32

const (
	ShouldNotCast ShouldCastRotationItemResult = iota
	ShouldBuild
	ShouldCast
	ShouldWait
)

func (rogue *Rogue) EnergyToBuild(points int32) float64 {
	costPerBuilder := rogue.Builder.DefaultCast.Cost
	buildersNeeded := math.Ceil(float64(points) / float64(rogue.BuilderPoints))
	return buildersNeeded * costPerBuilder
}

func (rogue *Rogue) TimeToBuild(sim *core.Simulation, points int32, builderPoints int32, eps float64, finisherCost float64) time.Duration {
	energyNeeded := rogue.EnergyToBuild(points) + finisherCost
	secondsNeeded := energyNeeded / eps
	globalsNeeded := math.Ceil(float64(points)/float64(builderPoints)) + 1
	// Return greater of the time it takes to use the globals and the time it takes to build the energy
	return core.MaxDuration(time.Second*time.Duration(secondsNeeded), time.Second*time.Duration(globalsNeeded))
}

func (rogue *Rogue) ShouldCastNextPrioItem(sim *core.Simulation, eps float64) ShouldCastRotationItemResult {
	if len(rogue.RotationItems) < 1 {
		panic("Empty rotation")
	}
	currentEnergy := rogue.CurrentEnergy()
	comboPoints := rogue.ComboPoints()
	currentTime := sim.CurrentTime

	// Adjusting for planning variance
	item := rogue.RotationItems[0]
	prio := rogue.PriorityItems[item.PrioIndex]
	tte := item.ExpiresAt - currentTime

	clippingThreshold := time.Second * 2

	timeUntilNextGCD := rogue.GCD.TimeToReady(sim)

	// A higher prio item will expire within the next GCD
	if len(rogue.RotationItems) > 0 {
		for _, nextItem := range rogue.RotationItems[1:] {
			if nextItem.ExpiresAt-currentTime <= time.Second*1 {
				return ShouldNotCast
			}
		}
	}
	if tte <= timeUntilNextGCD { // Expires before next GCD
		if comboPoints >= prio.MinimumComboPoints && currentEnergy >= prio.EnergyCost {
			return ShouldCast
		} else if comboPoints < prio.MinimumComboPoints && currentEnergy >= rogue.Builder.DefaultCast.Cost {
			return ShouldBuild
		} else {
			return ShouldWait
		}
	} else { // More than a GCD before expiration
		if comboPoints >= prio.MaximumComboPoints { // Don't need CP
			// Cast
			if tte <= clippingThreshold && currentEnergy >= prio.EnergyCost {
				return ShouldCast
			}
			// Pool energy
			if tte <= clippingThreshold && currentEnergy < prio.EnergyCost {
				return ShouldWait
			}
			// We have time to squeeze in another spell
			if tte > item.MinimumBuildDuration {
				// Find the first lower prio item that can be cast and use it
				for lpi, lowerPrio := range rogue.PriorityItems[item.PrioIndex+1:] {
					if comboPoints > lowerPrio.MinimumComboPoints && currentEnergy > lowerPrio.EnergyCost && lowerPrio.MaxCasts == 0 {
						rogue.RotationItems = append([]RogueRotationItem{
							{ExpiresAt: currentTime, PrioIndex: lpi + item.PrioIndex + 1},
						}, rogue.RotationItems...)
						return ShouldCast
					}
				}
			}
			// Overcap CP with builder
			if rogue.TimeToBuild(sim, 1, rogue.BuilderPoints, eps, 0) <= tte && currentEnergy >= rogue.Builder.DefaultCast.Cost {
				return ShouldBuild
			}
		} else if comboPoints < prio.MinimumComboPoints { // Need CP
			if currentEnergy >= rogue.Builder.DefaultCast.Cost {
				return ShouldBuild
			} else {
				return ShouldWait
			}
		} else { // TODO: Optionally build more CP
			if currentEnergy >= prio.EnergyCost && tte < time.Second*1 {
				return ShouldCast
			} else if currentEnergy >= rogue.Builder.DefaultCast.Cost {
				return ShouldBuild
			} else {
				return ShouldWait
			}
		}
		return ShouldWait
	}
}

func (rogue *Rogue) PlanRotation(sim *core.Simulation) []RogueRotationItem {
	rotationItems := make([]RogueRotationItem, 0)
	eps := rogue.GetExpectedEnergyPerSecond()
	for pi, prio := range rogue.PriorityItems {
		if prio.MaxCasts > 0 && prio.CastCount >= prio.MaxCasts {
			continue
		}
		expiresAt := core.NeverExpires
		if prio.Aura != nil {
			expiresAt = prio.Aura.ExpiresAt()
		} else if prio.MaxCasts == 1 {
			expiresAt = sim.CurrentTime
		} else {
			expiresAt = sim.CurrentTime
		}
		minimumBuildDuration := rogue.TimeToBuild(sim, prio.MinimumComboPoints, rogue.BuilderPoints, eps, prio.EnergyCost)
		maximumBuildDuration := rogue.TimeToBuild(sim, prio.MaxCasts, rogue.BuilderPoints, eps, prio.EnergyCost)
		rotationItems = append(rotationItems, RogueRotationItem{
			ExpiresAt:            expiresAt,
			MaximumBuildDuration: maximumBuildDuration,
			MinimumBuildDuration: minimumBuildDuration,
			PrioIndex:            pi,
		})
	}

	currentTime := sim.CurrentTime
	comboPoints := rogue.ComboPoints()
	currentEnergy := rogue.CurrentEnergy()

	prioStack := make([]RogueRotationItem, 0)
	for _, item := range rotationItems {
		prio := rogue.PriorityItems[item.PrioIndex]
		maxBuildAt := item.ExpiresAt - item.MaximumBuildDuration
		if prio.Aura == nil {
			timeValueOfResources := time.Duration((float64(comboPoints)*rogue.Builder.DefaultCast.Cost/float64(rogue.BuilderPoints) + currentEnergy) / eps)
			maxBuildAt = currentTime - item.MaximumBuildDuration - timeValueOfResources
		}
		if currentTime < maxBuildAt {
			// Put it on the to cast stack
			prioStack = append(prioStack, item)
			if prio.MinimumComboPoints > 0 {
				comboPoints = 0
			}
			currentTime += item.MaximumBuildDuration
		} else {
			cpUsed := core.MaxInt32(0, prio.MinimumComboPoints-comboPoints)
			energyUsed := core.MaxFloat(0, prio.EnergyCost-currentEnergy)
			minBuildTime := rogue.TimeToBuild(sim, cpUsed, rogue.BuilderPoints, eps, energyUsed)
			if currentTime+minBuildTime <= item.ExpiresAt {
				prioStack = append(prioStack, item)
				currentTime = item.ExpiresAt
				currentEnergy = 0
				if prio.MinimumComboPoints > 0 {
					comboPoints = 0
				}
			} else if len(prioStack) < 1 || (prio.Aura != nil && !prio.Aura.IsActive()) || prio.MaxCasts == 1 {
				// Plan to cast it as soon as possible
				prioStack = append(prioStack, item)
				currentTime += item.MinimumBuildDuration
				currentEnergy = 0
				if prio.MinimumComboPoints > 0 {
					comboPoints = 0
				}
			}
		}
	}

	// Reverse
	sort.Slice(prioStack, func(i, j int) bool {
		return j < i
	})

	return prioStack
}

func (rogue *Rogue) SetPriorityItems(sim *core.Simulation) {
	rogue.Builder = rogue.SinisterStrike
	rogue.BuilderPoints = 1
	if rogue.Talents.Mutilate {
		rogue.Builder = rogue.Mutilate
		rogue.BuilderPoints = 2
	}
	isMultiTarget := sim.GetNumTargets() > 3
	// Slice and Dice
	rogue.PriorityItems = make([]RoguePriorityItem, 0)

	sliceAndDice := RoguePriorityItem{
		MinimumComboPoints: 1,
		MaximumComboPoints: 5,
		Aura:               rogue.SliceAndDiceAura,
		EnergyCost:         rogue.SliceAndDice[1].DefaultCast.Cost,
		GetSpell: func(r *Rogue, cp int32) *core.Spell {
			return rogue.SliceAndDice[cp]
		},
	}
	if isMultiTarget {
		if rogue.Rotation.MultiTargetSliceFrequency != proto.Rogue_Rotation_Never {
			sliceAndDice.MinimumComboPoints = rogue.Rotation.MinimumComboPointsMultiTargetSlice
			if rogue.Rotation.MultiTargetSliceFrequency == proto.Rogue_Rotation_Once {
				sliceAndDice.MaxCasts = 1
			}
			rogue.PriorityItems = append(rogue.PriorityItems, sliceAndDice)
		}
	} else {
		rogue.PriorityItems = append(rogue.PriorityItems, sliceAndDice)
	}

	// Expose Armor
	if rogue.Rotation.ExposeArmorFrequency == proto.Rogue_Rotation_Maintain ||
		rogue.Rotation.ExposeArmorFrequency == proto.Rogue_Rotation_Once {
		minPoints := int32(1)
		maxCasts := int32(0)
		if rogue.Rotation.ExposeArmorFrequency == proto.Rogue_Rotation_Once {
			minPoints = rogue.Rotation.MinimumComboPointsExposeArmor
			maxCasts = 1
		}
		rogue.PriorityItems = append(rogue.PriorityItems, RoguePriorityItem{
			MaxCasts:           maxCasts,
			MaximumComboPoints: 5,
			MinimumComboPoints: minPoints,
			Aura:               rogue.ExposeArmorAura,
			EnergyCost:         rogue.ExposeArmor[1].DefaultCast.Cost,
			GetSpell: func(r *Rogue, cp int32) *core.Spell {
				return rogue.ExposeArmor[cp]
			},
		})
	}

	// Hunger for Blood
	if rogue.Talents.HungerForBlood {
		rogue.PriorityItems = append(rogue.PriorityItems, RoguePriorityItem{
			MaximumComboPoints: 0,
			Aura:               rogue.HungerForBloodAura,
			EnergyCost:         rogue.HungerForBlood.DefaultCast.Cost,
			GetSpell: func(r *Rogue, cp int32) *core.Spell {
				return r.HungerForBlood
			},
		})
	}

	// Dummy priority to enable CDs
	rogue.PriorityItems = append(rogue.PriorityItems, RoguePriorityItem{
		MaxCasts:           1,
		MaximumComboPoints: 0,
		GetSpell: func(r *Rogue, cp int32) *core.Spell {
			if r.disabledMCDs != nil {
				r.EnableAllCooldowns(r.disabledMCDs)
				r.disabledMCDs = nil
			}
			return nil
		},
	})

	// Rupture
	rupture := RoguePriorityItem{
		MinimumComboPoints: 3,
		MaximumComboPoints: 5,
		Aura:               rogue.RuptureDot.Aura,
		EnergyCost:         rogue.Rupture[1].DefaultCast.Cost,
		GetSpell: func(r *Rogue, cp int32) *core.Spell {
			return rogue.Rupture[cp]
		},
	}

	// Eviscerate
	eviscerate := RoguePriorityItem{
		MinimumComboPoints: 1,
		MaximumComboPoints: 5,
		EnergyCost:         rogue.Eviscerate[1].DefaultCast.Cost,
		GetSpell: func(r *Rogue, cp int32) *core.Spell {
			return rogue.Eviscerate[cp]
		},
	}

	if isMultiTarget {
		rogue.PriorityItems = append(rogue.PriorityItems, RoguePriorityItem{
			MaximumComboPoints: 0,
			EnergyCost:         rogue.FanOfKnives.DefaultCast.Cost,
			GetSpell: func(r *Rogue, i int32) *core.Spell {
				return r.FanOfKnives
			},
		})

	} else if rogue.Talents.MasterPoisoner > 0 || rogue.Talents.CutToTheChase > 0 {
		// Envenom
		envenom := RoguePriorityItem{
			MinimumComboPoints: 1,
			MaximumComboPoints: 5,
			Aura:               rogue.EnvenomAura,
			EnergyCost:         rogue.Envenom[1].DefaultCast.Cost,
			GetSpell: func(r *Rogue, cp int32) *core.Spell {
				return rogue.Envenom[cp]
			},
		}
		switch rogue.Rotation.AssassinationFinisherPriority {
		case proto.Rogue_Rotation_EnvenomRupture:
			envenom.MinimumComboPoints = core.MaxInt32(1, rogue.Rotation.MinimumComboPointsPrimaryFinisher)
			rogue.PriorityItems = append(rogue.PriorityItems, envenom)
			rupture.MinimumComboPoints = rogue.Rotation.MinimumComboPointsSecondaryFinisher
			if rupture.MinimumComboPoints > 0 && rupture.MinimumComboPoints <= 5 {
				rogue.PriorityItems = append(rogue.PriorityItems, rupture)
			}
		case proto.Rogue_Rotation_RuptureEnvenom:
			rupture.MinimumComboPoints = core.MaxInt32(1, rogue.Rotation.MinimumComboPointsPrimaryFinisher)
			rogue.PriorityItems = append(rogue.PriorityItems, rupture)
			envenom.MinimumComboPoints = rogue.Rotation.MinimumComboPointsSecondaryFinisher
			if envenom.MinimumComboPoints > 0 && envenom.MinimumComboPoints <= 5 {
				rogue.PriorityItems = append(rogue.PriorityItems, envenom)
			}
		}
	} else {
		switch rogue.Rotation.CombatFinisherPriority {
		case proto.Rogue_Rotation_RuptureEviscerate:
			rupture.MinimumComboPoints = core.MaxInt32(1, rogue.Rotation.MinimumComboPointsPrimaryFinisher)
			rogue.PriorityItems = append(rogue.PriorityItems, rupture)
			eviscerate.MinimumComboPoints = rogue.Rotation.MinimumComboPointsSecondaryFinisher
			if eviscerate.MinimumComboPoints > 0 && eviscerate.MinimumComboPoints <= 5 {
				rogue.PriorityItems = append(rogue.PriorityItems, eviscerate)
			}
		case proto.Rogue_Rotation_EviscerateRupture:
			eviscerate.MinimumComboPoints = core.MaxInt32(1, rogue.Rotation.MinimumComboPointsPrimaryFinisher)
			rogue.PriorityItems = append(rogue.PriorityItems, eviscerate)
			rupture.MinimumComboPoints = rogue.Rotation.MinimumComboPointsSecondaryFinisher
			if rupture.MinimumComboPoints > 0 && rupture.MinimumComboPoints <= 5 {
				rogue.PriorityItems = append(rogue.PriorityItems, rupture)
			}
		}
	}
	rogue.RotationItems = rogue.PlanRotation(sim)
}

func (rogue *Rogue) simpleRotation(sim *core.Simulation) {
	if len(rogue.RotationItems) < 1 {
		panic("Rotation is empty")
	}
	eps := rogue.GetExpectedEnergyPerSecond()
	shouldCast := rogue.ShouldCastNextPrioItem(sim, eps)
	item := rogue.RotationItems[0]
	prio := rogue.PriorityItems[item.PrioIndex]
	switch shouldCast {
	case ShouldNotCast:
		rogue.RotationItems = rogue.RotationItems[1:]
		rogue.simpleRotation(sim)
	case ShouldBuild:
		spell := rogue.Builder
		if spell == nil || spell.Cast(sim, rogue.CurrentTarget) {
			if rogue.GCD.IsReady(sim) {
				rogue.simpleRotation(sim)
			}
		} else {
			panic("Unexpected builder cast failure")
		}
	case ShouldCast:
		spell := prio.GetSpell(rogue, rogue.ComboPoints())
		if spell == nil || spell.Cast(sim, rogue.CurrentTarget) {
			rogue.PriorityItems[item.PrioIndex].CastCount += 1
			rogue.RotationItems = rogue.PlanRotation(sim)
			if rogue.GCD.IsReady(sim) {
				rogue.simpleRotation(sim)
			}
		} else {
			panic("Unexpected cast failure")
		}
	case ShouldWait:
		desiredEnergy := 100.0
		if rogue.ComboPoints() == 5 {
			if rogue.CurrentEnergy() >= 100 {
				panic("Rotation is capped on energy and cp")
			}
			desiredEnergy = prio.EnergyCost
		} else {
			if rogue.CurrentEnergy() < prio.EnergyCost && rogue.ComboPoints() >= prio.MinimumComboPoints {
				desiredEnergy = prio.EnergyCost
			} else if rogue.ComboPoints() < 5 {
				desiredEnergy = rogue.Builder.DefaultCast.Cost
			}
		}
		cdAvailableTime := time.Second * 10
		if sim.CurrentTime > cdAvailableTime {
			cdAvailableTime = core.NeverExpires
		}
		nextExpiration := cdAvailableTime
		for _, otherItem := range rogue.RotationItems {
			if otherItem.ExpiresAt < nextExpiration {
				nextExpiration = otherItem.ExpiresAt
			}
		}
		neededEnergy := desiredEnergy - rogue.CurrentEnergy()
		energyAvailableTime := time.Second*time.Duration(neededEnergy/eps) + 1*time.Second
		energyAt := sim.CurrentTime + energyAvailableTime
		if energyAt < nextExpiration {
			rogue.WaitForEnergy(sim, desiredEnergy)
		} else if nextExpiration > sim.CurrentTime {
			rogue.WaitUntil(sim, nextExpiration)
		} else {
			rogue.DoNothing()
		}
	}
}
