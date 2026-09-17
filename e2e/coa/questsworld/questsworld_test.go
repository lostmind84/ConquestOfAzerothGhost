//go:build e2e

package questsworld_test

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/azerothcore/AzerothGhost/client"
	"github.com/azerothcore/AzerothGhost/e2e/e2eharness"
)

const (
	classFelsworn uint8 = 14

	creatureMottledBoar uint32 = 3098
)

// Main project issue #1504: a Blood Elf Felsworn learns the Stormwind and Ironforge Fel Rifts instead of the Horde ones.
// SkillLineAbility.dbc gives Stormwind/Ironforge/Darnassus RaceMask 1101 (Alliance) and Orgrimmar/Thunder Bluff/
// Undercity RaceMask 690 (Horde), at Spell.dbc levels 26, 30 and 36.
//
//	go test -tags=e2e ./e2e/coa/questsworld -run FelRiftFaction -count=1 -v
func TestFelsworn_FelRiftFaction(t *testing.T) {
	type rift struct {
		spell uint32
		name  string
		horde bool
	}
	rifts := []rift{
		{535595, "Stormwind", false}, {535596, "Ironforge", false}, {535597, "Darnassus", false},
		{535598, "Orgrimmar", true}, {535599, "Thunder Bluff", true}, {535600, "Undercity", true},
	}
	for _, tc := range []struct {
		prefix string
		race   uint8
		horde  bool
	}{
		{"FsRifH", e2eharness.RaceBloodElf, true},
		{"FsRifA", e2eharness.RaceHuman, false},
	} {
		bot := newBot(t, tc.prefix, tc.race, classFelsworn, 40)
		time.Sleep(settle)
		failed := false
		for _, r := range rifts {
			knows := bot.World.KnowsSpell(r.spell)
			t.Logf("race %d: Fel Rift: %s (%d) known %v", tc.race, r.name, r.spell, knows)
			if knows != (r.horde == tc.horde) {
				failed = true
			}
		}
		if failed {
			t.Errorf("E2E_FAIL: race %d level 40 Felsworn does not own exactly its faction's three capital Fel Rifts (#1504)", tc.race)
			continue
		}
		t.Logf("E2E_PASS: race %d owns only its faction's capital Fel Rifts (#1504)", tc.race)
	}
}

// Main project issue #1504, existing characters: a Horde Felsworn who already learned the Alliance capital rifts loses
// them and gains the Horde ones at the next login.
//
//	go test -tags=e2e ./e2e/coa/questsworld -run FelRiftExistingCharacter -count=1 -v
func TestFelsworn_FelRiftExistingCharacter(t *testing.T) {
	bot := newBot(t, "FsRifX", e2eharness.RaceBloodElf, classFelsworn, 40)
	for _, spell := range []uint32{535595, 535596, 535597} {
		bot.GM(t, fmt.Sprintf(".learn %d", spell))
	}
	bot.FlushWorld(t)
	bot.Relog(t)
	time.Sleep(settle)
	for _, spell := range []uint32{535595, 535596, 535597} {
		if bot.World.KnowsSpell(spell) {
			t.Fatalf("E2E_FAIL: a Blood Elf Felsworn kept the Alliance Fel Rift %d after logging in again (#1504)", spell)
		}
	}
	for _, spell := range []uint32{535598, 535599, 535600} {
		if !bot.World.KnowsSpell(spell) {
			t.Fatalf("E2E_FAIL: a level 40 Blood Elf Felsworn lacks the Horde Fel Rift %d after logging in again (#1504)", spell)
		}
	}
	t.Logf("E2E_PASS: the Alliance rifts were removed and the Horde rifts granted at login (#1504)")
}

