//go:build e2e

package crashes_test

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"github.com/azerothcore/AzerothGhost/client"
	"github.com/azerothcore/AzerothGhost/e2e/e2eharness"
)

// Server crash reports: each test performs the reported action, checks that the action really
// happened, then checks that the world session is still alive. A worldserver crash drops every
// session.
//
//	go test -tags=e2e ./e2e/coa/crashes -count=1 -v -p 1

const (
	CreatureHarvestGolem       uint32 = 36
	settleDelay                       = 3 * time.Second
	smsgSpellNonMeleeDamageLog uint16 = 0x0250
)

// Main project issue #238 (and the Shadow Effigy entry of #265): placing Shadow Effigy with an
// enemy within 30 yards crashes the server.
func TestCrash_ShadowEffigyNearEnemy(t *testing.T) {
	const (
		classWitchDoctor  uint8  = 13
		spellFrostbolt    uint32 = 116
		spellShadowEffigy uint32 = 505339
		auraShadowField   uint32 = 504761 // applied by the effigy to nearby enemies
		creatureEffigy    uint32 = 50119
	)
	bot := e2eharness.NewSolo(t, e2eharness.ScenarioOpts{Prefix: "Effig", Race: e2eharness.RaceOrc, Class: classWitchDoctor, Level: 10})
	bot.TeleportPad(t, e2eharness.PackagePad(t))
	bot.Learn(t, spellFrostbolt)
	bot.Learn(t, spellShadowEffigy)
	bot.CheatPower(t)

	enemy := bot.Spawn(t, CreatureHarvestGolem, 10*time.Second)
	_ = bot.World.SetTarget(enemy)
	bot.GM(t, ".npc set level 10")
	bot.CombatReady(t)
	bot.Face(t, enemy)
	if res, err := bot.TryCast(t, spellFrostbolt, enemy, 5*time.Second); err != nil || !res.Success {
		t.Logf("Frostbolt: err=%v result=%+v", err, res)
	}
	bot.CastMust(t, spellShadowEffigy, 0, 5*time.Second)
	if bot.WaitUnitAny(t, 5*time.Second, creatureEffigy) == 0 {
		t.Fatalf("precondition: no Shadow Effigy appeared")
	}
	time.Sleep(10 * time.Second) // the effigy pulses on nearby enemies
	e2eharness.ProbeWorldAlive(t, bot, 238)
	if !bot.UnitHasAura(enemy, auraShadowField) {
		t.Errorf("precondition: the enemy never received Shadow Field (%d); the crash path was not exercised", auraShadowField)
	}
}

// Main project issue #265: Barbaric Whirl's off-hand helper (805232) asserts in
// Spell::SelectImplicitTargetObjectTargets when its explicit target is gone.
// Level 1 enemies die to the main-hand strike, so the off-hand helper fires at dying units.
func TestCrash_BarbaricWhirlKillingBlow(t *testing.T) {
	bot, offhandHits, cancel := barbaricWhirlBot(t, "Whirl")
	defer cancel()
	for round := 1; round <= barbaricWhirlRounds; round++ {
		for i := 0; i < barbaricWhirlEnemies; i++ {
			guid := bot.Spawn(t, CreatureHarvestGolem, 10*time.Second)
			_ = bot.World.SetTarget(guid)
			bot.GM(t, ".npc set level 1")
		}
		bot.CombatReady(t)
		bot.CastMust(t, spellBarbaricWhirl, 0, 5*time.Second)
		time.Sleep(settleDelay)
		e2eharness.ProbeWorldAlive(t, bot, 265)
		bot.GM(t, ".gm on")
		time.Sleep(time.Second)
	}
	if offhandHits.Load() == 0 {
		t.Errorf("precondition: the off-hand helper never hit; the crash path was not exercised")
	}
}

const (
	spellBarbaricWhirl   uint32 = 500002 // learned at level 14
	spellWhirlOffhand    uint32 = 805232
	barbaricWhirlEnemies        = 4
	barbaricWhirlRounds         = 5
)

// barbaricWhirlBot builds a level 20 dual-wielding Barbarian that knows Barbaric Whirl and counts the
// off-hand helper's hits. The client receives no SMSG_SPELL_GO for the triggered helper; its damage
// log proves it fired.
func barbaricWhirlBot(t *testing.T, prefix string) (*e2eharness.ScenarioBot, *atomic.Int32, func()) {
	t.Helper()
	const (
		classBarbarian  uint8  = 12
		spellDualWield  uint32 = 674
		itemStoneCutter uint32 = 629997 // starting two-handed weapon
		itemHandAxe     uint32 = 2134
	)
	bot := e2eharness.NewSolo(t, e2eharness.ScenarioOpts{Prefix: prefix, Race: e2eharness.RaceOrc, Class: classBarbarian, Level: 20})
	bot.TeleportPad(t, e2eharness.PackagePad(t))
	bot.Learn(t, spellDualWield)
	bot.Learn(t, spellBarbaricWhirl)
	bot.GM(t, fmt.Sprintf(".additem %d -1", itemStoneCutter)) // free both hands
	time.Sleep(500 * time.Millisecond)
	bot.EquipEntry(t, itemHandAxe, 1) // main hand
	bot.EquipEntry(t, itemHandAxe, 1) // off hand
	if bot.VisibleItemEntry(16) != itemHandAxe {
		t.Fatalf("precondition: off hand holds %d, want %d", bot.VisibleItemEntry(16), itemHandAxe)
	}
	bot.CheatPower(t)
	var offhandHits atomic.Int32
	return bot, &offhandHits, countDamageLogs(bot, spellWhirlOffhand, &offhandHits)
}

