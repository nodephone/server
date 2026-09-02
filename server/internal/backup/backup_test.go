package backup_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nodephone/server/internal/backup"
)

func TestBackupAndRestoreEngine(t *testing.T) {
	tempDataDir, err := os.MkdirTemp("", "backup_engine_test_*")
	if err != nil {
		t.Fatalf("failed to create temp data dir: %v", err)
	}
	defer os.RemoveAll(tempDataDir)

	// Create dummy server state files
	_ = os.WriteFile(filepath.Join(tempDataDir, "main.db"), []byte("SQLite DB Content Mock"), 0644)
	_ = os.WriteFile(filepath.Join(tempDataDir, "config.json"), []byte(`{"app":"test"}`), 0644)
	secDir := filepath.Join(tempDataDir, "secrets")
	_ = os.MkdirAll(secDir, 0755)
	_ = os.WriteFile(filepath.Join(secDir, "jwt.key"), []byte("SecretKey123"), 0644)
	stgDir := filepath.Join(tempDataDir, "storage", "docs")
	_ = os.MkdirAll(stgDir, 0755)
	_ = os.WriteFile(filepath.Join(stgDir, "hello.txt"), []byte("Hello Storage"), 0644)

	engine := backup.NewBackupEngine(tempDataDir, nil)

	// 1. Create Unencrypted Backup
	meta, err := engine.CreateBackup(backup.CreateBackupRequest{Name: "snapshot1"})
	if err != nil {
		t.Fatalf("CreateBackup failed: %v", err)
	}
	if meta.Filename != "snapshot1.npb" {
		t.Errorf("expected filename 'snapshot1.npb', got %q", meta.Filename)
	}
	if meta.Encrypted {
		t.Error("expected Encrypted == false")
	}

	// 2. Create Encrypted Backup
	encMeta, err := engine.CreateBackup(backup.CreateBackupRequest{
		Name:            "snapshot_enc",
		EncryptPassword: "SecurePassword123!",
	})
	if err != nil {
		t.Fatalf("CreateBackup encrypted failed: %v", err)
	}
	if !encMeta.Encrypted {
		t.Error("expected Encrypted == true")
	}

	// 3. List Backups
	list, err := engine.ListBackups()
	if err != nil {
		t.Fatalf("ListBackups failed: %v", err)
	}
	if len(list) != 2 {
		t.Errorf("expected 2 backups in list, got %d", len(list))
	}

	// 4. Verify Archive
	verifier := backup.NewVerifier()
	encPath := filepath.Join(tempDataDir, "backups", "snapshot_enc.npb")
	verifiedMeta, err := verifier.VerifyArchive(encPath, "SecurePassword123!")
	if err != nil {
		t.Fatalf("VerifyArchive failed: %v", err)
	}
	if verifiedMeta.ID != "snapshot_enc" {
		t.Errorf("expected ID 'snapshot_enc', got %q", verifiedMeta.ID)
	}

	// Test incorrect password error
	_, err = verifier.VerifyArchive(encPath, "WrongPassword")
	if err == nil {
		t.Error("expected error for incorrect decryption password")
	}

	// 5. Restore Server State
	err = engine.RestoreBackup(backup.RestoreBackupRequest{
		BackupID:        "snapshot_enc",
		DecryptPassword: "SecurePassword123!",
	})
	if err != nil {
		t.Fatalf("RestoreBackup failed: %v", err)
	}

	// Check restored content
	restoredContent, _ := os.ReadFile(filepath.Join(tempDataDir, "storage", "docs", "hello.txt"))
	if string(restoredContent) != "Hello Storage" {
		t.Errorf("unexpected restored content: %q", string(restoredContent))
	}

	// 6. Delete Backup
	if err := engine.DeleteBackup("snapshot1"); err != nil {
		t.Fatalf("DeleteBackup failed: %v", err)
	}

	listAfterDelete, _ := engine.ListBackups()
	if len(listAfterDelete) != 1 {
		t.Errorf("expected 1 backup after deletion, got %d", len(listAfterDelete))
	}

	// 7. Check Engine Status Metrics
	status := engine.GetStatus()
	if status.TotalBackups != 1 {
		t.Errorf("expected TotalBackups == 1, got %d", status.TotalBackups)
	}
}