// Main project issue #1419: a creature scaled up for a higher-level player keeps that level once the player leaves.
//
//	go test -tags=e2e ./e2e/coa/questsworld -run LevelScalingScalesBack -count=1 -v
func TestLevelScaling_ScalesBack(t *testing.T) {
	low := newBot(t, "LsLow", e2eharness.RaceHuman, 1, 1)
	high := newBot(t, "LsHigh", e2eharness.RaceHuman, 1, 25)
	boar := low.Spawn(t, creatureMottledBoar, 10*time.Second)
	original := unitLevel(low, boar)
	x, y, z, m := low.Pos()
	low.CombatReady(t)
	high.Teleport(t, x+2, y, z, m)
	high.CombatReady(t)
	scaled := waitUnitLevel(low, boar, 10*time.Second, func(l uint32) bool { return l >= 22 })
	t.Logf("boar level %d alone, %d with a level 25 player in range", original, scaled)
	if scaled < 22 {
		t.Fatalf("precondition: the boar did not scale to the level 25 player (level %d)", scaled)
	}
	high.TeleportPad(t, e2eharness.PackagePad(t))
	high.Teleport(t, x+500, y, z+50, m)
	back := waitUnitLevel(low, boar, 10*time.Second, func(l uint32) bool { return l < 22 })
	if back >= 22 {
		t.Fatalf("E2E_FAIL: the boar stayed level %d after the level 25 player left (#1419)", back)
	}
	t.Logf("E2E_PASS: the boar returned to level %d once the level 25 player left (#1419)", back)
}

// Main project issue #1444: creatures in Ragefire Chasm do not scale to the players inside.
//
//	go test -tags=e2e ./e2e/coa/questsworld -run LevelScalingDungeon -count=1 -v
func TestLevelScaling_Dungeon(t *testing.T) {
	const mapRagefire = 389
	bot := newBot(t, "LsRfc", e2eharness.RaceOrc, 1, 40)
	db, err := e2eharness.OpenWorldDB()
	if err != nil {
		t.Fatalf("world db: %v", err)
	}
	defer db.Close()
	var entry, minLevel uint32
	var x, y, z float32
	if err := db.QueryRow("SELECT c.id, t.minlevel, c.position_x, c.position_y, c.position_z FROM creature c "+
		"JOIN creature_template t ON t.entry = c.id WHERE c.map = ? AND t.minlevel < 30 ORDER BY c.guid LIMIT 1", mapRagefire).
		Scan(&entry, &minLevel, &x, &y, &z); err != nil {
		t.Fatalf("precondition: no Ragefire Chasm creature: %v", err)
	}
	// Stand out of aggro range: scaling only runs while the creature is out of combat.
	bot.Teleport(t, x+30, y, z, mapRagefire)
	if _, _, _, m := bot.Pos(); m != mapRagefire {
		t.Fatalf("precondition: not in Ragefire Chasm (map %d)", m)
	}
	bot.CombatReady(t)
	guid := bot.WaitUnit(t, entry, 10*time.Second)
	level := waitUnitLevel(bot, guid, 10*time.Second, func(l uint32) bool { return l >= 37 })
	t.Logf("creature %d (template min level %d, in combat %v) level %d with a level 40 player 30 yards away",
		entry, minLevel, bot.UnitInCombat(guid), level)
	if level < 37 {
		t.Fatalf("E2E_FAIL: Ragefire Chasm creature %d stayed level %d next to a level 40 player (#1444)", entry, level)
	}
	t.Logf("E2E_PASS: Ragefire Chasm creature scaled to level %d (#1444)", level)
}

// Main project issue #409: every quest is grey. Quest level scaling reports a lower quest at the player's level.
//
//	go test -tags=e2e ./e2e/coa/questsworld -run QuestLevelGrey -count=1 -v
func TestQuestLevel_NotGrey(t *testing.T) {
	const marshalDughan uint32 = 240
	bot := newBot(t, "QlGrey", e2eharness.RaceHuman, 1, 21)
	bot.GoCreatureID(t, marshalDughan)
	giver := bot.WaitUnit(t, marshalDughan, 10*time.Second)
	bot.CombatReady(t)
	quests := questgiverQuests(t, bot, giver)
	t.Logf("Marshal Dughan offers:%s", describeQuests(quests))
	if len(quests) == 0 {
		t.Fatalf("precondition: Marshal Dughan offered nothing to a level 21 Human")
	}
	for _, q := range quests {
		if q.level >= 0 && q.level < 21 {
			t.Fatalf("E2E_FAIL: quest %d reported at level %d to a level 21 player, the client shows it grey (#409)", q.id, q.level)
		}
	}
	t.Logf("E2E_PASS: offered quests are reported at the player's level or above (#409)")
}

