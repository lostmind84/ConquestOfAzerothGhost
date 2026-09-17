//go:build e2e

package systems_test

import (
	"encoding/binary"
	"sync"
	"testing"
	"time"

	"github.com/azerothcore/AzerothGhost/client"
	"github.com/azerothcore/AzerothGhost/e2e/e2eharness"
)

// Main project issue #95: the Random Dungeon Finder does not work.
// Five solo bots (one tank, one healer, three damage) queue for Random Classic Dungeon, accept the proposal and
// must be teleported into the dungeon.
//
//	go test -tags=e2e ./e2e/coa/systems -run RandomDungeon -count=1 -v
func TestLFG_RandomDungeon(t *testing.T) {
	const (
		dungeonRandomClassic uint32 = 258
		lfgTypeRandom        uint32 = 6
		roleTank             uint32 = 2
		roleHealer           uint32 = 4
		roleDamage           uint32 = 8
	)
	bots := e2eharness.NewScenario(t, e2eharness.ScenarioOpts{Prefix: "Rdf", Count: 5, Race: e2eharness.RaceOrc, Class: classKnightXoroth, Level: 20})
	roles := []uint32{roleTank, roleHealer, roleDamage, roleDamage, roleDamage}

	type joinResult struct{ result, state uint32 }
	var mu sync.Mutex
	joins := map[int]joinResult{}
	proposals := map[int]uint32{}
	var cancels []func()
	defer func() {
		for _, c := range cancels {
			c()
		}
	}()
	for i, bot := range bots {
		i := i
		bot.GM(t, ".gm off")
		cancels = append(cancels, bot.World.AddPacketHook(func(op uint16, data []byte) {
			mu.Lock()
			defer mu.Unlock()
			switch {
			case op == client.SmsgLfgJoinResult && len(data) >= 8:
				joins[i] = joinResult{binary.LittleEndian.Uint32(data[0:4]), binary.LittleEndian.Uint32(data[4:8])}
			case op == client.SmsgLfgProposalUpdate && len(data) >= 9:
				proposals[i] = binary.LittleEndian.Uint32(data[5:9])
			}
		}))
	}
	for i, bot := range bots {
		slot := dungeonRandomClassic | lfgTypeRandom<<24
		if err := bot.World.LfgJoin(roles[i], []uint32{slot}, ""); err != nil {
			t.Fatalf("join: %v", err)
		}
	}
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		n := len(proposals)
		mu.Unlock()
		if n == len(bots) {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	mu.Lock()
	t.Logf("join results: %+v, proposals: %+v", joins, proposals)
	for i := range bots {
		if j, ok := joins[i]; !ok || j.result != 0 {
			mu.Unlock()
			t.Fatalf("E2E_FAIL: bot %d join result %+v (present=%v) (#95)", i, j, ok)
		}
	}
	if len(proposals) != len(bots) {
		mu.Unlock()
		t.Fatalf("E2E_FAIL: %d of %d bots received a dungeon proposal within 60 s (#95)", len(proposals), len(bots))
	}
	ids := map[int]uint32{}
	for k, v := range proposals {
		ids[k] = v
	}
	mu.Unlock()
	for i, bot := range bots {
		if err := bot.World.LfgProposalResult(ids[i], true); err != nil {
			t.Fatalf("accept: %v", err)
		}
	}
	for deadline := time.Now().Add(30 * time.Second); time.Now().Before(deadline); time.Sleep(500 * time.Millisecond) {
		inside := 0
		for _, bot := range bots {
			if _, _, _, m := bot.Pos(); m != 1 && m != 0 && m != 530 {
				inside++
			}
		}
		if inside == len(bots) {
			_, _, _, m := bots[0].Pos()
			t.Logf("E2E_PASS: all bots teleported into dungeon map %d (#95)", m)
			return
		}
	}
	for i, bot := range bots {
		_, _, _, m := bot.Pos()
		t.Logf("bot %d on map %d", i, m)
	}
	t.Fatalf("E2E_FAIL: the accepted proposal did not teleport every bot into the dungeon (#95)")
}
