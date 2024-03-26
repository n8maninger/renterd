package bus

import (
	"fmt"
	"sync"
	"time"

	"go.sia.tech/core/rhp/v2"
	"go.sia.tech/core/types"
	"go.sia.tech/renterd/api"
)

const (
	// cacheExpiry is the amount of time after which an upload is pruned from
	// the cache, since the workers are expected to finish their uploads this is
	// there to prevent leaking memory, which is why it's set at 24h
	cacheExpiry = 24 * time.Hour
)

type (
	uploadingSectorsCache struct {
		mu          sync.Mutex
		uploads     map[api.UploadID]*ongoingUpload
		renewedFrom map[types.FileContractID]types.FileContractID
		renewedTo   map[types.FileContractID]types.FileContractID
	}

	ongoingUpload struct {
		mu              sync.Mutex
		started         time.Time
		contractSectors map[types.FileContractID][]types.Hash256
	}
)

func newUploadingSectorsCache() *uploadingSectorsCache {
	return &uploadingSectorsCache{
		uploads:     make(map[api.UploadID]*ongoingUpload),
		renewedFrom: make(map[types.FileContractID]types.FileContractID),
		renewedTo:   make(map[types.FileContractID]types.FileContractID),
	}
}

func (ou *ongoingUpload) addSector(fcid types.FileContractID, root types.Hash256) {
	ou.mu.Lock()
	defer ou.mu.Unlock()
	ou.contractSectors[fcid] = append(ou.contractSectors[fcid], root)
}

func (ou *ongoingUpload) sectors(fcid types.FileContractID) (roots []types.Hash256) {
	ou.mu.Lock()
	defer ou.mu.Unlock()
	if sectors, exists := ou.contractSectors[fcid]; exists && time.Since(ou.started) < cacheExpiry {
		roots = append(roots, sectors...)
	}
	return
}

func (usc *uploadingSectorsCache) fcids(fcid types.FileContractID) (types.FileContractID, types.FileContractID) {
	usc.mu.Lock()
	defer usc.mu.Unlock()

	if renewed, ok := usc.renewedTo[fcid]; ok {
		return renewed, fcid
	} else {
		return fcid, usc.renewedFrom[fcid] // might be the default but that is fine
	}
}

func (usc *uploadingSectorsCache) addRenewal(fcid, renewedFrom types.FileContractID) {
	usc.mu.Lock()
	defer usc.mu.Unlock()

	// to prevent leaking memory we delete the grand parent, this is fine as
	// long as a contract doesn't renew twice within the course of one upload
	if prev, ok := usc.renewedFrom[renewedFrom]; ok {
		delete(usc.renewedTo, prev)
		delete(usc.renewedFrom, renewedFrom)
	}

	usc.renewedFrom[fcid] = renewedFrom
	usc.renewedTo[renewedFrom] = fcid
}

func (usc *uploadingSectorsCache) addUploadingSector(uID api.UploadID, fcid types.FileContractID, root types.Hash256) error {
	// fetch ongoing upload
	usc.mu.Lock()
	ongoing, exists := usc.uploads[uID]
	usc.mu.Unlock()

	// add sector if upload exists
	if exists {
		ongoing.addSector(fcid, root)
		return nil
	}

	return fmt.Errorf("%w; id '%v'", api.ErrUnknownUpload, uID)
}

func (usc *uploadingSectorsCache) ongoingUploads() []*ongoingUpload {
	usc.mu.Lock()
	defer usc.mu.Unlock()
	var uploads []*ongoingUpload
	for _, ongoing := range usc.uploads {
		uploads = append(uploads, ongoing)
	}
	return uploads
}

func (usc *uploadingSectorsCache) pending(fcid types.FileContractID) (size uint64) {
	fcid, renewedFrom := usc.fcids(fcid)
	for _, ongoing := range usc.ongoingUploads() {
		size += uint64(len(ongoing.sectors(fcid))) * rhp.SectorSize
		size += uint64(len(ongoing.sectors(renewedFrom))) * rhp.SectorSize
	}
	return
}

func (usc *uploadingSectorsCache) sectors(fcid types.FileContractID) (roots []types.Hash256) {
	fcid, renewedFrom := usc.fcids(fcid)
	for _, ongoing := range usc.ongoingUploads() {
		roots = append(roots, ongoing.sectors(fcid)...)
		roots = append(roots, ongoing.sectors(renewedFrom)...)
	}
	return
}

func (usc *uploadingSectorsCache) finishUpload(uID api.UploadID) {
	usc.mu.Lock()
	defer usc.mu.Unlock()
	delete(usc.uploads, uID)

	// prune expired uploads
	for uID, ongoing := range usc.uploads {
		if time.Since(ongoing.started) > cacheExpiry {
			delete(usc.uploads, uID)
		}
	}
}

func (usc *uploadingSectorsCache) trackUpload(uID api.UploadID) error {
	usc.mu.Lock()
	defer usc.mu.Unlock()

	// check if upload already exists
	if _, exists := usc.uploads[uID]; exists {
		return fmt.Errorf("%w; id '%v'", api.ErrUploadAlreadyExists, uID)
	}

	usc.uploads[uID] = &ongoingUpload{
		started:         time.Now(),
		contractSectors: make(map[types.FileContractID][]types.Hash256),
	}
	return nil
}
