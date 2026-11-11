// Package text owns SourceView text storage, edit transactions, history,
// preedit projection, metric snapshots and editor geometry. It owns no Widget
// or native resources. Model mutations are sequential; measured data remains
// local to its view. Public SourceView values alias the value types only.
package text
