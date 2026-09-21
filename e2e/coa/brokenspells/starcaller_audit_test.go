//go:build e2e

package brokenspells_test

import (
	"testing"
	"time"

	"github.com/azerothcore/AzerothGhost/e2e/e2eharness"
)

const (
	spellLunarEclipseAbility uint32 = 800386 // CasterAuraSpell 704519, "4 Stacks of Lunar Phase"
	spellBrightMoon          uint32 = 801226 // SPELLMOD_MAX_AURA_STACKS +3 on Lunar Phase (cap 4 -> 8)
	auraLunarPhaseStacks     uint32 = 802985
	auraLunarPhaseMarker     uint32 = 704519 // 4-stack marker applied by the module
	creatureHostileGolem     uint32 = 36     // harness default hostile creature (faction 14)
)

// starcallerLevel80 creates a level 80 Starcaller with GM mode off and god on, as the scenario harness does.
func starcallerLevel80(t *testing.T, prefix string) *e2eharness.ScenarioBot {
	t.Helper()
	bot := newBot(t, prefix, e2eharness.RaceNightElf, classStarcaller, 80)
	bot.CombatReady(t)
	return bot
}

// setLunarPhase leaves the bot with exactly n Lunar Phase stacks and the 4-stack marker.
func setLunarPhase(t *testing.T, bot *e2eharness.ScenarioBot, n int) {
	t.Helper()
	bot.CancelAura(t, auraLunarPhaseStacks)
	bot.WaitAuraGone(t, auraLunarPhaseStacks, 2*time.Second)
	for i := 0; i < 2*n && bot.AuraStacks(auraLunarPhaseStacks) < n; i++ {
		bot.ApplyAura(t, auraLunarPhaseStacks)
	}
	if got := bot.AuraStacks(auraLunarPhaseStacks); got < n {
		t.Fatalf("precondition: could not reach %d Lunar Phase stacks, have %d", n, got)
	}
	if !bot.HasAura(auraLunarPhaseMarker) {
		bot.ApplyAura(t, auraLunarPhaseMarker)
	}
}

// Main project issue #4273: with Bright Moon (Lunar Phase caps at 8) Lunar Eclipse needed all 8 stacks and
// spent them all. Expected: it is castable at exactly 4 stacks (no CASTER_AURASTATE refusal) and spends 4.
// Server-side proof: apps/coa-gameplay-test scenario starcaller-lunar-eclipse-threshold.
//
//	go test -tags=e2e -p 1 ./e2e/coa/brokenspells -run LunarEclipseFourStacks -count=1 -v
func TestStarcaller_LunarEclipseFourStacksWithBrightMoon(t *testing.T) {
	bot := starcallerLevel80(t, "ScLe4")
	bot.Learn(t, spellLunarEclipseAbility)
	bot.Learn(t, spellBrightMoon)

	// Exactly four stacks: must be castable and must spend them.
	setLunarPhase(t, bot, 4)
	before := bot.AuraStacks(auraLunarPhaseStacks)
	res, err := bot.TryCast(t, spellLunarEclipseAbility, 0, castTimeout)
	if err != nil {
		t.Fatalf("Lunar Eclipse at 4 stacks: %v", err)
	}
	if !res.Success {
		t.Fatalf("E2E_FAIL: Lunar Eclipse refused at %d Lunar Phase stacks with Bright Moon: %s (#4273)",
			before, e2eharness.SpellFailReasonName(res.FailReason))
	}
	bot.WaitAura(t, spellLunarEclipseAbility, 2*time.Second)
	time.Sleep(settle)
	// One passive gain may land between the cast and the read, hence <= 1.
	if after := bot.AuraStacks(auraLunarPhaseStacks); after > 1 {
		t.Errorf("E2E_FAIL: %d Lunar Phase stacks after a 4-stack Eclipse, want at most 1 (#4273)", after)
	} else {
		t.Logf("E2E_PASS: Eclipse cast at 4 stacks (had %d), %d left", before, after)
	}

	// Eight stacks: only four are spent.
	bot.CancelAura(t, spellLunarEclipseAbility)
	bot.WaitAuraGone(t, spellLunarEclipseAbility, 2*time.Second)
	bot.GM(t, ".cooldown") // the bot is its own selection after ApplyAura
	time.Sleep(gcd)
	setLunarPhase(t, bot, 8)
	res, err = bot.TryCast(t, spellLunarEclipseAbility, 0, castTimeout)
	if err != nil {
		t.Fatalf("Lunar Eclipse at 8 stacks: %v", err)
	}
	if !res.Success {
		t.Fatalf("Lunar Eclipse refused at 8 stacks: %s", e2eharness.SpellFailReasonName(res.FailReason))
	}
	bot.WaitAura(t, spellLunarEclipseAbility, 2*time.Second)
	time.Sleep(settle)
	if after := bot.AuraStacks(auraLunarPhaseStacks); after < 4 {
		t.Errorf("E2E_FAIL: %d Lunar Phase stacks left after an 8-stack Eclipse, want at least 4 (#4273)", after)
	} else {
		t.Logf("E2E_PASS: 8-stack Eclipse left %d stacks", after)
	}
}
