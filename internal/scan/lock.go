package scan

import (
	"encoding/json"
	"os"
)

// Provenance is what the skills CLI lockfile records about an installed
// skill. Fleet reads it only; it never writes the lockfile.
type Provenance struct {
	// Source is the repo or package the skill came from, e.g.
	// "mattpocock/skills" for GitHub installs.
	Source string `json:"source"`
	// SourceType is the source kind, e.g. "github".
	SourceType string `json:"sourceType"`
	// SkillPath is the skill folder's path within the source repo, e.g.
	// "skills/engineering/tdd/SKILL.md". The update check locates the
	// folder in the repo tree with it.
	SkillPath string `json:"skillPath"`
	// Ref is the branch or tag the skill was installed from, empty for the
	// default branch.
	Ref string `json:"ref"`
	// Hash is the skillFolderHash recorded at install/update time.
	Hash string `json:"skillFolderHash"`
	// InstalledAt and UpdatedAt are RFC 3339 timestamps.
	InstalledAt string `json:"installedAt"`
	UpdatedAt   string `json:"updatedAt"`
}

// ReadLockfile parses the skills CLI lockfile. The map is keyed by the
// skill's on-disk (sanitized) directory name — the same key ScanStore's
// Skill.Dir uses. A missing file yields an empty map (every skill is then
// custom); a malformed file is an error rather than a silent loss of
// provenance.
func ReadLockfile(path string) (map[string]Provenance, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]Provenance{}, nil
		}
		return nil, err
	}

	var parsed struct {
		Skills map[string]Provenance `json:"skills"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, err
	}
	if parsed.Skills == nil {
		return map[string]Provenance{}, nil
	}
	return parsed.Skills, nil
}
