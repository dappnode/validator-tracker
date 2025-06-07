package domain

type Epoch uint64
type Slot uint64
type ValidatorIndex uint64
type CommitteeIndex uint64

// domain/domain.go
type ValidatorDuty struct {
	Slot                  Slot
	CommitteeIndex        CommitteeIndex
	ValidatorCommitteeIdx uint64
	ValidatorIndex        ValidatorIndex
}

type CommitteeSizeMap map[CommitteeIndex]int

type Attestation struct {
	DataSlot        Slot
	CommitteeBits   []byte
	AggregationBits []byte
}
