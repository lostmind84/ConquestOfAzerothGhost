//go:build e2e

package resources_test

import (
	"testing"
	"time"

	"github.com/azerothcore/AzerothGhost/e2e/e2eharness"
)

const (
	xorothLevel   = 10 // level reported in #1399
	decayWindow   = 12 * time.Second
	decayDummyLvl = 10
)

// Main project issue #1399: a Knight of Xoroth's Rage drops faster than expected once out of combat.
//
// Rage decay is core behaviour, not class data: Player::Regenerate subtracts 20 internal units
// (2 Rage) per 2 s regeneration tick while the player is out of combat and has no interrupt-regen
// aura, scaled by Rate.Rage.Loss (1 in this server's worldserver.conf). The Knight of Xoroth runs on
// Rage as its display power (ChrClasses.dbc power type 1), so it follows that same rule.
//
// This test measures the real decay so the report can be answered with a number. It fails only if
// the server decays faster than the formula it implements; whether 1 Rage per second is the rate
// CoA wants for this class is a design question the test cannot answer.
//
//	go test -tags=e2e ./e2e/coa/resources -run RageDecay -count=1 -v
func TestKnightOfXoroth_RageDecayOutOfCombat(t *testing.T) {
	bot := newBot(t, "KxRage", e2eharness.RaceOrc, classKnightXoroth, xorothLevel)
	dummy := spawnTarget(t, bot, e2eharness.CreatureHeroicTrainingDummy, decayDummyLvl)
	bot.CombatReady(t) // no `.cheat power`: it pins Rage at maximum and hides the decay

	// Melee swings are what builds Rage (Unit::DealDamage pays it for weapon damage only).
	bot.Face(t, dummy)
	bot.Engage(t, dummy, 15*time.Second)
	bot.Attack(t, dummy)
	time.Sleep(12 * time.Second)
	bot.CombatStop(t)

	// Wait for the combat flag to clear: decay only runs out of combat.
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); {
		if !bot.UnitInCombat(bot.World.CharGUID()) {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if bot.UnitInCombat(bot.World.CharGUID()) {
		t.Fatalf("bot never left combat; no out-of-combat decay can be measured")
	}

	start := rage(bot)
	if start == 0 {
		t.Fatalf("no Rage built by melee swings; nothing to measure")
	}
	began := time.Now()
	var end uint32
	var elapsed time.Duration
	for deadline := time.Now().Add(decayWindow); time.Now().Before(deadline); {
		end = rage(bot)
		elapsed = time.Since(began)
		if end == 0 {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}

	lost := float64(start - end)
	rate := lost / elapsed.Seconds()
	// Player::Regenerate: -20 internal per 2 s tick * Rate.Rage.Loss(1) = 10 internal per second.
	const formulaRate = 10.0
	t.Logf("Rage %d -> %d over %.1fs = %.1f internal units per second (1 unit = 0.1 Rage); "+
		"Player::Regenerate with Rate.Rage.Loss=1 gives %.1f", start, end, elapsed.Seconds(), rate, formulaRate)
	if rate > formulaRate*1.25 {
		t.Errorf("E2E_FAIL: Rage decays at %.1f internal units per second, faster than the %.1f the core's "+
			"own formula gives (#1399)", rate, formulaRate)
		return
	}
	t.Logf("E2E_PASS: Rage decay matches Player::Regenerate (%.1f vs %.1f internal units per second); "+
		"whether that rate suits the class is a design question (#1399)", rate, formulaRate)
}
