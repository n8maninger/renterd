package stores

import (
	"gorm.io/gorm"
)

type dbHostBlocklistEntryHost struct {
	DBBlocklistEntryID uint8 `gorm:"primarykey;column:db_blocklist_entry_id"`
	DBHostID           uint8 `gorm:"primarykey;index:idx_db_host_id;column:db_host_id"`
}

func (dbHostBlocklistEntryHost) TableName() string {
	return "host_blocklist_entry_hosts"
}

func performMigrations(tx *gorm.DB) error {
	m := tx.Migrator()

	// Perform pre-auto migrations
	//
	// If the consensus info table is missing the height column, drop it to
	// force a resync.
	if m.HasTable(&dbConsensusInfo{}) && !m.HasColumn(&dbConsensusInfo{}, "height") {
		if err := m.DropTable(&dbConsensusInfo{}); err != nil {
			return err
		}
	}
	// If the shards table exists, we add the db_slab_id column to slices and
	// sectors before then dropping the shards table as well as the db_slice_id
	// column from the slabs table.
	if m.HasTable("shards") {
		// add db_slab_id column to slices.
		if err := m.AddColumn(&dbSlice{}, "db_slab_id"); err != nil {
			return err
		}
		if err := tx.Exec(`UPDATE slices sli SET sli.db_slab_id=(
			SELECT sla.id FROM slabs sla WHERE sla.db_slice_id=sli.id)`).Error; err != nil {
			return err
		}
		// add db_slab_id column to sectors.
		if err := m.AddColumn(&dbSector{}, "db_slab_id"); err != nil {
			return err
		}
		if err := tx.Exec(`UPDATE sectors sec SET sec.db_slab_id=(
			SELECT sha.db_slab_id FROM shards sha WHERE sha.db_sector_id=sec.id)`).Error; err != nil {
			return err
		}
		// drop column db_slice_id from slabs.
		if err := m.DropColumn(&dbSlab{}, "db_slice_id"); err != nil {
			return err
		}
		// drop table shards.
		if err := m.DropTable("shards"); err != nil {
			return err
		}
	}

	// Perform auto migrations.
	tables := []interface{}{
		// bus.MetadataStore tables
		&dbArchivedContract{},
		&dbContract{},
		&dbContractSet{},
		&dbObject{},
		&dbSector{},
		&dbSlab{},
		&dbSlice{},

		// bus.HostDB tables
		&dbAnnouncement{},
		&dbConsensusInfo{},
		&dbHost{},
		&dbInteraction{},
		&dbAllowlistEntry{},
		&dbBlocklistEntry{},

		// wallet tables
		&dbSiacoinElement{},
		&dbTransaction{},

		// bus.SettingStore tables
		&dbSetting{},

		// bus.EphemeralAccountStore tables
		&dbAccount{},
	}
	if err := tx.AutoMigrate(tables...); err != nil {
		return err
	}

	// Perform post-auto migrations.
	if err := m.DropTable("host_sectors"); err != nil {
		return err
	}
	if !m.HasIndex(&dbHostBlocklistEntryHost{}, "DBHostID") {
		if err := m.CreateIndex(&dbHostBlocklistEntryHost{}, "DBHostID"); err != nil {
			return err
		}
	}
	return nil
}
