//go:build e2e

package systems_test

import (
	"encoding/binary"
	"testing"
	"time"

	"github.com/azerothcore/AzerothGhost/client"
	"github.com/azerothcore/AzerothGhost/e2e/e2eharness"
)

// Main project issue #2294: First Aid (and possibly other professions) is forgotten after a relog.
//
//	go test -tags=e2e ./e2e/coa/systems -run FirstAid -count=1 -v
func TestProfession_FirstAidKeptAfterRelog(t *testing.T) {
	const spellFirstAidApprentice uint32 = 3273
	for _, tc := range []struct {
		name string
		cmd  string
	}{
		{"Learn", ".learn 3273"},
		{"TrainerSpell", ".cast self 3279"}, // what First Aid trainers cast: learns 3273 and the skill
	} {
		t.Run(tc.name, func(t *testing.T) {
			bot := newBot(t, "FAid", e2eharness.RaceDraenei, classRanger, 12)
			bot.GM(t, tc.cmd)
			if !waitKnows(t, bot, spellFirstAidApprentice, 3*time.Second) {
				t.Fatalf("precondition: First Aid %d not learned by %q", spellFirstAidApprentice, tc.cmd)
			}
			bot.Relog(t)
			if !waitKnows(t, bot, spellFirstAidApprentice, 5*time.Second) {
				t.Fatalf("E2E_FAIL: First Aid %d forgotten after relog (#2294)", spellFirstAidApprentice)
			}
			t.Logf("E2E_PASS: First Aid %d still known after relog (#2294)", spellFirstAidApprentice)
		})
	}
}

// Main project issue #1433: "Reset all Dungeons" does not reset Ragefire Chasm.
//
//	go test -tags=e2e ./e2e/coa/systems -run ResetInstances -count=1 -v
func TestInstance_ResetRagefireChasm(t *testing.T) {
	const mapRagefireChasm uint32 = 389
	bot := e2eharness.NewSolo(t, e2eharness.ScenarioOpts{Prefix: "Reset", Race: e2eharness.RaceOrc, Class: classKnightXoroth, Level: 17})
	bot.Teleport(t, 3.81, -14.82, -17.84, mapRagefireChasm)
	time.Sleep(3 * time.Second)
	if _, _, _, m := bot.Pos(); m != mapRagefireChasm {
		t.Fatalf("precondition: bot on map %d, want %d", m, mapRagefireChasm)
	}
	bot.GM(t, ".gm off") // game masters are not bound to instances
	bot.Teleport(t, 1811.0, -4410.0, -18.0, 1)
	time.Sleep(time.Second)
	bot.Teleport(t, 3.81, -14.82, -17.84, mapRagefireChasm)
	time.Sleep(3 * time.Second)
	bot.Teleport(t, 1811.0, -4410.0, -18.0, 1)
	time.Sleep(3 * time.Second)

	results := make(chan [2]uint32, 4)
	cancel := bot.World.AddPacketHook(func(op uint16, data []byte) {
		if (op == client.SmsgInstanceReset || op == client.SmsgInstanceResetFailed) && len(data) >= 4 {
			var reason uint32
			m := binary.LittleEndian.Uint32(data[len(data)-4:])
			if op == client.SmsgInstanceResetFailed && len(data) >= 8 {
				reason = binary.LittleEndian.Uint32(data[0:4]) + 1
			}
			results <- [2]uint32{m, reason}
		}
	})
	defer cancel()
	if err := bot.World.ResetInstances(); err != nil {
		t.Fatalf("reset: %v", err)
	}
	select {
	case r := <-results:
		if r[1] != 0 {
			t.Fatalf("E2E_FAIL: SMSG_INSTANCE_RESET_FAILED for map %d, reason %d (#1433)", r[0], r[1]-1)
		}
		t.Logf("E2E_PASS: SMSG_INSTANCE_RESET for map %d (#1433)", r[0])
	case <-time.After(5 * time.Second):
		t.Fatalf("E2E_FAIL: no reset answer after entering Ragefire Chasm (no bind, or reset ignored) (#1433)")
	}
}

// Main project issue #306: a duel kills the loser instead of ending at 1 health.
//
//	go test -tags=e2e ./e2e/coa/systems -run Duel -count=1 -v
func TestDuel_LoserSurvives(t *testing.T) {
	const (
		classGuardian uint8  = 18
		spellDuel     uint32 = 7266
	)
	bots := e2eharness.NewScenario(t, e2eharness.ScenarioOpts{Prefix: "Duel", Count: 2, Race: e2eharness.RaceHuman, Class: classGuardian, Level: 21})
	a, b := bots[0], bots[1]
	pad := e2eharness.PackagePad(t)
	a.TeleportPad(t, pad)
	b.TeleportPad(t, pad)
	for _, bot := range bots {
		bot.GM(t, ".gm off")
	}
	time.Sleep(2 * time.Second)

	requested := make(chan uint64, 1)
	cancelReq := b.World.AddPacketHook(func(op uint16, data []byte) {
		if op == client.SmsgDuelRequested && len(data) >= 8 {
			select {
			case requested <- binary.LittleEndian.Uint64(data[0:8]):
			default:
			}
		}
	})
	defer cancelReq()
	winner := make(chan struct{}, 1)
	cancelWin := a.World.AddPacketHook(func(op uint16, _ []byte) {
		if op == client.SmsgDuelWinner {
			select {
			case winner <- struct{}{}:
			default:
			}
		}
	})
	defer cancelWin()

	a.Face(t, b.GUID)
	res, err := a.TryCast(t, spellDuel, b.GUID, castTimeout)
	if err != nil || !res.Success {
		t.Fatalf("precondition: duel request refused: err=%v result=%+v", err, res)
	}
	var arbiter uint64
	select {
	case arbiter = <-requested:
	case <-time.After(5 * time.Second):
		t.Fatalf("precondition: no SMSG_DUEL_REQUESTED")
	}
	if err := b.World.AcceptDuel(arbiter); err != nil {
		t.Fatalf("accept: %v", err)
	}
	time.Sleep(4 * time.Second) // countdown

	// Melee auto attacks, as in a player duel; a god-mode challenger keeps the fight one-sided.
	a.CheatGod(t)
	a.Attack(t, b.GUID)
	select {
	case <-winner:
	case <-time.After(90 * time.Second):
		t.Fatalf("precondition: no SMSG_DUEL_WINNER after 90 s of melee")
	}
	time.Sleep(time.Second)
	hp, _ := a.UnitHP(b.GUID)
	if hp == 0 {
		t.Fatalf("E2E_FAIL: the duel loser died (health 0) (#306)")
	}
	t.Logf("E2E_PASS: the duel loser survived with %d health (#306)", hp)
}

