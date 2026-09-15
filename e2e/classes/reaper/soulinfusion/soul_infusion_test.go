//go:build e2e

package soulinfusion_test

import (
	"fmt"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"github.com/azerothcore/AzerothGhost/e2e/e2eharness"
)

const (
	ClassReaper uint8 = 30

	AbilitySoulrend  uint32 = 573316 // Soulrend Rank 1, learned at level 1, requires Soul Infusion
	AuraSoulInfusion uint32 = 803031 // granted by the server at 3 Reaped Souls
	AuraReapedSoul   uint32 = 500363
	reapedSoulsCap          = 3
	botLevel                = 10
	auraWaitTimeout         = 3 * time.Second
	castAttempts            = 4
	globalCooldown          = 1600 * time.Millisecond
)

// Abilities that require Soul Infusion must consume it.
// Main project issue #163 reports that Reaper abilities do not consume Soul Infusion;
// at the reported level 10, Soulrend is the ability that requires it.
//
// Soul Infusion comes from three Reaped Souls, so the souls must go with it:
// otherwise the server grants Soul Infusion again right after the cast.
//
//	go test -tags=e2e ./e2e/classes/reaper/soulinfusion -count=1 -v
func TestReaper_SoulrendConsumesSoulInfusion(t *testing.T) {
	bot := e2eharness.NewSolo(t, e2eharness.ScenarioOpts{
		Prefix: "Reap",
		Race:   e2eharness.RaceOrc,
		Class:  ClassReaper,
		Level:  botLevel,
	})
	bot.TeleportPad(t, e2eharness.PackagePad(t))
	bot.Learn(t, AbilitySoulrend)

	dummy := bot.Spawn(t, e2eharness.CreatureHeroicTrainingDummy, 10*time.Second)
	_ = bot.World.SetTarget(dummy)
	bot.GM(t, fmt.Sprintf(".npc set level %d", botLevel)) // a level 83 dummy resists most attacks
	bot.CombatReady(t)
	bot.Face(t, dummy)

	for attempt := 1; attempt <= castAttempts; attempt++ {
		grantSoulInfusion(t, bot, dummy)
		res, err := bot.TryCast(t, AbilitySoulrend, dummy, 5*time.Second)
		if err != nil {
			t.Fatalf("cast Soulrend: %v", err)
		}
		if !res.Success {
			t.Fatalf("Soulrend was refused with Soul Infusion: %s", e2eharness.SpellFailReasonName(res.FailReason))
		}
		if bot.TryWaitAuraGone(t, AuraSoulInfusion, auraWaitTimeout) {
			if souls := bot.AuraStacks(AuraReapedSoul); souls != 0 {
				t.Errorf("E2E_FAIL: %d Reaped Soul(s) left after Soul Infusion was consumed (#163)", souls)
			}
			time.Sleep(time.Second)
			if bot.HasAura(AuraSoulInfusion) {
				t.Errorf("E2E_FAIL: Soul Infusion came back after Soulrend consumed it (#163)")
			} else {
				t.Logf("E2E_PASS: Soulrend consumed Soul Infusion and its Reaped Souls (cast %d)", attempt)
			}
			return
		}
		// A missed Soulrend may keep Soul Infusion, so allow a few casts.
		t.Logf("Soul Infusion still active after Soulrend cast %d", attempt)
		time.Sleep(globalCooldown)
	}
	t.Errorf("E2E_FAIL: Soul Infusion was never consumed by %d Soulrend casts (#163)", castAttempts)
}

// grantSoulInfusion adds Reaped Souls up to the cap, waits for the server to grant Soul Infusion
// and selects target again.
func grantSoulInfusion(t *testing.T, bot *e2eharness.ScenarioBot, target uint64) {
	t.Helper()
	_ = bot.World.SetTarget(bot.World.CharGUID()) // `.aura` applies to the current selection
	for bot.AuraStacks(AuraReapedSoul) < reapedSoulsCap {
		before := bot.AuraStacks(AuraReapedSoul)
		bot.GM(t, fmt.Sprintf(".aura %d", AuraReapedSoul))
		deadline := time.Now().Add(auraWaitTimeout)
		for bot.AuraStacks(AuraReapedSoul) == before {
			if time.Now().After(deadline) {
				t.Fatalf("Reaped Soul stayed at %d stack(s) after .aura", before)
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
	bot.WaitAura(t, AuraSoulInfusion, auraWaitTimeout)
	_ = bot.World.SetTarget(target)
}
