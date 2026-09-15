//go:build e2e

package felfury_test

import (
	"fmt"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"github.com/azerothcore/AzerothGhost/e2e/e2eharness"
)

const (
	ClassFelsworn uint8 = 14

	AbilityInnerDemon uint32 = 804216 // Inner Demon, class spell learned at level 2
	AuraFelfury       uint32 = 800058 // Felfury resource aura, one stack per point
	felfuryStacks            = 3
)

// Inner Demon must consume all Felfury, last five seconds per consumed point,
// and not be castable again without Felfury.
// Main project issues #245 and #289 report that it consumes nothing and can be spammed.
//
//	go test -tags=e2e ./e2e/classes/felsworn/felfury -run TestFelsworn_InnerDemonConsumesFelfury -count=1 -v
func TestFelsworn_InnerDemonConsumesFelfury(t *testing.T) {
	bot := e2eharness.NewSolo(t, e2eharness.ScenarioOpts{
		Prefix: "Fels",
		Race:   e2eharness.RaceOrc,
		Class:  ClassFelsworn,
		Level:  10,
	})
	bot.TeleportPad(t, e2eharness.PackagePad(t))
	bot.Learn(t, AbilityInnerDemon)

	assertCastRefused(t, bot, "without Felfury")

	// One `.aura` per point; each application adds a stack to the existing aura.
	for stacks := 1; stacks <= felfuryStacks; stacks++ {
		bot.GM(t, fmt.Sprintf(".aura %d", AuraFelfury))
		waitStacks(t, bot, AuraFelfury, stacks)
	}

	bot.CastMust(t, AbilityInnerDemon, 0, 5*time.Second)
	bot.WaitAura(t, AbilityInnerDemon, 3*time.Second)
	if !bot.TryWaitAuraGone(t, AuraFelfury, 2*time.Second) {
		t.Errorf("E2E_FAIL: Felfury still has %d stack(s) after Inner Demon, want 0", bot.AuraStacks(AuraFelfury))
	}
	if got, want := bot.AuraMaxDuration(AbilityInnerDemon), felfuryStacks*5*time.Second; got != want {
		t.Errorf("E2E_FAIL: Inner Demon lasts %v after consuming %d Felfury, want %v", got, felfuryStacks, want)
	}

	assertCastRefused(t, bot, "right after consuming Felfury")
}

func assertCastRefused(t *testing.T, bot *e2eharness.ScenarioBot, when string) {
	t.Helper()
	res, err := bot.TryCast(t, AbilityInnerDemon, 0, 3*time.Second)
	switch {
	case err != nil:
		t.Fatalf("cast Inner Demon %s: %v", when, err)
	case res.Success:
		t.Errorf("E2E_FAIL: Inner Demon was cast %s", when)
	default:
		t.Logf("E2E_PASS: Inner Demon refused %s (%s)", when, e2eharness.SpellFailReasonName(res.FailReason))
	}
}

func waitStacks(t *testing.T, bot *e2eharness.ScenarioBot, spellID uint32, want int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for bot.AuraStacks(spellID) != want {
		if time.Now().After(deadline) {
			t.Fatalf("aura %d has %d stack(s), want %d", spellID, bot.AuraStacks(spellID), want)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
