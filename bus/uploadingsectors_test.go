package bus

import (
	"errors"
	"testing"

	rhpv2 "go.sia.tech/core/rhp/v2"
	"go.sia.tech/core/types"
	"go.sia.tech/renterd/api"
	"lukechampine.com/frand"
)

func TestUploadingSectorsCache(t *testing.T) {
	c := newUploadingSectorsCache()

	uID1 := newTestUploadID()
	uID2 := newTestUploadID()

	fcid1 := types.FileContractID{1}
	fcid2 := types.FileContractID{2}
	fcid3 := types.FileContractID{3}

	c.trackUpload(uID1)
	c.trackUpload(uID2)

	_ = c.addUploadingSector(uID1, fcid1, types.Hash256{1})
	_ = c.addUploadingSector(uID1, fcid2, types.Hash256{2})
	_ = c.addUploadingSector(uID2, fcid2, types.Hash256{3})

	if roots1 := c.sectors(fcid1); len(roots1) != 1 || roots1[0] != (types.Hash256{1}) {
		t.Fatal("unexpected cached sectors")
	}
	if roots2 := c.sectors(fcid2); len(roots2) != 2 {
		t.Fatal("unexpected cached sectors", roots2)
	}
	if roots3 := c.sectors(fcid3); len(roots3) != 0 {
		t.Fatal("unexpected cached sectors")
	}

	if o1, exists := c.uploads[uID1]; !exists || o1.started.IsZero() {
		t.Fatal("unexpected")
	}
	if o2, exists := c.uploads[uID2]; !exists || o2.started.IsZero() {
		t.Fatal("unexpected")
	}

	c.finishUpload(uID1)
	if roots1 := c.sectors(fcid1); len(roots1) != 0 {
		t.Fatal("unexpected cached sectors")
	}
	if roots2 := c.sectors(fcid2); len(roots2) != 1 || roots2[0] != (types.Hash256{3}) {
		t.Fatal("unexpected cached sectors")
	}

	c.finishUpload(uID2)
	if roots2 := c.sectors(fcid1); len(roots2) != 0 {
		t.Fatal("unexpected cached sectors")
	}

	if err := c.addUploadingSector(uID1, fcid1, types.Hash256{1}); !errors.Is(err, api.ErrUnknownUpload) {
		t.Fatal("unexpected error", err)
	}
	if err := c.trackUpload(uID1); err != nil {
		t.Fatal("unexpected error", err)
	}
	if err := c.trackUpload(uID1); !errors.Is(err, api.ErrUploadAlreadyExists) {
		t.Fatal("unexpected error", err)
	}

	// reset cache
	c = newUploadingSectorsCache()

	// track upload that uploads across two contracts
	c.trackUpload(uID1)
	c.addUploadingSector(uID1, fcid1, types.Hash256{1})
	c.addUploadingSector(uID1, fcid1, types.Hash256{2})
	c.addRenewal(fcid2, fcid1)
	c.addUploadingSector(uID1, fcid2, types.Hash256{3})
	c.addUploadingSector(uID1, fcid2, types.Hash256{4})

	// assert pending sizes for both contracts should be 4 sectors
	p1 := c.pending(fcid1)
	p2 := c.pending(fcid2)
	if p1 != p2 || p1 != 4*rhpv2.SectorSize {
		t.Fatal("unexpected pending size", p1/rhpv2.SectorSize, p2/rhpv2.SectorSize)
	}

	// assert sectors for both contracts contain 4 sectors
	s1 := c.sectors(fcid1)
	s2 := c.sectors(fcid2)
	if len(s1) != 4 || len(s2) != 4 {
		t.Fatal("unexpected sectors", len(s1), len(s2))
	}

	// renew contract
	c.addRenewal(fcid3, fcid2)

	// assert renewal maps get pruned
	if len(c.renewedFrom) != 1 || len(c.renewedTo) != 1 {
		t.Fatal("unexpected", len(c.renewedFrom), len(c.renewedTo))
	}

	// repeat a similar upload
	c.trackUpload(uID2)
	c.addUploadingSector(uID2, fcid2, types.Hash256{1})
	c.addUploadingSector(uID2, fcid2, types.Hash256{2})
	c.addUploadingSector(uID2, fcid3, types.Hash256{3})
	c.addUploadingSector(uID2, fcid3, types.Hash256{4})

	// pending sizes should be 6 sectors because the 1st upload is still ongoing
	p1 = c.pending(fcid2)
	p2 = c.pending(fcid3)
	if p1 != p2 || p1 != 6*rhpv2.SectorSize {
		t.Fatal("unexpected pending size", p1/rhpv2.SectorSize, p2/rhpv2.SectorSize)
	}

	// finishing upload 1 brings it back to 4
	c.finishUpload(uID1)
	p1 = c.pending(fcid2)
	p2 = c.pending(fcid3)
	if p1 != p2 || p1 != 4*rhpv2.SectorSize {
		t.Fatal("unexpected pending size", p1/rhpv2.SectorSize, p2/rhpv2.SectorSize)
	}

	// finishing upload 2 brings it back to 0
	c.finishUpload(uID2)
	p1 = c.pending(fcid2)
	p2 = c.pending(fcid3)
	if p1 != p2 || p1 != 0 {
		t.Fatal("unexpected pending size", p1/rhpv2.SectorSize, p2/rhpv2.SectorSize)
	}
}

func newTestUploadID() api.UploadID {
	var uID api.UploadID
	frand.Read(uID[:])
	return uID
}
