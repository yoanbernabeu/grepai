package trace

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"

	"github.com/jackc/pgx/v5"
)

func advisoryKey(namespace string, values ...string) (int32, int32) {
	hash := sha256.New()
	_, _ = hash.Write([]byte(namespace))
	for _, value := range values {
		var size [8]byte
		binary.BigEndian.PutUint64(size[:], uint64(len(value)))
		_, _ = hash.Write(size[:])
		_, _ = hash.Write([]byte(value))
	}
	digest := hash.Sum(nil)
	return advisoryKeyPart(digest[:4]), advisoryKeyPart(digest[4:8])
}

func advisoryKeyPart(value []byte) int32 {
	return int32(value[0])<<24 | int32(value[1])<<16 | int32(value[2])<<8 | int32(value[3])
}

func migrationAdvisoryKey(projectID string) (int32, int32) {
	return advisoryKey("grepai:symbol-migration", projectID)
}

func fileMutationAdvisoryKey(projectID, filePath string) (int32, int32) {
	return advisoryKey("grepai:symbol-file", projectID, filePath)
}

func (s *PostgresSymbolStore) lockFileMutation(ctx context.Context, tx pgx.Tx, operation, filePath string) error {
	key1, key2 := fileMutationAdvisoryKey(s.projectID, filePath)
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1,$2)`, key1, key2); err != nil {
		return fmt.Errorf("failed to acquire symbol file mutation lock: %w", err)
	}
	if s.mutationHook != nil {
		if err := s.mutationHook(operation, filePath); err != nil {
			return fmt.Errorf("symbol file mutation hook failed: %w", err)
		}
	}
	return nil
}
