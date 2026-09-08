package pinnermcp

import (
	"context"
	"encoding/json"
	"io"
	"testing"

	"github.com/invopop/jsonschema"
	"github.com/stretchr/testify/require"

	"go.lumeweb.com/canimcp"
	"go.lumeweb.com/mcpforge"
	"go.lumeweb.com/mcpplane/model"
	"go.lumeweb.com/mcpplane/toolargs"
	"go.lumeweb.com/mcpplane/transfer"
)

// Characterization tests for the transfer-tool DESCRIPTOR halves, pinned
// against pinner-cli internal/mcp/core/transfer/{upload_file,upload_data,
// download_file}.go and toolforge/tools.go: per-profile descriptions, the
// feature-gated input schemas (source.mode enum, sink enum, file handoff),
// and the archive_mode transform. Test shapes mirror the pinner-cli suites
// (schema_enum_test.go, grok_upload_chooser_test.go, upload_file_openai_test.go).

func testProfileForTransport(t canimcp.TransportKind) mcpforge.FeatureSet {
	return profileForTransport(t).Features
}

func sourceModeEnumOf(t *testing.T, raw json.RawMessage) []string {
	t.Helper()
	var shaped struct {
		Properties struct {
			Source struct {
				Properties struct {
					Mode struct {
						Enum []string `json:"enum"`
					} `json:"mode"`
				} `json:"properties"`
			} `json:"source"`
		} `json:"properties"`
	}
	if len(raw) == 0 {
		return nil
	}
	require.NoError(t, json.Unmarshal(raw, &shaped))
	var out []string
	for _, v := range shaped.Properties.Source.Properties.Mode.Enum {
		out = append(out, v)
	}
	return out
}

func sinkEnumOf(t *testing.T, raw json.RawMessage) []string {
	t.Helper()
	if len(raw) == 0 {
		return nil
	}
	var shaped struct {
		Properties struct {
			Sink struct {
				Enum []string `json:"enum"`
			} `json:"sink"`
		} `json:"properties"`
	}
	require.NoError(t, json.Unmarshal(raw, &shaped))
	return shaped.Properties.Sink.Enum
}

// TestDownloadSinkSchemaEnum mirrors pinner-cli's TestDownloadSinkSchemaEnum:
// the comma-enum reflector pitfall must never publish a partial sink enum; the
// enum follows the coordinator wiring, not the struct tag.
func TestDownloadSinkSchemaEnum(t *testing.T) {
	onHTTP := transfer.RewriteSinkEnum(toolargs.ToolSchemaFor[DownloadFileInput](), true, false)
	require.Equal(t, []string{"local", "drop"}, sinkEnumOf(t, onHTTP),
		"HTTP transport with a filedrop coordinator must advertise local+drop")

	onTunnel := transfer.RewriteSinkEnum(toolargs.ToolSchemaFor[DownloadFileInput](), true, true)
	require.Equal(t, []string{"local"}, sinkEnumOf(t, onTunnel),
		"OpenAI tunnel (no reachable mux) must not advertise drop")

	noDrop := transfer.RewriteSinkEnum(toolargs.ToolSchemaFor[DownloadFileInput](), false, false)
	require.Equal(t, []string{"local"}, sinkEnumOf(t, noDrop),
		"no filedrop coordinator wired -> local only")
}

// TestUploadSourceModeSchemaEnum mirrors pinner-cli: the compiled upload_file
// schema's source.mode enum follows the profile's transport, never its relay
// capability features.
func TestUploadSourceModeSchemaEnum(t *testing.T) {
	for _, tc := range []struct {
		name      string
		transport canimcp.TransportKind
		want      []string
	}{
		{"stdio", canimcp.TransportStdio, []string{"path"}},
		{"http", canimcp.TransportHTTP, []string{"mint"}},
		{"openai", canimcp.TransportOpenAI, []string{"url", "data"}},
	} {
		features := testProfileForTransport(tc.transport)
		// Grok declares the relay features for the SEPARATE upload_data /
		// upload_url tools; they must NOT widen upload_file's enum.
		features[FeatSourceURL] = true
		features[FeatSourceData] = true
		require.Equal(t, tc.want, sourceModeEnumOf(t, uploadFileSchema(features)), tc.name)
	}
}

