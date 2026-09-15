# ConquestOfAzerothGhost

Fork of https://github.com/azerothcore/AzerothGhost to support `Conquest of AzerothCore` custom classes and features.

The primary goal is to support E2E testing so that regressions to classes, talents and features can be caught as early as possible.

Currently just has a tiny subset of Time Chronomancer done (see `e2e/classes/chronomancer`), but the plan is to cover all of the Time spec, then move onto as many other specs and classes as my hyperfocus allows.

## Usage:

```
$env:E2E_AUTH_DSN="root:pA$sw0rd!@tcp(127.0.0.1:3307)/acore_auth"
$env:E2E_CHAR_DSN="root:pA$sw0rd!@tcp(127.0.0.1:3307)/acore_characters"
$env:E2E_WORLD_DSN="root:pA$sw0rd!@tcp(127.0.0.1:3307)/acore_world"
$env:E2E_WORLD_LOG="debug"
$env:E2E_PLAINTEXT_HEADERS="1"
$env:E2E_DBC_PATH="C:\CoA\Data\dbc"

go test -tags=e2e ./e2e/classes/chronomancer/time -run TestChronomancer -v
```

## Example Output:

```
=== RUN   TestChronomancer_TimeTalentsAndAbilities
=== PAUSE TestChronomancer_TimeTalentsAndAbilities
=== CONT  TestChronomancer_TimeTalentsAndAbilities
    time_test.go:40: Chrono00b645ba04 realm=AzerothCore addr=127.0.0.1:8085
    session.go:97: [Chrono00b645ba04] session phase none -> connected (CMSG_AUTH_SESSION (plaintext))
    session.go:97: [Chrono00b645ba04] World auth successful
    session.go:97: [Chrono00b645ba04] session phase connected -> authed (SMSG_AUTH_RESPONSE)
    session.go:97: [Chrono00b645ba04] Character enum: 0 characters
    time_test.go:40: Chrono00b645ba04 create Chrleadrynno result=0x2F
    session.go:97: [Chrono00b645ba04] Character enum: 1 characters
    session.go:97: [Chrono00b645ba04] session phase authed -> loading (CMSG_PLAYER_LOGIN)
    session.go:97: [Chrono00b645ba04] Login verified map=0 pos=(-8950.0,-132.5,83.5)
    session.go:97: [Chrono00b645ba04] session phase loading -> in_world (SMSG_LOGIN_VERIFY_WORLD)
    session.go:97: [Chrono00b645ba04] Received 45 initial spells
    session.go:97: [Chrono00b645ba04] GM .gm on
    session.go:97: [Chrono00b645ba04] GM detail opcode=0x0095 encrypted=false lang=7
    session.go:97: [Chrono00b645ba04] CMSG_SET_SELECTION target=43
    session.go:97: [Chrono00b645ba04] GM .character level 60
    session.go:97: [Chrono00b645ba04] GM detail opcode=0x0095 encrypted=false lang=7
    session.go:97: [Chrono00b645ba04] Received 1270 initial spells (repeat)
    session.go:97: [Chrono00b645ba04] SMSG_SUPERCEDED_SPELL 33388 → 33391
    session.go:97: [Chrono00b645ba04] SMSG_SUPERCEDED_SPELL 33391 → 34090
    session.go:97: [Chrono00b645ba04] SMSG_SUPERCEDED_SPELL 34090 → 34091
    session.go:97: [Chrono00b645ba04] LEVEL UP! Now level 60
    session.go:97: [Chrono00b645ba04] SMSG_SUPERCEDED_SPELL 802229 → 803896
    session.go:97: [Chrono00b645ba04] SMSG_SUPERCEDED_SPELL 803896 → 803897
    time_test.go:40: level set to 60
    time_test.go:47: PackagePad suite=default pad=Hyjal1 map=1 (4516.7,-2312.2,1137.9)
    session.go:97: [Chrono00b645ba04] GM .go xyz 4516.70 -2312.19 1137.86 1
    session.go:97: [Chrono00b645ba04] GM detail opcode=0x0095 encrypted=false lang=7
    session.go:97: [Chrono00b645ba04] Transfer pending received
    session.go:97: [Chrono00b645ba04] SMSG_NEW_WORLD map=1 pos=(4516.7,-2312.2,1137.9)
    session.go:97: [Chrono00b645ba04] session phase in_world -> far_transfer (SMSG_NEW_WORLD)
    session.go:97: [Chrono00b645ba04] session phase far_transfer -> in_world (MSG_MOVE_WORLDPORT_ACK)
    session.go:97: [Chrono00b645ba04] GM .cheat power on
    session.go:97: [Chrono00b645ba04] GM detail opcode=0x0095 encrypted=false lang=7
    session.go:97: [Chrono00b645ba04] GM .localspec 31
    session.go:97: [Chrono00b645ba04] GM detail opcode=0x0095 encrypted=false lang=7
    session.go:97: [Chrono00b645ba04] Received 1367 initial spells (repeat)
    time_test.go:53: E2E_PASS: bot learned Time baseline Ability Aeon of Resilience (Specialization) [92119]
    time_test.go:75: BUG DETECTED: hidden aura 'CoA Aura - Chronomancer Class' (887110) is NOT applied — server should auto-apply on spec change
    time_test.go:75: BUG DETECTED: hidden aura 'CoA Aura - Chronomancer Time' (887154) is NOT applied — server should auto-apply on spec change
    time_test.go:93: E2E_PASS: bot knows Aeon 'Aeon of Renewal [806290]'
    time_test.go:93: E2E_PASS: bot knows Aeon 'Aeon of Resilience [806291]'
    time_test.go:101: E2E_FAIL: Aeon 'Aeon of Resilience [806291]' (806291) was learned 2 times (duplicate learn packets from server)
    time_test.go:93: E2E_PASS: bot knows Aeon 'Aeon of Protection [806292]'
    time_test.go:93: E2E_PASS: bot knows Aeon 'Aeon of Oblivion [806293]'
    time_test.go:106: Aeons are 'stances' and should be mutually exclusive
    time_test.go:108: Casting Aeon 'Aeon of Renewal [806290]'...
    session.go:97: [Chrono00b645ba04] GM .combatstop
    session.go:97: [Chrono00b645ba04] GM detail opcode=0x0095 encrypted=false lang=7
    time_test.go:112: Active auras after casting Aeon of Renewal [806290]: [806290]
    time_test.go:118: E2E_PASS: 'Aeon of Renewal [806290]' is active
    time_test.go:128: E2E_PASS: 'Aeon of Resilience [806291]' is inactive as expected
    time_test.go:128: E2E_PASS: 'Aeon of Protection [806292]' is inactive as expected
    time_test.go:128: E2E_PASS: 'Aeon of Oblivion [806293]' is inactive as expected
    time_test.go:108: Casting Aeon 'Aeon of Resilience [806291]'...
    session.go:97: [Chrono00b645ba04] GM .combatstop
    session.go:97: [Chrono00b645ba04] GM detail opcode=0x0095 encrypted=false lang=7
    time_test.go:112: Active auras after casting Aeon of Resilience [806291]: [806290 806291]
    time_test.go:118: E2E_PASS: 'Aeon of Resilience [806291]' is active
    time_test.go:125: E2E_FAIL: mutually exclusive aura 'Aeon of Renewal [806290]' is unexpectedly active while in 'Aeon of Resilience [806291]'
    time_test.go:128: E2E_PASS: 'Aeon of Protection [806292]' is inactive as expected
    time_test.go:128: E2E_PASS: 'Aeon of Oblivion [806293]' is inactive as expected
    time_test.go:108: Casting Aeon 'Aeon of Protection [806292]'...
    session.go:97: [Chrono00b645ba04] GM .combatstop
    session.go:97: [Chrono00b645ba04] GM detail opcode=0x0095 encrypted=false lang=7
    time_test.go:112: Active auras after casting Aeon of Protection [806292]: [806291 806292 806290]
    time_test.go:118: E2E_PASS: 'Aeon of Protection [806292]' is active
    time_test.go:125: E2E_FAIL: mutually exclusive aura 'Aeon of Renewal [806290]' is unexpectedly active while in 'Aeon of Protection [806292]'
    time_test.go:125: E2E_FAIL: mutually exclusive aura 'Aeon of Resilience [806291]' is unexpectedly active while in 'Aeon of Protection [806292]'
    time_test.go:128: E2E_PASS: 'Aeon of Oblivion [806293]' is inactive as expected
    time_test.go:108: Casting Aeon 'Aeon of Oblivion [806293]'...
    session.go:97: [Chrono00b645ba04] GM .combatstop
    session.go:97: [Chrono00b645ba04] GM detail opcode=0x0095 encrypted=false lang=7
    time_test.go:112: Active auras after casting Aeon of Oblivion [806293]: [806290 806291 806292 806293]
    time_test.go:118: E2E_PASS: 'Aeon of Oblivion [806293]' is active
    time_test.go:125: E2E_FAIL: mutually exclusive aura 'Aeon of Renewal [806290]' is unexpectedly active while in 'Aeon of Oblivion [806293]'
    time_test.go:125: E2E_FAIL: mutually exclusive aura 'Aeon of Resilience [806291]' is unexpectedly active while in 'Aeon of Oblivion [806293]'
    time_test.go:125: E2E_FAIL: mutually exclusive aura 'Aeon of Protection [806292]' is unexpectedly active while in 'Aeon of Oblivion [806293]'
    time_test.go:155: E2E_PASS: bot knows spell 'Epoch (Rank 1) [801270]'
    time_test.go:155: E2E_PASS: bot knows spell 'Epoch (Rank 2) [501779]'
    time_test.go:155: E2E_PASS: bot knows spell 'Epoch (Rank 3) [501780]'
    time_test.go:155: E2E_PASS: bot knows spell 'Epoch (Rank 4) [501781]'
    time_test.go:155: E2E_PASS: bot knows spell 'Epoch (Rank 5) [501782]'
    time_test.go:155: E2E_PASS: bot knows spell 'Epoch (Rank 6) [501783]'
    time_test.go:155: E2E_PASS: bot knows spell 'Epoch (Rank 7) [501784]'
    time_test.go:155: E2E_PASS: bot knows spell 'Epoch (Rank 8) [504575]'
    time_test.go:207: E2E_PASS: Cast 1 Cast Bar duration: 2000ms (expected ~2000ms)
    time_test.go:224: E2E_PASS: Cast 1 measured duration 1.9976847s within [1.7s, 2.3s] (target 2s)
    time_test.go:233: E2E_PASS: after Cast 1, player has 1 stacks of Sands of Time
    time_test.go:207: E2E_PASS: Cast 2 Cast Bar duration: 1600ms (expected ~1600ms)
    time_test.go:224: E2E_PASS: Cast 2 measured duration 1.6040776s within [1.3s, 1.9s] (target 1.6s)
    time_test.go:233: E2E_PASS: after Cast 2, player has 2 stacks of Sands of Time
    time_test.go:207: E2E_PASS: Cast 3 Cast Bar duration: 1200ms (expected ~1200ms)
    time_test.go:224: E2E_PASS: Cast 3 measured duration 1.196423s within [900ms, 1.5s] (target 1.2s)
    time_test.go:233: E2E_PASS: after Cast 3, player has 3 stacks of Sands of Time
    time_test.go:207: E2E_PASS: Cast 4 Cast Bar duration: 799ms (expected ~800ms)
    time_test.go:224: E2E_PASS: Cast 4 measured duration 806.5786ms within [550ms, 1.05s] (target 800ms)
    time_test.go:233: E2E_PASS: after Cast 4, player has 4 stacks of Sands of Time
    time_test.go:207: E2E_PASS: Cast 5 Cast Bar duration: 399ms (expected ~400ms)
    time_test.go:224: E2E_PASS: Cast 5 measured duration 401.1043ms within [200ms, 600ms] (target 400ms)
    time_test.go:233: E2E_PASS: after Cast 5, player has 5 stacks of Sands of Time
    time_test.go:200: E2E_PASS: Cast 6 Cast Bar duration: 0ms (Instant Cast)
    time_test.go:216: E2E_PASS: Cast 6 measured wall-clock duration: 105.1949ms (round-trip)
    time_test.go:233: E2E_PASS: after Cast 6, player has 0 stacks of Sands of Time
    time_test.go:239: E2E_PASS: 5 stacks consumed on 6th cast; no Sands of Time aura remains
    time_test.go:242: Verifying Epoch cast time resets back to 2.0s after stacks were consumed...
    time_test.go:252: E2E_PASS: Cast 7 Cast Bar confirmed reset to 2000ms
    time_test.go:257: E2E_PASS: Cast 7 measured duration 2.0000551s confirmed reset back to ~2.0s
    time_test.go:262: E2E_SUMMARY: Epoch Sands of Time cast progression test failed!
    session.go:97: [Chrono00b645ba04] SMSG_LEARNED_SPELL count=104 last=806301
--- FAIL: TestChronomancer_TimeTalentsAndAbilities (39.41s)
FAIL
FAIL    github.com/azerothcore/AzerothGhost/e2e/classes/chronomancer/time       39.498s
FAIL
```


## License:

MIT — see [LICENSE](LICENSE).