// Main project issue #142: Darsok Swiftdagger's quest cannot be started.
//
//	go test -tags=e2e ./e2e/coa/questsworld -run DarsokQuests -count=1 -v
func TestQuestgiver_DarsokSwiftdagger(t *testing.T) {
	const darsok uint32 = 3449
	bot := newBot(t, "QgDars", e2eharness.RaceOrc, 1, 15)
	bot.GoCreatureID(t, darsok)
	giver := bot.WaitUnit(t, darsok, 10*time.Second)
	bot.CombatReady(t)
	quests := questgiverQuests(t, bot, giver)
	t.Logf("Darsok Swiftdagger offers:%s", describeQuests(quests))
	if !hasQuest(quests, 867) {
		t.Fatalf("E2E_FAIL: Darsok Swiftdagger does not offer Harpy Raiders (867) to a level 15 Orc (#142)")
	}
	t.Logf("E2E_PASS: Darsok Swiftdagger offers Harpy Raiders (#142)")
}

// Main project issue #1488: Warning Fairbreeze Village is not offered after the Amani quests.
// quest_template_addon: 9363 requires 9360 Amani Invasion, whose ender Lieutenant Dawnrunner (15399) also starts 9363.
//
//	go test -tags=e2e ./e2e/coa/questsworld -run WarningFairbreeze -count=1 -v
func TestQuestgiver_WarningFairbreezeVillage(t *testing.T) {
	const dawnrunner uint32 = 15399
	bot := newBot(t, "QgFair", e2eharness.RaceBloodElf, 1, 12)
	// .quest add refuses item-started quests such as 9360, so mark both rewarded in the database and log in again.
	db, err := e2eharness.OpenCharDB()
	if err != nil {
		t.Fatalf("char db: %v", err)
	}
	defer db.Close()
	for _, quest := range []uint32{8476, 9360} {
		if _, err := db.Exec("REPLACE INTO character_queststatus_rewarded (guid, quest, active) VALUES (?, ?, 1)",
			bot.GUID&0xFFFFFFFF, quest); err != nil {
			t.Fatalf("precondition: reward quest %d: %v", quest, err)
		}
	}
	bot.Relog(t)
	var rewarded int
	_ = db.QueryRow("SELECT COUNT(*) FROM character_queststatus_rewarded WHERE guid = ? AND quest IN (8476, 9360)",
		bot.GUID&0xFFFFFFFF).Scan(&rewarded)
	t.Logf("rewarded Amani quests: %d of 2", rewarded)
	bot.GoCreatureID(t, dawnrunner)
	giver := bot.WaitUnit(t, dawnrunner, 10*time.Second)
	bot.CombatReady(t)
	quests := questgiverQuests(t, bot, giver)
	t.Logf("Lieutenant Dawnrunner offers:%s", describeQuests(quests))
	if !hasQuest(quests, 9363) {
		t.Fatalf("E2E_FAIL: Lieutenant Dawnrunner does not offer Warning Fairbreeze Village (9363) after Amani Invasion (#1488)")
	}
	t.Logf("E2E_PASS: Warning Fairbreeze Village is offered after Amani Invasion (#1488)")
}

