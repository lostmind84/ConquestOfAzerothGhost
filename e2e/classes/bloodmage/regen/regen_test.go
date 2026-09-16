//go:build e2e

package regen_test

import (
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"github.com/azerothcore/AzerothGhost/client"
	"github.com/azerothcore/AzerothGhost/e2e/e2eharness"
)

const (
	ClassBloodmage uint8 = 20

	SpellBloodShield   uint32 = 504296 // +50% health regen (aura 88), regen continues in combat at 100% (aura 116)
	botLevel                  = 5      // level reported in #362
	CreatureDefiasThug uint32 = 38     // level 3-4 humanoid, hits back

	unitFieldStat4 uint16 = 0x0058 // UNIT_FIELD_STAT4 (Spirit), OBJECT_END + 0x52

	// gtOCTRegenHP.dbc row (class 20 - 1) * 100 + level 5 - 1, read from /srv/coa/server-data/dbc.
	octRegenHPRatio = 0.273975
	// World config EnableLowLevelRegenBoost = 1: Rate.Health * (2.066 - level * 0.066).
	lowLevelBoost = 2.066 - botLevel*0.066
	sampleWindow  = 9 * time.Second
)

// Main project issue #362: Bloodmage health regeneration during combat feels too high at level 5
// with Blood Shield up.
//
// The test measures the in-combat regeneration ticks with Blood Shield and compares them with
// Player::RegenerateHealth: Spirit * gtOCTRegenHP * 2, times the low-level boost, times Blood
// Shield's +50%, kept at 100% in combat. It reports whether the server follows its own formula;
// whether that formula is too strong for a Bloodmage is a balance question the test cannot answer.
//
//	go test -tags=e2e ./e2e/classes/bloodmage/regen -count=1 -v
func TestBloodmage_BloodShieldCombatRegenFollowsFormula(t *testing.T) {
	bot := e2eharness.NewSolo(t, e2eharness.ScenarioOpts{
		Prefix: "BRegen",
		Race:   e2eharness.RaceHuman,
		Class:  ClassBloodmage,
		Level:  botLevel,
	})
	bot.TeleportPad(t, e2eharness.PackagePad(t))
	bot.Learn(t, SpellBloodShield)

	res, err := bot.TryCast(t, SpellBloodShield, bot.World.CharGUID(), 5*time.Second)
	if err != nil || !res.Success {
		t.Fatalf("Blood Shield failed: %v %s", err, e2eharness.SpellFailReasonName(res.FailReason))
	}
	bot.WaitAura(t, SpellBloodShield, 3*time.Second)

	self := bot.World.GetObject(bot.World.CharGUID())
	if self == nil {
		t.Fatalf("own player object not tracked")
	}
	spirit := self.Value(unitFieldStat4)
	maxHP := self.MaxHealth()
	expected := float64(spirit) * octRegenHPRatio * 2 * lowLevelBoost * 1.5
	bot.Damage(t, bot.World.CharGUID(), maxHP*3/4) // `.modify hp` would also lower max health

	// A hostile creature that fights back keeps the bot in combat, as in the report; GM mode
	// would stop it from attacking.
	thug := bot.Spawn(t, CreatureDefiasThug, 10*time.Second)
	bot.GM(t, ".gm off")
	_ = bot.World.SetTarget(thug)
	bot.Face(t, thug)
	bot.Engage(t, thug, 15*time.Second)

	ticks := sampleRegenTicks(bot, sampleWindow)
	inCombat := bot.World.GetObject(bot.World.CharGUID()).Value(client.UnitFieldFlags)&client.UnitFlagInCombat != 0
	t.Logf("level %d, Spirit %d, max health %d, in combat %v; regen ticks %v; formula %.1f per tick",
		botLevel, spirit, maxHP, inCombat, ticks, expected)
	if !inCombat {
		t.Fatalf("bot left combat during the sample; the measurement is not an in-combat one")
	}
	if len(ticks) < 3 {
		t.Fatalf("only %d regeneration ticks seen in %v", len(ticks), sampleWindow)
	}
	for _, tick := range ticks {
		if float64(tick) < expected-2 || float64(tick) > expected+2 {
			t.Errorf("E2E_FAIL: in-combat regeneration tick %d, formula gives %.1f (#362)", tick, expected)
		}
	}
	if !t.Failed() {
		t.Logf("E2E_PASS: in-combat regeneration follows the formula: %.1f health per 2 s tick (%.1f%% of max health)",
			expected, 100*expected/float64(maxHP))
	}
}

// sampleRegenTicks records each health increase of the bot during window; melee hits taken only
// lower health, so increases are regeneration ticks.
func sampleRegenTicks(bot *e2eharness.ScenarioBot, window time.Duration) []uint32 {
	var ticks []uint32
	last := bot.World.GetObject(bot.World.CharGUID()).Health()
	deadline := time.Now().Add(window)
	for time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
		hp := bot.World.GetObject(bot.World.CharGUID()).Health()
		if hp > last {
			ticks = append(ticks, hp-last)
		}
		last = hp
	}
	return ticks
}
