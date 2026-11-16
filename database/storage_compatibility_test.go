package database

import (
	"database/sql"
	"path/filepath"
	"testing"

	duckdb "github.com/duckdb/duckdb-go/v2"
)

func TestDuckDBStorageCompatibility(t *testing.T) {
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "database with spaces.db")
	store := NewStore(path)
	if err := store.Connect(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if _, err := store.DB().ExecContext(ctx, "CREATE TABLE probe(id INTEGER PRIMARY KEY); INSERT INTO probe VALUES (1)"); err != nil {
		t.Fatal(err)
	}
	assertStorageVersion(t, store.DB(), "v1.0.0+")
	attached := filepath.Join(t.TempDir(), "attached.db")
	if _, err := store.DB().ExecContext(ctx, "ATTACH '"+escapeSQLString(attached)+"' AS attached; CREATE TABLE attached.probe(id INTEGER)"); err != nil {
		t.Fatal(err)
	}
	var version string
	if err := store.DB().QueryRowContext(ctx, "SELECT tags['storage_version'] FROM duckdb_databases() WHERE database_name='attached'").Scan(&version); err != nil || version != "v1.0.0+" {
		t.Fatalf("attached storage version = %q, error = %v", version, err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if err := store.Connect(); err != nil {
		t.Fatal(err)
	}
	assertStorageVersion(t, store.DB(), "v1.0.0+")
}

func TestDuckDBStorageCompatibilityPreservesExistingFormat(t *testing.T) {
	path := filepath.Join(t.TempDir(), "existing.db")
	connector, err := duckdb.NewConnector(path+"?storage_compatibility_version=v1.4.0", nil)
	if err != nil {
		t.Fatal(err)
	}
	db := sql.OpenDB(connector)
	if _, err := db.ExecContext(t.Context(), "CREATE TABLE probe(id INTEGER); INSERT INTO probe VALUES (1); CHECKPOINT"); err != nil {
		t.Fatal(err)
	}
	assertStorageVersion(t, db, "v1.4.0+")
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	store := NewStore(path)
	if err := store.Connect(); err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := store.DB().ExecContext(t.Context(), "INSERT INTO probe VALUES (2); CHECKPOINT"); err != nil {
		t.Fatal(err)
	}
	assertStorageVersion(t, store.DB(), "v1.4.0+")
}

func assertStorageVersion(t *testing.T, db *sql.DB, want string) {
	t.Helper()
	var got string
	if err := db.QueryRowContext(t.Context(), "SELECT tags['storage_version'] FROM duckdb_databases() WHERE database_name=current_database()").Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("storage version = %q, want %q", got, want)
	}
}