// Main project issue #234: the "Investigate the Amani Catacombs" objective never completes.
// Quest 9193 needs area trigger 4071 (areatrigger_involvedrelation), placed inside the catacombs.
//
//	go test -tags=e2e ./e2e/coa/questsworld -run AmaniCatacombs -count=1 -v
func TestQuest_InvestigateAmaniCatacombs(t *testing.T) {
	const (
		quest   = 9193
		trigger = 4071
	)
	bot := newBot(t, "QtAmani", e2eharness.RaceBloodElf, 1, 17)
	bot.GM(t, fmt.Sprintf(".quest add %d", quest))
	bot.Teleport(t, 7567.67, -7359.47, 161.738, 530)
	bot.CombatReady(t)
	if err := bot.World.SendAreaTrigger(trigger); err != nil {
		t.Fatalf("area trigger: %v", err)
	}
	time.Sleep(settle)
	bot.Save(t)
	db, err := e2eharness.OpenCharDB()
	if err != nil {
		t.Fatalf("char db: %v", err)
	}
	defer db.Close()
	var explored int
	if err := db.QueryRow("SELECT explored FROM character_queststatus WHERE guid = ? AND quest = ?", bot.GUID&0xFFFFFFFF, quest).
		Scan(&explored); err != nil {
		t.Fatalf("precondition: quest %d not in the log: %v", quest, err)
	}
	if explored == 0 {
		t.Fatalf("E2E_FAIL: area trigger %d at its own position did not mark the catacombs explored (#234)", trigger)
	}
	t.Logf("E2E_PASS: area trigger %d marks Investigate the Amani Catacombs explored (#234)", trigger)
}

// Main project issue #1497: creatures killed in water cannot be looted.
// A Darkshore Thresher's loot can be empty, so kill several in the water and on land and compare.
//
//	go test -tags=e2e ./e2e/coa/questsworld -run LootInWater -count=1 -v
func TestLoot_InWater(t *testing.T) {
	const (
		darkshoreThresher uint32 = 2185
		kills                    = 5
	)
	bot := newBot(t, "LtWater", e2eharness.RaceHuman, 1, 12)
	looted := func(label string, x, y, z float32, mapID uint32) int {
		n := 0
		for i := 0; i < kills; i++ {
			bot.Teleport(t, x, y, z, mapID)
			guid := bot.SpawnKillLootable(t, darkshoreThresher, 45*time.Second)
			if items, ok := bot.TryOpenLoot(t, guid, 3*time.Second); ok {
				n++
				t.Logf("%s kill %d: looted %d items", label, i+1, len(items))
				bot.LootRelease(t, guid)
			}
		}
		return n
	}
	// The reporter's position: Darkshore coast, in the water.
	water := looted("water", 6683.02, 542.159, 1.35894, 1)
	pad := e2eharness.PackagePad(t)
	land := looted("land", pad.X, pad.Y, pad.Z, pad.Map)
	t.Logf("lootable corpses: %d of %d in the water, %d of %d on land", water, kills, land, kills)
	if land == 0 {
		t.Fatalf("precondition: no lootable corpse on land either")
	}
	if water == 0 {
		t.Fatalf("E2E_FAIL: no corpse killed in the water could be looted (#1497)")
	}
	t.Logf("E2E_PASS: creatures killed in the water can be looted (#1497)")
}

// Main project issue #428: Baron Longshore does not spawn at the Merchant Coast.
// Pool 144 holds his three spawns with max_limit 1.
//
//	go test -tags=e2e ./e2e/coa/questsworld -run BaronLongshore -count=1 -v
func TestSpawn_BaronLongshore(t *testing.T) {
	const longshore uint32 = 3467
	bot := newBot(t, "SpLong", e2eharness.RaceOrc, 1, 20)
	points := [][3]float32{{-1571.79, -3884.28, 16.2173}, {-1748.25, -3721.78, 16.2173}, {-1706.97, -3818.9, 13.1778}}
	for _, p := range points {
		bot.Teleport(t, p[0], p[1], p[2], 1)
		time.Sleep(2 * settle)
		if guid := bot.FindUnit(longshore, 60); guid != 0 {
			t.Logf("E2E_PASS: Baron Longshore is spawned near %.0f, %.0f (#428)", p[0], p[1])
			return
		}
	}
	t.Fatalf("E2E_FAIL: Baron Longshore is at none of his three pool spawn points (#428)")
}

