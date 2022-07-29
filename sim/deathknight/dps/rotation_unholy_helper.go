package dps

import (
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/deathknight"
)

type UnholyRotation struct {
	lastCastSpell *core.Spell

	syncDisease bool
	syncFF      bool
	syncGF      bool

	syncTimeFF time.Duration

	recastedFF bool
	recastedBP bool
}

func (ur *UnholyRotation) ResetUnholyRotation(sim *core.Simulation) {
	ur.syncDisease = false
	ur.syncFF = false

	ur.syncTimeFF = 0

	ur.recastedFF = false
	ur.recastedBP = false
}

func (dk *DpsDeathknight) shouldWaitForDnD(sim *core.Simulation, blood bool, frost bool, unholy bool) bool {
	return dk.Rotation.UseDeathAndDecay && !(dk.Talents.Morbidity == 0 || !(dk.DeathAndDecay.CD.IsReady(sim) || dk.DeathAndDecay.CD.TimeToReady(sim) < 4*time.Second) || ((!blood || dk.CurrentBloodRunes() > 1) && (!frost || dk.CurrentFrostRunes() > 1) && (!unholy || dk.CurrentUnholyRunes() > 1)))
}

func (dk *DpsDeathknight) UnholyDiseaseCheckWrapper(sim *core.Simulation, target *core.Unit, spell *core.Spell, costRunes bool) bool {
	success := false

	ffFirst := dk.Inputs.FirstDisease == proto.Deathknight_Rotation_FrostFever

	ffRemaining := dk.FrostFeverDisease[target.Index].RemainingDuration(sim)
	bpRemaining := dk.BloodPlagueDisease[target.Index].RemainingDuration(sim)
	castGcd := core.MinDuration(core.GCDMin, dk.ApplyCastSpeed(spell.CurCast.GCD))
	gracePeriodFrost := dk.CurrentFrostRuneGrace(sim)
	gracePeriodUnholy := dk.CurrentUnholyRuneGrace(sim)

	if ffFirst {
		if !dk.FrostFeverDisease[target.Index].IsActive() || ffRemaining < castGcd {
			// Refresh FF
			success = dk.CastIcyTouch(sim, target)
			dk.recastedFF = success && dk.LastCastOutcome.Matches(core.OutcomeLanded)
			dk.lastCastSpell = dk.IcyTouch
			dk.syncDisease = dk.recastedFF
		} else if dk.syncDisease {
			// Refresh BP
			dk.LastCastOutcome = core.OutcomeMiss
			success = dk.castClipDisease(false, gracePeriodUnholy, sim, dk.CanPlagueStrike(sim), dk.PlagueStrike, dk.BloodPlagueDisease[target.Index], target)
			dk.lastCastSpell = dk.PlagueStrike
			dk.recastedBP = success && dk.LastCastOutcome.Matches(core.OutcomeLanded)
			dk.syncDisease = !dk.recastedBP
		}
	} else {
		if !dk.BloodPlagueDisease[target.Index].IsActive() || bpRemaining < castGcd {
			// Refresh BP
			success = dk.CastPlagueStrike(sim, target)
			dk.recastedBP = success && dk.LastCastOutcome.Matches(core.OutcomeLanded)
			dk.lastCastSpell = dk.PlagueStrike
			dk.syncDisease = dk.recastedBP
		} else if dk.syncDisease {
			// Refresh FF
			dk.LastCastOutcome = core.OutcomeMiss
			success = dk.castClipDisease(false, gracePeriodFrost, sim, dk.CanIcyTouch(sim), dk.IcyTouch, dk.FrostFeverDisease[target.Index], target)
			dk.lastCastSpell = dk.IcyTouch
			dk.recastedFF = success && dk.LastCastOutcome.Matches(core.OutcomeLanded)
			dk.syncDisease = !dk.recastedFF
		}
	}

	if !success && dk.CanCast(sim, spell) {
		ffExpiresAt := ffRemaining + sim.CurrentTime
		bpExpiresAt := bpRemaining + sim.CurrentTime

		crpb := dk.CopyRunicPowerBar()
		runeCostForSpell := dk.RuneAmountForSpell(spell)
		spellCost := crpb.DetermineOptimalCost(sim, runeCostForSpell.Blood, runeCostForSpell.Frost, runeCostForSpell.Unholy)

		crpb.Spend(sim, spell, spellCost)

		afterCastTime := sim.CurrentTime + castGcd
		currentFrostRunes := crpb.CurrentFrostRunes()
		currentUnholyRunes := crpb.CurrentUnholyRunes()
		nextFrostRuneAt := crpb.FrostRuneReadyAt(sim)
		nextUnholyRuneAt := crpb.UnholyRuneReadyAt(sim)

		if ffFirst {
			// Check FF
			if dk.checkForDiseaseRecast(ffExpiresAt-dk.syncTimeFF, afterCastTime, spellCost.Frost, currentFrostRunes, nextFrostRuneAt) {
				success = dk.castClipDisease(true, gracePeriodFrost, sim, dk.CanIcyTouch(sim), dk.IcyTouch, dk.FrostFeverDisease[target.Index], target)
				dk.recastedFF = success && dk.LastCastOutcome.Matches(core.OutcomeLanded)
				return success
			}

			// Check BP
			if dk.checkForDiseaseRecast(bpExpiresAt, afterCastTime, spellCost.Unholy, currentUnholyRunes, nextUnholyRuneAt) {
				success = dk.castClipDisease(false, gracePeriodUnholy, sim, dk.CanPlagueStrike(sim), dk.PlagueStrike, dk.BloodPlagueDisease[target.Index], target)
				dk.recastedBP = success && dk.LastCastOutcome.Matches(core.OutcomeLanded)
				return success
			}
		} else {
			// Check BP
			if dk.checkForDiseaseRecast(bpExpiresAt, afterCastTime, spellCost.Unholy, currentUnholyRunes, nextUnholyRuneAt) {
				success = dk.castClipDisease(true, gracePeriodUnholy, sim, dk.CanPlagueStrike(sim), dk.PlagueStrike, dk.BloodPlagueDisease[target.Index], target)
				dk.recastedBP = success && dk.LastCastOutcome.Matches(core.OutcomeLanded)
				return success
			}

			// Check FF
			if dk.checkForDiseaseRecast(ffExpiresAt-dk.syncTimeFF, afterCastTime, spellCost.Frost, currentFrostRunes, nextFrostRuneAt) {
				success = dk.castClipDisease(false, gracePeriodFrost, sim, dk.CanIcyTouch(sim), dk.IcyTouch, dk.FrostFeverDisease[target.Index], target)
				dk.recastedFF = success && dk.LastCastOutcome.Matches(core.OutcomeLanded)
				return success
			}
		}

		if spell != dk.GhoulFrenzy {
			spell.Cast(sim, target)
		}
		dk.lastCastSpell = spell
		success = true
	}

	return success
}