// TestUploadTitleAndAnnotations pins the presentation annotation parity of the
// transfer tools (name, title, category, open-world hint).
func TestUploadTitleAndAnnotations(t *testing.T) {
	features := testProfileForTransport(canimcp.TransportHTTP)

	upload := NewUploadFileDescriptor(features, false, false, nil, nil, nil, nil, 0)
	require.Equal(t, "upload_file", upload.Name)
	require.Equal(t, "Upload a file to Pinner", upload.Title)
	require.True(t, upload.OpenWorldHint, "upload_file submits content to the Pinner/IPFS network")

	data := DataURIUploadDescriptor(nil, 0)
	require.Equal(t, "upload_data", data.Name)
	require.Equal(t, "Upload a file from a data URI", data.Title)
	require.True(t, data.OpenWorldHint)

	download := NewDownloadFileDescriptor(nil, nil, "", 0, false)
	require.Equal(t, "download_file", download.Name)
	require.Equal(t, "Download IPFS content to a file", download.Title)
	require.False(t, download.OpenWorldHint, "download_file is a closed workflow")
}

// TestUploadFileDescriptionPerProfile pins the composed upload_file
// description against the fragments pinner-cli's toolforge uploadFileDesc
// resolves for the same profiles.
func TestUploadFileDescriptionPerProfile(t *testing.T) {
	httpDesc := uploadFileDescription(testProfileForTransport(canimcp.TransportHTTP), canimcp.TransportHTTP)
	require.Contains(t, httpDesc, "Upload a file and pin it.")
	require.Contains(t, httpDesc, "source.mode=mint")
	require.Contains(t, httpDesc, "PUT your agent-local file to the returned url")
	require.Contains(t, httpDesc, "poll upload_status")
	require.Contains(t, httpDesc, "source.mode=mint and archive_mode=convert")
	require.NotContains(t, httpDesc, "source.mode=path with a host-side")
	// A generic HTTP profile has no file-input handoff.
	require.NotContains(t, httpDesc, "the OpenAI runtime converts it")

	stdioDesc := uploadFileDescription(testProfileForTransport(canimcp.TransportStdio), canimcp.TransportStdio)
	require.Contains(t, stdioDesc, "source.mode=path with a host-side file/directory/archive path")
	require.NotContains(t, stdioDesc, "source.mode=mint to get a one-time presigned")

	// The OpenAI tunnel description resolves against the effective features
	// including the ChatGPT host capabilities (the registry compiled the
	// schema and ChatGPT metadata from the same set), so the host-file
	// handoff instructions the `file` input enables MUST appear in the prose
	// — not just the url/data relay segments.
	tunnelDesc := uploadFileDescription(
		transportStartupFeatures(canimcp.TransportOpenAI), canimcp.TransportOpenAI)
	require.Contains(t, tunnelDesc, "source.mode=url (server-fetchable HTTPS URL) or source.mode=data")
	require.Contains(t, tunnelDesc, "If the upload fails with 'context canceled', retry")
	// Regression (host-file guidance on the tunnel): the FeatFileHostInput
	// segments render exactly like pinner-cli's original
	// ProfileForTransport(TransportOpenAI)-resolved description.
	require.Contains(t, tunnelDesc, "the OpenAI runtime converts it",
		"OpenAI tunnel description must carry the host-file handoff guidance")
	require.Contains(t, tunnelDesc, "file=<host file> and archive_mode=convert",
		"OpenAI tunnel description must carry the website-ZIP host-file route")
}

// TestDownloadFileDescriptionPerProfile pins the sink prose per wiring.
func TestDownloadFileDescriptionPerProfile(t *testing.T) {
	withDrop := downloadFileDescription(true, false)
	require.Contains(t, withDrop, "Download IPFS content (CID or CID/path) as a file")
	require.Contains(t, withDrop, "sink=local to write the bytes to a host-side output_path")
	require.Contains(t, withDrop, "sink=drop to get a one-time HTTP GET filedrop link")
	require.NotContains(t, withDrop, "The filedrop GET sink is unavailable on this transport.")

	require.Contains(t, withNoMuxPrefix(t), "sink=local")
	require.Contains(t, withNoMuxPrefix(t), "The filedrop GET sink is unavailable on this transport.")
}

// TestUploadDataDescription pins the upload_data forbid/allow copy.
func TestUploadDataDescription(t *testing.T) {
	openai := dataURIUploadDesc.Resolve(profileForTransport(canimcp.TransportOpenAI))
	require.Contains(t, openai, "Last resort — not for a host-provided or assistant-generated file.")

	generic := dataURIUploadDesc.Resolve(profileForTransport(canimcp.TransportHTTP))
	require.Contains(t, generic, "This transport has no data: URI relay.")
	require.NotContains(t, generic, "Last resort")
}

