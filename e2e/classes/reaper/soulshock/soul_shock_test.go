//go:build e2e

package soulshock_test

import (
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"github.com/azerothcore/AzerothGhost/e2e/e2eharness"
)

const (
	ClassReaper uint8 = 30

	SpellSoulShock     uint32 = 803989 // Mechanic 30 (sapped), 8 s, requires stealth (Stances 0x20000000)
	SpellUnderwalk     uint32 = 800797 // Reaper stealth (shapeshift form 30), learned at level 2
	CreatureDefiasThug uint32 = 38     // humanoid, a valid Soul Shock target
	botLevel                  = 8      // level reported in #197
	fullDuration              = 8 * time.Second
	castSpacing               = 1500 * time.Millisecond
	casts                     = 3
)

// Main project issue #197: Soul Shock can be recast on the same target for a full 8 s sap
// every time. The reporter expected 8 s, then about 4 s, then about 2 s.
//
// AzerothCore puts sapped spells in the disorient diminishing returns group, which only
// applies to players and to creatures flagged CREATURE_FLAG_EXTRA_ALL_DIMINISH.
// This test measures both targets.
//
//	go test -tags=e2e ./e2e/classes/reaper/soulshock -count=1 -v
func TestReaper_SoulShockOnCreature(t *testing.T) {
	bot := e2eharness.NewSolo(t, e2eharness.ScenarioOpts{
		Prefix: "Shock",
		Race:   e2eharness.RaceOrc,
		Class:  ClassReaper,
		Level:  botLevel,
	})
	bot.TeleportPad(t, e2eharness.PackagePad(t))
	bot.Learn(t, SpellUnderwalk)
	bot.Learn(t, SpellSoulShock)
	thug := bot.Spawn(t, CreatureDefiasThug, 10*time.Second)
	_ = bot.World.SetTarget(thug)
	bot.Face(t, thug)

	durations := make([]time.Duration, 0, casts)
	for i := 0; i < casts; i++ {
		enterStealth(t, bot)
		res, err := bot.TryCast(t, SpellSoulShock, thug, 5*time.Second)
		if err != nil || !res.Success {
			t.Fatalf("Soul Shock cast %d on the creature failed: %v %s", i+1, err,
				e2eharness.SpellFailReasonName(res.FailReason))
		}
		bot.WaitUnitAura(t, thug, SpellSoulShock, 3*time.Second)
		time.Sleep(200 * time.Millisecond)
		obj := bot.World.GetObject(thug)
		if obj == nil {
			t.Fatalf("creature no longer tracked")
		}
		durations = append(durations, obj.AuraMaxDuration(SpellSoulShock))
		time.Sleep(castSpacing)
	}
	t.Logf("E2E_INFO: Soul Shock max duration on a creature, casts 1..%d: %v", casts, durations)
}

func TestReaper_SoulShockOnPlayer(t *testing.T) {
	bots := e2eharness.NewScenario(t, e2eharness.ScenarioOpts{
		Prefix: "ShockP",
		Bots: []e2eharness.BotSpec{
			{Role: "reaper", Race: e2eharness.RaceOrc, Class: ClassReaper, Level: botLevel},
			{Role: "victim", Race: e2eharness.RaceHuman, Class: e2eharness.ClassWarrior, Level: botLevel},
		},
	})
	reaper, victim := bots[0], bots[1]
	e2eharness.TeleportAllPad(t, bots, e2eharness.PackagePad(t))
	reaper.Learn(t, SpellUnderwalk)
	reaper.Learn(t, SpellSoulShock)
	e2eharness.EnableHostilePvP(t, reaper, victim)
	victimGUID := victim.World.CharGUID()
	reaper.WaitUnitPvP(t, victimGUID, 5*time.Second)
	_ = reaper.World.SetTarget(victimGUID)
	reaper.Face(t, victimGUID)

	durations := make([]time.Duration, 0, casts)
	for i := 0; i < casts; i++ {
		enterStealth(t, reaper)
		res, err := reaper.TryCast(t, SpellSoulShock, victimGUID, 5*time.Second)
		if err != nil || !res.Success {
			t.Fatalf("Soul Shock cast %d on the player failed: %v %s", i+1, err,
				e2eharness.SpellFailReasonName(res.FailReason))
		}
		time.Sleep(500 * time.Millisecond)
		durations = append(durations, victim.AuraMaxDuration(SpellSoulShock))
		time.Sleep(castSpacing)
	}
	t.Logf("Soul Shock max duration on a player, casts 1..%d: %v", casts, durations)

	want := []time.Duration{fullDuration, fullDuration / 2, fullDuration / 4}
	for i, got := range durations {
		if got != want[i] {
			t.Errorf("E2E_FAIL: cast %d lasted %v on a player, want %v (diminishing returns, #197)", i+1, got, want[i])
		}
	}
	if !t.Failed() {
		t.Logf("E2E_PASS: Soul Shock diminishes on players: %v", durations)
	}
}

// enterStealth casts Underwalk unless the Reaper is already in it; Soul Shock requires stealth.
func enterStealth(t *testing.T, bot *e2eharness.ScenarioBot) {
	t.Helper()
	if bot.HasAura(SpellUnderwalk) {
		return
	}
	res, err := bot.TryCast(t, SpellUnderwalk, 0, 5*time.Second)
	if err != nil || !res.Success {
		t.Fatalf("Underwalk failed: %v %s", err, e2eharness.SpellFailReasonName(res.FailReason))
	}
	bot.WaitAura(t, SpellUnderwalk, 3*time.Second)
}
