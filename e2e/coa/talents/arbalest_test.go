//go:build e2e

package talents_test

import (
	"math"
	"testing"
	"time"

	"github.com/azerothcore/AzerothGhost/e2e/e2eharness"
)

const (
	// Crossbows: Spell.dbc 5011, verified via e2eharness.GetSpell against the live Spell.dbc (name "Crossbows"),
	// the same way spellProficiencyGuns (266, "Guns") is used for the gun-based Witch Hunter test above.
	spellProficiencyCrossbows uint32 = 5011

	// Crossbows the report says Witchbane needs equipped (from `.lookup item` on the live server).
	itemMakeshiftCrossbow uint32 = 419 // required level 3

	// Witchbane ranks (from `.lookup spell Witchbane` on the live server). 704342 is the internal family/channel
	// identifier the server code matches against, not a player-castable rank (Spell.dbc gives it no rank text).
	spellWitchbaneRank1 uint32 = 800165
	spellWitchbaneRank4 uint32 = 520213
	spellWitchbaneRank5 uint32 = 578297
	spellWitchbaneRank6 uint32 = 574319
	spellWitchbaneRank7 uint32 = 574320

	// Arbalest Mastery: Spell.dbc names 706240 "Passive" (tooltip: "Each subsequent shot of a Witchbane cast deals
	// $706241s1% more damage, up to a maximum of ...") and 706241 "Proc" — the stacking buff that actually carries
	// the per-shot multiplier read by the server's damage calculation.
	spellArbalestMastery         uint32 = 706240
	spellArbalestMasteryProgress uint32 = 706241
)

// witchbaneRanksHighToLow tries the highest rank first and falls back, so the test finds whichever rank a level 60
// Witch Hunter can actually cast instead of guessing a level gate.
var witchbaneRanksHighToLow = []uint32{
	spellWitchbaneRank7, spellWitchbaneRank6, spellWitchbaneRank5, spellWitchbaneRank4, spellWitchbaneRank1,
}

