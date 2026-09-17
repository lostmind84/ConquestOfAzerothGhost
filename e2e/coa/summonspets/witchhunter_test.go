//go:build e2e

package summonspets_test

import (
	"testing"
	"time"

	"github.com/azerothcore/AzerothGhost/e2e/e2eharness"
)

const (
	specHoundmaster         uint32 = 11
	specWitchHunterOther    uint32 = 10
	spellHoundmasterWhistle uint32 = 801343 // SUMMON_PET creature 50124
	spellCallFromShadows    uint32 = 578118 // SUMMON_DEAD_PET, taught with the Whistle
	spellRampagingHounds    uint32 = 570726 // talent passive
	spellRampagingFrenzy    uint32 = 562027 // proc aura that belongs on the hound
	talentRampagingHounds   uint32 = 34524
	spellShadowRageTalent   uint32 = 705455
	talentShadowRage        uint32 = 6334
	spellShadowRagePet      uint32 = 804192
	spellUnleashR1          uint32 = 807918 // summons 50224
	creatureShadowhound     uint32 = 50124
	creatureLesserHound     uint32 = 50224
)

var shadowblastRanks = []uint32{804191, 680264, 680265, 680266, 680267, 680268, 680269}

// houndmaster creates a Houndmaster Witch Hunter and summons the permanent Shadowhound.
func houndmaster(t *testing.T, prefix string, level int) (*e2eharness.ScenarioBot, uint64) {
	t.Helper()
	bot := newBot(t, prefix, e2eharness.RaceOrc, classWitchHunter, level)
	bot.SetSpecialization(t, specHoundmaster, spellHoundmasterWhistle)
	bot.CombatReadyFull(t)
	if res := castLanded(t, bot, spellHoundmasterWhistle, 0, 3); !res.Success {
		t.Fatalf("precondition: Houndmaster's Whistle refused: %s", e2eharness.SpellFailReasonName(res.FailReason))
	}
	pet := bot.WaitPlayerPet(t, 10*time.Second)
	bot.WaitUnitGUID(t, pet, 5*time.Second)
	return bot, pet
}

// Main project issue #219: Houndmaster's Whistle creates no pet bar and the Shadowhound only fights after /petattack.
// The server sends the pet bar; the Shadowhound starts passive like a stock warlock pet
// (see TestControl_WarlockImpReactState).
//
//	go test -tags=e2e ./e2e/coa/summonspets -run WhistlePetBar -count=1 -v
func TestWitchHunter_WhistlePetBar(t *testing.T) {
	bot := newBot(t, "WhBar", e2eharness.RaceOrc, classWitchHunter, 11)
	bot.SetSpecialization(t, specHoundmaster, spellHoundmasterWhistle)
	bot.CombatReadyFull(t)
	bars := watchPetSpells(t, bot)
	if res := castLanded(t, bot, spellHoundmasterWhistle, 0, 3); !res.Success {
		t.Fatalf("precondition: Houndmaster's Whistle refused: %s", e2eharness.SpellFailReasonName(res.FailReason))
	}
	pet := bot.WaitPlayerPet(t, 10*time.Second)
	time.Sleep(2 * time.Second)
	if ev, ok := bars.forPet(pet); ok {
		t.Logf("E2E_PASS: SMSG_PET_SPELLS sent for the Shadowhound, react %d, command %d, action bar spells %v (#219)",
			ev.react, ev.command, ev.spells)
	} else {
		t.Errorf("E2E_FAIL: no SMSG_PET_SPELLS for the Shadowhound, so the client has no pet bar (#219)")
	}
}

// Main project issue #1332: the Shadowhound stays out after the Witch Hunter leaves the Houndmaster specialization.
//
//	go test -tags=e2e ./e2e/coa/summonspets -run ShadowhoundAfterSpecChange -count=1 -v
func TestWitchHunter_ShadowhoundAfterSpecChange(t *testing.T) {
	bot, pet := houndmaster(t, "WhSpec", 10)
	bot.SetSpecialization(t, specWitchHunterOther)
	time.Sleep(3 * time.Second)
	if bot.PlayerPetGUID() == 0 && len(ownedUnits(bot, creatureShadowhound)) == 0 {
		t.Logf("E2E_PASS: the Shadowhound left with the Houndmaster specialization (#1332)")
		return
	}
	t.Errorf("E2E_FAIL: Shadowhound 0x%X still active after switching to specialization %d (#1332)", pet, specWitchHunterOther)
}

