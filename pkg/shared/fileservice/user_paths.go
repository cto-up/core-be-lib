package service

// ProfilePictureFilePath returns the object-storage path used by the core
// user profile-picture upload/download handlers. Exported so downstream
// cleanup code (GDPR erasure in skeells, account-recovery flows, etc.)
// can address the exact same key the upload handler writes — without
// re-deriving the path format and risking silent drift.
func ProfilePictureFilePath(userID string) string {
	return "/core/users/" + userID + "/profile-picture.jpg"
}
