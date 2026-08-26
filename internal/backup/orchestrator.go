package backup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/SSujitX/redgres/internal/securefile"
)

const (
	maxBackupArtifacts   = 1_024
	maxArtifactKindBytes = 64
	maxArtifactNameBytes = 512
	maxArtifactPathBytes = 4_096
	maxRetentionSets     = 1_024
)

var (
	ErrInvalidOrchestrator       = errors.New("backup orchestrator is not configured")
	ErrInvalidBackupRequest      = errors.New("backup request is invalid")
	ErrPostgresCaptureFailed     = errors.New("PostgreSQL backup capture failed")
	ErrRedisCaptureFailed        = errors.New("Redis backup capture failed")
	ErrSQLiteCaptureFailed       = errors.New("SQLite backup capture failed")
	ErrConfigCaptureFailed       = errors.New("configuration backup capture failed")
	ErrArtifactVerification      = errors.New("backup artifact verification failed")
	ErrOffHostCopyFailed         = errors.New("off-host backup copy failed")
	ErrRestoreVerificationFailed = errors.New("isolated restore verification failed")
	ErrRetentionPlanFailed       = errors.New("backup retention planning failed")
	ErrCatalogPublicationFailed  = errors.New("backup catalog publication failed")
)

// PostgreSQLBackup captures globals and per-database logical dumps and performs
// the adapter-owned structural checks required by the installed PostgreSQL tools.
type PostgreSQLBackup interface {
	CaptureLogical(context.Context, CaptureTarget) ([]ProducedArtifact, error)
}

// RedisBackup performs an atomic persistence capture, including the applicable
// RDB/AOF material, ACL file, and sanitized persistence metadata.
type RedisBackup interface {
	CaptureAtomic(context.Context, CaptureTarget) ([]ProducedArtifact, error)
}

// SQLiteBackup uses the SQLite online-backup mechanism and verifies integrity
// before returning its produced artifact.
type SQLiteBackup interface {
	CaptureOnline(context.Context, CaptureTarget) ([]ProducedArtifact, error)
}

// ConfigBackup captures only non-secret configuration and reconstruction data.
type ConfigBackup interface {
	CaptureSanitized(context.Context, CaptureTarget) ([]ProducedArtifact, error)
}

// OffHostBackup copies the complete snapshot using adapter-owned encryption and
// transport. A nil error is not enough: returned evidence is validated by Run.
type OffHostBackup interface {
	CopyEncrypted(context.Context, Snapshot) (OffHost, error)
}

// RestoreBackup restores the complete snapshot into an isolated target and
// returns the schema-v1 evidence only after adapter-owned logical checks pass.
type RestoreBackup interface {
	VerifyIsolated(context.Context, Snapshot) (RestoreEvidence, error)
}

// RetentionPlanner returns a bounded, non-executing plan. Run validates that
// every candidate is a manifest-set directory and never removes paths itself.
type RetentionPlanner interface {
	Plan(context.Context, RetentionInput) (RetentionPlan, error)
}

type OrchestratorAdapters struct {
	PostgreSQL PostgreSQLBackup
	Redis      RedisBackup
	SQLite     SQLiteBackup
	Config     ConfigBackup
	OffHost    OffHostBackup
	Restore    RestoreBackup
	Retention  RetentionPlanner
	Now        func() time.Time
}

type CaptureTarget struct {
	Root string
}

// ProducedArtifact names an existing regular file below CaptureTarget.Root.
// Path is relative to that root; adapters never choose a catalog destination.
type ProducedArtifact struct {
	Kind string
	Name string
	Path string
}

type SnapshotArtifact struct {
	Kind      string
	Name      string
	Path      string
	SHA256    string
	SizeBytes int64
}

// Snapshot is the immutable view supplied to evidence adapters. Artifact paths
// are relative to Root and have already been streamed and identity-checked.
type Snapshot struct {
	BackupSetID      string
	CompletedAt      time.Time
	SystemIdentifier string
	Redgres          RedgresIdentity
	Root             string
	Artifacts        []SnapshotArtifact
}

