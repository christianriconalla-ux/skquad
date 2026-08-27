package storage

import (
	"regexp"
	"testing"
)

func TestReadMigrationFilesSortsAndChecksumsEmbeddedSQL(t *testing.T) {
	t.Parallel()

	migrations, err := readMigrationFiles()
	if err != nil {
		t.Fatal(err)
	}
	if len(migrations) < 2 {
		t.Fatalf("migration count = %d, want at least 2", len(migrations))
	}
	if got, want := migrations[0].Version, "0001_init.sql"; got != want {
		t.Fatalf("first migration = %q, want %q", got, want)
	}
	if got, want := migrations[1].Version, "0002_schema_migrations.sql"; got != want {
		t.Fatalf("second migration = %q, want %q", got, want)
	}
	checksumPattern := regexp.MustCompile(`^[a-f0-9]{64}$`)
	for _, migration := range migrations {
		if migration.SQL == "" {
			t.Fatalf("migration %s has empty SQL", migration.Version)
		}
		if !checksumPattern.MatchString(migration.Checksum) {
			t.Fatalf("migration %s checksum = %q, want sha256 hex", migration.Version, migration.Checksum)
		}
	}
}

func TestMigrationChecksumChangesWithContent(t *testing.T) {
	t.Parallel()

	first := migrationChecksum([]byte("select 1;"))
	second := migrationChecksum([]byte("select 2;"))
	if first == second {
		t.Fatalf("checksums matched for different SQL: %s", first)
	}
}
