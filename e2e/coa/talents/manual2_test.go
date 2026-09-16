//go:build e2e

package talents_test

import (
	"bytes"
	"encoding/binary"
	"sync/atomic"
	"testing"
	"time"

	"github.com/azerothcore/AzerothGhost/e2e/e2eharness"
)

const (
	classRanger uint8 = 21

	// Guardian (#156)
	spellSpikedReinforcement       uint32 = 653131
	spellSpikedReinforcementDamage uint32 = 803131
	hitInfoBlock                   uint32 = 0x00002000

	// Templar (#303, #305)
	spellRighteousLunge uint32 = 801443
	spellCondemn        uint32 = 804906
	auraOathLunge       uint32 = 804904
	auraOathCondemn     uint32 = 804922
	spellScourgebane    uint32 = 92111
	spellScourgebaneHit uint32 = 807414
	specTemplarCrusader uint32 = 24
	talentScourgebane   uint32 = 4024

	// Bloodmage (#1416, #1439)
	specBloodmageEternal uint32 = 99
	spellEternalCurse    uint32 = 92114
	spellRotclaw         uint32 = 804197
	spellDualWield       uint32 = 674
	itemWornDagger       uint32 = 2092
	itemDirk             uint32 = 2139
	itemRitualTome       uint32 = 486317
	equipmentSlotOffHand uint8  = 16

	// Venomancer (#355)
	specVenomancerTank uint32 = 52
	spellSpiderLord    uint32 = 704264
	talentSpiderLord   uint32 = 6113
	spellBeetleForm    uint32 = 803183
	unitFieldDisplayID uint16 = 0x0006 + 0x003D // UNIT_FIELD_DISPLAYID

	playerFieldInvSlotHead uint16 = 0x0006 + 0x008E + 0x00B0 // PLAYER_FIELD_INV_SLOT_HEAD

	// Ranger (#244)
	spellBushcraft uint32 = 800267
)