// TestUploadSourceSchemaTransform pins the schema transform: the mode enum
// follows the transport and non-transport sibling fields are dropped, with
// file-input hosts told the source is only a fallback.
func TestUploadSourceSchemaTransform(t *testing.T) {
	shaped := func(fs mcpforge.FeatureSet) (enum []string, desc string, hasPath, hasURL, hasData bool) {
		s := toolargs.SchemaFor[transfer.UploadSource]()
		UploadSourceSchemaTransform(s, fs)
		raw, err := json.Marshal(s)
		require.NoError(t, err)
		var parsed struct {
			Properties struct {
				Mode struct {
					Enum        []any  `json:"enum"`
					Description string `json:"description"`
				} `json:"mode"`
			} `json:"properties"`
		}
		require.NoError(t, json.Unmarshal(raw, &parsed))
		for _, v := range parsed.Properties.Mode.Enum {
			enum = append(enum, v.(string))
		}
		desc = parsed.Properties.Mode.Description
		_, hasPath = s.Properties.Get("path")
		_, hasURL = s.Properties.Get("url")
		_, hasData = s.Properties.Get("data")
		return enum, desc, hasPath, hasURL, hasData
	}

	// HTTP / mint-only: no file prop fallback copy; siblings for stdio/openai
	// payload fields are dropped; Grok-style relay features must NOT widen the
	// enum past the transport.
	grok := mcpforge.FeatureSet{
		FeatSourceMint: true, FeatSinkLocal: true, FeatSinkDrop: true,
		FeatRemoteAccess: true, FeatSourceURL: true, FeatSourceData: true,
	}
	enum, desc, hasPath, hasURL, hasData := shaped(grok)
	require.Equal(t, []string{"mint"}, enum)
	require.Contains(t, desc, "Only source.mode this tool accepts on this transport.")
	require.Contains(t, desc, "use the separate upload_url tool")
	require.Contains(t, desc, "use the separate upload_data tool")
	require.False(t, hasPath, "path payload must be dropped on the HTTP transport")
	require.False(t, hasURL)
	require.False(t, hasData)

	// OpenAI file-input host: fallback copy.
	openai, _, _, _, _ := shaped(testProfileForTransport(canimcp.TransportOpenAI).Clone())
	hostFile := testProfileForTransport(canimcp.TransportOpenAI).Clone()
	hostFile[FeatFileHostInput] = true
	_, desc, _, _, _ = shaped(hostFile)
	require.Equal(t, sourceFallbackDesc, desc)
	require.Contains(t, openai[0], "url")

	// stdio: path kept, url/data dropped.
	_, _, hasPath, hasURL, hasData = shaped(testProfileForTransport(canimcp.TransportStdio))
	require.True(t, hasPath)
	require.False(t, hasURL)
	require.False(t, hasData)
}

// TestArchiveModeSchemaTransform pins the route-gated archive_mode copy:
// mint-only hosts get the focused preserve/convert copy; the transport clauses
// resolve from the mechanism features.
func TestArchiveModeSchemaTransform(t *testing.T) {
	grok := mcpforge.FeatureSet{
		FeatSourceMint: true, FeatSinkLocal: true, FeatSinkDrop: true,
		FeatRemoteAccess: true,
	}
	s := &jsonschema.Schema{}
	archiveModeSchemaTransform(s, grok)
	require.Contains(t, s.Description, "preserve (the default for the mint source)")
	require.Contains(t, s.Description, "pass archive_mode=convert for a website ZIP")
	require.NotContains(t, s.Description, "A path source defaults to convert")

	httpWithFile := mcpforge.FeatureSet{FeatSourceMint: true, FeatSinkLocal: true, FeatSinkDrop: true, FeatRemoteAccess: true, FeatFileHostInput: true, FeatXMcpFile: true, FeatMCPApps: true, FeatElicitation: true}
	s2 := &jsonschema.Schema{}
	archiveModeSchemaTransform(s2, httpWithFile)
	require.Contains(t, s2.Description, "A host-file source defaults to convert.")
	require.Contains(t, s2.Description, "The mint (presigned PUT) source defaults to preserve")
	require.NotContains(t, s2.Description, "A path source defaults to convert.")
}

