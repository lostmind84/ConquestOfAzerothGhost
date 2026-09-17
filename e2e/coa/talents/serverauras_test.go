//go:build e2e

package talents_test

import (
	"regexp"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/azerothcore/AzerothGhost/e2e/e2eharness"
)

const smsgMessageChat uint16 = 0x0096

var (
	listedAura   = regexp.MustCompile(`id: (\d+) \|c`)
	listedAmount = regexp.MustCompile(`id: (\d+) eff: (\d) amount: (-?\d+)`)
)

// serverAuras lists the unit's auras with `.list auras`, which also shows passives the client gets no aura slot for,
// and returns each aura's effect amounts (effect index -> amount; empty for an aura without listed amounts).
func serverAuras(t *testing.T, bot *e2eharness.ScenarioBot, guid uint64) map[uint32]map[int]int32 {
	t.Helper()
	var mu sync.Mutex
	auras := map[uint32]map[int]int32{}
	cancel := bot.World.AddPacketHook(func(op uint16, data []byte) {
		if op != smsgMessageChat {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		for _, m := range listedAura.FindAllSubmatch(data, -1) {
			id, _ := strconv.ParseUint(string(m[1]), 10, 32)
			if auras[uint32(id)] == nil {
				auras[uint32(id)] = map[int]int32{}
			}
		}
		for _, m := range listedAmount.FindAllSubmatch(data, -1) {
			id, _ := strconv.ParseUint(string(m[1]), 10, 32)
			eff, _ := strconv.Atoi(string(m[2]))
			amount, _ := strconv.ParseInt(string(m[3]), 10, 32)
			if auras[uint32(id)] == nil {
				auras[uint32(id)] = map[int]int32{}
			}
			auras[uint32(id)][eff] = int32(amount)
		}
	})
	defer cancel()
	previous := bot.World.TargetGUID()
	_ = bot.World.SetTarget(guid)
	bot.GM(t, ".list auras")
	time.Sleep(settle)
	_ = bot.World.SetTarget(previous)
	mu.Lock()
	defer mu.Unlock()
	return auras
}
