package backup

const (
	reasonInvalidManifest = "Backup manifest is invalid."
	reasonBackupTooOld    = "Backup is older than 24 hours."
	reasonClusterMismatch = "Backup cluster identity does not match."
	reasonNoArtifact      = "Backup has no matching PostgreSQL artifact."
	reasonOffHost         = "Off-host copy is incomplete."
	reasonRestore         = "Restore evidence is missing or stale."
)

func EvaluateDropGate(input DropGateInput) DropGateResult {
	if input.Now.IsZero() || validateManifestStructure(input.Manifest) != nil {
		return denied(reasonInvalidManifest)
	}
	if input.Manifest.CompletedAt.After(input.Now) {
		return denied(reasonInvalidManifest)
	}
	if input.Now.Sub(input.Manifest.CompletedAt) > BackupMaxAge {
		return denied(reasonBackupTooOld)
	}
	if input.Manifest.Cluster.SystemIdentifier != input.SystemIdentifier {
		return denied(reasonClusterMismatch)
	}
	if !hasPostgresArtifact(input.Manifest, input.Database) {
		return denied(reasonNoArtifact)
	}
	if !input.Manifest.OffHost.Completed ||
		input.Manifest.OffHost.CopiedAt.Before(input.Manifest.CompletedAt) ||
		input.Manifest.OffHost.CopiedAt.After(input.Now) {
		return denied(reasonOffHost)
	}
	restore := input.Manifest.Restore
	if !restore.Isolated ||
		restore.Outcome != RestoreOutcomeSucceeded ||
		restore.BackupSetID != input.Manifest.BackupSetID ||
		restore.CompletedAt.IsZero() ||
		restore.CompletedAt.Before(input.Manifest.CompletedAt) ||
		restore.CompletedAt.After(input.Now) ||
		input.Now.Sub(restore.CompletedAt) > RestoreMaxAge {
		return denied(reasonRestore)
	}
	return DropGateResult{Allowed: true}
}

func hasPostgresArtifact(manifest Manifest, database string) bool {
	for _, artifact := range manifest.Artifacts {
		if artifact.Kind != ArtifactKindPostgresDatabase {
			continue
		}
		if artifact.Name != database {
			continue
		}
		if !lowerHex(artifact.SHA256, 64) || artifact.SizeBytes < 0 || !jailLocal(artifact.Path) {
			continue
		}
		return true
	}
	return false
}

func denied(reason string) DropGateResult {
	return DropGateResult{Allowed: false, Reason: reason}
}