func (dk *DpsDeathknight) checkForDiseaseRecast(expiresAt time.Duration, afterCastTime time.Duration,
	spellCost int, currentRunes int32, nextRuneAt time.Duration) bool {
	if spellCost > 0 && currentRunes == 0 {
		if expiresAt < nextRuneAt {
			return true
		}
	} else if afterCastTime > expiresAt {
		return true
	}
	return false
}

func (dk *DpsDeathknight) castClipDisease(mainDisease bool, gracePeriod time.Duration, sim *core.Simulation, canCast bool, spell *core.Spell, dot *core.Dot, target *core.Unit) bool {
	if canCast {
		// Dont drop disease due to %dmg modifiers
		if dot.TickCount < dot.NumberOfTicks-1 {
			nextTickAt := dot.ExpiresAt() - dot.TickLength*time.Duration((dot.NumberOfTicks-1)-dot.TickCount)
			if nextTickAt > sim.CurrentTime && (nextTickAt < sim.CurrentTime+gracePeriod || nextTickAt < sim.CurrentTime+400*time.Millisecond) {
				// Delay disease for next tick
				dk.LastCastOutcome = core.OutcomeMiss
				dk.WaitUntil(sim, nextTickAt+50*time.Millisecond)
				return true
			}
		}

		spell.Cast(sim, target)
		dk.lastCastSpell = spell
		success := dk.LastCastOutcome.Matches(core.OutcomeLanded)
		if success && spell == dk.IcyTouch {
			dk.syncTimeFF = 0
		}
		if mainDisease {
			dk.syncDisease = success
		}
		return true
	}
	return false
}

func (dk *DpsDeathknight) shouldSpreadDisease(sim *core.Simulation) bool {
	return dk.recastedFF && dk.recastedBP && dk.Env.GetNumTargets() > 1
}

func (dk *DpsDeathknight) spreadDiseases(sim *core.Simulation, target *core.Unit, s *deathknight.Sequence) bool {
	casted := dk.UnholyDiseaseCheckWrapper(sim, target, dk.Pestilence, true)
	landed := dk.LastCastOutcome.Matches(core.OutcomeLanded)

	// Reset flags on succesfull cast
	dk.recastedFF = !(casted && landed)
	dk.recastedBP = !(casted && landed)
	return casted
}