// Main project issue #265, Barbaric Whirl variant: the assert needs the helper's explicit target to
// be gone from the map. Temporary spawns (`.npc add temp`) despawn their corpse at death, so the
// main-hand strike removes the target before the off-hand helper selects it.
func TestCrash_BarbaricWhirlTargetDespawns(t *testing.T) {
	bot, offhandHits, cancel := barbaricWhirlBot(t, "WhirD")
	defer cancel()
	for round := 1; round <= barbaricWhirlRounds; round++ {
		known := map[uint64]struct{}{}
		for _, u := range bot.UnitsByEntry(120, CreatureHarvestGolem) {
			known[u.GUID] = struct{}{}
		}
		for i := 0; i < barbaricWhirlEnemies; i++ {
			bot.GM(t, fmt.Sprintf(".npc add temp %d", CreatureHarvestGolem))
		}
		for _, u := range bot.WaitNewUnits(t, known, []uint32{CreatureHarvestGolem}, 5*time.Second) {
			_ = bot.World.SetTarget(u.GUID)
			bot.GM(t, ".npc set level 1")
		}
		_ = bot.World.SetTarget(bot.World.CharGUID())
		bot.GM(t, ".cooldown") // temporary spawns appear faster than Barbaric Whirl's cooldown
		bot.CombatReady(t)
		bot.CastMust(t, spellBarbaricWhirl, 0, 5*time.Second)
		time.Sleep(settleDelay)
		e2eharness.ProbeWorldAlive(t, bot, 265)
		bot.GM(t, ".gm on")
		time.Sleep(time.Second)
	}
	if offhandHits.Load() == 0 {
		t.Errorf("precondition: the off-hand helper never hit; the crash path was not exercised")
	}
}

// Main project issue #265: Destructo-Bot possession aborts in Player::StopCastingCharm when the
// Tinker dies while controlling it.
func TestCrash_DestructoBotOwnerDies(t *testing.T) {
	bot := possessDestructoBot(t, "Destr")
	bot.GM(t, ".cheat god off")
	_ = bot.World.SetTarget(bot.World.CharGUID())
	bot.GM(t, ".die")
	time.Sleep(settleDelay)
	e2eharness.ProbeWorldAlive(t, bot, 265)
}

// Main project issue #265: Destructo-Bot aborts in Unit::RemoveFromWorld ("has charmer guid when
// removed from world") when it expires while still possessed.
func TestCrash_DestructoBotExpiresWhileControlled(t *testing.T) {
	const expiryWait = 45 * time.Second
	bot := possessDestructoBot(t, "Expir")
	t.Logf("waiting %v for the Destructo-Bot to expire", expiryWait)
	time.Sleep(expiryWait)
	e2eharness.ProbeWorldAlive(t, bot, 265)
	if charmed(bot) != 0 {
		t.Errorf("the player still possesses a unit after %v", expiryWait)
	}
}

// possessDestructoBot builds a Destructo-Bot with a level 60 Tinker and checks that the player
// possesses it.
func possessDestructoBot(t *testing.T, prefix string) *e2eharness.ScenarioBot {
	t.Helper()
	const (
		classTinker            uint8  = 28
		spellBuildDestructoBot uint32 = 804673
		creatureDestructoBot   uint32 = 50300
	)
	bot := e2eharness.NewSolo(t, e2eharness.ScenarioOpts{Prefix: prefix, Race: e2eharness.RaceOrc, Class: classTinker, Level: 60})
	bot.TeleportPad(t, e2eharness.PackagePad(t))
	bot.Learn(t, spellBuildDestructoBot)
	bot.CheatPower(t)
	bot.CombatReady(t)

	bot.CastMust(t, spellBuildDestructoBot, 0, 5*time.Second)
	robot := bot.WaitUnitAny(t, 7*time.Second, creatureDestructoBot)
	if robot == 0 {
		t.Fatalf("precondition: no Destructo-Bot appeared")
	}
	deadline := time.Now().Add(5 * time.Second)
	for charmed(bot) != robot {
		if time.Now().After(deadline) {
			t.Fatalf("precondition: the player does not possess the Destructo-Bot (charm=%#x)", charmed(bot))
		}
		time.Sleep(50 * time.Millisecond)
	}
	return bot
}

// charmed returns the GUID in the player's UNIT_FIELD_CHARM.
func charmed(bot *e2eharness.ScenarioBot) uint64 {
	self := bot.World.GetObject(bot.World.CharGUID())
	if self == nil {
		return 0
	}
	return self.GUIDField(client.UnitFieldCharm)
}

// countDamageLogs counts SMSG_SPELLNONMELEEDAMAGELOG packets for spellID cast by the player.
func countDamageLogs(bot *e2eharness.ScenarioBot, spellID uint32, count *atomic.Int32) (cancel func()) {
	self := bot.World.CharGUID()
	return bot.World.AddPacketHook(func(opcode uint16, data []byte) {
		if opcode != smsgSpellNonMeleeDamageLog {
			return
		}
		r := bytes.NewReader(data)
		readPackedGUID(r) // target
		if readPackedGUID(r) != self {
			return
		}
		var id uint32
		if binary.Read(r, binary.LittleEndian, &id) == nil && id == spellID {
			count.Add(1)
		}
	})
}

func readPackedGUID(r *bytes.Reader) uint64 {
	mask, err := r.ReadByte()
	if err != nil {
		return 0
	}
	var guid uint64
	for i := uint8(0); i < 8; i++ {
		if mask&(1<<i) != 0 {
			b, err := r.ReadByte()
			if err != nil {
				return 0
			}
			guid |= uint64(b) << (i * 8)
		}
	}
	return guid
}
