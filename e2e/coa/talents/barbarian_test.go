//go:build e2e

package talents_test

import (
	"testing"
	"time"

	"github.com/azerothcore/AzerothGhost/e2e/e2eharness"
)

const (
	classBarbarian uint8 = 12

	// Spirit of Savagery: effect 0 is an enemy area aura (13 yd) with aura 344 (flat attack power, -33 - 4 per
	// level), effect 1 a dummy aura on the Barbarian.
	spellSpiritOfSavagery uint32 = 707775
)

// Main project issue #1476: Spirit of Savagery plays its animation but gives no buff.
//
//	go test -tags=e2e ./e2e/coa/talents -run SpiritOfSavagery -count=1 -v
func TestBarbarian_SpiritOfSavageryApplies(t *testing.T) {
	bot := newBot(t, "BaSpir", e2eharness.RaceOrc, classBarbarian, 9)
	if !bot.World.KnowsSpell(spellSpiritOfSavagery) {
		t.Logf("level 9 Barbarian does not know %d, learning it", spellSpiritOfSavagery)
		bot.Learn(t, spellSpiritOfSavagery)
	}
	thug := spawnTarget(t, bot, creatureDefiasThug, 9)
	bot.CombatReadyFull(t)
	if res := castLanded(t, bot, spellSpiritOfSavagery, 0, 3); !res.Success {
		t.Fatalf("Spirit of Savagery refused: %s", e2eharness.SpellFailReasonName(res.FailReason))
	}
	time.Sleep(2 * time.Second) // area auras refresh every second
	self := serverAuras(t, bot, bot.World.CharGUID())
	enemy := serverAuras(t, bot, thug)
	t.Logf("client aura on the Barbarian: %v; server: Barbarian %v, enemy %v", bot.HasAura(spellSpiritOfSavagery),
		self[spellSpiritOfSavagery], enemy[spellSpiritOfSavagery])
	if _, ok := self[spellSpiritOfSavagery]; !ok || !bot.HasAura(spellSpiritOfSavagery) {
		t.Errorf("E2E_FAIL: no Spirit of Savagery aura on the Barbarian after the cast (#1476)")
	}
	amounts, ok := enemy[spellSpiritOfSavagery]
	if !ok {
		t.Errorf("E2E_FAIL: no Spirit of Savagery aura on the enemy next to the Barbarian (#1476)")
		return
	}
	if amounts[0] >= 0 {
		t.Errorf("E2E_FAIL: Spirit of Savagery on the enemy does not reduce attack power (amount %d) (#1476)", amounts[0])
		return
	}
	t.Logf("E2E_PASS: Spirit of Savagery on the Barbarian, enemy attack power %d", amounts[0])
}
