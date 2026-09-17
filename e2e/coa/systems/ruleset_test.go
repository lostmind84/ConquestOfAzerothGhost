//go:build e2e

package systems_test

import (
	"testing"
	"time"

	"github.com/azerothcore/AzerothGhost/e2e/e2eharness"
)

const (
	spellSelectWarMode uint32 = 84420
	auraHighRisk       uint32 = 1004019
	auraWarMode        uint32 = 1004119 // "War Mode": also the base marker of the PvE ruleset
	auraPvE            uint32 = 9931032
)

// Main project issues #1421, #1454 and #146 (second part): selecting PvE mode keeps the War Mode buff.
// The client's C_Player:GetRuleset (Interface\FrameXML\Util\C_Player.lua in patch-B.MPQ) returns NoRiskPvE only
// when 9931032 is present together with 1004119, so the PvE ruleset is both auras by the client's own contract.
// The test records the aura sets and checks that neither No Risk ruleset flags the character for PvP.
//
//	go test -tags=e2e ./e2e/coa/systems -run Ruleset -count=1 -v
func TestRuleset_PvEAndWarModeAuras(t *testing.T) {
	bot := e2eharness.NewSolo(t, e2eharness.ScenarioOpts{Prefix: "Rules", Race: e2eharness.RaceHuman, Class: classKnightXoroth, Level: 20})
	bot.Teleport(t, -9464.0, -10.0, 58.0, 0) // Goldshire inn
	bot.GM(t, ".gm off")
	time.Sleep(2 * time.Second)
	if err := bot.World.SendAreaTrigger(562); err != nil {
		t.Fatalf("area trigger: %v", err)
	}
	time.Sleep(time.Second)
	t.Logf("login default: high risk=%v war mode=%v pve=%v", bot.HasAura(auraHighRisk), bot.HasAura(auraWarMode), bot.HasAura(auraPvE))
	if !bot.HasAura(auraWarMode) || !bot.HasAura(auraPvE) {
		t.Errorf("E2E_FAIL: a new character has no PvE ruleset at login (#146)")
	}

	for _, tc := range []struct {
		name             string
		spell            uint32
		wantWar, wantPvE bool
	}{
		{"WarMode", spellSelectWarMode, true, false},
		{"PvE", spellSelectPvE, true, true},
	} {
		time.Sleep(gcd)
		if !bot.World.KnowsSpell(tc.spell) {
			t.Fatalf("precondition: selection %d not known", tc.spell)
		}
		res, err := bot.TryCast(t, tc.spell, 0, castTimeout)
		if err != nil || !res.Success {
			t.Fatalf("precondition: %s selection refused: err=%v result=%+v", tc.name, err, res)
		}
		time.Sleep(time.Second)
		war, pve, pvp := bot.HasAura(auraWarMode), bot.HasAura(auraPvE), bot.World.SelfIsPvP()
		t.Logf("%s selected: war mode aura=%v pve aura=%v pvp flag=%v", tc.name, war, pve, pvp)
		if war != tc.wantWar || pve != tc.wantPvE || bot.HasAura(auraHighRisk) {
			t.Errorf("E2E_FAIL: %s selection gives war mode=%v pve=%v, want %v %v (#1421, #1454)", tc.name, war, pve, tc.wantWar, tc.wantPvE)
		}
		if pvp {
			t.Errorf("E2E_FAIL: %s selection flags the character for PvP (#1421, #1454)", tc.name)
		}
	}
	if !t.Failed() {
		t.Logf("E2E_PASS: PvE is War Mode + PvE markers as C_Player:GetRuleset requires, and neither flags for PvP (#1421, #1454, #146)")
	}
}
