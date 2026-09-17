//go:build e2e

package summonspets2_test

import (
	"testing"
	"time"

	"github.com/azerothcore/AzerothGhost/client"
	"github.com/azerothcore/AzerothGhost/e2e/e2eharness"
)

const (
	classGuardian uint8 = 18

	spellStandardOfRecovery  uint32 = 500260 // summons creature 50053
	spellReclaimStandards    uint32 = 574339 // CasterAuraSpell 808006
	spellActiveStandards     uint32 = 808006 // "You have active Standards and can now cast Reclaim Standards."
	creatureStandardRecovery uint32 = 50053
	guardianLevel                   = 10
)

func guardianWithStandard(t *testing.T, prefix string) (*e2eharness.ScenarioBot, uint64) {
	t.Helper()
	bot := newBot(t, prefix, e2eharness.RaceHuman, classGuardian, guardianLevel)
	for _, spell := range []uint32{spellStandardOfRecovery, spellReclaimStandards} {
		if !bot.World.KnowsSpell(spell) {
			bot.Learn(t, spell)
			waitKnows(t, bot, spell)
		}
	}
	bot.CombatReadyFull(t)
	x, y, z, _ := bot.Pos()
	if res := bot.CastAtPosition(t, spellStandardOfRecovery, x+3, y, z, castTimeout); !res.Success {
		t.Fatalf("precondition: Standard of Recovery refused: %s", e2eharness.SpellFailReasonName(res.FailReason))
	}
	var standard uint64
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline) && standard == 0; time.Sleep(100 * time.Millisecond) {
		if units := ownedUnits(bot, creatureStandardRecovery); len(units) > 0 {
			standard = units[0]
		}
	}
	if standard == 0 {
		t.Fatalf("precondition: no Standard of Recovery appeared")
	}
	time.Sleep(gcd)
	return bot, standard
}

// Main project issues #1470 and #1499: with a Standard placed, Reclaim Standards does not activate or fire.
//
//	go test -tags=e2e ./e2e/coa/summonspets2 -run ReclaimStandards -count=1 -v
func TestGuardian_ReclaimStandards(t *testing.T) {
	bot, standard := guardianWithStandard(t, "GdRecl")
	if bot.HasAura(spellActiveStandards) {
		t.Logf("the Active Standards marker is on the Guardian")
	} else {
		t.Errorf("E2E_FAIL: no Active Standards marker (%d) with a Standard placed, the client keeps Reclaim Standards disabled (#1470, #1499)", spellActiveStandards)
	}
	res, err := bot.TryCast(t, spellReclaimStandards, 0, castTimeout)
	if err != nil {
		t.Fatalf("Reclaim Standards: %v", err)
	}
	if !res.Success {
		t.Fatalf("E2E_FAIL: Reclaim Standards refused: %s (#1470, #1499)", e2eharness.SpellFailReasonName(res.FailReason))
	}
	gone := false
	for deadline := time.Now().Add(3 * time.Second); time.Now().Before(deadline) && !gone; time.Sleep(100 * time.Millisecond) {
		gone = unitDead(bot, standard)
	}
	if !gone {
		t.Fatalf("E2E_FAIL: the Standard is still there after Reclaim Standards (#1470, #1499)")
	}
	if !bot.TryWaitAuraGone(t, spellActiveStandards, 2*time.Second) {
		t.Errorf("E2E_FAIL: the Active Standards marker stayed after the Standard was reclaimed (#1470, #1499)")
		return
	}
	t.Logf("E2E_PASS: Reclaim Standards removed the Standard and its marker (#1470, #1499)")
}

// Main project issues #1502 and #1503: creatures attack a placed Standard; it should not be attackable, so they go
// for its Guardian instead.
//
//	go test -tags=e2e ./e2e/coa/summonspets2 -run StandardNotAttackable -count=1 -v
func TestGuardian_StandardNotAttackable(t *testing.T) {
	const unitFlagNonAttackable = 0x2
	bot, standard := guardianWithStandard(t, "GdAtk")
	o := bot.World.GetObject(standard)
	if o == nil {
		t.Fatalf("precondition: the Standard is not visible")
	}
	flags := o.Values[client.UnitFieldFlags]
	sx, sy, sz := o.PosX, o.PosY, o.PosZ
	x, y, z, m := bot.Pos()
	bot.Teleport(t, sx+2, sy, sz, m)
	boar := spawnTarget(t, bot, creatureMottledBoar, guardianLevel, factionHostile)
	bot.Teleport(t, x, y, z, m)
	// A game master is never attacked, so the Standard is the boar's only possible victim.
	attacked := false
	for deadline := time.Now().Add(6 * time.Second); time.Now().Before(deadline) && !attacked; time.Sleep(200 * time.Millisecond) {
		attacked = bot.UnitTarget(boar) == standard || unitDead(bot, standard)
	}
	t.Logf("Standard unit flags 0x%X, attacked by a hostile boar: %v", flags, attacked)
	if attacked || flags&unitFlagNonAttackable == 0 {
		t.Errorf("E2E_FAIL: the Standard can be attacked (flags 0x%X, attacked %v) (#1502, #1503)", flags, attacked)
		return
	}
	t.Logf("E2E_PASS: the Standard is not attackable (#1502, #1503)")
}
