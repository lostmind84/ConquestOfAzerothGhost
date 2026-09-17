//go:build e2e

package systems_test

import (
	"regexp"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/azerothcore/AzerothGhost/client"
	"github.com/azerothcore/AzerothGhost/e2e/e2eharness"
)

const smsgMessageChat uint16 = 0x0096

var manastormStatus = regexp.MustCompile(`Manastorm: depth (\d+), completed (\d+)`)

// manastormState runs `.manastorm status` and returns the reported depth and highest completed depth.
func manastormState(t *testing.T, bot *e2eharness.ScenarioBot) (depth, completed int) {
	t.Helper()
	var mu sync.Mutex
	depth, completed = -1, -1
	cancel := bot.World.AddPacketHook(func(op uint16, data []byte) {
		if op != smsgMessageChat {
			return
		}
		if m := manastormStatus.FindSubmatch(data); m != nil {
			mu.Lock()
			depth, _ = strconv.Atoi(string(m[1]))
			completed, _ = strconv.Atoi(string(m[2]))
			mu.Unlock()
		}
	})
	defer cancel()
	bot.GM(t, ".manastorm status")
	for deadline := time.Now().Add(3 * time.Second); time.Now().Before(deadline); time.Sleep(100 * time.Millisecond) {
		mu.Lock()
		d, c := depth, completed
		mu.Unlock()
		if d >= 0 {
			return d, c
		}
	}
	t.Fatalf("precondition: no Manastorm status reply")
	return
}

// Main project issue #180: Manastorm only offers one level; completing it repeats the same level instead of
// progressing. The bot enters depth 1, kills every creature of the scene with GM commands, then asks for the next
// level the way the guide does.
//
//	go test -tags=e2e ./e2e/coa/systems -run Manastorm -count=1 -v
func TestManastorm_ProgressesAfterCompletion(t *testing.T) {
	for _, via := range []string{"GuideCommand", "Portal"} {
		t.Run(via, func(t *testing.T) { manastormProgress(t, via) })
	}
}

func manastormProgress(t *testing.T, via string) {
	const creaturePortal uint32 = 12999 // summoned at the boss position once the level is completed
	bot := e2eharness.NewSolo(t, e2eharness.ScenarioOpts{Prefix: "Mstorm", Race: e2eharness.RaceOrc, Class: classKnightXoroth, Level: 20})
	bot.TeleportPad(t, e2eharness.PackagePad(t))
	time.Sleep(2 * time.Second)
	_, _, _, startMap := bot.Pos()
	bot.GM(t, ".manastorm enter 1")
	for deadline := time.Now().Add(20 * time.Second); time.Now().Before(deadline); time.Sleep(500 * time.Millisecond) {
		if _, _, _, m := bot.Pos(); m != startMap {
			break
		}
	}
	if _, _, _, m := bot.Pos(); m == startMap {
		t.Fatalf("precondition: `.manastorm enter 1` did not teleport the bot")
	}
	time.Sleep(3 * time.Second)
	if d, c := manastormState(t, bot); d != 1 {
		t.Fatalf("precondition: depth %d after entering depth 1 (completed %d)", d, c)
	}
	bot.GM(t, ".manastorm start")
	time.Sleep(2 * time.Second)

	completed := -1
	for round := 0; round < 20 && completed < 1; round++ {
		for _, u := range bot.NearbyUnits(300) {
			if u.GUID == bot.GUID {
				continue
			}
			if hp, _ := bot.UnitHP(u.GUID); hp == 0 {
				continue
			}
			_ = bot.World.SetTarget(u.GUID)
			bot.GM(t, ".die")
			time.Sleep(150 * time.Millisecond)
		}
		time.Sleep(2 * time.Second)
		_, completed = manastormState(t, bot)
	}
	if completed < 1 {
		t.Fatalf("precondition: depth 1 not completed after killing the scene (completed %d)", completed)
	}
	t.Logf("depth 1 completed")
	time.Sleep(3 * time.Second)

	if via == "Portal" {
		var portal *client.WorldObject
		for _, u := range bot.NearbyUnits(300) {
			if u.Entry == creaturePortal {
				portal = u
			}
		}
		if portal == nil {
			t.Fatalf("precondition: no Manastorm portal (%d) after completion", creaturePortal)
		}
		_, _, _, m := bot.Pos()
		bot.Teleport(t, portal.PosX+10, portal.PosY, portal.PosZ, m) // the portal arms beyond 6 yards
		time.Sleep(2 * time.Second)
		bot.Teleport(t, portal.PosX, portal.PosY, portal.PosZ, m)
	} else {
		bot.GM(t, ".manastorm next")
	}
	depth := 0
	for deadline := time.Now().Add(30 * time.Second); time.Now().Before(deadline); time.Sleep(2 * time.Second) {
		if depth, _ = manastormState(t, bot); depth >= 2 {
			break
		}
	}
	if depth != 2 {
		t.Fatalf("E2E_FAIL: after completing depth 1 and moving on, depth is %d, want 2 via %s (#180)", depth, via)
	}
	t.Logf("E2E_PASS: Manastorm moved from depth 1 to depth 2 via %s (#180)", via)
	bot.GM(t, ".manastorm leave")
}
