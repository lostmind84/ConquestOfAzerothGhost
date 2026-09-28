//go:build e2e

// Package wisdomball_test reproduces the "Wondrous Wisdomball" reports of the CoA server issue tracker.
//
//	go test -tags=e2e ./e2e/coa/wisdomball -count=1 -v -p 1
package wisdomball_test

import (
	"bytes"
	"encoding/binary"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"github.com/azerothcore/AzerothGhost/client"
	"github.com/azerothcore/AzerothGhost/e2e/e2eharness"
)

const (
	classFelsworn uint8 = 14
	classReaper   uint8 = 30

	itemWondrousWisdomball uint32 = 101169
	spellSummonWisdomball  uint32 = 83050
	creatureWisdomball     uint32 = 79025

	mapBlackrockDepths uint32 = 230

	// The Heart of the Mountain: unrestricted by race or class, MinLevel 50, part of the
	// Blackrock Depths quest sort the ball reconstructs for that dungeon.
	questHeartOfTheMountain uint32 = 4123

	castTimeout = 5 * time.Second
	settle      = time.Second
)

// wisdomballPad is the Blackrock Depths spot the coa-gameplay-test wisdomball-dungeon-quests
// scenario summons the ball at.
var wisdomballPad = e2eharness.Position3{X: 912.52, Y: -185.856, Z: -43.6204, Map: mapBlackrockDepths}

// findCreatureEntry returns the GUID of a nearby, alive creature with the given entry, or 0.
func findCreatureEntry(bot *e2eharness.ScenarioBot, entry uint32, maxDist float32) uint64 {
	for _, u := range bot.World.GetNearbyUnits(maxDist) {
		if u == nil || u.IsPlayer || !u.IsAlive() {
			continue
		}
		if u.Value(client.UnitFieldEntry) == entry {
			return u.GUID
		}
	}
	return 0
}

// waitCreatureEntry polls findCreatureEntry until it finds the creature or the timeout ends.
func waitCreatureEntry(t *testing.T, bot *e2eharness.ScenarioBot, entry uint32, maxDist float32, timeout time.Duration) uint64 {
	t.Helper()
	for deadline := time.Now().Add(timeout); time.Now().Before(deadline); time.Sleep(200 * time.Millisecond) {
		if guid := findCreatureEntry(bot, entry, maxDist); guid != 0 {
			return guid
		}
	}
	t.Fatalf("precondition: creature %d never appeared within %s", entry, timeout)
	return 0
}

// TestWisdomball_AcceptSharesQuestWithGroup covers issue #5502: accepting a quest from the
// Wondrous Wisdomball should share it with the group, exactly as CMSG_PUSHQUESTTOPARTY would,
// so a grouped, eligible member gets the quest details window with an Accept button.
func TestWisdomball_AcceptSharesQuestWithGroup(t *testing.T) {
	bots := e2eharness.NewScenario(t, e2eharness.ScenarioOpts{
		Prefix: "WisShr",
		Bots: []e2eharness.BotSpec{
			{Role: "seeker", Race: e2eharness.RaceBloodElf, Class: classFelsworn, Level: 60},
			{Role: "bystander", Race: e2eharness.RaceOrc, Class: classReaper, Level: 60},
		},
	})
	seeker, bystander := bots[0], bots[1]
	// Group before entering the dungeon: teleporting solo bots to the same coordinates would
	// each create their own instance of Blackrock Depths. Grouping first makes the bystander's
	// later teleport join the seeker's instance.
	e2eharness.FormParty(t, seeker, bystander)
	e2eharness.TeleportAllPad(t, []*e2eharness.ScenarioBot{seeker, bystander}, wisdomballPad)

	seeker.AddAndUseItem(t, itemWondrousWisdomball, 0)
	if _, err := seeker.TryCast(t, spellSummonWisdomball, 0, castTimeout); err != nil {
		t.Fatalf("cast %d (summon the ball): %v", spellSummonWisdomball, err)
	}
	time.Sleep(settle)

	ball := waitCreatureEntry(t, seeker, creatureWisdomball, 40, 5*time.Second)

	seeker.GM(t, ".gm off")
	bystander.GM(t, ".gm off")

	var (
		gotDetails = make(chan uint32, 1)
	)
	cancel := bystander.World.AddPacketHook(func(opcode uint16, data []byte) {
		if opcode != client.SmsgQuestgiverQuestDetails {
			return
		}
		r := bytes.NewReader(data)
		var guid, informer uint64
		var questID uint32
		_ = binary.Read(r, binary.LittleEndian, &guid)
		_ = binary.Read(r, binary.LittleEndian, &informer)
		_ = binary.Read(r, binary.LittleEndian, &questID)
		select {
		case gotDetails <- questID:
		default:
		}
	})
	defer cancel()

	if err := seeker.World.QuestgiverAcceptQuest(ball, questHeartOfTheMountain); err != nil {
		t.Fatalf("accept quest %d at the ball: %v", questHeartOfTheMountain, err)
	}

	select {
	case questID := <-gotDetails:
		if questID != questHeartOfTheMountain {
			t.Fatalf("E2E_FAIL: bystander got quest details for %d, want %d (#5502)", questID, questHeartOfTheMountain)
		}
		t.Logf("bystander received SMSG_QUESTGIVER_QUEST_DETAILS for quest %d", questID)
	case <-time.After(8 * time.Second):
		t.Fatalf("E2E_FAIL: bystander never received SMSG_QUESTGIVER_QUEST_DETAILS for quest %d after seeker accepted it from the ball (#5502)", questHeartOfTheMountain)
	}
}
