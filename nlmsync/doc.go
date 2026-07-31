// Package nlmsync mirrors a set of local files into a NotebookLM notebook as
// sources, keeping the notebook in step with the working tree.
//
// Files are discovered with git ls-files when the path is inside a checkout
// (falling back to a filesystem walk otherwise), filtered through --exclude
// patterns and any .nlmignore file, packed into txtar bundles sized to the
// server's per-source limits, and reconciled against the notebook's existing
// sources so unchanged content is skipped and orphaned sources are removed.
package nlmsync
