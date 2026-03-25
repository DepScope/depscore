// internal/actions/scriptdetect_test.go
package actions

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDetectScriptDownloads(t *testing.T) {
	tests := []struct {
		name   string
		script string
		want   int // number of downloads detected
	}{
		{"curl pipe bash", "curl -sSL https://example.com/install.sh | bash", 1},
		{"wget pipe sh", "wget -O- https://example.com/setup.sh | sh", 1},
		{"curl pipe python", "curl https://example.com/script.py | python3", 1},
		{"download then execute", "curl -o install.sh https://example.com/install.sh\nsh install.sh", 1},
		{"safe curl", "curl -o output.json https://api.example.com/data", 0},
		{"no downloads", "echo hello\nnpm test", 0},
		{"multiple", "curl https://a.com/x | bash\nwget https://b.com/y | sh", 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DetectScriptDownloads(tt.script)
			assert.Len(t, got, tt.want)
		})
	}
}

func TestScriptDownloadURL(t *testing.T) {
	downloads := DetectScriptDownloads("curl -sSL https://install.example.com/setup.sh | bash")
	assert.Len(t, downloads, 1)
	assert.Equal(t, "https://install.example.com/setup.sh", downloads[0].URL)
}
