//go:build !(darwin && cgo)

package server

// NewStoreWatch watches every directory under root with fsnotify.
func NewStoreWatch(root string) (*StoreWatch, error) {
	return newFSNotifyStoreWatch(root)
}
