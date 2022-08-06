package deathknight

import (
	//"github.com/wowsims/wotlk/sim/core/proto"

	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

func (dk *Deathknight) ApplyFrostTalents() {
	// Improved Icy Touch
	// Implemented outside

	// Toughness
	if dk.Talents.Toughness > 0 {
		armorCoeff := 0.02 * float64(dk.Talents.Toughness)
		dk.AddStatDependency(stats.Armor, stats.Armor, 1.0+armorCoeff)
	}

	// Icy Reach
	// Pointless to Implement

	// Black Ice
	dk.PseudoStats.FrostDamageDealtMultiplier *= 1.0 + 0.02*float64(dk.Talents.BlackIce)
	dk.PseudoStats.ShadowDamageDealtMultiplier *= 1.0 + 0.02*float64(dk.Talents.BlackIce)

	// Nerves Of Cold Steel
	if dk.Equip[proto.ItemSlot_ItemSlotMainHand].HandType == proto.HandType_HandTypeMainHand || dk.Equip[proto.ItemSlot_ItemSlotMainHand].HandType == proto.HandType_HandTypeOneHand {
		dk.AddStat(stats.MeleeHit, core.MeleeHitRatingPerHitChance*float64(dk.Talents.NervesOfColdSteel))
		dk.AutoAttacks.OHEffect.BaseDamage.Calculator = core.BaseDamageFuncMeleeWeapon(core.OffHand, false, 0, dk.nervesOfColdSteelBonus(), true)
	}

	// Icy Talons
	dk.applyIcyTalons()

	// Lichborne
	// Pointless to Implement

	// Annihilation

	// Killing Machine
	dk.applyKillingMachine()

	// Chill of the Grave
	// Implemented outside

	// Endless Winter
	if dk.Talents.EndlessWinter > 0 {
		strengthCoeff := 0.02 * float64(dk.Talents.EndlessWinter)
		dk.AddStatDependency(stats.Strength, stats.Strength, 1.0+strengthCoeff)
	}

	// Frigid Dreadplate
	if dk.Talents.FrigidDreadplate > 0 {
		dk.PseudoStats.BonusMeleeHitRatingTaken -= core.MeleeHitRatingPerHitChance * float64(dk.Talents.FrigidDreadplate)
	}

	// Glacier rot
	dk.applyGlacierRot()

	// Deathchill
	// TODO: Implement

	// Improved Icy Talons
	if dk.Talents.ImprovedIcyTalons {
		dk.PseudoStats.MeleeSpeedMultiplier *= 1.05
	}

	// Merciless Combat
	dk.applyMercilessCombat()

	dk.applyThreatOfThassarian()

	// Rime
	dk.applyRime()

	// Tundra Stalker
	dk.AddStat(stats.Expertise, 1.0*float64(dk.Talents.TundraStalker)*core.ExpertisePerQuarterPercentReduction)
	if dk.Talents.TundraStalker > 0 {
		dk.applyTundaStalker()
	}
}

func (dk *Deathknight) nervesOfColdSteelBonus() float64 {
	return []float64{1.0, 1.08, 1.16, 1.25}[dk.Talents.NervesOfColdSteel]
}

func (dk *Deathknight) applyGlacierRot() {
	dk.bonusCoeffs.glacierRotBonusCoeff = []float64{1.0, 1.07, 1.13, 1.20}[dk.Talents.GlacierRot]
}

func (dk *Deathknight) glacielRotBonus(target *core.Unit) float64 {
	return core.TernaryFloat64(dk.DiseasesAreActive(target), dk.bonusCoeffs.glacierRotBonusCoeff, 1.0)
}

func (dk *Deathknight) applyMercilessCombat() {
	dk.bonusCoeffs.mercilessCombatBonusCoeff = 1.0 + 0.06*float64(dk.Talents.MercilessCombat)
}

func (dk *Deathknight) mercilessCombatBonus(sim *core.Simulation) float64 {
	return core.TernaryFloat64(sim.IsExecutePhase35() && dk.Talents.MercilessCombat > 0, dk.bonusCoeffs.mercilessCombatBonusCoeff, 1.0)
}

func (dk *Deathknight) applyTundaStalker() {
	bonus := 1.0 + 0.03*float64(dk.Talents.TundraStalker)
	dk.RoRTSBonus = func(target *core.Unit) float64 {
		return core.TernaryFloat64(dk.FrostFeverDisease[target.Index].IsActive(), bonus, 1.0)
	}
}

func (dk *Deathknight) applyRime() {
	if dk.Talents.Rime == 0 {
		return
	}

	actionID := core.ActionID{SpellID: 59057}

	dk.RimeAura = dk.RegisterAura(core.Aura{
		ActionID: actionID,
		Label:    "Rime",
		Duration: time.Second * 15,
		OnGain: func(aura *core.Aura, sim *core.Simulation) {
			dk.HowlingBlast.CD.Reset()
		},
	})
}

func (dk *Deathknight) rimeCritBonus() float64 {
	return 5.0 * float64(dk.Talents.Rime)
}

func (dk *Deathknight) rimeHbChanceProc() float64 {
	return 0.05 * float64(dk.Talents.Rime)
}

func (dk *Deathknight) annihilationCritBonus() float64 {
	return 1.0 * float64(dk.Talents.Annihilation)
}

func (dk *Deathknight) applyKillingMachine() {
	if dk.Talents.KillingMachine == 0 {
		return
	}

	actionID := core.ActionID{SpellID: 51130}
	//weaponMH := dk.GetMHWeapon()
	//procChance := (weaponMH.SwingSpeed * 5.0 / 60.0) * float64(dk.Talents.KillingMachine)

	ppmm := dk.AutoAttacks.NewPPMManager(float64(dk.Talents.KillingMachine), core.ProcMaskMeleeMHAuto|core.ProcMaskMeleeMHSpecial)

	dk.KillingMachineAura = dk.RegisterAura(core.Aura{
		Label:    "Killing Machine Proc",
		ActionID: actionID,
		Duration: time.Second * 30.0,
	})

	core.MakePermanent(dk.GetOrRegisterAura(core.Aura{
		Label: "Killing Machine",
		OnSpellHitDealt: func(aura *core.Aura, sim *core.Simulation, spell *core.Spell, spellEffect *core.SpellEffect) {
			if !spellEffect.Landed() {
				return
			}

			if !ppmm.Proc(sim, spellEffect.ProcMask, "killing machine") {
				return
			}

			if !dk.KillingMachineAura.IsActive() {
				dk.KillingMachineAura.Activate(sim)
			} else {
				dk.KillingMachineAura.Refresh(sim)
			}
		},
	}))
}

func (dk *Deathknight) killingMachineOutcomeMod(outcomeApplier core.OutcomeApplier) core.OutcomeApplier {
	return func(sim *core.Simulation, spell *core.Spell, spellEffect *core.SpellEffect, attackTable *core.AttackTable) {
		if dk.KillingMachineAura.IsActive() {
			spell.BonusCritRating += 100 * core.CritRatingPerCritChance
			outcomeApplier(sim, spell, spellEffect, attackTable)
			spell.BonusCritRating -= 100 * core.CritRatingPerCritChance
		} else {
			outcomeApplier(sim, spell, spellEffect, attackTable)
		}
	}
}

func (dk *Deathknight) applyIcyTalons() {
	if dk.Talents.IcyTalons == 0 {
		return
	}

	icyTalonsCoeff := 1.0 + 0.04*float64(dk.Talents.IcyTalons)

	dk.IcyTalonsAura = dk.RegisterAura(core.Aura{
		ActionID: core.ActionID{SpellID: 50887}, // This probably doesnt need to be in metrics.
		Label:    "Icy Talons",
		Duration: time.Second * 20.0,
		OnGain: func(aura *core.Aura, sim *core.Simulation) {
			aura.Unit.PseudoStats.MeleeSpeedMultiplier *= icyTalonsCoeff
		},
		OnExpire: func(aura *core.Aura, sim *core.Simulation) {
			aura.Unit.PseudoStats.MeleeSpeedMultiplier /= icyTalonsCoeff
		},
	})
}

func (dk *Deathknight) bloodOfTheNorthCoeff() float64 {
	return []float64{1.0, 1.03, 1.06, 1.10}[dk.Talents.BloodOfTheNorth]
}

func (dk *Deathknight) botnAndReaping(sim *core.Simulation, spell *core.Spell) {
	if dk.Talents.BloodOfTheNorth == 0 && dk.Talents.Reaping == 0 {
		return
	}
	if core.RuneCost(spell.CurCast.Cost).Blood() == 0 {
		return // cant get death rune if we didnt spend blood
	}
	tp := dk.Talents.BloodOfTheNorth + dk.Talents.Reaping
	if tp < 3 {
		if sim.RandomFloat("Blood of The North / Reaping") > float64(tp)*0.33 {
			return
		}
	}

	// if slot == -1 that means we spent a death rune to trigger this.
	// Jooper: BoTN should still trigger the same effect if it spent a death rune in the a blood slot.
	// TODO: Clean this up since its kind of core-like code. Probably with PA refactor.
	r := dk.LastSpentBloodRune()
	dk.SetRuneToState(r, core.RuneState_DeathSpent, core.RuneKind_Death)
	r.BotnOrReaping = true
}

func (dk *Deathknight) applyThreatOfThassarian() {
	dk.bonusCoeffs.threatOfThassarianChance = []float64{0.0, 0.3, 0.6, 1.0}[dk.Talents.ThreatOfThassarian]
}

func (dk *Deathknight) threatOfThassarianWillProc(sim *core.Simulation) bool {
	return sim.RandomFloat("Threat of Thassarian") <= dk.bonusCoeffs.threatOfThassarianChance
}

func (dk *Deathknight) threatOfThassarianAdjustMetrics(sim *core.Simulation, spell *core.Spell, spellEffect *core.SpellEffect, mhOutcome core.HitOutcome) {
	spell.SpellMetrics[spellEffect.Target.TableIndex].Casts -= 1
	if mhOutcome == core.OutcomeHit {
		spell.SpellMetrics[spellEffect.Target.TableIndex].Hits -= 1
	} else if mhOutcome == core.OutcomeCrit {
		spell.SpellMetrics[spellEffect.Target.TableIndex].Hits -= 1
	} else {
		spell.SpellMetrics[spellEffect.Target.TableIndex].Hits -= 2
	}
}

func (dk *Deathknight) threatOfThassarianProcMasks(isMH bool, effect *core.SpellEffect, isGuileOfGorefiendStrike bool, isMightOfMograineStrike bool, wrapper func(outcomeApplier core.OutcomeApplier) core.OutcomeApplier) {
	critMultiplier := dk.critMultiplier()
	if isGuileOfGorefiendStrike || isMightOfMograineStrike {
		critMultiplier = dk.critMultiplierGoGandMoM()
	}

	if isMH {
		effect.ProcMask = core.ProcMaskMeleeMHSpecial
		effect.OutcomeApplier = wrapper(dk.OutcomeFuncMeleeSpecialHitAndCrit(critMultiplier))
	} else {
		effect.ProcMask = core.ProcMaskMeleeOHSpecial
		effect.OutcomeApplier = wrapper(dk.OutcomeFuncMeleeSpecialCritOnly(critMultiplier))
	}
}

func (dk *Deathknight) threatOfThassarianProc(sim *core.Simulation, spellEffect *core.SpellEffect, mhSpell *RuneSpell, ohSpell *RuneSpell) {
	mhSpell.Cast(sim, spellEffect.Target)
	if dk.Talents.ThreatOfThassarian > 0 && dk.GetOHWeapon() != nil && dk.threatOfThassarianWillProc(sim) {
		ohSpell.Cast(sim, spellEffect.Target)
	}
}
