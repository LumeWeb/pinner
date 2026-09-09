package uploads_test

import (
	"context"
	"encoding/json"
	"io/fs"
	"testing"
	"testing/fstest"

	"go.lumeweb.com/pinner/core/uploads"
)

// fakeUploadService pins the Service contract: any implementation over the
// transfer executors (or a concrete SDK service) must accept an
// fs.FS plus name/wait/wrap and return the result model.
type fakeUploadService struct {
	gotFS      fs.FS
	gotName    string
	gotWait    bool
	gotWrap    bool
	wantResult *uploads.UploadResult
}

func (f *fakeUploadService) Upload(ctx context.Context, filesystem fs.FS, name string, wait bool, wrap bool) (*uploads.UploadResult, error) {
	f.gotFS, f.gotName, f.gotWait, f.gotWrap = filesystem, name, wait, wrap
	return f.wantResult, nil
}

// TestServiceContract pins the Upload signature flow and that the result is
// the canonical UploadResult (CID + byte size + duration).
func TestServiceContract(t *testing.T) {
	svc := &fakeUploadService{wantResult: &uploads.UploadResult{CID: "bafyabc", Size: 42, Duration: 1500}}
	fsys := fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("<html></html>")}}

	res, err := svc.Upload(context.Background(), fsys, "site", true, true)
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if res.CID != "bafyabc" || res.Size != 42 || res.Duration != 1500 {
		t.Errorf("result = %+v, want cid bafyabc / size 42 / duration 1500", res)
	}
	if svc.gotName != "site" || !svc.gotWait || !svc.gotWrap {
		t.Errorf("svc got name=%q wait=%v wrap=%v", svc.gotName, svc.gotWait, svc.gotWrap)
	}
	if err := fstest.TestFS(svc.gotFS, "index.html"); err != nil {
		t.Errorf("uploaded fs must be readable: %v", err)
	}
}

// TestUploadResultJSONShape pins the lowercase snake_case wire tags every MCP
// surfaced consumer keys off: {"cid","size","duration"} (location omitempty).
func TestUploadResultJSONShape(t *testing.T) {
	raw, err := json.Marshal(uploads.UploadResult{CID: "cid1", Size: 7, Duration: 2000})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	want := `{"cid":"cid1","size":7,"duration":2000}`
	if string(raw) != want {
		t.Errorf("json = %s, want %s", raw, want)
	}
	raw, err = json.Marshal(uploads.UploadResult{CID: "cid1", Size: 7, Location: "us-east"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !json.Valid(raw) || json.RawMessage(raw) == nil {
		t.Fatalf("json = %s", raw)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded["location"] != "us-east" {
		t.Errorf("location not encoded: %s", raw)
	}
}
