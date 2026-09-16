//go:build e2e && setup

// Package setup_test prepares a character on the live local server so a human can log in with the
// game client and check something the protocol bots cannot see: greyed action buttons, tooltips,
// animations, models. These are not tests. They assert nothing, they leave state behind on purpose,
// and they are excluded from the normal suite by the `setup` build tag.
//
//	go test -tags=e2e,setup ./e2e/setup -run TestSetup_InnerDemonGlobalCooldown -count=1 -v
package setup_test

import (
	"fmt"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"github.com/azerothcore/AzerothGhost/e2e/e2eharness"
)

const (
	// One shared account for every manual check. GM level 3, trivial password: local development
	// server only. Never point this at anything reachable from outside the machine.
	setupAccount  = "test"
	setupPassword = "test"

	ClassFelsworn uint8 = 14

	AbilityInnerDemon   uint32 = 804216 // the ability under test, learned at level 2
	AbilityVengefulPact uint32 = 800029 // self-buff with a 1500 ms global cooldown, the control
	AbilityFelFireball  uint32 = 501281 // Fel Fireball rank 2, the rank level 10 grants
	AuraFelfury         uint32 = 800058 // Felfury resource aura, one stack per point, six maximum

	// Initiate's Training Dummy: rank 0, so it shows as an ordinary mob. The Heroic Training Dummy
	// (31146) is rank 3, and `.npc set level` does not change the rank, so it keeps the boss skull.
	dummyEntry    uint32 = 32541
	oldDummyEntry uint32 = 31146 // cleaned up so earlier runs do not leave a boss-ranked dummy behind
	dummyLevel           = 12    // its level 55 default is too high for a level 10 caster
	felfuryFull          = 6
	charLevel            = 10
)

// TestSetup_InnerDemonGlobalCooldown leaves a Felsworn standing next to a training dummy with a
// full Felfury pool, ready to check main project issue #391 in the client: whether the action bar
// greys Inner Demon for a second after a cast, and whether another ability's global cooldown greys
// it too. The server half is already covered by e2e/classes/felsworn/felfury; only the client's own
// reaction needs a human, and it needs a Spell.dbc patched by apps/coa-spells/inner_demon_gcd.py
// in the server repository — without that patch the button stays lit and the cast is refused by the
// server instead, which is still worth seeing once.
func TestSetup_InnerDemonGlobalCooldown(t *testing.T) {
	charName := "Felsgcd"

	authDB, charDB := e2eharness.OpenTestDBs(t)
	if err := e2eharness.EnsureAccount(authDB, setupAccount, setupPassword); err != nil {
		t.Fatalf("ensure account %s: %v", setupAccount, err)
	}
	if err := e2eharness.SetGM(authDB, setupAccount, 3); err != nil {
		t.Fatalf("set gm on %s: %v", setupAccount, err)
	}

	session, err := e2eharness.LoginBot(t, e2eharness.LoginOptions{
		User:     setupAccount,
		Password: setupPassword,
		CharName: charName,
		Race:     e2eharness.RaceOrc,
		Class:    ClassFelsworn,
	})
	if err != nil {
		t.Fatalf("login %s: %v", setupAccount, err)
	}
	defer session.Close()
	bot := &e2eharness.ScenarioBot{Session: session, AuthDB: authDB, CharDB: charDB}

	bot.GM(t, fmt.Sprintf(".character level %d", charLevel))
	for _, spell := range []uint32{AbilityInnerDemon, AbilityVengefulPact, AbilityFelFireball} {
		bot.Learn(t, spell)
	}
	pad := e2eharness.PackagePad(t)
	bot.TeleportPad(t, pad)

	// Grant Felfury before targeting anything: `.aura` applies to the current selection.
	for stacks := 1; stacks <= felfuryFull; stacks++ {
		bot.GM(t, fmt.Sprintf(".aura %d", AuraFelfury))
	}

	// Spawn the dummy by hand rather than through ScenarioBot.Spawn, which registers a cleanup
	// that would delete it the moment this function returns.
	bot.GM(t, ".gm on")
	bot.DespawnNearbyEntry(t, dummyEntry, 100)
	bot.DespawnNearbyEntry(t, oldDummyEntry, 100)
	known := map[uint64]struct{}{}
	for _, u := range bot.UnitsByEntry(100, dummyEntry) {
		known[u.GUID] = struct{}{}
	}
	bot.GM(t, fmt.Sprintf(".npc add %d", dummyEntry))
	spawned := bot.WaitNewUnits(t, known, []uint32{dummyEntry}, 15*time.Second)
	if len(spawned) == 0 {
		t.Fatalf("dummy %d did not appear after .npc add", dummyEntry)
	}
	_ = bot.World.SetTarget(spawned[0].GUID)
	bot.GM(t, fmt.Sprintf(".npc set level %d", dummyLevel))
	bot.GM(t, ".gm off")

	bot.Save(t)

	x, y, z, mapID := bot.Pos()
	t.Logf(`
Ready for a client session.

  account   %s / %s   (GM level 3 — say .gm on in game for GM commands)
  character %s, Orc Felsworn level %d
  standing  map %d at %.1f %.1f %.1f, training dummy (entry %d) set to level %d next to you
  spells    Inner Demon %d, Vengeful Pact %d (1500 ms global cooldown, the control),
            Fel Fireball %d (generates Felfury on hit)
  resource  %d Felfury granted now; auras do not always survive a relog, so top it up in game
            with a macro holding six lines of: .aura %d

What to look at for #391:
  1. Cast Inner Demon. The button should grey for about a second instead of being instantly reusable.
  2. Cast Vengeful Pact, then try Inner Demon straight away: it should be greyed as well.
  3. Without a patched client Spell.dbc the button will not grey; the server refuses the second cast
     and you get the "not ready yet" error instead. That difference is the point of the client patch.`,
		setupAccount, setupPassword, charName, charLevel,
		mapID, x, y, z, dummyEntry, dummyLevel,
		AbilityInnerDemon, AbilityVengefulPact, AbilityFelFireball,
		felfuryFull, AuraFelfury)
}
