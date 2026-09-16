//go:build e2e

package brokenspells_test

import (
	"testing"
	"time"

	"github.com/azerothcore/AzerothGhost/e2e/e2eharness"
)

const (
	spellCursedForm           uint32 = 562572 // Accursed Form, rank "Cursed Form"
	auraCursedFormRequired    uint32 = 525031 // CasterAuraSpell of Ravenous Strike
	spellRavenousStrike       uint32 = 500123
	spellTwinSliceR2          uint32 = 501257 // EquippedItemSubClassMask 173555 includes staves
	spellLineFormation        uint32 = 803130 // CasterAuraSpell of Advance
	spellAdvance              uint32 = 500170
	spellAdvanceStomp         uint32 = 500673 // temporary replacement of Advance once it has run 0.5 s
	bloodmageLevel                   = 6      // #411
	felswornLevel                    = 11     // #400
	guardianLevel                    = 21     // #408
	advanceRecastDelay               = 1500 * time.Millisecond
	advanceDurationUpperBound        = 8 * time.Second
)

// Main project issue #411: Ravenous Strike cannot be cast in Cursed Form (level 6 Bloodmage).
// Reported before b14920c04 (#314) granted the Cursed Form marker.
//
//	go test -tags=e2e ./e2e/coa/brokenspells -run RavenousStrike -count=1 -v
func TestBloodmage_RavenousStrikeInCursedForm(t *testing.T) {
	bot := newBot(t, "BmRav", e2eharness.RaceBloodElf, classBloodmage, bloodmageLevel)
	bot.Learn(t, spellCursedForm)
	bot.Learn(t, spellRavenousStrike)
	dummy := spawnTarget(t, bot, e2eharness.CreatureHeroicTrainingDummy, bloodmageLevel-2)
	bot.CombatReadyFull(t)

	castLanded(t, bot, spellCursedForm, 0, 2)
	bot.WaitAura(t, spellCursedForm, 3*time.Second)
	t.Logf("marker %d present: %v", auraCursedFormRequired, bot.HasAura(auraCursedFormRequired))
	time.Sleep(gcd)
	bot.Face(t, dummy)

	if res := castLanded(t, bot, spellRavenousStrike, dummy, 3); !res.Success {
		t.Errorf("E2E_FAIL: Ravenous Strike refused in Cursed Form: %s (#411)", e2eharness.SpellFailReasonName(res.FailReason))
		return
	}
	t.Logf("E2E_PASS: Ravenous Strike cast in Cursed Form")
}

// Main project issue #400: Twin Slice says it requires a melee weapon while a staff is equipped.
// New Felsworn start with two Faded Glaives 629940 (one-hand swords): that run is the control.
//
//	go test -tags=e2e ./e2e/coa/brokenspells -run TwinSlice -count=1 -v
func TestFelsworn_TwinSliceWithStaff(t *testing.T) {
	for _, weapon := range []struct {
		name  string
		entry uint32 // 0 keeps the starting weapons
	}{{"swords", 0}, {"staff", itemWalkingStick}} {
		t.Run(weapon.name, func(t *testing.T) {
			bot := newBot(t, "FsTw"+weapon.name[:2], e2eharness.RaceBloodElf, classFelsworn, felswornLevel)
			if weapon.entry != 0 {
				equip(t, bot, weapon.entry)
			}
			bot.Learn(t, spellTwinSliceR2)
			dummy := spawnTarget(t, bot, e2eharness.CreatureHeroicTrainingDummy, felswornLevel-2)
			bot.CombatReadyFull(t)
			bot.Face(t, dummy)

			if res := castLanded(t, bot, spellTwinSliceR2, dummy, 3); !res.Success {
				t.Errorf("E2E_FAIL: Twin Slice refused with %s: %s (#400)", weapon.name, e2eharness.SpellFailReasonName(res.FailReason))
				return
			}
			t.Logf("E2E_PASS: Twin Slice cast with %s", weapon.name)
		})
	}
}

// Main project issue #408: every use of Advance learns Rank 1 and unlearns it when the effect ends.
//
//	go test -tags=e2e ./e2e/coa/brokenspells -run AdvanceStaysLearned -count=1 -v
func TestGuardian_AdvanceStaysLearned(t *testing.T) {
	bot := guardianSetup(t, "GdAdv")
	book := watchSpellbook(t, bot, spellAdvance)
	stomp := watchSpellbook(t, bot, spellAdvanceStomp)

	castLanded(t, bot, spellAdvance, 0, 2)
	time.Sleep(advanceDurationUpperBound)

	learned, removed := book.counts()
	stompLearned, stompRemoved := stomp.counts()
	t.Logf("Advance %d: learned %d removed %d; Advance (Damage) %d: learned %d removed %d",
		spellAdvance, learned, removed, spellAdvanceStomp, stompLearned, stompRemoved)
	if removed > 0 || learned > 0 || stompLearned > 0 || stompRemoved > 0 {
		t.Errorf("E2E_FAIL: one Advance sent learn/unlearn packets: %d/%d for %d, %d/%d for %d (#408)",
			learned, removed, spellAdvance, stompLearned, stompRemoved, spellAdvanceStomp)
	} else {
		t.Logf("E2E_PASS: no learn/unlearn packet during Advance")
	}
}

// Main project issue #339: recasting Advance after the short delay should stop the run (and stomp).
//
//	go test -tags=e2e ./e2e/coa/brokenspells -run AdvanceRecast -count=1 -v
func TestGuardian_AdvanceRecastStopsTheRun(t *testing.T) {
	bot := guardianSetup(t, "GdRec")
	castLanded(t, bot, spellAdvance, 0, 2)
	bot.WaitAura(t, spellAdvance, 2*time.Second)
	time.Sleep(advanceRecastDelay)

	res, err := bot.TryCast(t, spellAdvanceStomp, 0, castTimeout)
	if err != nil {
		t.Fatalf("recast Advance: %v", err)
	}
	t.Logf("recast result: success=%v reason=%s", res.Success, e2eharness.SpellFailReasonName(res.FailReason))
	if bot.TryWaitAuraGone(t, spellAdvance, time.Second) {
		t.Logf("E2E_PASS: Advance ended on recast")
		return
	}
	t.Errorf("E2E_FAIL: Advance still active %s after recasting it (#339)", bot.AuraDuration(spellAdvance))
}

func guardianSetup(t *testing.T, prefix string) *e2eharness.ScenarioBot {
	t.Helper()
	bot := newBot(t, prefix, e2eharness.RaceHuman, classGuardian, guardianLevel)
	bot.Learn(t, spellLineFormation)
	bot.Learn(t, spellAdvance)
	bot.CombatReadyFull(t)
	castLanded(t, bot, spellLineFormation, 0, 2)
	bot.WaitAura(t, spellLineFormation, 3*time.Second)
	time.Sleep(gcd)
	return bot
}
