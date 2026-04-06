// Package store provides platform database access.
//
// The engine projector remains the primary writer for projected engine trace
// detail and projection-state fields. Purge/retention/repair are the only
// coordinated co-writers, and they must go through the projection CAS helpers
// and detail-deletion wrappers in this package.
package store
