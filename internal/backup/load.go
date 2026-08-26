package backup

const currentManifestName = "current.json"

func LoadCurrent(catalogDir string) (Manifest, error) {
	return ParseManifest(catalogDir, currentManifestName)
}