// Main project issue #1474: Whitebark's Spirit sometimes stays hostile instead of turning friendly near 25% health.
// Its SmartAI sets an invincibility floor of 25% of its maximum health when summoned, then turns friendly
// (faction 35) between 24% and 26% health. Level scaling raises its maximum health after the summon.
//
//	go test -tags=e2e ./e2e/coa/questsworld -run WhitebarkSpirit -count=1 -v
func TestQuest_WhitebarkSpiritTurnsFriendly(t *testing.T) {
	const (
		whitebark        uint32 = 19456
		factionFriendly         = 35
		unitFieldFaction uint16 = 0x0037
	)
	for _, tc := range []struct {
		prefix string
		level  int
	}{
		{"WbLow", 10},  // no scaling: template level 10
		{"WbHigh", 30}, // scaled to 27 after the summon
	} {
		bot := newBot(t, tc.prefix, e2eharness.RaceBloodElf, 1, tc.level)
		x, y, z, m := bot.Pos()
		var spirit uint64
		var summonMax, level uint32
		// Scaling runs once a second and only out of combat, so a summon does not always scale before the fight.
		// Summon again until the level 30 case sees the scaled spirit.
		for attempt := 1; attempt <= 5; attempt++ {
			bot.Teleport(t, x, y, z, m)
			bot.GM(t, ".gm on")
			bot.GM(t, fmt.Sprintf(".npc add temp %d", whitebark))
			spirit = 0
			for deadline := time.Now().Add(10 * time.Second); spirit == 0 && time.Now().Before(deadline); time.Sleep(100 * time.Millisecond) {
				for _, u := range bot.UnitsByEntry(10, whitebark) {
					if u.MaxHealth == u.Health && unitLevel(bot, u.GUID) == 10 {
						spirit = u.GUID
					}
				}
			}
			if spirit == 0 {
				t.Fatalf("precondition: Whitebark's Spirit not summoned")
			}
			_, summonMax = bot.UnitHP(spirit)
			// Stay out of its aggro range while it can scale.
			bot.Teleport(t, x+35, y, z, m)
			bot.CombatReady(t)
			level = waitUnitLevel(bot, spirit, 3*time.Second, func(l uint32) bool { return int(l) >= tc.level-3 })
			if int(level) >= tc.level-3 {
				break
			}
			_ = bot.World.SetTarget(spirit)
			bot.GM(t, ".die")
		}
		if int(level) < tc.level-3 {
			t.Fatalf("precondition: Whitebark's Spirit never scaled to a level %d player (level %d)", tc.level, level)
		}
		_, maxHP := bot.UnitHP(spirit)
		// One hit worth 90% of its health, as a group burst would do.
		bot.Damage(t, spirit, maxHP*9/10)
		var hp, faction uint32
		for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(200 * time.Millisecond) {
			hp, _ = bot.UnitHP(spirit)
			if o := bot.World.GetObject(spirit); o != nil {
				faction = o.Value(unitFieldFaction)
			}
			if faction == factionFriendly {
				break
			}
		}
		t.Logf("level %d player: spirit level %d, max health %d at summon and %d now, health %d (%d%%), faction %d",
			tc.level, level, summonMax, maxHP, hp, uint64(hp)*100/uint64(max(maxHP, 1)), faction)
		if faction != factionFriendly {
			t.Errorf("E2E_FAIL: Whitebark's Spirit stayed hostile at %d%% health with a level %d player (#1474)",
				uint64(hp)*100/uint64(max(maxHP, 1)), tc.level)
			continue
		}
		t.Logf("E2E_PASS: Whitebark's Spirit turned friendly with a level %d player (#1474)", tc.level)
	}
}