type RetentionInput struct {
	CatalogDir string
	Next       Manifest
}

// RetentionPlan contains set IDs, not arbitrary filesystem paths. Execution is
// deliberately outside this module because schema v1 has no retention evidence.
type RetentionPlan struct {
	RemoveSetIDs []string
}

type BackupRequest struct {
	CatalogDir       string
	BackupSetID      string
	SystemIdentifier string
	Redgres          RedgresIdentity
}

type BackupResult struct {
	Manifest  Manifest
	Retention RetentionPlan
}

type Orchestrator struct {
	adapters OrchestratorAdapters
}

func NewOrchestrator(adapters OrchestratorAdapters) (*Orchestrator, error) {
	if adapterMissing(adapters.PostgreSQL) || adapterMissing(adapters.Redis) || adapterMissing(adapters.SQLite) ||
		adapterMissing(adapters.Config) || adapterMissing(adapters.OffHost) || adapterMissing(adapters.Restore) ||
		adapterMissing(adapters.Retention) || adapters.Now == nil {
		return nil, ErrInvalidOrchestrator
	}
	return &Orchestrator{adapters: adapters}, nil
}

func adapterMissing(adapter any) bool {
	if adapter == nil {
		return true
	}
	value := reflect.ValueOf(adapter)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

// Run creates one complete backup set. current.json is replaced by a same-dir
// rename only after every capture, checksum, external copy, isolated restore,
// and retention-plan validation succeeds.
func (orchestrator *Orchestrator) Run(ctx context.Context, request BackupRequest) (BackupResult, error) {
	if orchestrator == nil || ctx == nil {
		return BackupResult{}, ErrInvalidBackupRequest
	}
	if err := ctx.Err(); err != nil {
		return BackupResult{}, err
	}
	if err := validateBackupRequest(request); err != nil {
		return BackupResult{}, err
	}
	if err := validateCatalogRoot(request.CatalogDir); err != nil {
		return BackupResult{}, ErrInvalidBackupRequest
	}

	stageRoot, err := os.MkdirTemp(request.CatalogDir, ".pending-"+request.BackupSetID+"-")
	if err != nil {
		return BackupResult{}, ErrCatalogPublicationFailed
	}
	removeStage := true
	defer func() {
		if removeStage {
			removeBackupTree(request.CatalogDir, stageRoot)
		}
	}()

	var captured []SnapshotArtifact
	stages := []struct {
		name    string
		capture func(context.Context, CaptureTarget) ([]ProducedArtifact, error)
		failure error
	}{
		{name: "postgres", capture: orchestrator.adapters.PostgreSQL.CaptureLogical, failure: ErrPostgresCaptureFailed},
		{name: "redis", capture: orchestrator.adapters.Redis.CaptureAtomic, failure: ErrRedisCaptureFailed},
		{name: "sqlite", capture: orchestrator.adapters.SQLite.CaptureOnline, failure: ErrSQLiteCaptureFailed},
		{name: "config", capture: orchestrator.adapters.Config.CaptureSanitized, failure: ErrConfigCaptureFailed},
	}
	identities := make(map[string]struct{})
	paths := make(map[string]struct{})
	for _, stage := range stages {
		if err := ctx.Err(); err != nil {
			return BackupResult{}, err
		}
		root := filepath.Join(stageRoot, stage.name)
		if err := securefile.EnsureRealDir(root, 0o700); err != nil {
			return BackupResult{}, stage.failure
		}
		produced, captureErr := stage.capture(ctx, CaptureTarget{Root: root})
		if captureErr != nil {
			return BackupResult{}, contextOr(ctx, stage.failure)
		}
		if len(produced) == 0 || len(captured)+len(produced) > maxBackupArtifacts {
			return BackupResult{}, stage.failure
		}
		for _, artifact := range produced {
			verified, verifyErr := verifyProducedArtifact(ctx, root, stage.name, artifact)
			if verifyErr != nil {
				return BackupResult{}, contextOr(ctx, ErrArtifactVerification)
			}
			identity := verified.Kind + "\x00" + verified.Name
			if _, duplicate := identities[identity]; duplicate {
				return BackupResult{}, ErrArtifactVerification
			}
			if _, duplicate := paths[verified.Path]; duplicate {
				return BackupResult{}, ErrArtifactVerification
			}
			identities[identity] = struct{}{}
			paths[verified.Path] = struct{}{}
			captured = append(captured, verified)
		}
	}

	completedAt := orchestrator.adapters.Now().UTC()
	if completedAt.IsZero() {
		return BackupResult{}, ErrInvalidBackupRequest
	}
	snapshot := Snapshot{
		BackupSetID:      request.BackupSetID,
		CompletedAt:      completedAt,
		SystemIdentifier: request.SystemIdentifier,
		Redgres:          request.Redgres,
		Root:             stageRoot,
		Artifacts:        append([]SnapshotArtifact(nil), captured...),
	}
	offHost, err := orchestrator.adapters.OffHost.CopyEncrypted(ctx, snapshot)
	if err != nil {
		return BackupResult{}, contextOr(ctx, ErrOffHostCopyFailed)
	}
	if !offHost.Completed || offHost.CopiedAt.IsZero() || offHost.CopiedAt.Before(completedAt) {
		return BackupResult{}, ErrOffHostCopyFailed
	}
	restore, err := orchestrator.adapters.Restore.VerifyIsolated(ctx, snapshot)
	if err != nil {
		return BackupResult{}, contextOr(ctx, ErrRestoreVerificationFailed)
	}
	if !restore.Isolated || restore.Outcome != RestoreOutcomeSucceeded ||
		restore.BackupSetID != request.BackupSetID || restore.CompletedAt.IsZero() {
		return BackupResult{}, ErrRestoreVerificationFailed
	}
	if err := reverifySnapshot(ctx, stageRoot, captured); err != nil {
		return BackupResult{}, contextOr(ctx, ErrArtifactVerification)
	}

	manifest := Manifest{
		SchemaVersion: SchemaVersion,
		BackupSetID:   request.BackupSetID,
		CompletedAt:   completedAt,
		Cluster:       ClusterIdentity{SystemIdentifier: request.SystemIdentifier},
		OffHost:       offHost,
		Restore:       restore,
		Redgres:       request.Redgres,
		Artifacts:     make([]Artifact, 0, len(captured)),
	}
	setRelative := filepath.ToSlash(filepath.Join("sets", request.BackupSetID))
	for _, artifact := range captured {
		manifest.Artifacts = append(manifest.Artifacts, Artifact{
			Kind:      artifact.Kind,
			Name:      artifact.Name,
			SHA256:    artifact.SHA256,
			SizeBytes: artifact.SizeBytes,
			Path:      filepath.ToSlash(filepath.Join(setRelative, filepath.FromSlash(artifact.Path))),
		})
	}
	plan, err := orchestrator.adapters.Retention.Plan(ctx, RetentionInput{CatalogDir: request.CatalogDir, Next: manifest})
	if err != nil {
		return BackupResult{}, contextOr(ctx, ErrRetentionPlanFailed)
	}
	if err := validateRetentionPlan(plan, request.BackupSetID); err != nil {
		return BackupResult{}, ErrRetentionPlanFailed
	}
	if err := ctx.Err(); err != nil {
		return BackupResult{}, err
	}

	setsRoot := filepath.Join(request.CatalogDir, "sets")
	if err := securefile.EnsureRealDir(setsRoot, 0o700); err != nil {
		return BackupResult{}, ErrCatalogPublicationFailed
	}
	finalRoot := filepath.Join(setsRoot, request.BackupSetID)
	if _, err := os.Lstat(finalRoot); !errors.Is(err, os.ErrNotExist) {
		return BackupResult{}, ErrCatalogPublicationFailed
	}
	if err := os.Rename(stageRoot, finalRoot); err != nil {
		return BackupResult{}, ErrCatalogPublicationFailed
	}
	removeStage = false
	published := false
	defer func() {
		if !published {
			removeBackupTree(request.CatalogDir, finalRoot)
		}
	}()
	if err := validateParsedManifest(request.CatalogDir, manifest); err != nil {
		return BackupResult{}, ErrCatalogPublicationFailed
	}
	if err := publishCurrent(ctx, request.CatalogDir, manifest); err != nil {
		return BackupResult{}, contextOr(ctx, ErrCatalogPublicationFailed)
	}
	published = true
	return BackupResult{Manifest: manifest, Retention: plan}, nil
}

func validateBackupRequest(request BackupRequest) error {
	if !lowerHex(request.BackupSetID, 32) || !decimalString(request.SystemIdentifier) {
		return ErrInvalidBackupRequest
	}
	if request.CatalogDir == "" || len(request.CatalogDir) > maxArtifactPathBytes ||
		!filepath.IsAbs(request.CatalogDir) || strings.ContainsRune(request.CatalogDir, 0) ||
		strings.ContainsAny(request.CatalogDir, "?#%") {
		return ErrInvalidBackupRequest
	}
	if len(request.Redgres.Version) > maxArtifactNameBytes ||
		len(request.Redgres.CompatibilityPolicyRevision) > maxArtifactNameBytes ||
		!utf8.ValidString(request.Redgres.Version) ||
		!utf8.ValidString(request.Redgres.CompatibilityPolicyRevision) {
		return ErrInvalidBackupRequest
	}
	return nil
}

func validateCatalogRoot(root string) error {
	info, err := os.Lstat(root)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return ErrInvalidBackupRequest
	}
	// The final component check above preserves the contract that callers must
	// create the catalog. EnsureRealDir then verifies every existing ancestor
	// with Lstat without creating anything, so a symlink cannot retarget staging.
	if err := securefile.EnsureRealDir(root, 0o700); err != nil {
		return ErrInvalidBackupRequest
	}
	return nil
}