func withNoMuxPrefix(t *testing.T) string {
	t.Helper()
	return downloadFileDescription(false, true)
}

// TestUploadFileHandlerDeterministicSourceSelection pins the handler's
// exactly-one-source contract without wiring any executor.
func TestUploadFileHandlerDeterministicSourceSelection(t *testing.T) {
	features := testProfileForTransport(canimcp.TransportHTTP)
	desc := NewUploadFileDescriptor(features, false, false, nil, nil, nil, nil, 0)
	require.NotNil(t, desc.Handler)

	call := func(args string) (any, error) {
		var argsAny map[string]any
		require.NoError(t, json.Unmarshal([]byte(args), &argsAny))
		res, err := desc.Handler(context.Background(), model.ToolRequest{Arguments: argsAny})
		if err != nil {
			return nil, err
		}
		return res.StructuredContent, nil
	}

	_, err := call(`{}`)
	require.ErrorContains(t, err, "an upload source is required")

	_, err = call(`{"file":{"download_url":"https://x/y","file_id":"f"},"source":{"mode":"mint"}}`)
	require.ErrorContains(t, err, "provide exactly one upload source")

	// HTTP transport rejects a non-mint mode before reaching any executor.
	_, err = call(`{"source":{"mode":"path","path":"/tmp/x"}}`)
	require.ErrorContains(t, err, `source mode "path" is not available on the http transport`)

	// An unwired presigned coordinator is refused on the HTTP transport's mint
	// branch before anything else.
	_, err = call(`{"source":{"mode":"mint"}}`)
	require.ErrorContains(t, err, "presigned upload endpoint is not configured")
}

// TestFileBaseName pins the default upload-label derivation.
func TestFileBaseName(t *testing.T) {
	require.Equal(t, "f.pdf", FileBaseName("/tmp/a/f.pdf"))
	require.Equal(t, "d", FileBaseName("/tmp/d/"))
	require.Equal(t, "upload", FileBaseName(""))
	require.Equal(t, "upload", FileBaseName("/"))
	require.Equal(t, "file", FileBaseName("file"))
}

// TestTransferDescriptorWiringErrors pins the nil-executor errors of the
// wired branches.
func TestTransferDescriptorWiringErrors(t *testing.T) {
	relayDesc := NewUploadFileDescriptor(testProfileForTransport(canimcp.TransportHTTP), false, false, nil, nil, nil, nil, 0)
	var args map[string]any
	require.NoError(t, json.Unmarshal([]byte(`{"file":{"download_url":"https://x/y","file_id":"f"}}`), &args))
	_, err := relayDesc.Handler(context.Background(), model.ToolRequest{Arguments: args})
	require.ErrorContains(t, err, "file upload executor is not configured")

	dl := NewDownloadFileDescriptor(nil, nil, "", 0, false)
	var dargs map[string]any
	require.NoError(t, json.Unmarshal([]byte(`{"ipfs_path":"bafy","sink":"drop"}`), &dargs))
	_, err = dl.Handler(context.Background(), model.ToolRequest{Arguments: dargs})
	require.ErrorContains(t, err, "sink \"drop\"", "drop without an HTTP mux must be refused")

	var dargs2 map[string]any
	require.NoError(t, json.Unmarshal([]byte(`{"ipfs_path":"bafy","sink":"local"}`), &dargs2))
	_, err = dl.Handler(context.Background(), model.ToolRequest{Arguments: dargs2})
	require.ErrorContains(t, err, "IPFS download handler is not configured")
}

// TestUploadDataRequiredFile pins the required-file field on upload_data and
// the unwired-executor guard order.
func TestUploadDataRequiredFile(t *testing.T) {
	desc := DataURIUploadDescriptor(nil, 0)
	_, err := desc.Handler(context.Background(), model.ToolRequest{Arguments: map[string]any{}})
	require.ErrorContains(t, err, "data URI upload handler is not configured", "DecodeArgsFor guards the wired executor")

	// With an executor wired, the empty data URI is refused.
	desc = DataURIUploadDescriptor(func(ctx context.Context, r io.Reader, size int64, name string, wait bool, archiveMode string, wrap bool) (any, error) {
		return map[string]any{"cid": "bafy"}, nil
	}, 0)
	_, err = desc.Handler(context.Background(), model.ToolRequest{Arguments: map[string]any{"name": "x"}})
	require.ErrorContains(t, err, "file (data URI) is required")
}
