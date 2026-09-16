//go:build e2e

package brokenspells_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/azerothcore/AzerothGhost/e2e/e2eharness"
)

const (
	spellFireEngravingPassive uint32 = 653211 // equip spell of enchant 1000 (Weapon Engraving: Fire 653022)
	auraFirebrand             uint32 = 653210
	spellFirebrandExplosion   uint32 = 653212 // cast once per stack when Firebrand expires
	engravingLevel                   = 14     // #161
)

// Main project issue #161: Weapon Engraving: Fire never procs.
// Enchant 1000 carries equip spell 653211, a proc-trigger aura whose ProcFlags are 0 in Spell.dbc.
// The harness cannot cast a spell on an item (no TARGET_FLAG_ITEM), so the test applies the enchant's
// equip aura 653211 directly and checks the proc.
//
//	go test -tags=e2e ./e2e/coa/brokenspells -run WeaponEngraving -count=1 -v
func TestRunemaster_WeaponEngravingFireProcs(t *testing.T) {
	bot := newBot(t, "RmEngr", e2eharness.RaceBloodElf, classRunemaster, engravingLevel)
	dummy := spawnTarget(t, bot, e2eharness.CreatureHeroicTrainingDummy, engravingLevel-2)
	bot.CombatReadyFull(t)

	// 653211 is hidden (SPELL_ATTR0_DO_NOT_DISPLAY), so the bot cannot see it: apply without waiting.
	_ = bot.World.SetTarget(bot.World.CharGUID())
	bot.GM(t, fmt.Sprintf(".aura %d", spellFireEngravingPassive))
	time.Sleep(settle)
	_ = bot.World.SetTarget(dummy)
	explosions := watchDamage(t, bot, spellFirebrandExplosion)
	bot.Face(t, dummy)
	bot.Attack(t, dummy)
	defer func() { _ = bot.World.AttackStop() }()

	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		if bot.UnitHasAura(dummy, auraFirebrand) {
			t.Logf("Firebrand %d applied by melee with Fire Engraving", auraFirebrand)
			_ = bot.World.AttackStop()
			time.Sleep(5 * time.Second) // Firebrand lasts 3 s
			if n := len(explosions.snapshot()); n == 0 {
				t.Errorf("E2E_FAIL: Firebrand expired without its explosion %d (#161)", spellFirebrandExplosion)
			} else {
				t.Logf("E2E_PASS: Firebrand applied and exploded (%d damage logs of %d)", n, spellFirebrandExplosion)
			}
			return
		}
		time.Sleep(250 * time.Millisecond)
	}
	hp, maxHP := bot.UnitHP(dummy)
	if hp == maxHP {
		t.Fatalf("precondition: the dummy took no melee damage in 60 s")
	}
	t.Errorf("E2E_FAIL: 60 s of melee with Fire Engraving 653211 never applied Firebrand %d (#161)", auraFirebrand)
}
