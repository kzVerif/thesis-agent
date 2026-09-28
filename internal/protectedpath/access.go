package protectedpath

// Boundary enforces read-only ACL validation at dynamic I/O boundaries. Only the
// Service supplies it, after the Known Folder startup gate. Console/provisioning
// keep their existing I/O behavior. It never repairs historical dynamic files.
type Boundary struct {
	Root            string
	Report          Reporter
	sharedDownloads bool
}

// NewDownloadBoundary preserves the configured folder's ACL while enforcing
// pinned local paths, object types and rejection of reparse points/hard links.
// Never use this for identity, configuration or other private runtime state.
func NewDownloadBoundary(root string, report Reporter) *Boundary {
	return &Boundary{Root: root, Report: report, sharedDownloads: true}
}
