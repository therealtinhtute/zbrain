package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	zruntime "github.com/therealtinhtute/zbrain/internal/runtime"
)

// registerResources registers the two read-only workspace resources.
//
// Resources are read-only: domain failures (missing claim, missing evidence,
// invalid workspace) return ResourceNotFoundError; workspace boundary
// violations return an error with isError set.
func registerResources(server *mcp.Server, opts Options) error {
	handler := func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		return handleResource(ctx, req, opts)
	}

	server.AddResourceTemplate(&mcp.ResourceTemplate{
		URITemplate:  "zbrain://workspace/{workspace}/claim/{id}",
		Name:         "Claim",
		Description:  "Read a canonical claim by workspace and ID.",
		MIMEType:     "application/json",
	}, handler)

	server.AddResourceTemplate(&mcp.ResourceTemplate{
		URITemplate:  "zbrain://workspace/{workspace}/evidence/{id}",
		Name:         "Evidence",
		Description:  "Read an evidence record by workspace and ID.",
		MIMEType:     "application/json",
	}, handler)

	return nil
}

// handleResource dispatches the resource URI to the correct read handler.
func handleResource(ctx context.Context, req *mcp.ReadResourceRequest, opts Options) (*mcp.ReadResourceResult, error) {
	uri := req.Params.URI
	u, err := url.Parse(uri)
	if err != nil {
		return nil, mcp.ResourceNotFoundError(uri)
	}
	if u.Scheme != "zbrain" || u.Host != "workspace" {
		return nil, mcp.ResourceNotFoundError(uri)
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	// Expected: {workspace}/claim/{id} or {workspace}/evidence/{id}
	if len(parts) != 3 {
		return nil, mcp.ResourceNotFoundError(uri)
	}
	workspace := parts[0]
	resourceType := parts[1] // "claim" or "evidence"
	id := parts[2]

	if _, err := zruntime.ValidateWorkspace(opts.Paths, workspace); err != nil {
		return nil, mcp.ResourceNotFoundError(uri)
	}

	switch resourceType {
	case "claim":
		return readClaimResource(uri, workspace, id, opts)
	case "evidence":
		return readEvidenceResource(uri, workspace, id, opts)
	default:
		return nil, mcp.ResourceNotFoundError(uri)
	}
}

// readClaimResource returns the canonical claim as JSON text content.
func readClaimResource(uri, workspace, id string, opts Options) (*mcp.ReadResourceResult, error) {
	claim, err := (zruntime.ClaimStore{Paths: opts.Paths, Now: opts.Now}).Read(workspace, id)
	if err != nil {
		return nil, mcp.ResourceNotFoundError(uri)
	}
	encoded, err := json.MarshalIndent(claim, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal claim: %w", err)
	}
	return &mcp.ReadResourceResult{
		Contents: []*mcp.ResourceContents{{
			URI:  uri,
			Text: string(encoded),
		}},
	}, nil
}

// readEvidenceResource returns the evidence metadata and raw content as JSON.
func readEvidenceResource(uri, workspace, id string, opts Options) (*mcp.ReadResourceResult, error) {
	evidence, err := (zruntime.EvidenceStore{Paths: opts.Paths, Now: opts.Now}).Read(workspace, id)
	if err != nil {
		return nil, mcp.ResourceNotFoundError(uri)
	}

	// Read the raw snapshot content.
	rawPath, err := rawEvidenceFilePath(opts.Paths, workspace, id)
	if err != nil {
		return nil, mcp.ResourceNotFoundError(uri)
	}
	rawBytes, err := os.ReadFile(rawPath)
	if err != nil {
		return nil, mcp.ResourceNotFoundError(uri)
	}

	out := struct {
		SchemaVersion int                  `json:"schema_version"`
		Evidence      zruntime.Evidence    `json:"evidence"`
		RawContent    string               `json:"raw_content"`
	}{1, evidence, string(rawBytes)}

	encoded, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal evidence: %w", err)
	}
	return &mcp.ReadResourceResult{
		Contents: []*mcp.ResourceContents{{
			URI:  uri,
			Text: string(encoded),
		}},
	}, nil
}

// rawEvidenceFilePath returns the absolute path to the raw evidence file.
func rawEvidenceFilePath(paths zruntime.Paths, workspace, id string) (string, error) {
	relative := filepath.ToSlash(filepath.Join("evidence", "sources", id, "raw"))
	return zruntime.ResolveWorkspacePath(paths, workspace, relative)
}