// Main project issue #1496: during Escape from the Catacombs, Ranger Lilatha stops after "I can see the light at the
// end of the tunnel!" (script waypoint 18, where she summons a Mummified Headhunter and a Shadowpine Oracle).
// The bot follows her by teleporting and kills the summons, as an escorting player would.
//
//	go test -tags=e2e ./e2e/coa/questsworld -run LilathaEscort -count=1 -v
func TestQuest_RangerLilathaEscort(t *testing.T) {
	const (
		lilatha            uint32 = 16295
		captainHelios      uint32 = 16220
		quest              uint32 = 9212
		headhunter         uint32 = 16342
		oracle             uint32 = 16343
		smsgQuestupdateCmp        = 0x0198 // SMSG_QUESTUPDATE_COMPLETE
		stallLimit                = 30 * time.Second
	)
	bot := newBot(t, "EsLila", e2eharness.RaceBloodElf, 1, 20)
	bot.GoCreatureID(t, lilatha)
	// A failed escort despawns her for her 300 s respawn time.
	bot.GM(t, ".respawn")
	var ranger uint64
	for deadline := time.Now().Add(6 * time.Minute); ranger == 0 && time.Now().Before(deadline); time.Sleep(5 * time.Second) {
		ranger = bot.FindUnit(lilatha, 20)
	}
	if ranger == 0 {
		t.Fatalf("precondition: Ranger Lilatha did not respawn")
	}
	bot.CombatReady(t)
	complete := make(chan struct{}, 1)
	cancel := bot.World.AddPacketHook(func(opcode uint16, data []byte) {
		if opcode == smsgQuestupdateCmp && len(data) >= 4 && binary.LittleEndian.Uint32(data) == quest {
			select {
			case complete <- struct{}{}:
			default:
			}
		}
	})
	defer cancel()
	if err := bot.World.QuestgiverAcceptQuest(ranger, quest); err != nil {
		t.Fatalf("accept: %v", err)
	}
	var lastX, lastY float32
	lastMove := time.Now()
	killed := 0
	for deadline := time.Now().Add(5 * time.Minute); time.Now().Before(deadline); time.Sleep(time.Second) {
		select {
		case <-complete:
			t.Logf("E2E_PASS: Escape from the Catacombs completed, %d Mummified Headhunter or Shadowpine Oracle hits (#1496)", killed)
			return
		default:
		}
		o := bot.World.GetObject(ranger)
		if o == nil {
			t.Fatalf("E2E_FAIL: Ranger Lilatha is no longer visible before the escort completed (#1496)")
		}
		x, y, z := o.PosX, o.PosY, o.PosZ
		if e2eharness.Distance3D(x, y, 0, lastX, lastY, 0) > 1 {
			lastX, lastY, lastMove = x, y, time.Now()
		}
		if hp, _ := bot.UnitHP(ranger); hp == 0 {
			t.Fatalf("precondition: Ranger Lilatha died at %.1f, %.1f, %.1f before the escort completed", x, y, z)
		}
		// Kill every creature near her, as an escorting player would: the catacombs' own mobs attack her too.
		for _, u := range bot.World.GetNearbyUnits(30) {
			entry := u.Entry
			if u.Health() == 0 || u.GUID>>52 != 0xF13 || entry == lilatha || entry == captainHelios || entry == 0 {
				continue
			}
			if o := bot.World.GetObject(u.GUID); o != nil && e2eharness.Distance3D(o.PosX, o.PosY, o.PosZ, x, y, z) > 25 {
				continue
			}
			bot.Damage(t, u.GUID, 1_000_000)
			if entry == headhunter || entry == oracle {
				killed++
			}
		}
		if time.Since(lastMove) > stallLimit && !bot.UnitInCombat(ranger) {
			_ = bot.World.SetTarget(ranger)
			first := gmOutput(t, bot, ".gps")
			time.Sleep(3 * time.Second)
			t.Logf("server position of Ranger Lilatha, 3 s apart:\n%s\n%s\nnpc info: %s", first,
				gmOutput(t, bot, ".gps"), gmOutput(t, bot, ".npc info"))
			t.Fatalf("E2E_FAIL: Ranger Lilatha stood still out of combat for %s at %.1f, %.1f, %.1f (#1496)", stallLimit, x, y, z)
		}
		bx, by, _, _ := bot.Pos()
		if e2eharness.Distance3D(bx, by, 0, x, y, 0) > 8 {
			bot.Teleport(t, x-3, y, z+1, 530)
		}
	}
	t.Fatalf("E2E_FAIL: Escape from the Catacombs not completed within 5 minutes (#1496)")
}

