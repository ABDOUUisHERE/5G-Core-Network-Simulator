package integration

import (
	"context"
	corev1 "core5g/api/gen/core/v1"
	"core5g/internal/store"
	"database/sql"
	"errors"
	"github.com/google/uuid"
	"os"
	"strings"
	"testing"
)

func TestPostgresConstraintsAndOwnership(t *testing.T) {
	url := os.Getenv("CORE5G_TEST_POSTGRES")
	if url == "" {
		t.Skip("set CORE5G_TEST_POSTGRES for real PostgreSQL integration")
	}
	ctx := context.Background()
	db, err := sql.Open("pgx", url)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	smf, err := store.NewPostgres(ctx, strings.Replace(url, "core5g:local-lab-admin", "core5g_smf:local-lab-smf", 1), "smf")
	if err != nil {
		t.Fatal(err)
	}
	defer smf.Close()
	one := &corev1.Session{SessionId: uuid.NewString(), RequestId: uuid.NewString(), UeId: uuid.NewString(), IpAddress: "10.99.255.254", State: "ACTIVE"}
	two := &corev1.Session{SessionId: uuid.NewString(), RequestId: uuid.NewString(), UeId: one.UeId, IpAddress: one.IpAddress, State: "ACTIVE"}
	defer db.ExecContext(ctx, "DELETE FROM smf.sessions WHERE id IN ($1,$2)", one.SessionId, two.SessionId)
	if err = smf.Run(ctx, func(tx *store.Tx) error { return tx.Put("sessions", one.SessionId, one) }); err != nil {
		t.Fatal(err)
	}
	if err = smf.Run(ctx, func(tx *store.Tx) error { return tx.Put("sessions", two.SessionId, two) }); err == nil {
		t.Fatal("duplicate active IP accepted")
	}
	one.State = "RELEASED"
	if err = smf.Run(ctx, func(tx *store.Tx) error { return tx.Put("sessions", one.SessionId, one) }); err != nil {
		t.Fatal(err)
	}
	if err = smf.Run(ctx, func(tx *store.Tx) error { return tx.Put("sessions", two.SessionId, two) }); err != nil {
		t.Fatal("released IP not reusable", err)
	}
	rollback := errors.New("rollback")
	if err = smf.Run(ctx, func(tx *store.Tx) error {
		two.State = "FAILED"
		if err := tx.Put("sessions", two.SessionId, two); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatal(err)
	}
	if err = smf.Run(ctx, func(tx *store.Tx) error {
		var got corev1.Session
		_, err := tx.Get("sessions", two.SessionId, &got)
		if got.State != "ACTIVE" {
			t.Fatal("rollback failed", &got)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	restricted, err := sql.Open("pgx", strings.Replace(url, "core5g:local-lab-admin", "core5g_udm:local-lab-udm", 1))
	if err != nil {
		t.Fatal(err)
	}
	defer restricted.Close()
	if _, err = restricted.ExecContext(ctx, "SELECT data FROM smf.sessions LIMIT 1"); err == nil {
		t.Fatal("UDM can read SMF tables")
	}
}
