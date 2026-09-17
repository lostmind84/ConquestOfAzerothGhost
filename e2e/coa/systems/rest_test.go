//go:build e2e

package systems_test

import (
	"testing"
	"time"

	"github.com/azerothcore/AzerothGhost/client"
	"github.com/azerothcore/AzerothGhost/e2e/e2eharness"
)

const (
	classKnightXoroth uint8  = 17
	spellSelectPvE    uint32 = 84422 // ruleset selection: CheckCast requires PLAYER_FLAGS_RESTING
)

// restProbe reports whether the server treats the bot as resting: the PvE ruleset selection is refused
// with SPELL_FAILED_NOT_HERE outside rested areas.
func restProbe(t *testing.T, bot *e2eharness.ScenarioBot) bool {
	t.Helper()
	if !bot.World.KnowsSpell(spellSelectPvE) {
		bot.Learn(t, spellSelectPvE)
		waitKnows(t, bot, spellSelectPvE, 3*time.Second)
	}
	res, err := bot.TryCast(t, spellSelectPvE, 0, castTimeout)
	if err != nil {
		t.Fatalf("PvE selection: %v", err)
	}
	t.Logf("PvE selection result: success=%v reason=%s", res.Success, e2eharness.SpellFailReasonName(res.FailReason))
	return res.Success
}

// logoutInstant sends a logout request and returns the instant flag of SMSG_LOGOUT_RESPONSE.
// Harness accounts are game masters, and the core logs game masters out instantly anywhere, so the flag is
// only logged.
func logoutInstant(t *testing.T, bot *e2eharness.ScenarioBot) bool {
	t.Helper()
	ch := make(chan []byte, 1)
	cancel := bot.World.AddPacketHook(func(op uint16, data []byte) {
		if op == client.SmsgLogoutResponse {
			select {
			case ch <- append([]byte(nil), data...):
			default:
			}
		}
	})
	defer cancel()
	if err := bot.World.SendLogout(); err != nil {
		t.Fatalf("logout request: %v", err)
	}
	select {
	case data := <-ch:
		if len(data) < 5 {
			t.Fatalf("short SMSG_LOGOUT_RESPONSE: %x", data)
		}
		return data[4] != 0
	case <-time.After(5 * time.Second):
		t.Fatalf("no SMSG_LOGOUT_RESPONSE")
	}
	return false
}

// Main project issues #98 and #363: no rested state in an inn (Tarren Mill), and logout is not instant there.
// Tarren Mill's inn trigger (721) is Horde-only in areatrigger_tavern, so an Alliance character does not rest there.
// The game client sends CMSG_AREATRIGGER when it enters the inn's AreaTrigger.dbc zone; the bot sends it
// itself at the reported position.
//
//	go test -tags=e2e ./e2e/coa/systems -run Rest -count=1 -v
func TestRest_Inn(t *testing.T) {
	for _, tc := range []struct {
		name     string
		race     uint8
		trigger  uint32
		x, y, z  float32
		wantRest bool
	}{
		{"TarrenMillHorde", e2eharness.RaceOrc, 721, -5.28, -938.17, 57.17, true},
		{"GoldshireAlliance", e2eharness.RaceHuman, 562, -9464.0, -10.0, 58.0, true},
		{"TarrenMillAlliance", e2eharness.RaceHuman, 721, -5.28, -938.17, 57.17, false}, // Horde-only inn
	} {
		t.Run(tc.name, func(t *testing.T) {
			bot := e2eharness.NewSolo(t, e2eharness.ScenarioOpts{Prefix: "Rest", Race: tc.race, Class: classKnightXoroth, Level: 40})
			bot.Teleport(t, tc.x, tc.y, tc.z, 0)
			bot.GM(t, ".gm off")
			time.Sleep(2 * time.Second)
			if restProbe(t, bot) {
				t.Logf("resting before the area trigger (city or zone rest flag)")
			}
			if err := bot.World.SendAreaTrigger(tc.trigger); err != nil {
				t.Fatalf("area trigger: %v", err)
			}
			time.Sleep(time.Second)
			time.Sleep(gcd)
			rested := restProbe(t, bot)
			instant := logoutInstant(t, bot)
			t.Logf("after trigger %d: resting=%v instant logout=%v", tc.trigger, rested, instant)
			if rested != tc.wantRest {
				t.Errorf("E2E_FAIL: inn trigger %d: resting=%v, want %v (#98, #363)", tc.trigger, rested, tc.wantRest)
				return
			}
			t.Logf("E2E_PASS: inn trigger %d gives resting=%v (#98, #363)", tc.trigger, rested)
		})
	}
}