// Main project issue #1514: the four Scarlet Monastery Graveyard rares were not there.
// Each has one spawn on map 189: Fallen Champion 6488, Ironspine 6489, Azshir the Sleepless 6490.
//
//	go test -tags=e2e ./e2e/coa/questsworld -run GraveyardRares -count=1 -v
func TestSpawn_ScarletMonasteryGraveyardRares(t *testing.T) {
	bot := newBot(t, "SpSmgy", e2eharness.RaceUndead, 1, 40)
	db, err := e2eharness.OpenWorldDB()
	if err != nil {
		t.Fatalf("world db: %v", err)
	}
	defer db.Close()
	rows, err := db.Query("SELECT id, position_x, position_y, position_z FROM creature WHERE map = 189 AND id IN (6488, 6489, 6490)")
	if err != nil {
		t.Fatalf("world db: %v", err)
	}
	type spawn struct {
		entry   uint32
		x, y, z float32
	}
	var spawns []spawn
	for rows.Next() {
		var s spawn
		if err := rows.Scan(&s.entry, &s.x, &s.y, &s.z); err != nil {
			t.Fatalf("world db: %v", err)
		}
		spawns = append(spawns, s)
	}
	rows.Close()
	t.Logf("graveyard rare spawns in the world database: %d", len(spawns))
	missing := 0
	for _, s := range spawns {
		bot.Teleport(t, s.x, s.y, s.z+2, 189)
		for _, u := range bot.World.GetNearbyUnits(30) {
			if u.Entry == s.entry {
				t.Logf("rare %d seen right after entering: health %d at %.1f, %.1f, %.1f", s.entry, u.Health(), u.PosX, u.PosY, u.PosZ)
			}
		}
		t.Logf("ground at the spawn: %s", gmOutput(t, bot, ".gps"))
		time.Sleep(settle)
		if bot.FindUnit(s.entry, 40) == 0 {
			_, _, _, m := bot.Pos()
			t.Logf("rare %d is not at its spawn %.0f, %.0f (bot map %d, %d units within 60 yards)", s.entry, s.x, s.y, m,
				len(bot.World.GetNearbyUnits(60)))
			missing++
			continue
		}
		t.Logf("rare %d is at its spawn", s.entry)
	}
	if missing > 0 || len(spawns) < 3 {
		t.Fatalf("E2E_FAIL: %d of %d Graveyard rare spawns are empty in a new instance (#1514)", missing, len(spawns))
	}
	t.Logf("E2E_PASS: every Graveyard rare spawn is filled in a new instance (#1514)")
}

// Main project issue #350: quest items do not always drop. Red Burlap Bandana (752) from Defias Thug (38) for
// Brotherhood of Thieves (18) drops every time on Ascension (db.exil.es export of 2026-09-13).
//
//	go test -tags=e2e ./e2e/coa/questsworld -run RedBurlapBandana -count=1 -v
func TestLoot_RedBurlapBandanaAlwaysDrops(t *testing.T) {
	const (
		defiasThug uint32 = 38
		bandana    uint32 = 752
		quest      uint32 = 18
		kills             = 8
	)
	bot := newBot(t, "LtBand", e2eharness.RaceHuman, 1, 5)
	bot.GM(t, fmt.Sprintf(".quest add %d", quest))
	bot.FlushWorld(t)
	x, y, z, m := bot.Pos()
	drops := 0
	for i := 0; i < kills; i++ {
		bot.Teleport(t, x, y, z, m)
		guid := bot.SpawnKillLootable(t, defiasThug, 45*time.Second)
		items, _ := bot.TryOpenLoot(t, guid, 3*time.Second)
		for _, item := range items {
			if item.ItemID == bandana {
				drops++
				break
			}
		}
		bot.LootRelease(t, guid)
	}
	t.Logf("Red Burlap Bandana dropped %d times in %d kills with the quest active", drops, kills)
	if drops != kills {
		t.Fatalf("E2E_FAIL: Red Burlap Bandana dropped %d of %d times (#350)", drops, kills)
	}
	t.Logf("E2E_PASS: Red Burlap Bandana dropped on every kill (#350)")
}

