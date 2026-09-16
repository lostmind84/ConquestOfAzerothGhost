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
	// Vengeful Pact is a Felsworn self-buff learned at level 4. Client Spell.dbc gives it
	// StartRecoveryCategory 133 and StartRecoveryTime 1500, so it both triggers and observes
	// the shared global cooldown. It is the control for the Inner Demon measurements below.
	AbilityVengefulPact uint32 = 800029

	innerDemonAttempts = 8

	// Long enough to outlast Vengeful Pact's 1500 ms global cooldown.
	controlGlobalCooldown = 1600 * time.Millisecond
)

// Main project issue #391 reports that Inner Demon consumes no Felfury and triggers no global
// cooldown, so it can be spammed. The Felfury half is covered by
// TestFelsworn_InnerDemonConsumesFelfury; this test covers the global cooldown half.
//
//	go test -tags=e2e ./e2e/classes/felsworn/felfury -run TestFelsworn_InnerDemonGlobalCooldown -count=1 -v
func TestFelsworn_InnerDemonGlobalCooldown(t *testing.T) {
	bot := e2eharness.NewSolo(t, e2eharness.ScenarioOpts{
		Prefix: "FelsGCD",
		Race:   e2eharness.RaceOrc,
		Class:  ClassFelsworn,
		Level:  10,
	})
	bot.TeleportPad(t, e2eharness.PackagePad(t))
	bot.Learn(t, AbilityInnerDemon)
	bot.Learn(t, AbilityVengefulPact)

	// Control: a spell that does carry a global cooldown refuses its own immediate recast.
	// Without this the rest of the test cannot tell "no global cooldown" from "harness too slow".
	bot.CastMust(t, AbilityVengefulPact, 0, 5*time.Second)
	res, err := bot.TryCast(t, AbilityVengefulPact, 0, 3*time.Second)
	switch {
	case err != nil:
		t.Fatalf("control recast of Vengeful Pact: %v", err)
	case res.Success:
		t.Fatalf("control failed: Vengeful Pact was recast immediately, so this run cannot observe a global cooldown")
	default:
		t.Logf("control: Vengeful Pact refused during its own global cooldown (%s)",
			e2eharness.SpellFailReasonName(res.FailReason))
	}

	// Inner Demon cast while that global cooldown is still running.
	bot.GM(t, fmt.Sprintf(".aura %d", AuraFelfury))
	waitStacks(t, bot, AuraFelfury, 1)
	res, err = bot.TryCast(t, AbilityInnerDemon, 0, 3*time.Second)
	if err != nil {
		t.Fatalf("cast Inner Demon during Vengeful Pact global cooldown: %v", err)
	}
	if res.Success {
		t.Errorf("E2E_FAIL: Inner Demon was cast while the shared global cooldown was running (#391)")
	} else {
		t.Logf("E2E_PASS: Inner Demon refused during the shared global cooldown (%s)",
			e2eharness.SpellFailReasonName(res.FailReason))
	}
	// Spam: keep at least one Felfury available and cast as fast as the session allows,
	// starting from a clean global cooldown. Only the first attempt may land.
	time.Sleep(controlGlobalCooldown)
	casts, start := 0, time.Now()
	for attempt := 1; attempt <= innerDemonAttempts; attempt++ {
		if bot.AuraStacks(AuraFelfury) == 0 {
			bot.GM(t, fmt.Sprintf(".aura %d", AuraFelfury))
			waitStacks(t, bot, AuraFelfury, 1)
		}
		res, err := bot.TryCast(t, AbilityInnerDemon, 0, 3*time.Second)
		if err != nil {
			t.Fatalf("spam attempt %d: %v", attempt, err)
		}
		if res.Success {
			casts++
			continue
		}
		t.Logf("spam attempt %d refused (%s)", attempt, e2eharness.SpellFailReasonName(res.FailReason))
	}
	elapsed := time.Since(start)
	switch {
	case casts > 1:
		t.Errorf("E2E_FAIL: Inner Demon was cast %d times in %v (%d attempts), want at most 1 per global cooldown (#391)",
			casts, elapsed.Round(time.Millisecond), innerDemonAttempts)
	case casts == 0:
		t.Errorf("E2E_FAIL: no Inner Demon cast landed in %d attempts; the spam measurement is inconclusive", innerDemonAttempts)
	default:
		t.Logf("E2E_PASS: 1 of %d Inner Demon casts landed in %v", innerDemonAttempts, elapsed.Round(time.Millisecond))
	}
}