// Main project issue #156: Spiked Reinforcement on the shield deals no damage back on block.
//
//	go test -tags=e2e ./e2e/coa/talents -run SpikedReinforcement -count=1 -v
func TestGuardian_SpikedReinforcementDamagesOnBlock(t *testing.T) {
	bot := newBot(t, "GdSpike", e2eharness.RaceHuman, classGuardian, 10)
	equip(t, bot, itemLargeRoundShield)
	for _, id := range []uint32{spellSpikedReinforcement, spellTowerFormation, spellRaiseShield} {
		if !bot.World.KnowsSpell(id) {
			bot.Learn(t, id)
		}
	}
	bot.CombatReadyFull(t)
	// The enchantment targets the equipped shield, as the client sends it.
	var shield uint64
	if obj := bot.World.GetObject(bot.World.CharGUID()); obj != nil {
		shield = obj.GUIDField(playerFieldInvSlotHead + 2*uint16(equipmentSlotOffHand))
	}
	if shield == 0 {
		t.Fatalf("precondition: no item GUID in the off-hand slot")
	}
	bot.ArmSpellWaiter()
	if err := bot.World.CastSpellOnItem(spellSpikedReinforcement, shield); err != nil {
		t.Fatalf("Spiked Reinforcement: %v", err)
	}
	if res, err := bot.WaitSpellID(spellSpikedReinforcement, castTimeout); err != nil || !res.Success {
		t.Fatalf("Spiked Reinforcement on the shield refused: %v %s", err, e2eharness.SpellFailReasonName(res.FailReason))
	}
	castLanded(t, bot, spellTowerFormation, 0, 2)
	damage := watchSpellLog(t, bot, smsgSpellNonMeleeDamageLog, spellSpikedReinforcementDamage)
	var blocks, hits atomic.Int32
	self := bot.World.CharGUID()
	cancelBlocks := bot.World.AddPacketHook(func(op uint16, data []byte) {
		if op != smsgAttackerStateUpdate || len(data) < 6 {
			return
		}
		r := bytes.NewReader(data[4:])
		readPackedGUID(r)
		if readPackedGUID(r) != self {
			return
		}
		hits.Add(1)
		if binary.LittleEndian.Uint32(data)&hitInfoBlock != 0 {
			blocks.Add(1)
		}
	})
	defer cancelBlocks()
	thug := spawnTarget(t, bot, creatureDefiasThug, 10)
	bot.GM(t, ".npc set faction 14") // hostile to everyone, the pad's faction relations vary
	bot.CombatReadyFull(t)           // the GM commands above turn GM mode back on; creatures ignore GMs
	bot.Engage(t, thug, 15*time.Second)
	bot.Attack(t, thug)
	deadline := time.Now().Add(40 * time.Second)
	for time.Now().Before(deadline) {
		if !bot.HasAura(spellRaiseShield) {
			_, _ = bot.TryCast(t, spellRaiseShield, 0, castTimeout)
		}
		if got := damage.snapshot(); len(got) > 0 {
			t.Logf("E2E_PASS: Spiked Reinforcement dealt %d on block", got[0].amount)
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
	if blocks.Load() < 3 {
		t.Fatalf("precondition: only %d block(s) out of %d attack(s) taken in 40 s (Raise Shield up: %v)",
			blocks.Load(), hits.Load(), bot.HasAura(spellRaiseShield))
	}
	t.Errorf("E2E_FAIL: %d blocks with Spiked Reinforcement on the shield, no %d damage (#156)", blocks.Load(),
		spellSpikedReinforcementDamage)
}

// Main project issue #303: gaining a second kind of Oath removes the first.
//
//	go test -tags=e2e ./e2e/coa/talents -run Templar_Oaths -count=1 -v
func TestTemplar_OathsOfDifferentKindsStack(t *testing.T) {
	bot := newBot(t, "TpOath", e2eharness.RaceHuman, classTemplar, 6)
	for _, id := range []uint32{spellRighteousLunge, spellCondemn} {
		if !bot.World.KnowsSpell(id) {
			bot.Learn(t, id)
		}
	}
	dummy := spawnTarget(t, bot, e2eharness.CreatureHeroicTrainingDummy, 6)
	bot.CombatReadyFull(t)
	if res := castLanded(t, bot, spellRighteousLunge, dummy, 3); !res.Success {
		t.Fatalf("Righteous Lunge refused: %s", e2eharness.SpellFailReasonName(res.FailReason))
	}
	if !waitAura(bot, auraOathLunge, 2*time.Second) {
		t.Fatalf("precondition: Righteous Lunge granted no Oath %d", auraOathLunge)
	}
	time.Sleep(gcd)
	if res := castLanded(t, bot, spellCondemn, dummy, 3); !res.Success {
		t.Fatalf("Condemn refused: %s", e2eharness.SpellFailReasonName(res.FailReason))
	}
	if !waitAura(bot, auraOathCondemn, 2*time.Second) {
		t.Fatalf("precondition: Condemn granted no Oath %d", auraOathCondemn)
	}
	if !bot.HasAura(auraOathLunge) {
		t.Errorf("E2E_FAIL: Condemn's Oath %d removed Righteous Lunge's Oath %d (#303)", auraOathCondemn, auraOathLunge)
		return
	}
	t.Logf("E2E_PASS: both Oaths kept")
}

// Main project issue #305: Scourgebane procs for 400-500 damage at level 10. The report gives no expected value;
// this test only records the observed hits for the balance question.
//
//	go test -tags=e2e ./e2e/coa/talents -run Scourgebane -count=1 -v
func TestTemplar_ScourgebaneDamageAtLevel10(t *testing.T) {
	bot := newBot(t, "TpScour", e2eharness.RaceHuman, classTemplar, 10)
	takeTalent(t, bot, specTemplarCrusader, talentScourgebane, spellScourgebane)
	damage := watchSpellLog(t, bot, smsgSpellNonMeleeDamageLog)
	dummy := spawnTarget(t, bot, e2eharness.CreatureHeroicTrainingDummy, 10)
	swings := watchSwings(t, bot)
	bot.CombatReady(t)
	bot.Attack(t, dummy)
	deadline := time.Now().Add(40 * time.Second)
	for time.Now().Before(deadline) {
		for _, ev := range damage.snapshot() {
			if ev.spellID != 0 {
				t.Logf("E2E_INFO: level 10 Templar, %d swing(s), spell damage logs %+v", swings.swings(), damage.snapshot())
				return
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Logf("E2E_INFO: level 10 Templar, %d swing(s), no Scourgebane damage in 40 s", swings.swings())
}

// Main project issue #1416: choosing the Eternal specialization does not grant Rotclaw like the other Bloodmage
// specializations grant their first row.
//
//	go test -tags=e2e ./e2e/coa/talents -run EternalGrantsRotclaw -count=1 -v
func TestBloodmage_EternalGrantsRotclaw(t *testing.T) {
	bot := newBot(t, "BmRot", e2eharness.RaceHuman, classBloodmage, 10)
	bot.SetSpecialization(t, specBloodmageEternal)
	if !waitSpell(bot, spellEternalCurse, 5*time.Second) {
		t.Fatalf("precondition: Eternal Curse %d not granted by the Eternal specialization", spellEternalCurse)
	}
	if !waitSpell(bot, spellRotclaw, 3*time.Second) {
		t.Errorf("E2E_FAIL: Eternal specialization at level 10, Rotclaw %d not granted (#1416)", spellRotclaw)
		return
	}
	t.Logf("E2E_PASS: Eternal specialization granted Rotclaw")
}

// Main project issue #1439: Eternal Bloodmage's Dual Wield is learned again at each login and the off-hand weapon
// is unequipped and mailed.
//
//	go test -tags=e2e ./e2e/coa/talents -run EternalDualWield -count=1 -v
func TestBloodmage_EternalDualWieldSurvivesRelog(t *testing.T) {
	bot := newBot(t, "BmDual", e2eharness.RaceHuman, classBloodmage, 11)
	bot.SetSpecialization(t, specBloodmageEternal)
	if !waitSpell(bot, spellDualWield, 5*time.Second) {
		t.Fatalf("precondition: Dual Wield %d not granted by the Eternal specialization", spellDualWield)
	}
	equip(t, bot, itemWornDagger)
	// The starter kit holds a Ritual Tome in the off-hand: swap the Dirk from the backpack into that slot.
	bot.GM(t, ".additem 2139")
	time.Sleep(settle)
	for slot := uint8(23); slot < 39 && bot.VisibleItemEntry(equipmentSlotOffHand) != itemDirk; slot++ {
		_ = bot.World.SwapInvItem(slot, equipmentSlotOffHand)
		time.Sleep(300 * time.Millisecond)
		if entry := bot.VisibleItemEntry(equipmentSlotOffHand); entry != itemDirk && entry != itemRitualTome {
			_ = bot.World.SwapInvItem(slot, equipmentSlotOffHand) // put back whatever else was swapped in
			time.Sleep(300 * time.Millisecond)
		}
	}
	if bot.VisibleItemEntry(equipmentSlotOffHand) != itemDirk {
		t.Fatalf("precondition: Dirk not in the off-hand before the relog (off-hand %d)",
			bot.VisibleItemEntry(equipmentSlotOffHand))
	}
	bot.Save(t)
	bot.Relog(t)
	time.Sleep(2 * time.Second)
	offHand := bot.VisibleItemEntry(equipmentSlotOffHand)
	t.Logf("after relog: main hand %d, off-hand %d, Dual Wield known %v", bot.VisibleItemEntry(equipmentSlotOffHand-1),
		offHand, bot.World.KnowsSpell(spellDualWield))
	if offHand != itemDirk {
		t.Errorf("E2E_FAIL: the off-hand Dirk was unequipped by the relog (off-hand now %d) (#1439)", offHand)
		return
	}
	t.Logf("E2E_PASS: off-hand weapon kept across the relog")
}

// Main project issue #355: Spider Lord does not change the Beetle Form model.
//
//	go test -tags=e2e ./e2e/coa/talents -run SpiderLord -count=1 -v
func TestVenomancer_SpiderLordChangesBeetleModel(t *testing.T) {
	bot := newBot(t, "VnLord", e2eharness.RaceHuman, classVenomancer, 60)
	bot.SetSpecialization(t, specVenomancerTank)
	if !bot.World.KnowsSpell(spellBeetleForm) {
		bot.Learn(t, spellBeetleForm)
	}
	bot.CombatReadyFull(t)
	beetleModel := func() uint32 {
		if res := castLanded(t, bot, spellBeetleForm, 0, 2); !res.Success {
			t.Fatalf("Beetle Form refused: %s", e2eharness.SpellFailReasonName(res.FailReason))
		}
		if !waitAura(bot, spellBeetleForm, 2*time.Second) {
			t.Fatalf("precondition: no Beetle Form aura")
		}
		time.Sleep(settle)
		model := selfValue(bot, unitFieldDisplayID)
		bot.CancelAura(t, spellBeetleForm)
		time.Sleep(settle)
		return model
	}
	without := beetleModel()
	bot.SetTalentRank(t, talentSpiderLord, 1)
	if !waitSpell(bot, spellSpiderLord, 5*time.Second) {
		t.Fatalf("precondition: Spider Lord not learned")
	}
	with := beetleModel()
	t.Logf("Beetle Form display: %d without Spider Lord, %d with", without, with)
	if with == without {
		t.Errorf("E2E_FAIL: Spider Lord leaves the Beetle Form model unchanged (%d) (#355)", with)
		return
	}
	t.Logf("E2E_PASS: Spider Lord changed the Beetle Form model")
}

// Main project issue #244: Bushcraft does nothing and cannot be used.
//
//	go test -tags=e2e ./e2e/coa/talents -run Bushcraft -count=1 -v
func TestRanger_BushcraftCast(t *testing.T) {
	bot := newBot(t, "RgBush", e2eharness.RaceHuman, classRanger, 5)
	if !bot.World.KnowsSpell(spellBushcraft) {
		bot.Learn(t, spellBushcraft)
	}
	res, err := bot.TryCast(t, spellBushcraft, 0, castTimeout)
	if err != nil {
		t.Fatalf("Bushcraft: %v", err)
	}
	t.Logf("E2E_INFO: Bushcraft cast success=%v reason=%s", res.Success, e2eharness.SpellFailReasonName(res.FailReason))
}