// Main project issue #1450: a dead Shadowhound disappears and the Whistle summons a fresh one, instead of staying
// dead until Call From The Shadows revives it like a hunter pet. Kept as is by decision: this records the behaviour.
//
//	go test -tags=e2e ./e2e/coa/summonspets -run ShadowhoundDeath -count=1 -v
func TestWitchHunter_ShadowhoundDeath(t *testing.T) {
	bot, pet := houndmaster(t, "WhDead", 49)
	t.Logf("knows Call From The Shadows %d: %v", spellCallFromShadows, bot.World.KnowsSpell(spellCallFromShadows))
	killUnit(t, bot, pet)
	time.Sleep(3 * time.Second)
	t.Logf("after death: pet field 0x%X, pet unit visible %v", bot.PlayerPetGUID(), bot.World.GetObject(pet) != nil)
	_ = bot.World.SetTarget(bot.GUID)
	time.Sleep(200 * time.Millisecond)
	bot.GM(t, ".cooldown")
	time.Sleep(500 * time.Millisecond)
	res, err := bot.TryCast(t, spellHoundmasterWhistle, 0, castTimeout)
	if err != nil {
		t.Fatalf("Whistle after death: %v", err)
	}
	if res.Success {
		t.Logf("E2E_MEASURE: Houndmaster's Whistle summons a new Shadowhound right after the old one died (#1450)")
		return
	}
	t.Logf("E2E_MEASURE: Whistle refused after death: %s (#1450)", e2eharness.SpellFailReasonName(res.FailReason))
	if res, err := bot.TryCast(t, spellCallFromShadows, 0, 10*time.Second); err == nil {
		time.Sleep(2 * time.Second)
		t.Logf("Call From The Shadows: success %v %s, pet field after 0x%X", res.Success,
			e2eharness.SpellFailReasonName(res.FailReason), bot.PlayerPetGUID())
	}
}

// Main project issue #301: Rampaging Hounds' proc aura shows on the player instead of the hound.
//
//	go test -tags=e2e ./e2e/coa/summonspets -run RampagingHounds -count=1 -v
func TestWitchHunter_RampagingHoundsOnHound(t *testing.T) {
	bot, pet := houndmaster(t, "WhRamp", 37)
	bot.SetTalentRank(t, talentRampagingHounds, 1)
	waitKnows(t, bot, spellRampagingHounds)
	time.Sleep(3 * time.Second)
	if bot.HasAura(spellRampagingFrenzy) {
		t.Errorf("E2E_FAIL: the player carries Rampaging Frenzy %d (#301)", spellRampagingFrenzy)
		return
	}
	if !bot.UnitHasAura(pet, spellRampagingFrenzy) {
		t.Errorf("E2E_FAIL: the Shadowhound has no Rampaging Frenzy %d (#301)", spellRampagingFrenzy)
		return
	}
	t.Logf("E2E_PASS: Rampaging Frenzy is on the hound, not on the player; player keeps talent passive %d (#301)", spellRampagingHounds)
}

// Main project issue #311: Unleash the Hounds summons do not get Shadow Rage from Shadowblast. Not a bug: the tooltip
// grants Shadow Rage to "your Shadow Hound" only, so the permanent Shadowhound gets it and the lesser hounds do not.
//
//	go test -tags=e2e ./e2e/coa/summonspets -run ShadowRageUnleash -count=1 -v
func TestWitchHunter_ShadowRageUnleashHounds(t *testing.T) {
	bot, pet := houndmaster(t, "WhRage", 38)
	bot.SetTalentRank(t, talentShadowRage, 1)
	waitKnows(t, bot, spellShadowRageTalent)
	bot.Learn(t, spellUnleashR1)
	dummy := spawnTarget(t, bot, e2eharness.CreatureHeroicTrainingDummy, 36, 0)
	x, y, z, m := bot.Pos()
	bot.Teleport(t, x+12, y, z, m)
	bot.Face(t, dummy)
	if res := castLanded(t, bot, spellUnleashR1, dummy, 3); !res.Success {
		t.Fatalf("precondition: Unleash the Hounds refused: %s", e2eharness.SpellFailReasonName(res.FailReason))
	}
	var hounds []uint64
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline) && len(hounds) == 0; time.Sleep(250 * time.Millisecond) {
		hounds = ownedUnits(bot, creatureLesserHound)
	}
	if len(hounds) == 0 {
		t.Fatalf("precondition: no Lesser Shadowhound appeared")
	}
	shadowblast := knownRank(t, bot, shadowblastRanks...)
	time.Sleep(gcd)
	if res := castLanded(t, bot, shadowblast, dummy, 3); !res.Success {
		t.Fatalf("precondition: Shadowblast %d refused: %s", shadowblast, e2eharness.SpellFailReasonName(res.FailReason))
	}
	time.Sleep(settle)
	if !bot.UnitHasAura(pet, spellShadowRagePet) {
		t.Errorf("E2E_FAIL: the permanent Shadowhound has no Shadow Rage %d after Shadowblast (#311)", spellShadowRagePet)
		return
	}
	for _, h := range hounds {
		if bot.UnitHasAura(h, spellShadowRagePet) {
			t.Errorf("E2E_FAIL: Lesser Shadowhound 0x%X got Shadow Rage, the tooltip only covers your Shadow Hound (#311)", h)
			return
		}
	}
	t.Logf("E2E_PASS: Shadow Rage on the permanent Shadowhound only, as the tooltip states (#311)")
}
