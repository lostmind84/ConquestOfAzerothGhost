//go:build e2e

package summonspets_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/azerothcore/AzerothGhost/e2e/e2eharness"
)

const spellBuildScrapmaw uint32 = 500242 // SUMMON_PET creature 50048

// Main project issue #1411: the Tinker pet stays alive at 0 health instead of dying.
//
//	go test -tags=e2e ./e2e/coa/summonspets -run TinkerPetDies -count=1 -v
func TestTinker_PetDiesAtZeroHealth(t *testing.T) {
	bot := newBot(t, "TkPet", e2eharness.RaceGnome, classTinker, 20)
	if !bot.World.KnowsSpell(spellBuildScrapmaw) {
		bot.Learn(t, spellBuildScrapmaw)
		waitKnows(t, bot, spellBuildScrapmaw)
	}
	bot.CombatReadyFull(t)
	if res := bot.Cast(t, spellBuildScrapmaw, 0, 15*time.Second); !res.Success {
		t.Fatalf("precondition: Build: Scrapmaw refused: %s", e2eharness.SpellFailReasonName(res.FailReason))
	}
	pet := bot.WaitPlayerPet(t, 10*time.Second)
	bot.WaitUnitGUID(t, pet, 5*time.Second)
	time.Sleep(2 * time.Second)
	hp, maxHP := bot.UnitHP(pet)
	t.Logf("pet health %d/%d", hp, maxHP)

	// Lethal damage in small hits, so the owner-based rescaling runs between hits as it would in a fight.
	_ = bot.World.SetTarget(pet)
	time.Sleep(200 * time.Millisecond)
	step := maxHP/12 + 1
	for deadline := time.Now().Add(30 * time.Second); time.Now().Before(deadline) && !unitDead(bot, pet); {
		bot.GM(t, fmt.Sprintf(".damage %d", step))
		time.Sleep(700 * time.Millisecond)
	}
	if !unitDead(bot, pet) {
		hp, maxHP = bot.UnitHP(pet)
		t.Errorf("E2E_FAIL: the Tinker pet is still alive after repeated damage, health %d/%d (#1411)", hp, maxHP)
		return
	}
	time.Sleep(5 * time.Second)
	if o := bot.World.GetObject(pet); o != nil {
		if hp, maxHP = bot.UnitHP(pet); hp > 0 {
			t.Errorf("E2E_FAIL: the dead Tinker pet came back with health %d/%d (#1411)", hp, maxHP)
			return
		}
	}
	t.Logf("E2E_PASS: the Tinker pet died from repeated damage and stayed dead (#1411)")
}
