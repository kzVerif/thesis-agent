package protectedpath

// Boundary enforces read-only ACL validation at dynamic I/O boundaries. Only the
// Service supplies it, after the Known Folder startup gate. Console/provisioning
// keep their existing I/O behavior. It never repairs historical dynamic files.
type Boundary struct {
	Root   string
	Report Reporter
}
