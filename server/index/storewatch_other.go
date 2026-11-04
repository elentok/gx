//go:build !(darwin && cgo)

package index

// NewStoreWatch watches every directory under root with fsnotify.
func NewStoreWatch(root string) (*StoreWatch, error) {
	return newFSNotifyStoreWatch(root)
}