// Main project issue #1515: the Scarlet Monastery outside guards are not elite on this server.
// Scarlet Scout, Preserver and Sentry are rank 1 (elite) on Ascension (db.exil.es export of 2026-09-13).
//
//	go test -tags=e2e ./e2e/coa/questsworld -run ScarletOutsideElites -count=1 -v
func TestCreature_ScarletOutsideElites(t *testing.T) {
	bot := newBot(t, "CrScar", e2eharness.RaceUndead, 1, 30)
	ranks := map[uint32]uint32{}
	var mu sync.Mutex
	cancel := bot.World.AddPacketHook(func(opcode uint16, data []byte) {
		if opcode != client.SmsgCreatureQueryResponse || len(data) < 4 {
			return
		}
		r := bytes.NewReader(data)
		var entry uint32
		_ = binary.Read(r, binary.LittleEndian, &entry)
		for i := 0; i < 6; i++ { // name, three unused names, subname, icon name
			cString(r)
		}
		var typeFlags, creatureType, family, rank uint32
		_ = binary.Read(r, binary.LittleEndian, &typeFlags)
		_ = binary.Read(r, binary.LittleEndian, &creatureType)
		_ = binary.Read(r, binary.LittleEndian, &family)
		if binary.Read(r, binary.LittleEndian, &rank) == nil {
			mu.Lock()
			ranks[entry] = rank
			mu.Unlock()
		}
	})
	defer cancel()
	names := map[uint32]string{4280: "Scarlet Preserver", 4281: "Scarlet Scout", 4283: "Scarlet Sentry"}
	for entry := range names {
		if err := bot.World.CreatureQuery(entry, 0); err != nil {
			t.Fatalf("creature query: %v", err)
		}
	}
	time.Sleep(2 * settle)
	mu.Lock()
	defer mu.Unlock()
	failed := false
	for entry, name := range names {
		rank, ok := ranks[entry]
		t.Logf("%s (%d): rank %d (answered %v)", name, entry, rank, ok)
		if !ok {
			t.Fatalf("precondition: no creature query answer for %d", entry)
		}
		if rank != 1 {
			failed = true
		}
	}
	if failed {
		t.Fatalf("E2E_FAIL: the Scarlet Monastery outside guards are not all elite (#1515)")
	}
	t.Logf("E2E_PASS: Scarlet Preserver, Scout and Sentry are elite (#1515)")
}

// Main project issue #1400: Watch Commander Zalaphil, a rare, drops only white items. Its loot, shared with the
// Durotar rares Warlord Kolkanis and Geolord Mottle, drew one table among two white tables and one green table.
// Each kill should give a green (quality 2) item.
//
//	go test -tags=e2e ./e2e/coa/questsworld -run DurotarRaresGreen -count=1 -v
func TestLoot_DurotarRaresGreen(t *testing.T) {
	const kills = 4
	bot := newBot(t, "LtRare", e2eharness.RaceOrc, 1, 10)
	db, err := e2eharness.OpenWorldDB()
	if err != nil {
		t.Fatalf("world db: %v", err)
	}
	defer db.Close()
	x, y, z, m := bot.Pos()
	for _, rare := range []struct {
		entry uint32
		name  string
	}{{5809, "Watch Commander Zalaphil"}, {5808, "Warlord Kolkanis"}, {5826, "Geolord Mottle"}} {
		green := 0
		for i := 0; i < kills; i++ {
			bot.Teleport(t, x, y, z, m)
			guid := bot.SpawnKillLootable(t, rare.entry, 45*time.Second)
			items, _ := bot.TryOpenLoot(t, guid, 3*time.Second)
			for _, item := range items {
				var quality int
				if db.QueryRow("SELECT Quality FROM item_template WHERE entry = ?", item.ItemID).Scan(&quality) == nil &&
					quality == 2 {
					green++
					break
				}
			}
			bot.LootRelease(t, guid)
		}
		t.Logf("%s: a green item in %d of %d kills", rare.name, green, kills)
		if green != kills {
			t.Errorf("E2E_FAIL: %s gave a green item in %d of %d kills (#1400)", rare.name, green, kills)
		}
	}
}
