package client

import (
	"bytes"
	"encoding/binary"
)

// Area trigger, duel and dungeon finder opcodes (3.3.5a).
const (
	CmsgAreaTrigger       uint16 = 0x00B4
	SmsgDuelRequested     uint16 = 0x0167
	SmsgDuelComplete      uint16 = 0x016A
	SmsgDuelWinner        uint16 = 0x016B
	CmsgDuelAccepted      uint16 = 0x016C
	CmsgLfgJoin           uint16 = 0x035C
	SmsgLfgProposalUpdate uint16 = 0x0361
	CmsgLfgProposalResult uint16 = 0x0362
	SmsgLfgJoinResult     uint16 = 0x0364
	SmsgLfgUpdatePlayer   uint16 = 0x0367
)

// SendAreaTrigger sends CMSG_AREATRIGGER, as the game client does when the player enters an AreaTrigger.dbc zone.
func (w *WorldClient) SendAreaTrigger(triggerID uint32) error {
	data := make([]byte, 4)
	binary.LittleEndian.PutUint32(data, triggerID)
	return w.sendPacket(CmsgAreaTrigger, data)
}

// AcceptDuel sends CMSG_DUEL_ACCEPTED for the duel flag game object from SMSG_DUEL_REQUESTED.
func (w *WorldClient) AcceptDuel(arbiterGUID uint64) error {
	data := make([]byte, 8)
	binary.LittleEndian.PutUint64(data, arbiterGUID)
	return w.sendPacket(CmsgDuelAccepted, data)
}

// LfgJoin sends CMSG_LFG_JOIN for the given LFGDungeons ids (roles: 2 tank, 4 healer, 8 damage).
func (w *WorldClient) LfgJoin(roles uint32, dungeons []uint32, comment string) error {
	var buf bytes.Buffer
	_ = binary.Write(&buf, binary.LittleEndian, roles)
	buf.WriteByte(0) // NoPartialClear
	buf.WriteByte(0) // Achievements
	buf.WriteByte(uint8(len(dungeons)))
	for _, d := range dungeons {
		_ = binary.Write(&buf, binary.LittleEndian, d)
	}
	buf.WriteByte(3) // needs count, always 3 in the client
	buf.Write([]byte{0, 0, 0})
	buf.WriteString(comment)
	buf.WriteByte(0)
	return w.sendPacket(CmsgLfgJoin, buf.Bytes())
}

// LfgProposalResult answers an SMSG_LFG_PROPOSAL_UPDATE proposal.
func (w *WorldClient) LfgProposalResult(proposalID uint32, accept bool) error {
	data := make([]byte, 5)
	binary.LittleEndian.PutUint32(data, proposalID)
	if accept {
		data[4] = 1
	}
	return w.sendPacket(CmsgLfgProposalResult, data)
}
