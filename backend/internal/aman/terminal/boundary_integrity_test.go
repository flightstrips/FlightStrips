package terminal

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

const (
	ekchBoundaryCommit = "d860ed77135b057168148184880a41cc183bf881"
	ekchBoundarySHA256 = "818b9417a444fec91e43a2e9d98f147d027fd85d78b4b7cd2d589895d89d203a"
)

func TestVendoredEKCHBoundaryIntegrity(t *testing.T) {
	directory := ekchBoundaryDirectory(t)
	boundary, err := os.ReadFile(filepath.Join(directory, "EKCH.json"))
	require.NoError(t, err)
	sum := sha256.Sum256(boundary)
	require.Equal(t, ekchBoundarySHA256, hex.EncodeToString(sum[:]))

	var provenance struct {
		SchemaVersion    int    `json:"schemaVersion"`
		Artifact         string `json:"artifact"`
		SourceRepository string `json:"sourceRepository"`
		SourcePath       string `json:"sourcePath"`
		SourceCommit     string `json:"sourceCommit"`
		SourceURL        string `json:"sourceUrl"`
		UpstreamGitBlob  string `json:"upstreamGitBlob"`
		SHA256           string `json:"sha256"`
		ImportedAt       string `json:"importedAt"`
		ImportMethod     string `json:"importMethod"`
	}
	metadata, err := os.ReadFile(filepath.Join(directory, "EKCH.provenance.json"))
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(metadata, &provenance))
	require.Equal(t, 1, provenance.SchemaVersion)
	require.Equal(t, "EKCH.json", provenance.Artifact)
	require.Equal(t, "https://github.com/vatsimnetwork/simaware-tracon-project", provenance.SourceRepository)
	require.Equal(t, "Boundaries/EKCH/EKCH.json", provenance.SourcePath)
	require.Equal(t, ekchBoundaryCommit, provenance.SourceCommit)
	require.Equal(t, "https://raw.githubusercontent.com/vatsimnetwork/simaware-tracon-project/"+ekchBoundaryCommit+"/Boundaries/EKCH/EKCH.json", provenance.SourceURL)
	require.Equal(t, "cb103b63566b09b69e5da24e83f9cd0a550705fb", provenance.UpstreamGitBlob)
	require.Equal(t, ekchBoundarySHA256, provenance.SHA256)
	require.Equal(t, "2026-09-11", provenance.ImportedAt)
	require.Equal(t, "Byte-for-byte copy from sourceUrl; no geometry transformation", provenance.ImportMethod)

	var feature struct {
		Type       string `json:"type"`
		Properties struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"properties"`
		Geometry struct {
			Type        string          `json:"type"`
			Coordinates json.RawMessage `json:"coordinates"`
		} `json:"geometry"`
	}
	require.NoError(t, json.Unmarshal(boundary, &feature))
	require.Equal(t, "Feature", feature.Type)
	require.Equal(t, "EKCH", feature.Properties.ID)
	require.Equal(t, "Copenhagen Approach", feature.Properties.Name)
	require.Equal(t, "MultiPolygon", feature.Geometry.Type)
	require.NotEqual(t, json.RawMessage("null"), feature.Geometry.Coordinates)
}

func ekchBoundaryDirectory(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "..", "config", "aman", "boundaries"))
}