func verifyProducedArtifact(ctx context.Context, root, stage string, produced ProducedArtifact) (SnapshotArtifact, error) {
	if err := ctx.Err(); err != nil {
		return SnapshotArtifact{}, err
	}
	if !validArtifactText(produced.Kind, maxArtifactKindBytes) ||
		!validArtifactText(produced.Name, maxArtifactNameBytes) ||
		len(produced.Path) > maxArtifactPathBytes || !jailLocal(produced.Path) {
		return SnapshotArtifact{}, ErrArtifactVerification
	}
	path := filepath.Join(root, filepath.FromSlash(strings.ReplaceAll(produced.Path, `\`, "/")))
	rel, err := filepath.Rel(root, path)
	if err != nil || !filepath.IsLocal(rel) {
		return SnapshotArtifact{}, ErrArtifactVerification
	}
	file, err := securefile.OpenRegular(path, os.O_RDONLY, 0)
	if err != nil {
		return SnapshotArtifact{}, ErrArtifactVerification
	}
	defer file.Close()
	size, sum, err := streamArtifact(ctx, file)
	if err != nil {
		return SnapshotArtifact{}, err
	}
	if err := securefile.VerifyRegularPath(path, file); err != nil {
		return SnapshotArtifact{}, ErrArtifactVerification
	}
	return SnapshotArtifact{
		Kind:      produced.Kind,
		Name:      produced.Name,
		Path:      filepath.ToSlash(filepath.Join(stage, rel)),
		SHA256:    hex.EncodeToString(sum[:]),
		SizeBytes: size,
	}, nil
}

func validArtifactText(value string, max int) bool {
	return value != "" && len(value) <= max && utf8.ValidString(value) &&
		value == strings.TrimSpace(value) && !strings.ContainsRune(value, 0)
}

func streamArtifact(ctx context.Context, file *os.File) (int64, [sha256.Size]byte, error) {
	hasher := sha256.New()
	buffer := make([]byte, 64*1024)
	var size int64
	for {
		if err := ctx.Err(); err != nil {
			return 0, [sha256.Size]byte{}, err
		}
		n, readErr := file.Read(buffer)
		if n > 0 {
			size += int64(n)
			_, _ = hasher.Write(buffer[:n])
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				break
			}
			return 0, [sha256.Size]byte{}, ErrArtifactVerification
		}
	}
	var sum [sha256.Size]byte
	copy(sum[:], hasher.Sum(nil))
	return size, sum, nil
}

func reverifySnapshot(ctx context.Context, root string, artifacts []SnapshotArtifact) error {
	for _, artifact := range artifacts {
		path := filepath.Join(root, filepath.FromSlash(artifact.Path))
		file, err := securefile.OpenRegular(path, os.O_RDONLY, 0)
		if err != nil {
			return ErrArtifactVerification
		}
		size, sum, streamErr := streamArtifact(ctx, file)
		identityErr := securefile.VerifyRegularPath(path, file)
		closeErr := file.Close()
		if streamErr != nil {
			return streamErr
		}
		if identityErr != nil || closeErr != nil || size != artifact.SizeBytes ||
			hex.EncodeToString(sum[:]) != artifact.SHA256 {
			return ErrArtifactVerification
		}
	}
	return nil
}

func validateRetentionPlan(plan RetentionPlan, nextSetID string) error {
	if len(plan.RemoveSetIDs) > maxRetentionSets {
		return ErrRetentionPlanFailed
	}
	seen := make(map[string]struct{}, len(plan.RemoveSetIDs))
	for _, setID := range plan.RemoveSetIDs {
		if !lowerHex(setID, 32) || setID == nextSetID {
			return ErrRetentionPlanFailed
		}
		if _, duplicate := seen[setID]; duplicate {
			return ErrRetentionPlanFailed
		}
		seen[setID] = struct{}{}
	}
	return nil
}

func publishCurrent(ctx context.Context, catalogDir string, manifest Manifest) error {
	raw, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return ErrCatalogPublicationFailed
	}
	raw = append(raw, '\n')
	if _, err := parseManifestBytes(catalogDir, raw); err != nil {
		return ErrCatalogPublicationFailed
	}
	temp, err := os.CreateTemp(catalogDir, ".current-")
	if err != nil {
		return ErrCatalogPublicationFailed
	}
	tempPath := temp.Name()
	removeTemp := true
	defer func() {
		_ = temp.Close()
		if removeTemp {
			_ = os.Remove(tempPath)
		}
	}()
	if err := temp.Chmod(0o600); err != nil {
		return ErrCatalogPublicationFailed
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, err := temp.Write(raw); err != nil {
		return ErrCatalogPublicationFailed
	}
	if err := temp.Sync(); err != nil {
		return ErrCatalogPublicationFailed
	}
	if err := temp.Close(); err != nil {
		return ErrCatalogPublicationFailed
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.Rename(tempPath, filepath.Join(catalogDir, currentManifestName)); err != nil {
		return ErrCatalogPublicationFailed
	}
	removeTemp = false
	return nil
}

func contextOr(ctx context.Context, fallback error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return fallback
}

func removeBackupTree(catalogDir, target string) {
	catalogAbs, err := filepath.Abs(catalogDir)
	if err != nil {
		return
	}
	targetAbs, err := filepath.Abs(target)
	if err != nil {
		return
	}
	rel, err := filepath.Rel(catalogAbs, targetAbs)
	if err != nil || !filepath.IsLocal(rel) || rel == "." {
		return
	}
	base := filepath.Base(targetAbs)
	isPending := filepath.Dir(targetAbs) == catalogAbs && strings.HasPrefix(base, ".pending-")
	isSet := filepath.Dir(filepath.Dir(targetAbs)) == catalogAbs && filepath.Base(filepath.Dir(targetAbs)) == "sets" && lowerHex(base, 32)
	if !isPending && !isSet {
		return
	}
	_ = os.RemoveAll(targetAbs)
}