// Main project issue #185: GM status only works for the server host's account.
// The account's access row is written the way `.account set gmlevel` does, for this realm and for all realms.
//
//	go test -tags=e2e ./e2e/coa/systems -run GMAccess -count=1 -v
func TestGMAccess_OtherAccount(t *testing.T) {
	for _, tc := range []struct {
		name  string
		realm int
		level int
	}{
		{"None", 0, 0},
		{"AllRealms", -1, 3},
		{"ThisRealm", 1, 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bot := e2eharness.NewSolo(t, e2eharness.ScenarioOpts{Prefix: "GmAcc", Race: e2eharness.RaceHuman, Class: classKnightXoroth, Level: 5})
			db, err := e2eharness.OpenAuthDB()
			if err != nil {
				t.Fatalf("auth db: %v", err)
			}
			defer db.Close()
			var id int
			if err := db.QueryRow(`SELECT id FROM account WHERE username=UPPER(?)`, bot.Ident.Account).Scan(&id); err != nil {
				t.Fatalf("account id: %v", err)
			}
			if _, err := db.Exec(`DELETE FROM account_access WHERE id=?`, id); err != nil {
				t.Fatalf("clear access: %v", err)
			}
			if tc.level > 0 {
				if _, err := db.Exec(`INSERT INTO account_access (id, gmlevel, RealmID) VALUES (?,?,?)`, id, tc.level, tc.realm); err != nil {
					t.Fatalf("grant access: %v", err)
				}
			}
			bot.Relog(t)
			before := bot.PlayerMoney()
			_ = bot.World.SendGMCommand(".modify money 12345")
			time.Sleep(2 * time.Second)
			after := bot.PlayerMoney()
			works := after >= before+12345
			t.Logf("gm level %d realm %d: money %d -> %d", tc.level, tc.realm, before, after)
			if works != (tc.level > 0) {
				t.Fatalf("E2E_FAIL: GM command works=%v with gm level %d on realm %d (#185)", works, tc.level, tc.realm)
			}
			t.Logf("E2E_PASS: GM command works=%v with gm level %d on realm %d (#185)", works, tc.level, tc.realm)
		})
	}
}

// Main project issues #368, #1410 and #1966: crafting (First Aid bandages here) gives no character experience.
// The server has no crafting or gathering experience rule (Ascension's was server-side); this test fails until one
// exists, and the amount is not known.
//
//	go test -tags=e2e ./e2e/coa/systems -run CraftingXP -count=1 -v
func TestProfession_CraftingXP(t *testing.T) {
	const (
		spellFirstAidApprentice uint32 = 3273
		spellLinenBandage       uint32 = 3275
		itemLinenCloth          uint32 = 2589
		itemLinenBandage        uint32 = 1251
	)
	bot := newBot(t, "Craft", e2eharness.RaceOrc, classKnightXoroth, 13)
	bot.Learn(t, spellFirstAidApprentice)
	bot.Learn(t, spellLinenBandage)
	if !waitKnows(t, bot, spellLinenBandage, 3*time.Second) {
		t.Fatalf("precondition: Linen Bandage not learned")
	}
	bot.AddItem(t, itemLinenCloth, 10)
	time.Sleep(time.Second)
	bot.GM(t, ".gm off")
	before := bot.PlayerXP()
	res, err := bot.TryCast(t, spellLinenBandage, 0, 10*time.Second)
	if err != nil || !res.Success {
		t.Fatalf("precondition: Linen Bandage craft refused: err=%v result=%+v", err, res)
	}
	time.Sleep(5 * time.Second) // 3 s cast
	if n := bot.InventoryCount(t, itemLinenBandage); n == 0 {
		t.Fatalf("precondition: no Linen Bandage crafted (cast %+v, linen cloth left %d)", res, bot.InventoryCount(t, itemLinenCloth))
	}
	after := bot.PlayerXP()
	if after == before {
		t.Fatalf("E2E_FAIL: crafting a Linen Bandage at level 13 gave no experience (%d -> %d) (#368, #1410, #1966)", before, after)
	}
	t.Logf("E2E_PASS: crafting gave %d experience (#368, #1410, #1966)", after-before)
}
