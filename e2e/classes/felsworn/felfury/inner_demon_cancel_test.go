//go:build e2e

package felfury_test

import (
	"fmt"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"github.com/azerothcore/AzerothGhost/e2e/e2eharness"
)

// Review of main project PR #1164: Inner Demon can start its global cooldown while another cast is still in
// progress. Cancelling that older cast used to clear the whole category, and so Inner Demon's newer cooldown too,
// letting the next ability through at once.
//
//	go test -tags=e2e ./e2e/classes/felsworn/felfury -run TestFelsworn_InnerDemonCooldownSurvivesCancel -count=1 -v
func TestFelsworn_InnerDemonCooldownSurvivesCancel(t *testing.T) {
	bot := e2eharness.NewSolo(t, e2eharness.ScenarioOpts{
		Prefix: "FelsCan",
		Race:   e2eharness.RaceOrc,
		Class:  ClassFelsworn,
		Level:  10,
	})
	bot.TeleportPad(t, e2eharness.PackagePad(t))
	for _, spell := range []uint32{AbilityInnerDemon, AbilityVengefulPact, AbilityFelFireballRank2} {
		bot.Learn(t, spell)
	}
	// `.aura` applies to the current selection, so grant Felfury before targeting the dummy.
	for stacks := 1; stacks <= 3; stacks++ {
		bot.GM(t, fmt.Sprintf(".aura %d", AuraFelfury))
		waitStacks(t, bot, AuraFelfury, stacks)
	}
	dummy := bot.Spawn(t, e2eharness.CreatureHeroicTrainingDummy, 10*time.Second)
	_ = bot.World.SetTarget(dummy)
	bot.GM(t, fmt.Sprintf(".npc set level %d", dummyLevel))
	bot.CombatReady(t)
	bot.Face(t, dummy)

	// Fel Fireball takes 2 s to cast; its own global cooldown ends after 1 s, while it is still casting.
	bot.ArmSpellWaiter()
	if err := bot.World.CastSpell(AbilityFelFireballRank2, dummy); err != nil {
		t.Fatalf("start Fel Fireball: %v", err)
	}
	time.Sleep(globalCooldown)

	// Inner Demon starts a new cooldown in the same category while Fel Fireball keeps casting.
	if err := bot.World.CastSpell(AbilityInnerDemon, 0); err != nil {
		t.Fatalf("cast Inner Demon during Fel Fireball: %v", err)
	}
	if res, err := bot.WaitSpellID(AbilityInnerDemon, 3*time.Second); err != nil || !res.Success {
		t.Fatalf("Inner Demon was not cast during Fel Fireball (err=%v, reason=%s); the scenario cannot run",
			err, e2eharness.SpellFailReasonName(res.FailReason))
	}

	// Cancel the older cast, then try an ability that shares the category, well inside Inner Demon's cooldown.
	if err := bot.World.CancelCastSpell(AbilityFelFireballRank2); err != nil {
		t.Fatalf("cancel Fel Fireball: %v", err)
	}
	time.Sleep(100 * time.Millisecond)
	if err := bot.World.CastSpell(AbilityVengefulPact, 0); err != nil {
		t.Fatalf("cast Vengeful Pact: %v", err)
	}
	res, err := bot.WaitSpellID(AbilityVengefulPact, 3*time.Second)
	if err != nil {
		t.Fatalf("wait for Vengeful Pact result: %v", err)
	}

	// Without a real cancel the check above would pass for the wrong reason: Fel Fireball must not complete.
	if fireball, err := bot.WaitSpellID(AbilityFelFireballRank2, 2*time.Second); err == nil && fireball.Success {
		t.Fatalf("Fel Fireball completed: the cancel was not applied, so this run proves nothing")
	}

	if res.Success {
		t.Errorf("E2E_FAIL: Vengeful Pact was cast right after cancelling Fel Fireball: " +
			"the cancel cleared Inner Demon's global cooldown (PR #1164 review)")
	} else {
		t.Logf("E2E_PASS: Vengeful Pact still refused after the cancel (%s)",
			e2eharness.SpellFailReasonName(res.FailReason))
	}
}