// Main project issue #3935: Arbalest Mastery is being displayed wrong. Reported from the in-game form (class id 15,
// level 60): "It shows the debuff on you, not the enemy, damage amplification is correct though." Arbalest Mastery
// is a passive: "Each subsequent shot of a Witchbane cast deals 15% more damage, up to a maximum of 150%."
//
//	go test -tags=e2e ./e2e/coa/talents -run Arbalest -count=1 -v
func TestWitchHunter_ArbalestMasteryAuraPlacement(t *testing.T) {
	bot := newBot(t, "ArbMast", e2eharness.RaceHuman, classWitchHunter, 60)

	// Witchbane needs a ranged weapon: #3935's own repro (a plain Warrior with .learn, no crossbow) got
	// SPELL_FAILED_BAD_TARGETS / "Must have the proper item equipped". Give the bot proficiency and a crossbow,
	// mirroring TestWitchHunter_BurrowBoltPulls's Learn(gun proficiency)+equip(gun) pattern for this class.
	bot.Learn(t, spellProficiencyCrossbows)
	equip(t, bot, itemMakeshiftCrossbow)

	if !bot.World.KnowsSpell(spellArbalestMastery) {
		t.Logf("level 60 Witch Hunter does not know Arbalest Mastery %d, learning it", spellArbalestMastery)
		bot.Learn(t, spellArbalestMastery)
	}
	for _, r := range witchbaneRanksHighToLow {
		bot.Learn(t, r)
	}

	thug := spawnTarget(t, bot, creatureDefiasThug, 60)
	// Neutral like TestWitchHunter_BurrowBoltPulls sets it: a live hostile Defias Thug walks into melee range once
	// hit, and Witchbane (a ranged crossbow ability) then refuses with SPELL_FAILED_TOO_CLOSE on the following casts
	// (confirmed by an earlier run of this test without this line).
	bot.GM(t, ".npc set faction 7")
	bot.CombatReadyFull(t)

	// Witchbane needs range; even neutral, the thug still closes in after being hit, so re-establish 15 yd from its
	// *current* position before every cast rather than once (confirmed necessary by an earlier run: cast 1 landed
	// at 15 yd, casts 2-3 then refused with SPELL_FAILED_TOO_CLOSE once the thug had closed the gap).
	backOffFromTarget := func() {
		t.Helper()
		obj := bot.World.GetObject(thug)
		if obj == nil {
			t.Fatalf("precondition: target %d not tracked", thug)
		}
		tx, ty, tz := obj.InterpolatedPosition()
		bx, by, _, mapID := bot.Pos()
		dx, dy := float64(bx-tx), float64(by-ty)
		dist := math.Hypot(dx, dy)
		if dist < 1 {
			dx, dy, dist = 1, 0, 1
		}
		nx, ny := dx/dist, dy/dist
		bot.Teleport(t, tx+float32(nx*15), ty+float32(ny*15), tz, mapID)
		time.Sleep(settle)
		bot.Face(t, thug)
		t.Logf("distance to target before cast: was %.1f yd, now ~15 yd", dist)
	}
	backOffFromTarget()

	// Find the highest Witchbane rank the bot can actually cast at level 60.
	var rank uint32
	var lastRefusal e2eharness.SpellCastResult
	for _, candidate := range witchbaneRanksHighToLow {
		res := castLanded(t, bot, candidate, thug, 2)
		if res.Success {
			rank = candidate
			break
		}
		lastRefusal = res
	}
	if rank == 0 {
		t.Fatalf("E2E_FAIL: no Witchbane rank could be cast by a level 60 Witch Hunter with a crossbow equipped (#3935); "+
			"last refusal: %s", e2eharness.SpellFailReasonName(lastRefusal.FailReason))
	}
	t.Logf("casting %s", bot.DescribeSpell(rank))

	// Two more casts (three total) so Arbalest Mastery's per-shot stacks have a chance to build and apply.
	for cast := 2; cast <= 3; cast++ {
		time.Sleep(3 * time.Second) // let the previous channel finish firing its shots
		backOffFromTarget()
		if res := castLanded(t, bot, rank, thug, 3); !res.Success {
			t.Fatalf("E2E_FAIL: Witchbane (%s) refused on cast %d/3: %s (#3935)",
				bot.DescribeSpell(rank), cast, e2eharness.SpellFailReasonName(res.FailReason))
		}
	}
	time.Sleep(3 * time.Second) // let the last channel finish before reading auras
	time.Sleep(settle)

	selfGUID := bot.World.CharGUID()
	selfAuras := serverAuras(t, bot, selfGUID)
	enemyAuras := serverAuras(t, bot, thug)
	t.Logf("server auras on the Witch Hunter (self, %d): %v", selfGUID, selfAuras)
	t.Logf("server auras on the target (%d): %v", thug, enemyAuras)
	t.Logf("client HasAura on self: mastery(%d)=%v progress(%d)=%v",
		spellArbalestMastery, bot.HasAura(spellArbalestMastery),
		spellArbalestMasteryProgress, bot.HasAura(spellArbalestMasteryProgress))
	t.Logf("client UnitHasAura on target: mastery(%d)=%v progress(%d)=%v",
		spellArbalestMastery, bot.UnitHasAura(thug, spellArbalestMastery),
		spellArbalestMasteryProgress, bot.UnitHasAura(thug, spellArbalestMasteryProgress))

	if len(selfAuras) == 0 && len(enemyAuras) == 0 {
		t.Errorf("E2E_FAIL: no aura at all on the Witch Hunter or the target after 3 Witchbane casts (#3935)")
		return
	}

	// Log every aura seen on either unit so the real id behind the report is visible, not just the two we expect.
	for id := range selfAuras {
		t.Logf("aura %d present on the Witch Hunter (server)", id)
	}
	for id := range enemyAuras {
		t.Logf("aura %d present on the target (server)", id)
	}

	_, masteryOnBotServer := selfAuras[spellArbalestMastery]
	_, masteryOnTargetServer := enemyAuras[spellArbalestMastery]
	masteryOnBot := masteryOnBotServer || bot.HasAura(spellArbalestMastery)
	masteryOnTarget := masteryOnTargetServer || bot.UnitHasAura(thug, spellArbalestMastery)

	_, progressOnBotServer := selfAuras[spellArbalestMasteryProgress]
	_, progressOnTargetServer := enemyAuras[spellArbalestMasteryProgress]
	progressOnBot := progressOnBotServer || bot.HasAura(spellArbalestMasteryProgress)
	progressOnTarget := progressOnTargetServer || bot.UnitHasAura(thug, spellArbalestMasteryProgress)

	if !masteryOnBot && !masteryOnTarget && !progressOnBot && !progressOnTarget {
		t.Errorf("E2E_FAIL: neither Arbalest Mastery (%d) nor its stacking proc (%d) appeared on the Witch Hunter "+
			"or the target after 3 Witchbane casts (#3935)", spellArbalestMastery, spellArbalestMasteryProgress)
		return
	}

	// The issue's claim: the mastery aura must be on the target, not on the caster.
	failed := false
	if masteryOnBot && !masteryOnTarget {
		t.Errorf("E2E_FAIL: Arbalest Mastery (%d) is on the Witch Hunter, not on the target (#3935)", spellArbalestMastery)
		failed = true
	} else if masteryOnTarget {
		t.Logf("E2E_PASS: Arbalest Mastery (%d) is on the target", spellArbalestMastery)
	}
	if progressOnBot && !progressOnTarget {
		t.Errorf("E2E_FAIL: Arbalest Mastery Progress (%d), the stacking aura that carries the per-shot damage "+
			"bonus, is on the Witch Hunter, not on the target (#3935)", spellArbalestMasteryProgress)
		failed = true
	} else if progressOnTarget {
		t.Logf("E2E_PASS: Arbalest Mastery Progress (%d) is on the target", spellArbalestMasteryProgress)
	}
	if !failed {
		t.Logf("E2E_PASS: no Arbalest Mastery aura landed on the Witch Hunter instead of the target")
	}
}
