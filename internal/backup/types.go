package backup

import (
	"time"
)

const (
	SchemaVersion                = 1
	ArtifactKindPostgresDatabase = "postgres.database"
	RestoreOutcomeSucceeded      = "succeeded"
	BackupMaxAge                 = 24 * time.Hour
	RestoreMaxAge                = 30 * 24 * time.Hour
)

type Manifest struct {
	SchemaVersion int             `json:"schema_version"`
	BackupSetID   string          `json:"backup_set_id"`
	CompletedAt   time.Time       `json:"completed_at"`
	Cluster       ClusterIdentity `json:"cluster"`
	Artifacts     []Artifact      `json:"artifacts"`
	OffHost       OffHost         `json:"off_host"`
	Restore       RestoreEvidence `json:"restore"`
	Redgres       RedgresIdentity `json:"redgres"`
}

type ClusterIdentity struct {
	SystemIdentifier string `json:"system_identifier"`
}

type Artifact struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	SHA256    string `json:"sha256"`
	SizeBytes int64  `json:"size_bytes"`
	Path      string `json:"path"`
}

type OffHost struct {
	Completed bool      `json:"completed"`
	CopiedAt  time.Time `json:"copied_at"`
}

type RestoreEvidence struct {
	Isolated    bool      `json:"isolated"`
	Outcome     string    `json:"outcome"`
	BackupSetID string    `json:"backup_set_id"`
	CompletedAt time.Time `json:"completed_at"`
}

type RedgresIdentity struct {
	Version                     string `json:"version"`
	CompatibilityPolicyRevision string `json:"compatibility_policy_revision"`
}

type DropGateInput struct {
	Database         string
	SystemIdentifier string
	Now              time.Time
	Manifest         Manifest
}

type DropGateResult struct {
	Allowed bool
	Reason  string
}
